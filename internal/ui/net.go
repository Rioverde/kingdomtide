package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// outboxCap is the buffer size of the outbound-message channel. A single
// slot is enough — the game is turn-at-a-time and Bubble Tea only emits
// one command per key press — but even with faster keymashing a size-1
// buffer unblocks the Update thread from the Send goroutine.
const outboxCap = 8

// connectingMsg is emitted when Update starts dialling. It carries no
// payload; its value is purely the state-machine transition.
type connectingMsg struct{}

// connectedMsg arrives once the Play stream is open and the writer
// goroutine is installed. The Model takes ownership of cancel, stream
// and outbox from this point on.
type connectedMsg struct {
	conn   *grpc.ClientConn
	stream pb.GameService_PlayClient
	cancel context.CancelFunc
	outbox chan *pb.ClientMessage
}

// acceptedMsg is the JoinAccepted reply — carries the server-assigned ID and
// the authoritative world seed. The seed is stored read-only on the Model so
// the client can construct a local NoiseRegionSource for cosmetic per-tile
// tint sampling; gameplay identity (region name, character) always travels
// on the wire inside Snapshot.Region.
type acceptedMsg struct {
	PlayerID  string
	WorldSeed int64
}

// snapshotMsg is the full world snapshot, sent once on join.
type snapshotMsg struct {
	Snapshot *pb.Snapshot
}

// eventMsg is any server-broadcast event (join/leave/move).
type eventMsg struct {
	Event *pb.Event
}

// netErrorMsg is a terminal error — either dial or stream.Recv.
type netErrorMsg struct {
	Err error
}

// serverErrorMsg is a non-fatal rule violation the server returned for a
// specific command (e.g. "destination blocked"). The stream stays open; the
// client just shows the message in the log and carries on.
//
// Err is the reconstructed gRPC Status error extracted from the wire
// ErrorResponse. renderServerError (in errors.go) extracts the attached
// LocalizedMessage detail from Err so the player sees a localized string
// while the raw Status.Message stays in developer logs.
type serverErrorMsg struct {
	Err error
}

// connectCmd dials the server, opens the Play stream, and installs the
// outbox writer goroutine. Returns connectedMsg on success or
// netErrorMsg on any failure.
//
// Goroutine discipline: the writer goroutine this starts exits when
// either ctx is cancelled (caller invoked cancel) or outbox is closed.
// The caller — Update — is expected to call cancel on disconnect, which
// also aborts the stream and unblocks any in-flight stream.Recv in
// listenCmd.
func connectCmd(parent context.Context, addr string) tea.Cmd {
	return func() tea.Msg {
		// Separate context per connection attempt. Not derived from the
		// Bubble Tea Cmd's execution context — that doesn't exist. Use
		// the program-wide parent so Ctrl-C cancels everything.
		ctx, cancel := context.WithCancel(parent)

		conn, err := grpc.NewClient(
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			cancel()
			return netErrorMsg{Err: fmt.Errorf("dial %s: %w", addr, err)}
		}

		client := pb.NewGameServiceClient(conn)
		stream, err := client.Play(ctx)
		if err != nil {
			_ = conn.Close()
			cancel()
			return netErrorMsg{Err: fmt.Errorf("open play stream: %w", err)}
		}

		outbox := make(chan *pb.ClientMessage, outboxCap)
		go runOutbox(ctx, stream, outbox)

		return connectedMsg{
			conn:   conn,
			stream: stream,
			cancel: cancel,
			outbox: outbox,
		}
	}
}

// runOutbox forwards messages from outbox to stream.Send until either
// the context is done or outbox is closed. The function owns stream.Send
// — nothing else is allowed to call it concurrently.
//
// On any Send error the goroutine exits: the error is surfaced on the
// next stream.Recv which listenCmd catches, so we don't need a second
// channel back into the Model.
func runOutbox(ctx context.Context, stream pb.GameService_PlayClient, outbox <-chan *pb.ClientMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-outbox:
			if !ok {
				return
			}
			if err := stream.Send(msg); err != nil {
				return
			}
		}
	}
}

// listenCmd performs exactly one stream.Recv. The typed message it
// returns triggers Update, which must re-fire listenCmd if the stream
// is still live. Bubble Tea runs Cmds in its own goroutine pool, so
// this shape never has a long-lived goroutine of its own.
func listenCmd(stream pb.GameService_PlayClient) tea.Cmd {
	return func() tea.Msg {
		msg, err := stream.Recv()
		if err != nil {
			return netErrorMsg{Err: fmt.Errorf("stream recv: %w", err)}
		}
		switch payload := msg.GetPayload().(type) {
		case *pb.ServerMessage_Accepted:
			return acceptedMsg{
				PlayerID:  payload.Accepted.GetPlayerId(),
				WorldSeed: payload.Accepted.GetWorldSeed(),
			}
		case *pb.ServerMessage_Snapshot:
			return snapshotMsg{Snapshot: payload.Snapshot}
		case *pb.ServerMessage_Event:
			return eventMsg{Event: payload.Event}
		case *pb.ServerMessage_Error:
			return serverErrorMsg{Err: errorResponseToErr(payload.Error)}
		default:
			// Unknown payload — keep listening rather than killing the
			// session. Returning a non-fatal "continue" is a problem for
			// another day; for now just emit an event nil.
			return eventMsg{Event: nil}
		}
	}
}

// sendJoinCmd queues a Join message on the outbox, tagged with the
// client's requested viewport size, BCP-47 language tag, and the six
// ability scores the player assembled during phaseCharacterCreation.
// The server uses the viewport size when building Snapshot responses
// for this client; the language is stored on the session so future
// server-generated LocalizedMessage payloads can route through the
// right catalog; the stats are revalidated through NewStatsPointBuy
// before acceptance. Non-blocking; a full outbox means the writer
// goroutine is dead and the session is doomed.
func sendJoinCmd(outbox chan<- *pb.ClientMessage, name, lang string, stats [6]int, viewW, viewH int) tea.Cmd {
	return sendNonBlocking(outbox, &pb.ClientMessage{
		Payload: &pb.ClientMessage_Join{
			Join: &pb.JoinRequest{
				Name:           name,
				Language:       lang,
				ViewportWidth:  int32(viewW),
				ViewportHeight: int32(viewH),
				Stats: &pb.CoreStats{
					Strength:     int32(stats[0]),
					Dexterity:    int32(stats[1]),
					Constitution: int32(stats[2]),
					Intelligence: int32(stats[3]),
					Wisdom:       int32(stats[4]),
					Charisma:     int32(stats[5]),
				},
			},
		},
	}, "join")
}

// sendMoveCmd queues a MoveCmd. Same backpressure contract as
// sendJoinCmd.
func sendMoveCmd(outbox chan<- *pb.ClientMessage, dx, dy int) tea.Cmd {
	return sendNonBlocking(outbox, &pb.ClientMessage{
		Payload: &pb.ClientMessage_Move{
			Move: &pb.MoveCmd{Dx: int32(dx), Dy: int32(dy)},
		},
	}, "move")
}

// errorResponseToErr reconstructs a gRPC Status error from a wire
// ErrorResponse so the client can run it through renderServerError for
// localization. The ErrorResponse carries two string fields: Message (English
// developer text) and Code (a short string tag such as "rule_violation"). We
// map Code to a grpc codes.Code for the Status and, when the ErrorResponse
// includes a recognized LocalizedMessage-style code, attach a LocalizedMessage
// detail so renderServerError can do a catalog lookup.
//
// Since the current server's sendError path (service.go) attaches only a plain
// ErrorResponse (not a full gRPC Status with Details), we synthesize a
// LocalizedMessage from the Code field: "error.<code>" is used as the message
// ID. If the catalog has that key the player sees a localized string; if not,
// renderServerError falls back to error.unknown. This round-trip is lossless:
// the server log already has the English Message; the player sees the
// localized version.
func errorResponseToErr(e *pb.ErrorResponse) error {
	if e == nil {
		return nil
	}
	msg := e.GetMessage()
	code := e.GetCode()

	// Build a LocalizedMessage detail so renderServerError can do a catalog
	// lookup. We derive the message_id as "error.<code>" and pass no args —
	// the current rule-violation errors are short phrases with no placeholders.
	messageID := "error.unknown"
	if code != "" {
		messageID = "error." + code
	}
	detail := &pb.LocalizedMessage{
		MessageId: messageID,
		Args:      map[string]string{},
	}

	grpcCode := codes.FailedPrecondition
	st := status.New(grpcCode, msg)
	enriched, err := st.WithDetails(detail)
	if err != nil {
		// WithDetails only fails for non-marshallable messages; fallback to
		// a plain status so the caller still gets a non-nil error.
		return st.Err()
	}
	return enriched.Err()
}

// sendViewportCmd tells the server this client wants a differently-sized
// Snapshot from now on. Triggered by tea.WindowSizeMsg after the terminal
// resizes. Same non-blocking backpressure contract as sendJoinCmd.
func sendViewportCmd(outbox chan<- *pb.ClientMessage, viewW, viewH int) tea.Cmd {
	return sendNonBlocking(outbox, &pb.ClientMessage{
		Payload: &pb.ClientMessage_Viewport{
			Viewport: &pb.ViewportCmd{Width: int32(viewW), Height: int32(viewH)},
		},
	}, "viewport")
}

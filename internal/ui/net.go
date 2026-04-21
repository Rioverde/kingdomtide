package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

// acceptedMsg is the JoinAccepted reply — carries the server-assigned ID.
type acceptedMsg struct {
	PlayerID string
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
type serverErrorMsg struct {
	Message string
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
			return acceptedMsg{PlayerID: payload.Accepted.GetPlayerId()}
		case *pb.ServerMessage_Snapshot:
			return snapshotMsg{Snapshot: payload.Snapshot}
		case *pb.ServerMessage_Event:
			return eventMsg{Event: payload.Event}
		case *pb.ServerMessage_Error:
			return serverErrorMsg{Message: payload.Error.GetMessage()}
		default:
			// Unknown payload — keep listening rather than killing the
			// session. Returning a non-fatal "continue" is a problem for
			// another day; for now just emit an event nil.
			return eventMsg{Event: nil}
		}
	}
}

// sendJoinCmd queues a Join message on the outbox, tagged with the
// client's requested viewport size. The server uses that size when
// building Snapshot responses for this client. Non-blocking; a full
// outbox means the writer goroutine is dead and the session is doomed.
func sendJoinCmd(outbox chan<- *pb.ClientMessage, name string, viewW, viewH int) tea.Cmd {
	return sendNonBlocking(outbox, &pb.ClientMessage{
		Payload: &pb.ClientMessage_Join{
			Join: &pb.JoinRequest{
				Name:           name,
				ViewportWidth:  int32(viewW),
				ViewportHeight: int32(viewH),
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

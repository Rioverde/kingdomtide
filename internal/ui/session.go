package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/server"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// sessionDriver is the small surface the Model needs from server.Service
// when running in SSH session mode. Defined at the consumer so the UI
// package does not take a hard structural dependency on the full
// Service type — any type that fulfils this contract (in particular a
// test fake) can stand in for it. Mirrors the gRPC outbox surface: one
// call per client-originated command.
type sessionDriver interface {
	JoinSession(name string, dims viewportDims, stats game.CoreStats) (server.SessionJoinResult, error)
	Subscribe(playerID string) (<-chan server.SessionEvent, func())
	EnqueueMoveSession(playerID string, dx, dy int) error
	UpdateSessionViewport(playerID string, width, height int)
	LeaveSession(playerID string)
}

// viewportDims mirrors the server-internal value type so the Model can
// request a snapshot size without reaching into package server. Kept as
// a plain pair of ints here — the server-side viewportDims is
// unexported and this is the narrow UI-side view of the same concept.
type viewportDims = server.ViewportDims

// sessionTickMsg carries a single SessionEvent from the in-process hub
// into Bubble Tea's Update loop. The pump goroutine translates every
// channel recv into one of these; Update fans it out to the existing
// applySnapshot / applyEvent helpers so the render path is shared
// between gRPC and SSH modes.
type sessionTickMsg struct {
	evt server.SessionEvent
}

// sessionAcceptedMsg is the session-mode analogue of acceptedMsg: it
// carries the post-join identity data (player ID + world seed + spawn)
// plus the initial snapshot. Emitted synchronously by joinSessionCmd so
// the Model advances to phasePlaying on the first tick of the pump.
type sessionAcceptedMsg struct {
	Result server.SessionJoinResult
}

// NewSession builds a Model wired to an in-process server.Service via
// the SSH session s. Commands route through sessionDriver methods
// instead of a gRPC outbox, and tick events arrive on a
// server.SessionEvent channel routed through a goroutine that emits
// sessionTickMsg values into the Bubble Tea loop.
//
// ctx must be the ssh.Session's context — when the SSH connection
// closes, ctx is cancelled and the event pump exits cleanly. The
// Session is stored on the model so the Model can read the terminal
// window size via sess.Pty() on init if no tea.WindowSizeMsg has
// arrived yet.
func NewSession(ctx context.Context, svc sessionDriver, sess ssh.Session) *Model {
	m := New(ctx, "(ssh)")
	m.sessionSvc = svc
	m.sess = sess
	return m
}

// joinSessionCmd runs the in-process Join handshake and emits
// sessionAcceptedMsg on success or netErrorMsg on failure. Unlike the
// gRPC variant it is a single synchronous call: svc.JoinSession
// applies the command under the world mutex and returns the spawn +
// snapshot atomically. Errors are wrapped into a netErrorMsg so the
// Model's existing disconnect handler fires without special-casing SSH
// mode.
func joinSessionCmd(svc sessionDriver, name string, dims viewportDims, stats game.CoreStats) tea.Cmd {
	return func() tea.Msg {
		res, err := svc.JoinSession(name, dims, stats)
		if err != nil {
			return netErrorMsg{Err: fmt.Errorf("session join: %w", err)}
		}
		return sessionAcceptedMsg{Result: res}
	}
}

// pumpSessionEventsCmd recv's a single SessionEvent from ch and wraps
// it in a sessionTickMsg. Called once by Update per event so Bubble
// Tea's Cmd runner owns goroutine scheduling — no long-lived goroutine
// of our own, matching the listenCmd pattern used for gRPC.
//
// Returns a nil msg (terminal) when ch is closed or ctx is cancelled,
// signalling the end of the session to the Update loop.
func pumpSessionEventsCmd(ctx context.Context, ch <-chan server.SessionEvent) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-ctx.Done():
			return netErrorMsg{Err: ctx.Err()}
		case evt, ok := <-ch:
			if !ok {
				return netErrorMsg{Err: fmt.Errorf("session stream closed")}
			}
			return sessionTickMsg{evt: evt}
		}
	}
}

// sendMoveSessionCmd is the session-mode analogue of sendMoveCmd. It
// enqueues a MoveIntent directly on the world via the sessionDriver
// instead of serialising a pb.MoveCmd onto a gRPC outbox. On error
// (unknown player, invalid step) it returns a serverErrorMsg so the
// existing renderServerError path logs a localized line — the stream
// stays open exactly like the gRPC path.
func sendMoveSessionCmd(svc sessionDriver, playerID string, dx, dy int) tea.Cmd {
	return func() tea.Msg {
		if err := svc.EnqueueMoveSession(playerID, dx, dy); err != nil {
			return sessionMoveErrorMsg{Err: err}
		}
		return nil
	}
}

// sessionMoveErrorMsg carries a non-fatal move-enqueue error from
// EnqueueMoveSession. Kept separate from netErrorMsg so a bad key
// press does not tear the session down — mirrors serverErrorMsg on the
// gRPC path.
type sessionMoveErrorMsg struct {
	Err error
}

// sendViewportSessionCmd pushes a new viewport size to the service for
// this SSH session. Same non-blocking contract as sendViewportCmd on
// the gRPC path — the service applies the dims under its mutex and
// emits a follow-up snapshot on the session hub.
func sendViewportSessionCmd(svc sessionDriver, playerID string, w, h int) tea.Cmd {
	return func() tea.Msg {
		svc.UpdateSessionViewport(playerID, w, h)
		return nil
	}
}

// applySessionEvent folds one SessionEvent into the Model. Snapshots
// replace the viewport (same path as the gRPC snapshotMsg); domain
// events go through a pb translation layer because applyEvent already
// handles pb.Event values and we want one rendering code path.
func applySessionEvent(m *Model, evt server.SessionEvent) {
	if evt.IsSnapshot {
		if evt.Snapshot != nil {
			applySnapshot(m, evt.Snapshot)
			if m.phase != phasePlaying {
				m.phase = phasePlaying
				m.status = ""
			}
		}
		return
	}
	if ev := domainEventToPB(evt.Event); ev != nil {
		applyEvent(m, ev)
	}
}

// domainEventToPB translates a domain game.Event into its pb.Event
// wire representation so applyEvent (already tuned for pb payloads)
// can handle it without a parallel dispatch. Returns nil for unknown
// events so the caller can simply skip them.
//
// This mirror of server.eventToServerMessage lives in the UI package
// because the server-side helper returns a *pb.ServerMessage — wire-
// level framing we explicitly do not want for in-process sessions.
// Keeping the per-event shape identical is what lets applyEvent stay
// transport-agnostic.
func domainEventToPB(e game.Event) *pb.Event {
	switch v := e.(type) {
	case game.PlayerJoinedEvent:
		return &pb.Event{Payload: &pb.Event_PlayerJoined{PlayerJoined: &pb.PlayerJoined{
			Entity: &pb.Entity{
				Id:       v.PlayerID,
				Name:     v.Name,
				Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
				Position: domainPositionToPB(v.Position),
			},
		}}}
	case game.PlayerLeftEvent:
		return &pb.Event{Payload: &pb.Event_PlayerLeft{PlayerLeft: &pb.PlayerLeft{
			PlayerId: v.PlayerID,
		}}}
	case game.EntityMovedEvent:
		return &pb.Event{Payload: &pb.Event_EntityMoved{EntityMoved: &pb.EntityMoved{
			EntityId: v.EntityID,
			From:     domainPositionToPB(v.From),
			To:       domainPositionToPB(v.To),
		}}}
	case game.IntentFailedEvent:
		return &pb.Event{Payload: &pb.Event_IntentFailed{IntentFailed: &pb.IntentFailed{
			EntityId: v.EntityID,
			Reason:   v.Reason,
		}}}
	}
	return nil
}

// domainPositionToPB is the inverse of positionFromPB — Position → pb.Position.
// Kept adjacent to domainEventToPB because the two only collaborate.
func domainPositionToPB(p game.Position) *pb.Position {
	return &pb.Position{X: int32(p.X), Y: int32(p.Y)}
}

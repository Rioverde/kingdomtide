package ui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/ssh"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/event"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/stats"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/server"
)

// sessionDriver is the small surface the Model needs from server.Service
// when running in SSH session mode. Defined at the consumer so the UI
// package does not take a hard structural dependency on the full
// Service type — any type that fulfils this contract (in particular a
// test fake) can stand in for it. Mirrors the gRPC outbox surface: one
// call per client-originated command.
type sessionDriver interface {
	JoinSession(name string, dims viewportDims, stats stats.CoreStats) (server.SessionJoinResult, error)
	Subscribe(ctx context.Context, playerID string) (<-chan server.SessionEvent, func())
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
func joinSessionCmd(svc sessionDriver, name string, dims viewportDims, stats stats.CoreStats) tea.Cmd {
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

// domainEventToPB translates a domain event.Event into its pb.Event
// wire representation so applyEvent (already tuned for pb payloads)
// can handle it without a parallel dispatch. Returns nil for unknown
// events so the caller can simply skip them.
//
// This mirror of server.eventToServerMessage lives in the UI package
// because the server-side helper returns a *pb.ServerMessage — wire-
// level framing we explicitly do not want for in-process sessions.
// Keeping the per-event shape identical is what lets applyEvent stay
// transport-agnostic.
func domainEventToPB(e event.Event) *pb.Event {
	switch v := e.(type) {
	case event.PlayerJoinedEvent:
		return &pb.Event{Payload: &pb.Event_PlayerJoined{PlayerJoined: &pb.PlayerJoined{
			Entity: &pb.Entity{
				Id:       v.PlayerID,
				Name:     v.Name,
				Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
				Position: domainPositionToPB(v.Position),
			},
		}}}
	case event.PlayerLeftEvent:
		return &pb.Event{Payload: &pb.Event_PlayerLeft{PlayerLeft: &pb.PlayerLeft{
			PlayerId: v.PlayerID,
		}}}
	case event.EntityMovedEvent:
		return &pb.Event{Payload: &pb.Event_EntityMoved{EntityMoved: &pb.EntityMoved{
			EntityId: v.EntityID,
			From:     domainPositionToPB(v.From),
			To:       domainPositionToPB(v.To),
		}}}
	case event.IntentFailedEvent:
		return &pb.Event{Payload: &pb.Event_IntentFailed{IntentFailed: &pb.IntentFailed{
			EntityId: v.EntityID,
			Reason:   v.Reason,
		}}}
	case event.TimeTickEvent:
		return &pb.Event{Payload: &pb.Event_TimeTick{TimeTick: &pb.TimeTick{
			CurrentTick: v.CurrentTick,
			GameTime:    domainGameTimeToPB(v.GameTime),
		}}}
	}
	return nil
}

// domainGameTimeToPB mirrors server.gameTimeToPB in the UI package so
// the in-process SSH event pipeline can reuse applyEvent's wire-shaped
// dispatch without importing the server-internal helper (which returns
// a full *pb.ServerMessage). Keeps domainEventToPB a self-contained
// domain→wire translation.
func domainGameTimeToPB(gt calendar.GameTime) *pb.GameTime {
	return &pb.GameTime{
		Year:       gt.Year,
		Month:      domainMonthToPB(gt.Month),
		DayOfMonth: gt.DayOfMonth,
		TickOfDay:  gt.TickOfDay,
		Season:     domainSeasonToPB(gt.Season),
	}
}

// domainMonthToPBMapping is the 1:1 translation table from the domain
// Month enum to its wire counterpart. Kept parallel to the server-side
// monthPBMapping so any twelve-month assumption lives in exactly one
// place per package.
var domainMonthToPBMapping = map[calendar.Month]pb.CalendarMonth{
	calendar.MonthJanuary:   pb.CalendarMonth_CALENDAR_MONTH_JANUARY,
	calendar.MonthFebruary:  pb.CalendarMonth_CALENDAR_MONTH_FEBRUARY,
	calendar.MonthMarch:     pb.CalendarMonth_CALENDAR_MONTH_MARCH,
	calendar.MonthApril:     pb.CalendarMonth_CALENDAR_MONTH_APRIL,
	calendar.MonthMay:       pb.CalendarMonth_CALENDAR_MONTH_MAY,
	calendar.MonthJune:      pb.CalendarMonth_CALENDAR_MONTH_JUNE,
	calendar.MonthJuly:      pb.CalendarMonth_CALENDAR_MONTH_JULY,
	calendar.MonthAugust:    pb.CalendarMonth_CALENDAR_MONTH_AUGUST,
	calendar.MonthSeptember: pb.CalendarMonth_CALENDAR_MONTH_SEPTEMBER,
	calendar.MonthOctober:   pb.CalendarMonth_CALENDAR_MONTH_OCTOBER,
	calendar.MonthNovember:  pb.CalendarMonth_CALENDAR_MONTH_NOVEMBER,
	calendar.MonthDecember:  pb.CalendarMonth_CALENDAR_MONTH_DECEMBER,
}

// domainMonthToPB translates the domain Month enum to the wire
// CalendarMonth. MonthZero and any out-of-range value fall back to
// CALENDAR_MONTH_UNSPECIFIED so a zero-value GameTime round-trips
// cleanly through the client's gameTimeFromPB guard.
func domainMonthToPB(m calendar.Month) pb.CalendarMonth {
	if v, ok := domainMonthToPBMapping[m]; ok {
		return v
	}
	return pb.CalendarMonth_CALENDAR_MONTH_UNSPECIFIED
}

// domainSeasonToPBMapping is the 1:1 translation table from the domain
// Season enum to its wire counterpart. Kept parallel to the server-side
// seasonPBMapping so both packages agree on the axis.
var domainSeasonToPBMapping = map[calendar.Season]pb.CalendarSeason{
	calendar.SeasonWinter: pb.CalendarSeason_CALENDAR_SEASON_WINTER,
	calendar.SeasonSpring: pb.CalendarSeason_CALENDAR_SEASON_SPRING,
	calendar.SeasonSummer: pb.CalendarSeason_CALENDAR_SEASON_SUMMER,
	calendar.SeasonAutumn: pb.CalendarSeason_CALENDAR_SEASON_AUTUMN,
}

// domainSeasonToPB translates the domain Season enum to the wire
// CalendarSeason. Out-of-range values fall back to
// CALENDAR_SEASON_UNSPECIFIED.
func domainSeasonToPB(s calendar.Season) pb.CalendarSeason {
	if v, ok := domainSeasonToPBMapping[s]; ok {
		return v
	}
	return pb.CalendarSeason_CALENDAR_SEASON_UNSPECIFIED
}

// domainPositionToPB is the inverse of positionFromPB — Position → pb.Position.
// Kept adjacent to domainEventToPB because the two only collaborate.
func domainPositionToPB(p geom.Position) *pb.Position {
	return &pb.Position{X: int32(p.X), Y: int32(p.Y)}
}

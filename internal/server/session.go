package server

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/event"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/stats"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// SessionJoinResult is the bundle a fresh SSH-mode subscriber needs
// to start rendering: the server-assigned player ID, spawn position,
// world seed (for local influence sampling), and the initial viewport
// snapshot. Returned by JoinSession so the caller can present the
// first frame synchronously before wiring the channel-based event feed.
//
// Calendar carries the server's authoritative calendar cadence so the
// SSH-mode client can construct a local calendar.Calendar mirror without a
// round-trip. Zero-valued Calendar (ticksPerDay == 0) indicates the
// service has no Calendar wired and the client should skip the date
// HUD. This is the session-mode analogue of JoinAccepted.calendar on
// the gRPC path.
type SessionJoinResult struct {
	PlayerID  string
	Spawn     geom.Position
	WorldSeed int64
	Snapshot  *pb.Snapshot
	Calendar  calendar.Calendar
	// Events carries the events produced by the Join itself (one
	// PlayerJoined). Returned so the caller can broadcast them to other
	// subscribers without having to peek at the internal state.
	Events []event.Event
}

// JoinSession admits a new SSH-mode player under the world mutex and
// captures the spawn-centred snapshot atomically, mirroring the gRPC
// bootJoin path. The returned result carries everything the session
// needs to render its first frame; the caller (Handler) wires
// Subscribe immediately after so follow-up ticks reach the session.
//
// On failure (invalid stats, no spawn tile available) JoinSession
// returns an error; the session should show it to the player and
// close the SSH connection.
func (s *Service) JoinSession(
	name string,
	dims ViewportDims,
	stats stats.CoreStats,
) (SessionJoinResult, error) {
	if name == "" {
		return SessionJoinResult{}, errors.New("session join: name required")
	}
	validated, err := validateDomainJoinStats(stats)
	if err != nil {
		return SessionJoinResult{}, err
	}
	stats = validated

	playerID := uuid.NewString()
	spawn, snap, events, err := s.bootJoin(playerID, name, dims.toInternal(), stats)
	if err != nil {
		return SessionJoinResult{}, err
	}
	s.log.Info("session player joined", "id", playerID, "name", name, "spawn", spawn,
		"viewport", dims)
	return SessionJoinResult{
		PlayerID:  playerID,
		Spawn:     spawn,
		WorldSeed: s.world.Seed(),
		Snapshot:  snap,
		Calendar:  s.world.Calendar(),
		Events:    events,
	}, nil
}

// Subscribe registers a session subscriber by playerID and returns a
// read-only SessionEvent channel plus an unsubscribe function. The hub
// is internal to Service; exposing the channel directly keeps the
// session package free of server-internals awareness.
//
// ctx is the per-session context (typically the SSH session's context).
// A watcher goroutine fires unsub when ctx is cancelled, so an SSH
// disconnect automatically releases the hub slot even if the caller
// forgets to invoke unsub. The returned unsub remains idempotent and
// callers are still encouraged to invoke it explicitly on graceful
// teardown so the server-side cleanup does not wait on a
// transport-level timeout.
//
// Contract: callers MUST either drain the channel or let the ctx
// watcher unsubscribe them promptly — the broadcast path uses a
// non-blocking send and will drop events for stuck subscribers, but
// an undrained channel that is never unsubscribed leaks the buffer
// until Service shutdown.
func (s *Service) Subscribe(ctx context.Context, playerID string) (<-chan SessionEvent, func()) {
	if s.sessions == nil {
		s.sessions = newSessionHub(s.log)
	}
	ch, unsub := s.sessions.subscribe(playerID)
	go s.ctxWatch(ctx, playerID, unsub)
	return ch, unsub
}

// LeaveSession fires a LeaveCmd and broadcasts the resulting events.
// The caller is responsible for invoking the unsubscribe function
// returned from Subscribe; LeaveSession only touches world state and
// the event bus. Safe to call even if the session was never fully
// joined (the ApplyCommand path returns ErrPlayerNotFound which we
// swallow since nothing to clean up).
func (s *Service) LeaveSession(playerID string) {
	leaveEvents, err := s.applyCmd(world.LeaveCmd{PlayerID: playerID})
	if err != nil {
		return
	}
	s.mu.Lock()
	delete(s.viewports, playerID)
	s.mu.Unlock()
	s.broadcastEvents(leaveEvents)
}

// EnqueueMoveSession is the session-mode analogue of enqueueMoveIntent.
// It funnels a MoveIntent into the world's pending-intent slot for
// playerID under the world mutex. Errors are returned verbatim so the
// caller (the Model) can decide how to surface them — typically as a
// localized log entry. Returning the error instead of publishing it on
// the session hub keeps session error rendering symmetric with gRPC's
// serverErrorMsg path.
func (s *Service) EnqueueMoveSession(playerID string, dx, dy int) error {
	s.mu.Lock()
	err := s.world.EnqueueIntent(playerID, world.MoveIntent{DX: dx, DY: dy})
	s.mu.Unlock()
	return err
}

// UpdateSessionViewport changes the stored viewport dims for a
// session-mode player and pushes a fresh snapshot on the session hub.
// Used when the SSH client's terminal resizes — mirrors the gRPC
// updateViewport path.
func (s *Service) UpdateSessionViewport(playerID string, width, height int) {
	s.mu.Lock()
	s.viewports[playerID] = viewportDims{width: width, height: height}
	pos, ok := s.world.PositionOf(playerID)
	var snap *pb.Snapshot
	if ok {
		snap = s.snapshotFor(playerID, pos)
	}
	s.mu.Unlock()
	if snap == nil {
		return
	}
	if s.sessions != nil {
		s.sessions.sendTo(playerID, SessionEvent{IsSnapshot: true, Snapshot: snap})
	}
}

// publishEventsToSessions fans out a batch of domain events to every
// session subscriber. Follows the non-blocking invariant enforced by
// sessionHub.trySend so a slow session never stalls DoTick.
func (s *Service) publishEventsToSessions(events []event.Event) {
	if s.sessions == nil || len(events) == 0 {
		return
	}
	for _, ev := range events {
		s.sessions.broadcast(SessionEvent{Event: ev})
	}
}

// publishSnapshotsToSessions sends a per-player follow-up snapshot on
// the session hub. Mirrors the gRPC DoTick path where moved players
// receive a fresh viewport centred on their new position.
func (s *Service) publishSnapshotsToSessions(snaps map[string]*pb.Snapshot) {
	if s.sessions == nil || len(snaps) == 0 {
		return
	}
	for id, snap := range snaps {
		s.sessions.sendTo(id, SessionEvent{IsSnapshot: true, Snapshot: snap})
	}
}

// ctxWatch is the goroutine body that unsubscribes a session when the
// per-session SSH context is cancelled. Blocks on ctx.Done() and then
// calls unsub; because unsub is idempotent (guarded by sync.Once in
// the hub) a concurrent explicit teardown remains safe. playerID is
// captured for diagnostic logging when the future cancellation path
// needs to distinguish between many open sessions.
func (s *Service) ctxWatch(ctx context.Context, playerID string, unsub func()) {
	<-ctx.Done()
	s.log.Debug("session ctx cancelled, unsubscribing", "id", playerID)
	unsub()
}

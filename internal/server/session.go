package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// SessionJoinResult is the bundle a fresh SSH-mode subscriber needs
// to start rendering: the server-assigned player ID, spawn position,
// world seed (for local influence sampling), and the initial viewport
// snapshot. Returned by JoinSession so the caller can present the
// first frame synchronously before wiring the channel-based event feed.
type SessionJoinResult struct {
	PlayerID  string
	Spawn     game.Position
	WorldSeed int64
	Snapshot  *pb.Snapshot
	// Events carries the events produced by the Join itself (one
	// PlayerJoined). Returned so the caller can broadcast them to other
	// subscribers without having to peek at the internal state.
	Events []game.Event
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
	stats game.CoreStats,
) (SessionJoinResult, error) {
	if name == "" {
		return SessionJoinResult{}, errors.New("session join: name required")
	}
	if stats == (game.CoreStats{}) {
		stats = game.DefaultCoreStats()
	} else if _, err := game.NewStatsPointBuy(
		stats.Strength, stats.Dexterity, stats.Constitution,
		stats.Intelligence, stats.Wisdom, stats.Charisma,
	); err != nil {
		return SessionJoinResult{}, fmt.Errorf("session join: invalid stats: %w", err)
	}

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
		Events:    events,
	}, nil
}

// Subscribe registers a session subscriber by playerID and returns a
// read-only SessionEvent channel plus an unsubscribe function. The hub
// is internal to Service; exposing the channel directly keeps the
// session package free of server-internals awareness.
//
// Contract: callers MUST either drain the channel or call the returned
// unsubscribe function promptly — the broadcast path uses a non-blocking
// send and will drop events for stuck subscribers, but an undrained
// channel that is never unsubscribed leaks the buffer until Service
// shutdown.
func (s *Service) Subscribe(playerID string) (<-chan SessionEvent, func()) {
	if s.sessions == nil {
		s.sessions = newSessionHub(s.log)
	}
	return s.sessions.subscribe(playerID)
}

// LeaveSession fires a LeaveCmd and broadcasts the resulting events.
// The caller is responsible for invoking the unsubscribe function
// returned from Subscribe; LeaveSession only touches world state and
// the event bus. Safe to call even if the session was never fully
// joined (the ApplyCommand path returns ErrPlayerNotFound which we
// swallow since nothing to clean up).
func (s *Service) LeaveSession(playerID string) {
	leaveEvents, err := s.applyCmd(game.LeaveCmd{PlayerID: playerID})
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
	err := s.world.EnqueueIntent(playerID, game.MoveIntent{DX: dx, DY: dy})
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
func (s *Service) publishEventsToSessions(events []game.Event) {
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

// ctxWatch is a small helper for sessions that want to unsubscribe when
// the per-session ssh context is cancelled. The returned goroutine
// exits cleanly both on ctx.Done() (SSH connection closing) and when
// done is closed (explicit session teardown). Kept here rather than in
// internal/session so sessions have a single entry point into the
// Service's cleanup contract.
func (s *Service) ctxWatch(ctx context.Context, done <-chan struct{}, unsub func()) {
	go func() {
		select {
		case <-ctx.Done():
		case <-done:
		}
		unsub()
	}()
}

package server

import (
	"log/slog"
	"sync"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// sessionChanCap is the per-subscriber outbox buffer. Sized to absorb a
// single tick's worth of events without blocking the broadcast loop — the
// current domain emits at most a handful of events per tick (one per
// player action), and a size-16 buffer gives enough slack for bursty
// periods (join + initial snapshot + a few moves) while still detecting
// a slow subscriber quickly.
const sessionChanCap = 16

// SessionEvent is what an in-process session subscriber (SSH Bubble Tea
// session) receives. It wraps a domain game.Event plus a snapshot marker:
// when IsSnapshot is true the session should replace its local viewport
// with Snapshot; otherwise Event is an incremental domain event. Keeps the
// wire format (protobuf) decoupled from the session channel so SSH
// sessions do not have to reimplement gRPC wire glue to render events.
type SessionEvent struct {
	// Event is the domain event emitted by the World (PlayerJoined,
	// EntityMoved, etc.). Zero when IsSnapshot is true.
	Event game.Event

	// IsSnapshot signals that Snapshot carries a fresh viewport for the
	// subscriber. Used for the initial post-join snapshot and for
	// follow-up snapshots when the player moves.
	IsSnapshot bool

	// Snapshot carries a centred viewport + region for the subscriber.
	// Non-nil iff IsSnapshot is true. Reuses the gRPC wire type so the
	// SSH-mode Model can share the same render pipeline as the gRPC
	// client — the type is already plain data, no network dependency.
	Snapshot *pb.Snapshot

	// Accepted carries the post-join metadata (player ID, spawn, world
	// seed). Non-nil only on the one-shot accept event delivered
	// immediately after a successful Subscribe.
	Accepted *SessionAccepted
}

// SessionAccepted is the session analogue of the gRPC JoinAccepted
// message: it carries the identity data the client needs to anchor
// rendering (player ID for "me" detection, world seed for local
// influence sampling) and the spawn position that will also appear in
// the first snapshot. The session hub sends exactly one of these per
// subscription, immediately after a successful Join.
type SessionAccepted struct {
	PlayerID  string
	Spawn     game.Position
	WorldSeed int64
}

// sessionHub is the in-process analogue of Hub. Where Hub fans out
// *pb.ServerMessage values to gRPC subscribers, sessionHub fans out
// SessionEvent values to in-process Bubble Tea sessions. Separating the
// two hubs keeps the wire encoding (protobuf) out of the SSH path.
//
// sessionHub is safe for concurrent use.
type sessionHub struct {
	mu   sync.RWMutex
	subs map[string]chan SessionEvent
	log  *slog.Logger
}

// newSessionHub constructs an empty sessionHub. If log is nil, slog.Default
// is used so the hub can always log dropped events without a nil guard.
func newSessionHub(log *slog.Logger) *sessionHub {
	if log == nil {
		log = slog.Default()
	}
	return &sessionHub{
		subs: make(map[string]chan SessionEvent),
		log:  log,
	}
}

// subscribe registers a session by playerID and returns the read-only
// outbox plus an unsubscribe function. Calling unsubscribe is idempotent;
// the channel is closed at most once, inside the hub's lock.
func (h *sessionHub) subscribe(playerID string) (<-chan SessionEvent, func()) {
	ch := make(chan SessionEvent, sessionChanCap)

	h.mu.Lock()
	if _, exists := h.subs[playerID]; exists {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subs[playerID] = ch
	h.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			existing, ok := h.subs[playerID]
			if !ok || existing != ch {
				return
			}
			delete(h.subs, playerID)
			close(ch)
		})
	}
	return ch, unsub
}

// sendTo delivers evt only to the subscriber with playerID. Returns true
// iff the subscriber exists and the outbox accepted the event. Uses a
// non-blocking send so a slow subscriber never stalls the caller — this is
// the critical invariant that keeps DoTick from blocking on one stuck session.
func (h *sessionHub) sendTo(playerID string, evt SessionEvent) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	ch, ok := h.subs[playerID]
	if !ok {
		return false
	}
	return h.trySend(playerID, ch, evt)
}

// broadcast sends evt to every current subscriber without blocking. Slow
// subscribers have the event dropped and logged; the hub never stalls the
// broadcast loop (would cascade into the tick loop). The subscriber set
// is snapshot under the read mutex, then released before any sends run —
// trySend is non-blocking so holding the lock was functionally safe, but
// releasing it first removes a subtle coupling between broadcast cadence
// and sync.Once-guarded unsub paths that briefly contest the write lock.
func (h *sessionHub) broadcast(evt SessionEvent) {
	h.mu.RLock()
	subs := make([]sessionSubscriber, 0, len(h.subs))
	for id, ch := range h.subs {
		subs = append(subs, sessionSubscriber{id: id, ch: ch})
	}
	h.mu.RUnlock()
	for _, sub := range subs {
		h.trySend(sub.id, sub.ch, evt)
	}
}

// sessionSubscriber is the snapshot pair used by broadcast to fan out
// events without holding the hub mutex. A value type so the snapshot
// slice is a single heap allocation per broadcast.
type sessionSubscriber struct {
	id string
	ch chan SessionEvent
}

// trySend attempts a non-blocking send. Logs and returns false if the
// subscriber's outbox is full. sendTo calls this under the read lock
// (channel lookup + send as one critical section); broadcast calls it
// after releasing the lock on a snapshot copy of the subscriber set.
//
// The snapshot-then-send pattern races with unsub, which takes the
// write lock to delete + close ch. A send on a closed channel panics;
// recover absorbs that rare interleaving so broadcast finishes the
// remaining subscribers cleanly instead of crashing the tick loop.
// The returned bool is false on panic too, matching the "drop" branch.
func (h *sessionHub) trySend(playerID string, ch chan SessionEvent, evt SessionEvent) (sent bool) {
	defer func() {
		if r := recover(); r != nil {
			h.log.Debug("session hub: send on closed channel, subscriber gone",
				"id", playerID)
			sent = false
		}
	}()
	select {
	case ch <- evt:
		return true
	default:
		h.log.Warn("session hub: dropped event for slow subscriber", "id", playerID)
		return false
	}
}

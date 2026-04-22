// apply.go contains all world-mutation handlers: lifecycle operations (join,
// leave), the legacy MoveCmd adapter kept for pre-tick domain tests, and the
// actual move resolvers (applyMoveIntent, applyMonsterMoveIntent) called by
// Tick via resolveIntent in tick.go.
//
// File not renamed to lifecycle.go: after the M2 refactor it still hosts
// applyMoveIntent and applyMonsterMoveIntent, which are gameplay — not
// lifecycle — logic. A rename would misrepresent the file's scope. If a
// future refactor extracts move resolution into its own file, the remainder
// (applyJoin, applyLeave, findSpawn) would then cleanly live in lifecycle.go.
//
// For new gameplay actions use EnqueueIntent + Tick (see tick.go).
// For join/leave use ApplyCommand directly.

package game

import "fmt"

// spawnSearchRadius caps how far from the origin the spawn scanner looks
// before giving up. An infinite world cannot be exhaustively searched, so a
// finite budget guarantees Join eventually fails rather than hanging.
const spawnSearchRadius = 32

// ApplyCommand advances world state by one domain command, returning the
// events that describe the transition. On error the world is left unchanged
// and the returned event slice is empty. ApplyCommand is pure in the usual
// domain sense: no I/O, no wall clock, no randomness. It is NOT safe for
// concurrent use; the server serialises calls with a mutex.
func (w *World) ApplyCommand(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case JoinCmd:
		return w.applyJoin(c)
	case MoveCmd:
		return w.applyMove(c)
	case LeaveCmd:
		return w.applyLeave(c)
	default:
		return nil, ErrUnknownCommand
	}
}

func (w *World) applyJoin(c JoinCmd) ([]Event, error) {
	if c.PlayerID == "" || c.Name == "" {
		return nil, fmt.Errorf("join: %w", ErrInvalidPlayer)
	}
	if _, exists := w.players[c.PlayerID]; exists {
		return nil, fmt.Errorf("join: %w", ErrPlayerExists)
	}

	spawn, ok := w.findSpawn()
	if !ok {
		return nil, fmt.Errorf("join: %w", ErrNoSpawn)
	}

	// Fall back to the neutral baseline when the caller passed the zero
	// value — applyJoin is called both from the server (validated stats)
	// and from domain tests that predate the stats payload and still
	// issue bare JoinCmd{PlayerID, Name} literals.
	stats := c.Stats
	if stats == (CoreStats{}) {
		stats = DefaultCoreStats()
	}

	player, err := NewPlayer(c.PlayerID, c.Name, stats, spawn)
	if err != nil {
		return nil, fmt.Errorf("join: %w", err)
	}

	w.players[c.PlayerID] = player
	w.positions[c.PlayerID] = spawn
	w.occupants[spawn] = player

	events := make([]Event, 0, 1)
	events = append(events, PlayerJoinedEvent{
		PlayerID: c.PlayerID,
		Name:     c.Name,
		Position: spawn,
	})
	return events, nil
}

// applyMove is the legacy Command-level entry point kept for domain tests
// that predate the tick-resolution refactor. New gameplay code (and the
// server) funnels moves through EnqueueIntent + Tick, which call
// applyMoveIntent directly. The two paths agree on shape: applyMove
// wraps applyMoveIntent's structured outcome back into ApplyCommand's
// (events, error) contract so callers outside the tick loop stay fluent.
func (w *World) applyMove(c MoveCmd) ([]Event, error) {
	player, ok := w.players[c.PlayerID]
	if !ok {
		return nil, fmt.Errorf("move: %w", ErrPlayerNotFound)
	}
	if _, ok := w.positions[c.PlayerID]; !ok {
		return nil, fmt.Errorf("move: %w", ErrPlayerNotFound)
	}
	events, ok := w.applyMoveIntent(player, MoveIntent{DX: c.DX, DY: c.DY})
	if !ok {
		return nil, intentFailReasonToError(reasonFromEvents(events))
	}
	return events, nil
}

// applyMoveIntent resolves a MoveIntent for the given player, returning the
// events produced and a success flag. On success the returned slice holds
// an EntityMovedEvent and ok is true; the world has been mutated. On
// failure the slice holds a single IntentFailedEvent carrying a locale
// catalog key in Reason and ok is false; the world is left unchanged so
// the caller (Tick) can refund Energy. applyMoveIntent never panics on
// bad input — an invalid step produces an IntentFailedEvent, not an
// error, so Tick treats every failure mode uniformly.
func (w *World) applyMoveIntent(p *Player, i MoveIntent) ([]Event, bool) {
	if !validStep(i.DX, i.DY) {
		return []Event{IntentFailedEvent{
			EntityID: p.ID,
			Reason:   ReasonIntentMoveInvalid,
		}}, false
	}
	from, ok := w.positions[p.ID]
	if !ok {
		return []Event{IntentFailedEvent{
			EntityID: p.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}
	to := from.Add(i.DX, i.DY)

	target, _ := w.TileAt(to)
	if !target.Terrain.Passable() {
		return []Event{IntentFailedEvent{
			EntityID: p.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}
	if _, occupied := w.occupants[to]; occupied {
		return []Event{IntentFailedEvent{
			EntityID: p.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}

	delete(w.occupants, from)
	w.occupants[to] = p
	w.positions[p.ID] = to

	events := make([]Event, 0, 1)
	events = append(events, EntityMovedEvent{
		EntityID: p.ID,
		From:     from,
		To:       to,
	})
	return events, true
}

// applyMonsterMoveIntent mirrors applyMoveIntent for monsters. Monster
// positions are NOT tracked in the positions or occupants maps in M4 —
// monsters are server-side infra only and do not yet interact with terrain
// or player occupancy. The method emits an EntityMovedEvent so the tick
// loop can observe monster movement in tests; full spatial integration is
// deferred to Phase 6 when AI is introduced.
//
// M4 scope: monsters that receive a MoveIntent (set directly in tests)
// will emit an EntityMovedEvent and consume Energy. No collision or
// bounds check is performed — there are no world positions for monsters yet.
func (w *World) applyMonsterMoveIntent(m *Monster, i MoveIntent) ([]Event, bool) {
	if !validStep(i.DX, i.DY) {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveInvalid,
		}}, false
	}
	return []Event{EntityMovedEvent{
		EntityID: m.ID,
		From:     Position{},
		To:       Position{X: i.DX, Y: i.DY},
	}}, true
}

// reasonFromEvents pulls the Reason out of an IntentFailedEvent in a
// short slice produced by applyMoveIntent on failure. Returns the empty
// string when no IntentFailedEvent is present, which never happens on
// the failure path but keeps applyMove's adapter defensive.
func reasonFromEvents(events []Event) string {
	for _, ev := range events {
		if f, ok := ev.(IntentFailedEvent); ok {
			return f.Reason
		}
	}
	return ""
}

// intentFailReasonToError maps a locale-key reason to the legacy sentinel
// error the Command-style path returns. Keeps ApplyCommand(MoveCmd)'s
// error-return contract stable for the existing domain tests while the
// tick-based path carries structured IntentFailedEvent.
func intentFailReasonToError(reason string) error {
	switch reason {
	case ReasonIntentMoveInvalid:
		return fmt.Errorf("move: %w", ErrInvalidMove)
	case ReasonIntentMoveBlocked:
		return fmt.Errorf("move: %w", ErrBlocked)
	default:
		return fmt.Errorf("move: %w", ErrBlocked)
	}
}

func (w *World) applyLeave(c LeaveCmd) ([]Event, error) {
	if _, ok := w.players[c.PlayerID]; !ok {
		return nil, fmt.Errorf("leave: %w", ErrPlayerNotFound)
	}
	if pos, ok := w.positions[c.PlayerID]; ok {
		delete(w.occupants, pos)
	}
	delete(w.players, c.PlayerID)
	delete(w.positions, c.PlayerID)

	events := make([]Event, 0, 1)
	events = append(events, PlayerLeftEvent(c))
	return events, nil
}

// findSpawn walks outward from the origin in expanding square rings and
// returns the first passable, unoccupied tile. The radius is bounded by
// spawnSearchRadius so the scan terminates even on a pathological seed
// (e.g. origin in the middle of an ocean).
func (w *World) findSpawn() (Position, bool) {
	if p, ok := w.spawnCandidate(0, 0); ok {
		return p, true
	}
	for r := 1; r <= spawnSearchRadius; r++ {
		// Top and bottom edges of the ring.
		for x := -r; x <= r; x++ {
			if p, ok := w.spawnCandidate(x, -r); ok {
				return p, true
			}
			if p, ok := w.spawnCandidate(x, r); ok {
				return p, true
			}
		}
		// Left and right edges, excluding the corners already covered above.
		for y := -r + 1; y <= r-1; y++ {
			if p, ok := w.spawnCandidate(-r, y); ok {
				return p, true
			}
			if p, ok := w.spawnCandidate(r, y); ok {
				return p, true
			}
		}
	}
	return Position{}, false
}

// spawnCandidate tests a single tile for spawn eligibility.
func (w *World) spawnCandidate(x, y int) (Position, bool) {
	p := Position{X: x, Y: y}
	if _, busy := w.occupants[p]; busy {
		return Position{}, false
	}
	t := w.source.TileAt(x, y)
	if !t.Terrain.Passable() {
		return Position{}, false
	}
	return p, true
}

// validStep reports whether (dx, dy) is a four-directional unit step: exactly
// one axis moves by +-1, the other is zero.
func validStep(dx, dy int) bool {
	if dx == 0 && dy == 0 {
		return false
	}
	if dx != 0 && dy != 0 {
		return false
	}
	if dx < -1 || dx > 1 || dy < -1 || dy > 1 {
		return false
	}
	return true
}

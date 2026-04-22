// apply.go contains all world-mutation handlers: lifecycle operations (join,
// leave) driven by ApplyCommand and the resolvers (applyMoveIntent,
// applyMonsterMoveIntent) called by Tick via resolveIntent in tick.go.
//
// For gameplay actions use EnqueueIntent + Tick (see tick.go). For
// join/leave use ApplyCommand directly. MoveCmd is the wire-level
// command type used by the server mapper to translate pb.MoveCmd into
// a domain value before handing it off to EnqueueIntent; it does not
// flow through ApplyCommand.

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
//
// MoveCmd is not handled here — gameplay actions (moves, attacks,
// future intents) flow through EnqueueIntent + Tick so refund
// semantics and turn ordering stay in a single path. Passing a
// MoveCmd to ApplyCommand returns ErrUnknownCommand.
func (w *World) ApplyCommand(cmd Command) ([]Event, error) {
	switch c := cmd.(type) {
	case JoinCmd:
		return w.applyJoin(c)
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
	if _, occupied := w.monsterOccupants[to]; occupied {
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

// applyMonsterMoveIntent mirrors applyMoveIntent for monsters, running
// the same four-step collision check: step shape, world bounds,
// destination terrain, destination occupancy. On success the monster's
// Position field and the monsterOccupants map update atomically and an
// EntityMovedEvent is emitted; on failure an IntentFailedEvent carries
// the locale key back to Tick, which refunds Energy.
//
// Occupancy is checked against both monsters (monsterOccupants) and
// players (occupants) so a monster cannot step onto a player's tile
// and vice versa; players check the symmetric condition inside
// applyMoveIntent.
func (w *World) applyMonsterMoveIntent(m *Monster, i MoveIntent) ([]Event, bool) {
	if !validStep(i.DX, i.DY) {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveInvalid,
		}}, false
	}
	from := m.Position
	to := from.Add(i.DX, i.DY)
	if !w.InBounds(to) {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}

	target, _ := w.TileAt(to)
	if !target.Terrain.Passable() {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}
	if _, occupied := w.occupants[to]; occupied {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}
	if other, occupied := w.monsterOccupants[to]; occupied && other != m {
		return []Event{IntentFailedEvent{
			EntityID: m.ID,
			Reason:   ReasonIntentMoveBlocked,
		}}, false
	}

	delete(w.monsterOccupants, from)
	w.monsterOccupants[to] = m
	m.Position = to

	events := make([]Event, 0, 1)
	events = append(events, EntityMovedEvent{
		EntityID: m.ID,
		From:     from,
		To:       to,
	})
	return events, true
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

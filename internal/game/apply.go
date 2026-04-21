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

	player, err := NewPlayer(c.PlayerID, c.Name, 1, 1, 1)
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

func (w *World) applyMove(c MoveCmd) ([]Event, error) {
	if !validStep(c.DX, c.DY) {
		return nil, fmt.Errorf("move: %w", ErrInvalidMove)
	}
	player, ok := w.players[c.PlayerID]
	if !ok {
		return nil, fmt.Errorf("move: %w", ErrPlayerNotFound)
	}
	from, ok := w.positions[c.PlayerID]
	if !ok {
		return nil, fmt.Errorf("move: %w", ErrPlayerNotFound)
	}
	to := from.Add(c.DX, c.DY)

	target, _ := w.TileAt(to)
	if !target.Terrain.Passable() {
		return nil, fmt.Errorf("move: %w", ErrBlocked)
	}
	if _, occupied := w.occupants[to]; occupied {
		return nil, fmt.Errorf("move: %w", ErrBlocked)
	}

	delete(w.occupants, from)
	w.occupants[to] = player
	w.positions[c.PlayerID] = to

	events := make([]Event, 0, 1)
	events = append(events, EntityMovedEvent{
		EntityID: c.PlayerID,
		From:     from,
		To:       to,
	})
	return events, nil
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

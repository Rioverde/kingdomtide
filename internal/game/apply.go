package game

import "fmt"

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
	w.tiles[w.index(spawn)].Occupant = player

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
		// players and positions are kept in sync by this package; a miss
		// here is a bug, but surfacing it as ErrPlayerNotFound is the
		// safest external behaviour.
		return nil, fmt.Errorf("move: %w", ErrPlayerNotFound)
	}
	to := from.Add(c.DX, c.DY)

	if !w.InBounds(to) {
		return nil, fmt.Errorf("move: %w", ErrBlocked)
	}
	target := &w.tiles[w.index(to)]
	if !target.Terrain.Passable() {
		return nil, fmt.Errorf("move: %w", ErrBlocked)
	}
	if target.Occupant != nil {
		return nil, fmt.Errorf("move: %w", ErrBlocked)
	}

	w.tiles[w.index(from)].Occupant = nil
	target.Occupant = player
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
		w.tiles[w.index(pos)].Occupant = nil
	}
	delete(w.players, c.PlayerID)
	delete(w.positions, c.PlayerID)

	events := make([]Event, 0, 1)
	events = append(events, PlayerLeftEvent(c))
	return events, nil
}

// findSpawn scans the interior of the world (excluding the border row/column)
// in row-major order and returns the first passable, unoccupied tile.
func (w *World) findSpawn() (Position, bool) {
	for y := 1; y < w.height-1; y++ {
		for x := 1; x < w.width-1; x++ {
			p := Position{X: x, Y: y}
			t := w.tiles[w.index(p)]
			if t.Terrain.Passable() && t.Occupant == nil {
				return p, true
			}
		}
	}
	return Position{}, false
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

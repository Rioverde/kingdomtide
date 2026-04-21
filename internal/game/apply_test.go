package game

import (
	"errors"
	"testing"
)

// unknownCmd is a test-local Command used to exercise the ApplyCommand
// default branch. It lives in the test binary only and never reaches
// production code.
type unknownCmd struct{}

func (unknownCmd) isCommand() {}

// joinPlayer is a test helper that runs a JoinCmd and returns the spawn
// position reported by the resulting PlayerJoinedEvent. It fails the test on
// any error or unexpected event shape so callers stay short.
func joinPlayer(t *testing.T, w *World, id, name string) Position {
	t.Helper()
	events, err := w.ApplyCommand(JoinCmd{PlayerID: id, Name: name})
	if err != nil {
		t.Fatalf("join %q: %v", id, err)
	}
	if len(events) != 1 {
		t.Fatalf("join %q: events = %d, want 1", id, len(events))
	}
	joined, ok := events[0].(PlayerJoinedEvent)
	if !ok {
		t.Fatalf("join %q: event = %T, want PlayerJoinedEvent", id, events[0])
	}
	if joined.PlayerID != id || joined.Name != name {
		t.Fatalf("join %q: event = %+v", id, joined)
	}
	return joined.Position
}

func TestApplyJoinHappyPath(t *testing.T) {
	w := NewMockWorld()
	events, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	joined, ok := events[0].(PlayerJoinedEvent)
	if !ok {
		t.Fatalf("event = %T, want PlayerJoinedEvent", events[0])
	}
	if joined.PlayerID != "p1" || joined.Name != "Alice" {
		t.Fatalf("event = %+v", joined)
	}

	// Player is registered.
	players := w.Players()
	if len(players) != 1 || players[0].ID != "p1" {
		t.Fatalf("players = %+v, want single p1", players)
	}

	// Position matches the event.
	pos, ok := w.PositionOf("p1")
	if !ok {
		t.Fatalf("PositionOf(p1) !ok")
	}
	if !pos.Equal(joined.Position) {
		t.Fatalf("PositionOf = %+v, want %+v", pos, joined.Position)
	}

	// Tile occupant is the created player.
	tile, ok := w.TileAt(joined.Position)
	if !ok {
		t.Fatalf("TileAt(%+v) !ok", joined.Position)
	}
	if tile.Occupant == nil {
		t.Fatalf("spawn tile Occupant = nil")
	}
	stored, ok := w.PlayerByID("p1")
	if !ok {
		t.Fatalf("PlayerByID(p1) !ok")
	}
	if tile.Occupant != stored {
		t.Fatalf("tile.Occupant != stored player")
	}
}

func TestApplyJoinEmptyID(t *testing.T) {
	w := NewMockWorld()
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "", Name: "Alice"})
	if !errors.Is(err, ErrInvalidPlayer) {
		t.Fatalf("err = %v, want ErrInvalidPlayer", err)
	}
	if len(w.Players()) != 0 {
		t.Fatalf("players after failed join: %d, want 0", len(w.Players()))
	}
}

func TestApplyJoinEmptyName(t *testing.T) {
	w := NewMockWorld()
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: ""})
	if !errors.Is(err, ErrInvalidPlayer) {
		t.Fatalf("err = %v, want ErrInvalidPlayer", err)
	}
	if len(w.Players()) != 0 {
		t.Fatalf("players after failed join: %d, want 0", len(w.Players()))
	}
}

func TestApplyJoinDuplicateID(t *testing.T) {
	w := NewMockWorld()
	joinPlayer(t, w, "p1", "Alice")

	before := w.Players()
	pos1, _ := w.PositionOf("p1")

	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice2"})
	if !errors.Is(err, ErrPlayerExists) {
		t.Fatalf("err = %v, want ErrPlayerExists", err)
	}

	// State unchanged.
	after := w.Players()
	if len(after) != len(before) {
		t.Fatalf("players changed: was %d, now %d", len(before), len(after))
	}
	pos2, _ := w.PositionOf("p1")
	if !pos1.Equal(pos2) {
		t.Fatalf("position changed after failed rejoin: %+v -> %+v", pos1, pos2)
	}
}

func TestApplyMoveHappyPath(t *testing.T) {
	w := NewMockWorld()
	from := joinPlayer(t, w, "p1", "Alice")

	events, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	moved, ok := events[0].(EntityMovedEvent)
	if !ok {
		t.Fatalf("event = %T, want EntityMovedEvent", events[0])
	}
	if moved.EntityID != "p1" {
		t.Fatalf("moved.EntityID = %q, want p1", moved.EntityID)
	}
	if !moved.From.Equal(from) {
		t.Fatalf("moved.From = %+v, want %+v", moved.From, from)
	}
	want := from.Add(1, 0)
	if !moved.To.Equal(want) {
		t.Fatalf("moved.To = %+v, want %+v", moved.To, want)
	}

	pos, _ := w.PositionOf("p1")
	if !pos.Equal(want) {
		t.Fatalf("PositionOf = %+v, want %+v", pos, want)
	}
	oldTile, _ := w.TileAt(from)
	if oldTile.Occupant != nil {
		t.Fatalf("old tile Occupant = %+v, want nil", oldTile.Occupant)
	}
	newTile, _ := w.TileAt(want)
	if newTile.Occupant == nil {
		t.Fatalf("new tile Occupant = nil")
	}
}

func TestApplyMoveOntoMountain(t *testing.T) {
	// On the default world, p1 spawns at (1,1). Stepping up (DY=-1) lands on
	// the mountain border at (1,0).
	w := NewMockWorld()
	joinPlayer(t, w, "p1", "Alice")

	_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 0, DY: -1})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveOntoWater(t *testing.T) {
	// Ocean in the mock layout is at x=5..7, y=4..5. Place the player on
	// the beach west of the lake (4,4); stepping east lands on ocean (5,4).
	w := NewMockWorld()
	player, err := NewPlayer("p1", "Alice", 1, 1, 1)
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	start := Position{X: 4, Y: 4}
	w.players["p1"] = player
	w.positions["p1"] = start
	w.tiles[w.index(start)].Occupant = player

	_, err = w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveOntoAnotherPlayer(t *testing.T) {
	w := NewMockWorld()
	p1Pos := joinPlayer(t, w, "p1", "Alice")
	// p2 spawns on the next interior tile in row-major scan.
	p2Pos := joinPlayer(t, w, "p2", "Bob")

	// Sanity: the two spawns are adjacent along X (scan order).
	if !p2Pos.Equal(p1Pos.Add(1, 0)) {
		t.Fatalf("spawns not adjacent: p1=%+v p2=%+v", p1Pos, p2Pos)
	}

	// p1 tries to step east onto p2's tile.
	_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveOffMap(t *testing.T) {
	// All-passable 3x3 world so the border check does NOT short-circuit
	// stepping off: the only way out is bounds.
	w := NewWorld(3, 3)
	player, err := NewPlayer("p1", "Alice", 1, 1, 1)
	if err != nil {
		t.Fatalf("new player: %v", err)
	}
	start := Position{X: 0, Y: 0}
	w.players["p1"] = player
	w.positions["p1"] = start
	w.tiles[w.index(start)].Occupant = player

	_, err = w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: -1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
	_, err = w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 0, DY: -1})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveInvalidShapes(t *testing.T) {
	w := NewMockWorld()
	joinPlayer(t, w, "p1", "Alice")

	cases := []struct {
		name   string
		dx, dy int
	}{
		{"diagonal", 1, 1},
		{"anti-diagonal", -1, 1},
		{"zero", 0, 0},
		{"oversized dx", 2, 0},
		{"oversized dy", 0, -3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: tc.dx, DY: tc.dy})
			if !errors.Is(err, ErrInvalidMove) {
				t.Fatalf("err = %v, want ErrInvalidMove", err)
			}
		})
	}
}

func TestApplyMoveUnknownPlayer(t *testing.T) {
	w := NewMockWorld()
	_, err := w.ApplyCommand(MoveCmd{PlayerID: "ghost", DX: 1, DY: 0})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("err = %v, want ErrPlayerNotFound", err)
	}
}

func TestApplyLeaveHappyPath(t *testing.T) {
	w := NewMockWorld()
	pos := joinPlayer(t, w, "p1", "Alice")

	events, err := w.ApplyCommand(LeaveCmd{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	left, ok := events[0].(PlayerLeftEvent)
	if !ok {
		t.Fatalf("event = %T, want PlayerLeftEvent", events[0])
	}
	if left.PlayerID != "p1" {
		t.Fatalf("left.PlayerID = %q, want p1", left.PlayerID)
	}

	if _, ok := w.PlayerByID("p1"); ok {
		t.Fatalf("player still registered after leave")
	}
	if _, ok := w.PositionOf("p1"); ok {
		t.Fatalf("position still registered after leave")
	}
	tile, _ := w.TileAt(pos)
	if tile.Occupant != nil {
		t.Fatalf("tile still occupied: %+v", tile.Occupant)
	}
}

func TestApplyLeaveUnknown(t *testing.T) {
	w := NewMockWorld()
	_, err := w.ApplyCommand(LeaveCmd{PlayerID: "ghost"})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("err = %v, want ErrPlayerNotFound", err)
	}
}

func TestApplyUnknownCommand(t *testing.T) {
	w := NewMockWorld()
	events, err := w.ApplyCommand(unknownCmd{})
	if !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("err = %v, want ErrUnknownCommand", err)
	}
	if len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
}

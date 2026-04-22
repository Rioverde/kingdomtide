package game

import (
	"errors"
	"testing"
)

// testTiles is an ad-hoc TileSource used by the apply tests. It lets each
// case paint a tiny handful of cells (walls, water, occupied tiles) and
// treats every other coordinate as plains so movement is unconstrained.
type testTiles map[Position]Tile

func (s testTiles) TileAt(x, y int) Tile {
	if t, ok := s[Position{X: x, Y: y}]; ok {
		return t
	}
	return Tile{Terrain: TerrainPlains}
}

// newTestWorld builds a world from an ad-hoc TileSource. Spawn scanning
// starts at the origin so tests that care about spawn position should
// place a passable tile at (0, 0).
func newTestWorld(src TileSource) *World {
	return NewWorldFromSource(src)
}

// unknownCmd exercises the ApplyCommand default branch.
type unknownCmd struct{}

func (unknownCmd) isCommand() {}

func TestApplyJoinHappyPath(t *testing.T) {
	w := newTestWorld(testTiles{})
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
		t.Fatalf("event fields: %+v", joined)
	}
	// Spawn must be a passable tile and the world must now report the player
	// at that position.
	got, ok := w.PositionOf("p1")
	if !ok || got != joined.Position {
		t.Fatalf("PositionOf(p1) = (%+v, %v), want %+v", got, ok, joined.Position)
	}
	tile, _ := w.TileAt(joined.Position)
	if !tile.Terrain.Passable() {
		t.Fatalf("spawn tile terrain %q is not passable", tile.Terrain)
	}
	if tile.Occupant == nil {
		t.Fatalf("spawn tile has no occupant")
	}
}

func TestApplyJoinValidation(t *testing.T) {
	cases := []struct {
		name string
		cmd  JoinCmd
		want error
	}{
		{"empty id", JoinCmd{PlayerID: "", Name: "Alice"}, ErrInvalidPlayer},
		{"empty name", JoinCmd{PlayerID: "p1", Name: ""}, ErrInvalidPlayer},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWorld(testTiles{})
			_, err := w.ApplyCommand(tc.cmd)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want wrapping %v", err, tc.want)
			}
		})
	}
}

func TestApplyJoinTwice(t *testing.T) {
	w := newTestWorld(testTiles{})
	if _, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"}); err != nil {
		t.Fatalf("first join: %v", err)
	}
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if !errors.Is(err, ErrPlayerExists) {
		t.Fatalf("second join err = %v, want ErrPlayerExists", err)
	}
}

func TestApplyMoveHappyPath(t *testing.T) {
	w := newTestWorld(testTiles{})
	joinEvents, _ := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	start := joinEvents[0].(PlayerJoinedEvent).Position

	events, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	moved, ok := events[0].(EntityMovedEvent)
	if !ok {
		t.Fatalf("event = %T, want EntityMovedEvent", events[0])
	}
	wantTo := Position{X: start.X + 1, Y: start.Y}
	if moved.From != start || moved.To != wantTo {
		t.Fatalf("event = %+v, want From=%+v To=%+v", moved, start, wantTo)
	}
	if pos, _ := w.PositionOf("p1"); pos != wantTo {
		t.Fatalf("PositionOf(p1) = %+v, want %+v", pos, wantTo)
	}
}

func TestApplyMoveValidation(t *testing.T) {
	cases := []struct {
		name   string
		dx, dy int
	}{
		{"zero", 0, 0},
		{"diagonal", 1, 1},
		{"too big dx", 2, 0},
		{"too big dy", 0, -2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := newTestWorld(testTiles{})
			_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
			_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: tc.dx, DY: tc.dy})
			if !errors.Is(err, ErrInvalidMove) {
				t.Fatalf("err = %v, want ErrInvalidMove", err)
			}
		})
	}
}

func TestApplyMoveUnknownPlayer(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, err := w.ApplyCommand(MoveCmd{PlayerID: "ghost", DX: 1, DY: 0})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("err = %v, want ErrPlayerNotFound", err)
	}
}

func TestApplyMoveIntoWall(t *testing.T) {
	// Wall one tile east of the origin (where spawn will land).
	src := testTiles{
		Position{X: 1, Y: 0}: {Terrain: TerrainMountain},
	}
	w := newTestWorld(src)
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveOntoWater(t *testing.T) {
	src := testTiles{
		Position{X: 1, Y: 0}: {Terrain: TerrainOcean},
	}
	w := newTestWorld(src)
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	_, err := w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyMoveOntoAnotherPlayer(t *testing.T) {
	// Manually place two players on adjacent tiles — the spawn spiral's
	// natural next position is diagonal, which would hit ErrInvalidMove
	// before reaching the occupancy check.
	w := newTestWorld(testTiles{})
	at := Position{X: 0, Y: 0}
	next := Position{X: 1, Y: 0}
	p1, err := NewPlayer("p1", "Alice", DefaultCoreStats(), at)
	if err != nil {
		t.Fatalf("new p1: %v", err)
	}
	p2, err := NewPlayer("p2", "Bob", DefaultCoreStats(), next)
	if err != nil {
		t.Fatalf("new p2: %v", err)
	}
	w.players["p1"] = p1
	w.positions["p1"] = at
	w.occupants[at] = p1
	w.players["p2"] = p2
	w.positions["p2"] = next
	w.occupants[next] = p2

	_, err = w.ApplyCommand(MoveCmd{PlayerID: "p1", DX: 1, DY: 0})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("err = %v, want ErrBlocked", err)
	}
}

func TestApplyLeaveHappyPath(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	events, err := w.ApplyCommand(LeaveCmd{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, ok := events[0].(PlayerLeftEvent); !ok {
		t.Fatalf("event = %T, want PlayerLeftEvent", events[0])
	}
	if _, ok := w.PlayerByID("p1"); ok {
		t.Fatalf("player p1 not removed")
	}
}

func TestApplyLeaveUnknown(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, err := w.ApplyCommand(LeaveCmd{PlayerID: "ghost"})
	if !errors.Is(err, ErrPlayerNotFound) {
		t.Fatalf("err = %v, want ErrPlayerNotFound", err)
	}
}

func TestApplyUnknownCommand(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, err := w.ApplyCommand(unknownCmd{})
	if !errors.Is(err, ErrUnknownCommand) {
		t.Fatalf("err = %v, want ErrUnknownCommand", err)
	}
}

func TestApplyNoSpawnAvailable(t *testing.T) {
	// Paint a disc of mountain around the origin — no passable tile exists
	// within spawnSearchRadius.
	src := testTiles{}
	for y := -spawnSearchRadius; y <= spawnSearchRadius; y++ {
		for x := -spawnSearchRadius; x <= spawnSearchRadius; x++ {
			src[Position{X: x, Y: y}] = Tile{Terrain: TerrainMountain}
		}
	}
	w := newTestWorld(src)
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if !errors.Is(err, ErrNoSpawn) {
		t.Fatalf("err = %v, want ErrNoSpawn", err)
	}
}

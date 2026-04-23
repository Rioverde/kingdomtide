package world

import (
	"errors"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/entity"
	"github.com/Rioverde/gongeons/internal/game/event"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// testTiles is an ad-hoc TileSource used by the apply tests. It lets each
// case paint a tiny handful of cells (walls, water, occupied tiles) and
// treats every other coordinate as plains so movement is unconstrained.
type testTiles map[geom.Position]Tile

func (s testTiles) TileAt(x, y int) Tile {
	if t, ok := s[geom.Position{X: x, Y: y}]; ok {
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
	joined, ok := events[0].(event.PlayerJoinedEvent)
	if !ok {
		t.Fatalf("event = %T, want event.PlayerJoinedEvent", events[0])
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

// reasonOf returns the event.IntentFailedEvent reason in a single-event
// failure slice. Fatals the test on any shape mismatch so the caller
// can read the event layout off the helper's name alone.
func reasonOf(t *testing.T, events []event.Event) string {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1 event.IntentFailedEvent", len(events))
	}
	fail, ok := events[0].(event.IntentFailedEvent)
	if !ok {
		t.Fatalf("event = %T, want event.IntentFailedEvent", events[0])
	}
	return fail.Reason
}

func TestApplyMoveIntentHappyPath(t *testing.T) {
	w := newTestWorld(testTiles{})
	joinEvents, _ := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	start := joinEvents[0].(event.PlayerJoinedEvent).Position

	p, ok := w.PlayerByID("p1")
	if !ok {
		t.Fatalf("PlayerByID(p1) missing after join")
	}
	events, moved := w.applyMoveIntent(p, MoveIntent{DX: 1, DY: 0})
	if !moved {
		t.Fatalf("move failed: events=%+v", events)
	}
	em, ok := events[0].(event.EntityMovedEvent)
	if !ok {
		t.Fatalf("event = %T, want event.EntityMovedEvent", events[0])
	}
	wantTo := geom.Position{X: start.X + 1, Y: start.Y}
	if em.From != start || em.To != wantTo {
		t.Fatalf("event = %+v, want From=%+v To=%+v", em, start, wantTo)
	}
	if pos, _ := w.PositionOf("p1"); pos != wantTo {
		t.Fatalf("PositionOf(p1) = %+v, want %+v", pos, wantTo)
	}
}

func TestApplyMoveIntentValidation(t *testing.T) {
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
			p, _ := w.PlayerByID("p1")
			events, ok := w.applyMoveIntent(p, MoveIntent{DX: tc.dx, DY: tc.dy})
			if ok {
				t.Fatalf("move succeeded, want failure: events=%+v", events)
			}
			if got := reasonOf(t, events); got != event.ReasonIntentMoveInvalid {
				t.Fatalf("reason = %q, want %q", got, event.ReasonIntentMoveInvalid)
			}
		})
	}
}

func TestApplyMoveIntentIntoWall(t *testing.T) {
	// Wall one tile east of the origin (where spawn will land).
	src := testTiles{
		geom.Position{X: 1, Y: 0}: {Terrain: TerrainMountain},
	}
	w := newTestWorld(src)
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	p, _ := w.PlayerByID("p1")
	events, ok := w.applyMoveIntent(p, MoveIntent{DX: 1, DY: 0})
	if ok {
		t.Fatalf("move into wall succeeded, want failure")
	}
	if got := reasonOf(t, events); got != event.ReasonIntentMoveBlocked {
		t.Fatalf("reason = %q, want %q", got, event.ReasonIntentMoveBlocked)
	}
}

func TestApplyMoveIntentOntoWater(t *testing.T) {
	src := testTiles{
		geom.Position{X: 1, Y: 0}: {Terrain: TerrainOcean},
	}
	w := newTestWorld(src)
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	p, _ := w.PlayerByID("p1")
	events, ok := w.applyMoveIntent(p, MoveIntent{DX: 1, DY: 0})
	if ok {
		t.Fatalf("move onto water succeeded, want failure")
	}
	if got := reasonOf(t, events); got != event.ReasonIntentMoveBlocked {
		t.Fatalf("reason = %q, want %q", got, event.ReasonIntentMoveBlocked)
	}
}

func TestApplyMoveIntentOntoAnotherPlayer(t *testing.T) {
	// Manually place two players on adjacent tiles — the spawn spiral's
	// natural next position is diagonal, which would hit
	// event.ReasonIntentMoveInvalid before reaching the occupancy check.
	w := newTestWorld(testTiles{})
	at := geom.Position{X: 0, Y: 0}
	next := geom.Position{X: 1, Y: 0}
	p1, err := entity.NewPlayer("p1", "Alice", stats.DefaultCoreStats(), at)
	if err != nil {
		t.Fatalf("new p1: %v", err)
	}
	p2, err := entity.NewPlayer("p2", "Bob", stats.DefaultCoreStats(), next)
	if err != nil {
		t.Fatalf("new p2: %v", err)
	}
	w.players["p1"] = p1
	w.positions["p1"] = at
	w.occupants[at] = p1
	w.players["p2"] = p2
	w.positions["p2"] = next
	w.occupants[next] = p2

	events, ok := w.applyMoveIntent(p1, MoveIntent{DX: 1, DY: 0})
	if ok {
		t.Fatalf("move onto occupied tile succeeded, want failure")
	}
	if got := reasonOf(t, events); got != event.ReasonIntentMoveBlocked {
		t.Fatalf("reason = %q, want %q", got, event.ReasonIntentMoveBlocked)
	}
}

func TestApplyLeaveHappyPath(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, _ = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	events, err := w.ApplyCommand(LeaveCmd{PlayerID: "p1"})
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, ok := events[0].(event.PlayerLeftEvent); !ok {
		t.Fatalf("event = %T, want event.PlayerLeftEvent", events[0])
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
			src[geom.Position{X: x, Y: y}] = Tile{Terrain: TerrainMountain}
		}
	}
	w := newTestWorld(src)
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if !errors.Is(err, ErrNoSpawn) {
		t.Fatalf("err = %v, want ErrNoSpawn", err)
	}
}

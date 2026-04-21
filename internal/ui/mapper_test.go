package ui

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

func TestPositionFromPB(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   *pb.Position
		want game.Position
	}{
		{"nil returns origin", nil, game.Position{}},
		{"positive", &pb.Position{X: 3, Y: 5}, game.Position{X: 3, Y: 5}},
		{"negative", &pb.Position{X: -1, Y: -2}, game.Position{X: -1, Y: -2}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := positionFromPB(tc.in)
			if got != tc.want {
				t.Fatalf("positionFromPB(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// newTestModel returns a Model whose local viewport covers world columns
// 10..12 and rows 10..11. Picking a non-zero origin catches index-math
// bugs that a (0,0)-origin viewport would hide.
func newTestModel() *Model {
	m := &Model{
		players: make(map[string]playerInfo),
		width:   3,
		height:  2,
		origin:  game.Position{X: 10, Y: 10},
	}
	tiles := make([]*pb.Tile, 6)
	for i := range tiles {
		tiles[i] = &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS}
	}
	m.tiles = tiles
	return m
}

func TestApplySnapshotResetsState(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.players["stale"] = playerInfo{ID: "stale"}
	m.logLines = []string{"old"}

	snap := &pb.Snapshot{
		Width:  2,
		Height: 2,
		Origin: &pb.Position{X: 5, Y: 5},
		Tiles: []*pb.Tile{
			{Terrain: pb.Terrain_TERRAIN_MOUNTAIN},
			{Terrain: pb.Terrain_TERRAIN_PLAINS},
			{Terrain: pb.Terrain_TERRAIN_PLAINS, Occupant: pb.OccupantKind_OCCUPANT_PLAYER, EntityId: "a"},
			{Terrain: pb.Terrain_TERRAIN_OCEAN},
		},
		Entities: []*pb.Entity{
			{Id: "a", Name: "alice", Kind: pb.OccupantKind_OCCUPANT_PLAYER, Position: &pb.Position{X: 5, Y: 6}},
		},
	}
	applySnapshot(m, snap)

	if m.width != 2 || m.height != 2 {
		t.Fatalf("dims = %dx%d, want 2x2", m.width, m.height)
	}
	if m.origin != (game.Position{X: 5, Y: 5}) {
		t.Fatalf("origin = %+v, want (5,5)", m.origin)
	}
	if len(m.tiles) != 4 {
		t.Fatalf("tiles len = %d, want 4", len(m.tiles))
	}
	if _, ok := m.players["stale"]; ok {
		t.Fatalf("stale player was not cleared")
	}
	got, ok := m.players["a"]
	if !ok {
		t.Fatalf("entity a missing from players map")
	}
	if got.Name != "alice" || got.Pos != (game.Position{X: 5, Y: 6}) {
		t.Fatalf("entity a = %+v", got)
	}
}

func TestApplyEventPlayerJoined(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	ev := &pb.Event{
		Payload: &pb.Event_PlayerJoined{
			PlayerJoined: &pb.PlayerJoined{
				Entity: &pb.Entity{
					Id:       "bob",
					Name:     "bob",
					Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
					Position: &pb.Position{X: 11, Y: 10}, // local (1,0) given origin (10,10)
				},
			},
		},
	}
	applyEvent(m, ev)

	if _, ok := m.players["bob"]; !ok {
		t.Fatalf("bob not added to players map")
	}
	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(m.logLines))
	}
	tile := m.tiles[1] // local (1,0)
	if tile.GetEntityId() != "bob" || tile.GetOccupant() != pb.OccupantKind_OCCUPANT_PLAYER {
		t.Fatalf("tile local(1,0) = %+v, want bob occupant", tile)
	}
}

func TestApplyEventEntityMovedTracksMyPosition(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.myID = "me"
	m.players["me"] = playerInfo{ID: "me", Name: "me", Pos: game.Position{X: 10, Y: 10}}
	m.tiles[0].Occupant = pb.OccupantKind_OCCUPANT_PLAYER
	m.tiles[0].EntityId = "me"

	ev := &pb.Event{
		Payload: &pb.Event_EntityMoved{
			EntityMoved: &pb.EntityMoved{
				EntityId: "me",
				From:     &pb.Position{X: 10, Y: 10},
				To:       &pb.Position{X: 11, Y: 10},
			},
		},
	}
	applyEvent(m, ev)

	if pos := m.players["me"].Pos; pos != (game.Position{X: 11, Y: 10}) {
		t.Fatalf("my position = %v, want (11,10)", pos)
	}
	if m.tiles[0].GetEntityId() != "" {
		t.Fatalf("old tile still claims me: %+v", m.tiles[0])
	}
	if m.tiles[1].GetEntityId() != "me" {
		t.Fatalf("new tile missing me: %+v", m.tiles[1])
	}
}

func TestApplyEventPlayerLeft(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.players["a"] = playerInfo{ID: "a", Name: "alice", Pos: game.Position{X: 12, Y: 11}}
	idx := 1*m.width + 2 // local (2,1)
	m.tiles[idx].Occupant = pb.OccupantKind_OCCUPANT_PLAYER
	m.tiles[idx].EntityId = "a"

	ev := &pb.Event{
		Payload: &pb.Event_PlayerLeft{
			PlayerLeft: &pb.PlayerLeft{PlayerId: "a"},
		},
	}
	applyEvent(m, ev)

	if _, ok := m.players["a"]; ok {
		t.Fatalf("player a not removed")
	}
	if m.tiles[idx].GetEntityId() != "" {
		t.Fatalf("tile still claims a: %+v", m.tiles[idx])
	}
}

func TestAppendLogCap(t *testing.T) {
	t.Parallel()
	m := &Model{}
	for range logLinesCap + 3 {
		m.appendLog("line")
	}
	if len(m.logLines) != logLinesCap {
		t.Fatalf("log len = %d, want %d", len(m.logLines), logLinesCap)
	}
}

func TestLookTileKnownAndUnknown(t *testing.T) {
	t.Parallel()
	known := &pb.Tile{Terrain: pb.Terrain_TERRAIN_FOREST}
	r, _ := lookTile(known)
	if r == runeUnspecified {
		t.Fatalf("lookTile(known biome) returned unspecified rune")
	}
	unknown := &pb.Tile{Terrain: pb.Terrain(999)}
	r, _ = lookTile(unknown)
	if r != runeUnspecified {
		t.Fatalf("lookTile(unknown) = %q, want %q", r, runeUnspecified)
	}
}

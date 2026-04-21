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

func TestBucketOf(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   pb.Terrain
		want terrainBucket
	}{
		{pb.Terrain_TERRAIN_UNSPECIFIED, terrainBucketUnspecified},
		{pb.Terrain_TERRAIN_FLOOR, terrainBucketFloor},
		{pb.Terrain_TERRAIN_GRASS, terrainBucketFloor},
		{pb.Terrain_TERRAIN_WALL, terrainBucketWall},
		{pb.Terrain_TERRAIN_WATER, terrainBucketWater},
	}
	for _, tc := range cases {
		if got := bucketOf(tc.in); got != tc.want {
			t.Fatalf("bucketOf(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// newTestModel returns a Model prepared with a 3x2 world so the
// applySnapshot / applyEvent helpers have somewhere to write.
func newTestModel() *Model {
	m := &Model{
		players: make(map[string]playerInfo),
	}
	tiles := make([]*pb.Tile, 6)
	for i := range tiles {
		tiles[i] = &pb.Tile{Terrain: pb.Terrain_TERRAIN_FLOOR}
	}
	m.tiles = tiles
	m.width = 3
	m.height = 2
	return m
}

func TestApplySnapshotResetsState(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	// Seed with stale data to verify the reset.
	m.players["stale"] = playerInfo{ID: "stale"}
	m.logLines = []string{"old"}

	snap := &pb.Snapshot{
		Width:  2,
		Height: 2,
		Tiles: []*pb.Tile{
			{Terrain: pb.Terrain_TERRAIN_WALL},
			{Terrain: pb.Terrain_TERRAIN_FLOOR},
			{Terrain: pb.Terrain_TERRAIN_FLOOR, Occupant: pb.OccupantKind_OCCUPANT_PLAYER, EntityId: "a"},
			{Terrain: pb.Terrain_TERRAIN_WATER},
		},
		Entities: []*pb.Entity{
			{Id: "a", Name: "alice", Kind: pb.OccupantKind_OCCUPANT_PLAYER, Position: &pb.Position{X: 0, Y: 1}},
		},
	}
	applySnapshot(m, snap)

	if m.width != 2 || m.height != 2 {
		t.Fatalf("dims = %dx%d, want 2x2", m.width, m.height)
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
	if got.Name != "alice" || got.Pos != (game.Position{X: 0, Y: 1}) {
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
					Position: &pb.Position{X: 1, Y: 0},
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
	tile := m.tiles[0*m.width+1]
	if tile.GetEntityId() != "bob" || tile.GetOccupant() != pb.OccupantKind_OCCUPANT_PLAYER {
		t.Fatalf("tile(1,0) = %+v, want bob occupant", tile)
	}
}

func TestApplyEventEntityMovedTracksMyPosition(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.myID = "me"
	m.players["me"] = playerInfo{ID: "me", Name: "me", Pos: game.Position{X: 0, Y: 0}}
	m.tiles[0].Occupant = pb.OccupantKind_OCCUPANT_PLAYER
	m.tiles[0].EntityId = "me"

	ev := &pb.Event{
		Payload: &pb.Event_EntityMoved{
			EntityMoved: &pb.EntityMoved{
				EntityId: "me",
				From:     &pb.Position{X: 0, Y: 0},
				To:       &pb.Position{X: 1, Y: 0},
			},
		},
	}
	applyEvent(m, ev)

	if pos := m.players["me"].Pos; pos != (game.Position{X: 1, Y: 0}) {
		t.Fatalf("my position = %v, want (1,0)", pos)
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
	m.players["a"] = playerInfo{ID: "a", Name: "alice", Pos: game.Position{X: 2, Y: 1}}
	// Mark the tile so the clear code path runs.
	idx := 1*m.width + 2
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

package server

import (
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

func TestClientMessageToCommandJoin(t *testing.T) {
	msg := &pb.ClientMessage{Payload: &pb.ClientMessage_Join{Join: &pb.JoinRequest{Name: "alice"}}}
	cmd, err := clientMessageToCommand(msg, "pid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jc, ok := cmd.(game.JoinCmd)
	if !ok {
		t.Fatalf("expected JoinCmd, got %T", cmd)
	}
	if jc.PlayerID != "pid-1" || jc.Name != "alice" {
		t.Fatalf("bad mapping: %+v", jc)
	}
}

func TestClientMessageToCommandMove(t *testing.T) {
	msg := &pb.ClientMessage{Payload: &pb.ClientMessage_Move{Move: &pb.MoveCmd{Dx: 1, Dy: 0}}}
	cmd, err := clientMessageToCommand(msg, "pid-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mc, ok := cmd.(game.MoveCmd)
	if !ok {
		t.Fatalf("expected MoveCmd, got %T", cmd)
	}
	if mc.PlayerID != "pid-2" || mc.DX != 1 || mc.DY != 0 {
		t.Fatalf("bad mapping: %+v", mc)
	}
}

func TestClientMessageToCommandEmpty(t *testing.T) {
	_, err := clientMessageToCommand(nil, "pid-3")
	if !errors.Is(err, errEmptyPayload) {
		t.Fatalf("want errEmptyPayload, got %v", err)
	}
	_, err = clientMessageToCommand(&pb.ClientMessage{}, "pid-3")
	if !errors.Is(err, errEmptyPayload) {
		t.Fatalf("want errEmptyPayload on bare ClientMessage, got %v", err)
	}
}

func TestEventToServerMessagePlayerJoined(t *testing.T) {
	ev := game.PlayerJoinedEvent{PlayerID: "p1", Name: "alice", Position: game.Position{X: 3, Y: 4}}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_PlayerJoined{
					PlayerJoined: &pb.PlayerJoined{
						Entity: &pb.Entity{
							Id:   "p1",
							Name: "alice",
							Kind: pb.OccupantKind_OCCUPANT_PLAYER,
							Position: &pb.Position{
								X: 3,
								Y: 4,
							},
						},
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(PlayerJoinedEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestEventToServerMessagePlayerLeft(t *testing.T) {
	ev := game.PlayerLeftEvent{PlayerID: "p1"}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_PlayerLeft{
					PlayerLeft: &pb.PlayerLeft{
						PlayerId: "p1",
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(PlayerLeftEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestEventToServerMessageEntityMoved(t *testing.T) {
	ev := game.EntityMovedEvent{
		EntityID: "p1",
		From:     game.Position{X: 1, Y: 2},
		To:       game.Position{X: 2, Y: 2},
	}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_EntityMoved{
					EntityMoved: &pb.EntityMoved{
						EntityId: "p1",
						From:     &pb.Position{X: 1, Y: 2},
						To:       &pb.Position{X: 2, Y: 2},
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(EntityMovedEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestTerrainToPBMapping(t *testing.T) {
	cases := map[game.Terrain]pb.Terrain{
		game.TerrainPlains:    pb.Terrain_TERRAIN_PLAINS,
		game.TerrainGrassland: pb.Terrain_TERRAIN_GRASSLAND,
		game.TerrainForest:    pb.Terrain_TERRAIN_FOREST,
		game.TerrainMountain:  pb.Terrain_TERRAIN_MOUNTAIN,
		game.TerrainOcean:     pb.Terrain_TERRAIN_OCEAN,
		game.TerrainDeepOcean: pb.Terrain_TERRAIN_DEEP_OCEAN,
		game.TerrainBeach:     pb.Terrain_TERRAIN_BEACH,
		game.TerrainHills:     pb.Terrain_TERRAIN_HILLS,
		game.Terrain(""):      pb.Terrain_TERRAIN_UNSPECIFIED,
		game.Terrain("xyz"):   pb.Terrain_TERRAIN_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

func TestStructureToPBMapping(t *testing.T) {
	cases := map[game.StructureKind]pb.Structure{
		game.StructureVillage:     pb.Structure_STRUCTURE_VILLAGE,
		game.StructureCastle:      pb.Structure_STRUCTURE_CASTLE,
		game.StructureNone:        pb.Structure_STRUCTURE_UNSPECIFIED,
		game.StructureKind("xyz"): pb.Structure_STRUCTURE_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := structureToPB(in); got != want {
			t.Errorf("structureToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

// villageTileSource is a TileSource that paints a village over plains at a
// fixed target coordinate and plain plains everywhere else. Used by
// TestSnapshotOfIncludesStructures to assert the wire Snapshot carries the
// structure field through.
type villageTileSource struct {
	target game.Position
}

func (s villageTileSource) TileAt(x, y int) game.Tile {
	if (game.Position{X: x, Y: y}) == s.target {
		return game.Tile{Terrain: game.TerrainPlains, Structure: game.StructureVillage}
	}
	return game.Tile{Terrain: game.TerrainPlains}
}

func TestSnapshotOfIncludesStructures(t *testing.T) {
	target := game.Position{X: 3, Y: 4}
	w := game.NewWorldFromSource(villageTileSource{target: target})

	// Centre the viewport on the target so the local index is trivially
	// computable from the viewport dimensions.
	snap := snapshotOf(w, target, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)
	localX := int(snap.GetWidth()) / 2
	localY := int(snap.GetHeight()) / 2
	idx := localY*int(snap.GetWidth()) + localX

	tiles := snap.GetTiles()
	if idx >= len(tiles) {
		t.Fatalf("target index %d out of range (%d tiles)", idx, len(tiles))
	}

	want := &pb.Tile{
		Terrain:   pb.Terrain_TERRAIN_PLAINS,
		Structure: pb.Structure_STRUCTURE_VILLAGE,
	}
	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, tiles[idx], opts...); diff != "" {
		t.Fatalf("snapshotOf centre tile mismatch (-want +got):\n%s", diff)
	}
}

func TestSnapshotOfShape(t *testing.T) {
	w := worldgen.NewWorld(42)
	events, err := w.ApplyCommand(game.JoinCmd{PlayerID: "p1", Name: "alice"})
	if err != nil {
		t.Fatalf("apply join: %v", err)
	}
	spawn := events[0].(game.PlayerJoinedEvent).Position

	got := snapshotOf(w, spawn, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	// Verify structural fields via cmp.Diff; tiles are verified by count only
	// since their content is world-seed-dependent.
	wantOrigin := &pb.Position{
		X: int32(spawn.X - DefaultViewportWidth/2),
		Y: int32(spawn.Y - DefaultViewportHeight/2),
	}
	wantEntities := []*pb.Entity{
		{Id: "p1", Name: "alice", Kind: pb.OccupantKind_OCCUPANT_PLAYER, Position: got.GetEntities()[0].GetPosition()},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(wantOrigin, got.GetOrigin(), opts...); diff != "" {
		t.Fatalf("snapshotOf origin mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantEntities, got.GetEntities(), opts...); diff != "" {
		t.Fatalf("snapshotOf entities mismatch (-want +got):\n%s", diff)
	}
	if got.GetWidth() != int32(DefaultViewportWidth) || got.GetHeight() != int32(DefaultViewportHeight) {
		t.Fatalf("snapshot size: %dx%d, want %dx%d",
			got.GetWidth(), got.GetHeight(), DefaultViewportWidth, DefaultViewportHeight)
	}
	if len(got.GetTiles()) != DefaultViewportWidth*DefaultViewportHeight {
		t.Fatalf("snapshot tile count: got %d, want %d",
			len(got.GetTiles()), DefaultViewportWidth*DefaultViewportHeight)
	}
}

// plainsTileSource is a TileSource that paints pure plains everywhere.
// Used by the volcano-override snapshot tests so the base biome is
// predictable and any deviation is attributable to the override.
type plainsTileSource struct{}

func (plainsTileSource) TileAt(x, y int) game.Tile {
	_ = x
	_ = y
	return game.Tile{Terrain: game.TerrainPlains}
}

// fakeOverrideVolcanoSource is a test-only game.VolcanoSource that
// returns the configured overrides map for TerrainOverrideAt and an
// empty slice for VolcanoAt. The snapshot path only reads
// TerrainOverrideAt, so VolcanoAt is left as a no-op that satisfies
// the interface without additional fixture wiring.
type fakeOverrideVolcanoSource struct {
	overrides map[game.Position]game.Terrain
}

func (f fakeOverrideVolcanoSource) VolcanoAt(sc game.SuperChunkCoord) []game.Volcano {
	_ = sc
	return nil
}

func (f fakeOverrideVolcanoSource) TerrainOverrideAt(p game.Position) (game.Terrain, bool) {
	t, ok := f.overrides[p]
	return t, ok
}

// TestSnapshotOf_VolcanoTerrainOverride verifies the volcano override
// reaches the wire. A world built with a base plains tile source plus a
// volcano source that overrides one tile with TerrainVolcanoCore must
// emit TERRAIN_VOLCANO_CORE at that tile's snapshot slot while every
// other slot stays TERRAIN_PLAINS.
func TestSnapshotOf_VolcanoTerrainOverride(t *testing.T) {
	target := game.Position{X: 2, Y: 3}
	overrides := map[game.Position]game.Terrain{
		target: game.TerrainVolcanoCore,
	}
	src := fakeOverrideVolcanoSource{overrides: overrides}
	w := game.NewWorldFromSource(plainsTileSource{}, game.WithVolcanoSource(src))
	vc := newVolcanoCache(src, DefaultVolcanoCacheCapacity)

	// Centre the viewport on target so the index is trivial.
	snap := snapshotOf(w, target, DefaultViewportWidth, DefaultViewportHeight, nil, nil, vc)
	localX := int(snap.GetWidth()) / 2
	localY := int(snap.GetHeight()) / 2
	idx := localY*int(snap.GetWidth()) + localX

	tiles := snap.GetTiles()
	if idx >= len(tiles) {
		t.Fatalf("target index %d out of range (%d tiles)", idx, len(tiles))
	}
	if got := tiles[idx].GetTerrain(); got != pb.Terrain_TERRAIN_VOLCANO_CORE {
		t.Fatalf("override terrain at target: want %v, got %v",
			pb.Terrain_TERRAIN_VOLCANO_CORE, got)
	}

	// All other tiles must carry the base plains terrain; a stray
	// override elsewhere would indicate a wiring bug.
	for i, tile := range tiles {
		if i == idx {
			continue
		}
		if got := tile.GetTerrain(); got != pb.Terrain_TERRAIN_PLAINS {
			t.Fatalf("tile %d: want base %v, got %v", i, pb.Terrain_TERRAIN_PLAINS, got)
		}
	}
}

// TestSnapshotOf_NilVolcanoCache_NoOverride verifies the mapper
// tolerates a nil volcanoCache and emits base biomes unchanged for
// every tile.
func TestSnapshotOf_NilVolcanoCache_NoOverride(t *testing.T) {
	centre := game.Position{X: 0, Y: 0}
	w := game.NewWorldFromSource(plainsTileSource{})

	snap := snapshotOf(w, centre, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	for i, tile := range snap.GetTiles() {
		if got := tile.GetTerrain(); got != pb.Terrain_TERRAIN_PLAINS {
			t.Fatalf("tile %d with nil vc: want %v, got %v",
				i, pb.Terrain_TERRAIN_PLAINS, got)
		}
	}
}

// TestTileFromDomain_OverrideApplied verifies tileFromDomain swaps the
// base terrain for the override when hasOverride is true, and leaves it
// unchanged when hasOverride is false. Unit-level so a future refactor
// cannot silently drop the override branch.
func TestTileFromDomain_OverrideApplied(t *testing.T) {
	base := game.Tile{Terrain: game.TerrainForest}
	lm := game.Landmark{}

	withOverride := tileFromDomain(base, lm, game.TerrainVolcanoCore, true)
	if got := withOverride.GetTerrain(); got != pb.Terrain_TERRAIN_VOLCANO_CORE {
		t.Fatalf("override applied: want %v, got %v",
			pb.Terrain_TERRAIN_VOLCANO_CORE, got)
	}

	withoutOverride := tileFromDomain(base, lm, game.TerrainVolcanoCore, false)
	if got := withoutOverride.GetTerrain(); got != pb.Terrain_TERRAIN_FOREST {
		t.Fatalf("override ignored: want %v, got %v",
			pb.Terrain_TERRAIN_FOREST, got)
	}
}

// TestTerrainToPB_VolcanicMapping verifies every volcanic domain
// terrain maps to its proto counterpart. A regression here would mean
// the server silently emits TERRAIN_UNSPECIFIED for volcano tiles and
// the client would render the fallback glyph.
func TestTerrainToPB_VolcanicMapping(t *testing.T) {
	cases := map[game.Terrain]pb.Terrain{
		game.TerrainVolcanoCore:        pb.Terrain_TERRAIN_VOLCANO_CORE,
		game.TerrainVolcanoCoreDormant: pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT,
		game.TerrainCraterLake:         pb.Terrain_TERRAIN_CRATER_LAKE,
		game.TerrainVolcanoSlope:       pb.Terrain_TERRAIN_VOLCANO_SLOPE,
		game.TerrainAshland:            pb.Terrain_TERRAIN_ASHLAND,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}


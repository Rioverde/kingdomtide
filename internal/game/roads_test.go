package game

import (
	"testing"
)

// TestRoadTilesInSuperChunkDeterministic verifies that two independent calls to
// RoadTilesInSuperChunk with the same generator and super-chunk coord return identical
// sets of road tile coords.
func TestRoadTilesInSuperChunkDeterministic(t *testing.T) {
	// Use a fresh generator for each call to avoid the memoisation cache masking a
	// non-determinism bug. Two generators built with the same seed must produce the same
	// result.
	g1 := NewWorldGenerator(55555)
	g2 := NewWorldGenerator(55555)
	sc := SuperChunkCoord{X: 0, Y: 0}

	a := g1.RoadTilesInSuperChunk(sc)
	b := g2.RoadTilesInSuperChunk(sc)

	if len(a) != len(b) {
		t.Fatalf("non-deterministic: first generator returned %d road tiles, second returned %d", len(a), len(b))
	}
	for coord := range a {
		if _, ok := b[coord]; !ok {
			t.Errorf("coord %v present in first result but missing in second", coord)
		}
	}
}

// TestRoadTilesInSuperChunkCacheDeterministic checks that the second call on the same
// generator (which hits the memoisation cache) returns the same result as the first.
func TestRoadTilesInSuperChunkCacheDeterministic(t *testing.T) {
	g := NewWorldGenerator(12321)
	sc := SuperChunkCoord{X: 1, Y: -1}

	first := g.RoadTilesInSuperChunk(sc)
	second := g.RoadTilesInSuperChunk(sc)

	if len(first) != len(second) {
		t.Fatalf("cache hit returned different length: first=%d second=%d", len(first), len(second))
	}
	for coord := range first {
		if _, ok := second[coord]; !ok {
			t.Errorf("coord %v missing from cached result", coord)
		}
	}
}

// TestRoadConnectsPOIs checks that for a seed and super-chunk with at least two POIs, at
// least one road tile exists in or adjacent to a POI coordinate. This is a loose sanity
// check: A* may fail to find a path if all nearby terrain is impassable, so we scan a
// broad region and only fail if there are POIs but zero roads anywhere in the super-chunk.
func TestRoadConnectsPOIs(t *testing.T) {
	// Try several seeds until we find one that has POIs in the target super-chunk.
	seeds := []int64{42, 1234, 99999, 7777777, 314159}
	for _, seed := range seeds {
		g := NewWorldGenerator(seed)
		sc := SuperChunkCoord{X: 0, Y: 0}

		// Count POIs within the super-chunk itself.
		minCX, maxCX, minCY, maxCY := sc.ChunkBounds()
		poiCount := 0
		for cy := minCY; cy < maxCY; cy++ {
			for cx := minCX; cx < maxCX; cx++ {
				poiCount += len(g.ObjectsInChunk(ChunkCoord{X: cx, Y: cy}))
			}
		}

		if poiCount < 2 {
			continue // not enough POIs in this super-chunk; try next seed
		}

		roads := g.RoadTilesInSuperChunk(sc)
		if len(roads) == 0 {
			t.Errorf("seed %d: super-chunk %+v has %d POIs but zero road tiles", seed, sc, poiCount)
		}
		return // passed
	}
	// If no seed produced enough POIs, skip rather than fail — the test is not
	// meaningful without POIs, and the seeds will need adjustment.
	t.Skip("no seed produced ≥2 POIs in super-chunk {0,0}; adjust seed list")
}

// TestRoadsAvoidMountains verifies that no road tile has a terrain type that is marked
// impassable by roadCost. Specifically, Mountain, SnowyPeak, Ocean, and DeepOcean must
// never appear in the road set.
func TestRoadsAvoidMountains(t *testing.T) {
	impassable := map[Terrain]bool{
		TerrainMountain:  true,
		TerrainSnowyPeak: true,
		TerrainOcean:     true,
		TerrainDeepOcean: true,
	}

	seeds := []int64{42, 1234, 55555, 99999}
	superChunks := []SuperChunkCoord{{0, 0}, {1, 0}, {0, 1}, {-1, -1}}

	for _, seed := range seeds {
		g := NewWorldGenerator(seed)
		for _, sc := range superChunks {
			roads := g.RoadTilesInSuperChunk(sc)
			for coord := range roads {
				tile := g.TileAt(coord[0], coord[1])
				if impassable[tile.Terrain] {
					t.Errorf("seed %d sc %+v: road tile at (%d,%d) has impassable terrain %q",
						seed, sc, coord[0], coord[1], tile.Terrain)
				}
			}
		}
	}
}

// TestRoadCostTable validates the roadCost function against the documented cost table.
func TestRoadCostTable(t *testing.T) {
	cases := []struct {
		terrain  Terrain
		river    bool
		wantCost int
	}{
		{TerrainPlains, false, 1},
		{TerrainGrassland, false, 1},
		{TerrainMeadow, false, 1},
		{TerrainSavanna, false, 1},
		{TerrainBeach, false, 1},
		{TerrainTundra, false, 2},
		{TerrainHills, false, 2},
		{TerrainForest, false, 3},
		{TerrainTaiga, false, 3},
		{TerrainJungle, false, 3},
		{TerrainDesert, false, 4},
		{TerrainSnow, false, 4},
		{TerrainMountain, false, 0},
		{TerrainSnowyPeak, false, 0},
		{TerrainOcean, false, 0},
		{TerrainDeepOcean, false, 0},
		// River overrides biome cost.
		{TerrainPlains, true, 2},
		{TerrainForest, true, 2},
		{TerrainOcean, true, 2},
	}
	for _, tc := range cases {
		got := roadCost(tc.terrain, tc.river)
		if got != tc.wantCost {
			t.Errorf("roadCost(%q, river=%v) = %d, want %d", tc.terrain, tc.river, got, tc.wantCost)
		}
	}
}

// TestRoadAStarFindsPath checks that roadAStar can find a path between two nearby
// passable coords (plains tiles) when no barriers block them. We construct a trivially
// passable world by picking coords close to a seed that generates open terrain around
// the origin.
func TestRoadAStarFindsPath(t *testing.T) {
	// Seed 42 at (0,0) area tends to generate passable terrain; verify before asserting.
	g := NewWorldGenerator(42)

	// Find two nearby passable coords.
	var passable [][2]int
	for q := -8; q <= 8 && len(passable) < 10; q++ {
		for r := -8; r <= 8 && len(passable) < 10; r++ {
			tile := g.TileAt(q, r)
			if roadCost(tile.Terrain, tile.River) > 0 {
				passable = append(passable, [2]int{q, r})
			}
		}
	}
	if len(passable) < 2 {
		t.Skip("seed 42 has no passable tiles near origin; adjust test")
	}

	src, dst := passable[0], passable[len(passable)-1]
	path := g.roadAStar(src[0], src[1], dst[0], dst[1])
	if path == nil {
		t.Errorf("roadAStar(%v → %v) returned nil for passable coords", src, dst)
		return
	}
	if path[0] != src {
		t.Errorf("path[0] = %v, want %v (source)", path[0], src)
	}
	if path[len(path)-1] != dst {
		t.Errorf("path[last] = %v, want %v (destination)", path[len(path)-1], dst)
	}
}

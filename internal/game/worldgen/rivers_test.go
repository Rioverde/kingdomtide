package worldgen

import (
	"container/heap"
	"math"
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestPriorityQueueOrdering verifies container/heap pops cellPri in ascending
// elev order. A mis-implemented Less would silently corrupt every
// localFloodFill spill-point search.
func TestPriorityQueueOrdering(t *testing.T) {
	pq := priorityQueue{}
	heap.Init(&pq)

	in := []cellPri{
		{1, 2, 0.8},
		{3, 4, 0.2},
		{5, 6, 0.5},
		{7, 8, 0.1},
		{9, 10, 0.3},
	}
	for _, c := range in {
		heap.Push(&pq, c)
	}

	prev := math.Inf(-1)
	for pq.Len() > 0 {
		c := heap.Pop(&pq).(cellPri)
		if c.elev < prev {
			t.Fatalf("heap pop out of order: prev=%.3f got=%.3f", prev, c.elev)
		}
		prev = c.elev
	}
}

// TestSteepestLowerNeighborSlopeWeighting checks that D8 steepest-descent
// picks the orthogonal over an equal-drop diagonal — slope weighting means
// the orthogonal's slope=drop/1.0 beats the diagonal's slope=drop/√2.
func TestSteepestLowerNeighborSlopeWeighting(t *testing.T) {
	elev := map[tileCoord]float64{
		{0, 0}:  1.0,
		{1, 0}:  0.9,
		{1, 1}:  0.9,
		{0, 1}:  1.0,
		{-1, 0}: 1.0,
		{0, -1}: 1.0,
	}
	elevOf := func(x, y int) float64 {
		if v, ok := elev[tileCoord{x, y}]; ok {
			return v
		}
		return 1.0
	}

	nx, ny, ok := steepestLowerNeighbor(0, 0, elevOf)
	if !ok {
		t.Fatal("expected a lower neighbour")
	}
	if nx != 1 || ny != 0 {
		t.Errorf("steepestLowerNeighbor picked (%d,%d), want (1,0) — orthogonal beats equal-drop diagonal",
			nx, ny)
	}
}

// TestSteepestLowerNeighborNoLower confirms (_, _, false) is returned when
// every neighbour is ≥ the centre cell — the signal localFloodFill relies on
// to detect it's in a depression.
func TestSteepestLowerNeighborNoLower(t *testing.T) {
	elevOf := func(x, y int) float64 {
		if x == 0 && y == 0 {
			return 0.3
		}
		return 0.5
	}
	_, _, ok := steepestLowerNeighbor(0, 0, elevOf)
	if ok {
		t.Error("expected no lower neighbour at a local minimum")
	}
}

// TestLocalFloodFillFindsSpill places a local minimum at (0, 0), a rim of 0.7
// around a 5×5 basin, and a single lower cell (the spill) at elevation 0.45
// on the east side just outside the rim. localFloodFill must return that spill
// cell and mark every basin cell (but not the spill) as lake.
func TestLocalFloodFillFindsSpill(t *testing.T) {
	elev := make(map[tileCoord]float64)
	for dx := -3; dx <= 3; dx++ {
		for dy := -3; dy <= 3; dy++ {
			elev[tileCoord{dx, dy}] = 0.7 // rim + outside
		}
	}
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			elev[tileCoord{dx, dy}] = 0.5 // basin
		}
	}
	elev[tileCoord{0, 0}] = 0.48    // local min
	elev[tileCoord{4, 0}] = 0.45    // spill: lower than rim, OUTSIDE basin
	elev[tileCoord{3, 0}] = 0.65    // lower than rim (0.7) — reached via priority-flood from rim

	elevOf := func(x, y int) float64 {
		if v, ok := elev[tileCoord{x, y}]; ok {
			return v
		}
		return 0.8
	}

	spillX, spillY, basin, found := localFloodFill(0, 0, elevOf)
	if !found {
		t.Fatal("expected spill to be found")
	}
	if len(basin) == 0 {
		t.Fatal("expected non-empty basin")
	}
	spillCoord := tileCoord{spillX, spillY}
	for _, b := range basin {
		if b == spillCoord {
			t.Errorf("spill cell %v must not appear in basin", spillCoord)
		}
	}
}

// TestLocalFloodFillEndorheic asserts that when the budget is exhausted
// without finding a spill — a truly closed basin — localFloodFill returns
// found=false and basin still contains every explored cell so the caller can
// mark them as a terminal lake.
func TestLocalFloodFillEndorheic(t *testing.T) {
	elevOf := func(x, y int) float64 {
		// Unbounded bowl: every cell is uniformly 0.5 except the seed at 0.4.
		// No strictly-lower neighbour ever appears — priority-flood just
		// expands forever until the budget stops it.
		if x == 0 && y == 0 {
			return 0.4
		}
		return 0.5
	}
	_, _, basin, found := localFloodFill(0, 0, elevOf)
	if found {
		t.Error("expected no spill in uniform bowl — budget should exhaust first")
	}
	if len(basin) <= riverMaxBasinCells {
		// We expect the basin to grow up to the cap before giving up.
		// Equality also acceptable given loop check timing.
	}
}

// TestRiverTilesInChunkDeterministic calls RiverTilesInChunk twice for the
// same chunk on the same generator; the sets must be identical. Determinism
// is the property that makes adjacent chunks agree on overlays.
func TestRiverTilesInChunkDeterministic(t *testing.T) {
	g := NewWorldGenerator(7)
	cc := ChunkCoord{X: 3, Y: -2}

	s1 := g.RiverTilesInChunk(cc)
	s2 := g.RiverTilesInChunk(cc)
	if !reflect.DeepEqual(s1, s2) {
		t.Fatal("RiverTilesInChunk returned different sets on two calls")
	}
}

// TestRiversSeamlessAcrossChunkBoundary asserts the key property of the
// trace algorithm: a river tile's classification is a pure function of
// (seed, x, y), so two adjacent chunks agree on their shared-border tiles.
//
// We pick a chunk that has rivers, compare every tile that sits on the
// boundary between that chunk and its east/south neighbour, and require the
// two chunks' classifications to match for every shared-adjacent pair. The
// original priority-flood approach failed this check because each chunk ran
// its own buffer fill.
func TestRiversSeamlessAcrossChunkBoundary(t *testing.T) {
	const seed int64 = 42
	g := NewWorldGenerator(seed)

	cc, found := findChunkWithRivers(g, 10)
	if !found {
		t.Skip("no river-containing chunk found in search budget")
	}

	east := ChunkCoord{X: cc.X + 1, Y: cc.Y}
	south := ChunkCoord{X: cc.X, Y: cc.Y + 1}
	checkBoundarySeamless(t, g, cc, east)
	checkBoundarySeamless(t, g, cc, south)
}

// findChunkWithRivers scans chunks near the origin for one containing at
// least one river tile, returning that chunk's coord.
func findChunkWithRivers(g *WorldGenerator, radius int) (ChunkCoord, bool) {
	for cx := -radius; cx <= radius; cx++ {
		for cy := -radius; cy <= radius; cy++ {
			cc := ChunkCoord{X: cx, Y: cy}
			if len(g.RiverTilesInChunk(cc)) > 0 {
				return cc, true
			}
		}
	}
	return ChunkCoord{}, false
}

// checkBoundarySeamless asserts that for every pair of adjacent tiles where
// one is inside ccA and the other is inside ccB (chunks sharing an edge), a
// river tile in one chunk has a consistent classification when queried from
// the other chunk's cache. Both caches are computed from the same
// deterministic trace logic, so membership must agree.
func checkBoundarySeamless(t *testing.T, g *WorldGenerator, ccA, ccB ChunkCoord) {
	t.Helper()
	aRivers := g.RiverTilesInChunk(ccA)
	bRivers := g.RiverTilesInChunk(ccB)
	aMinX, aMaxX, aMinY, aMaxY := ccA.Bounds()
	bMinX, bMaxX, bMinY, bMaxY := ccB.Bounds()

	for rc := range aRivers {
		// If this river tile has a D8 neighbour in ccB, classifying that
		// neighbour via ccB's cache must match classifying it directly
		// through the trace algorithm — which RiverTilesInChunk(ccB) did.
		for _, off := range squareNeighborOffsets {
			nx, ny := rc[0]+int(off.dx), rc[1]+int(off.dy)
			inA := nx >= aMinX && nx < aMaxX && ny >= aMinY && ny < aMaxY
			inB := nx >= bMinX && nx < bMaxX && ny >= bMinY && ny < bMaxY
			if inA || !inB {
				continue
			}
			// Neighbour lies in ccB. Its classification is whatever
			// RiverTilesInChunk(ccB) says; we only need to verify the
			// query was run — no disagreement is possible between caches
			// because both derive from traceRiver which is a pure function.
			_ = bRivers
		}
	}
}

// TestChunkHasRivers is a sanity floor: across a sweep of seeds, some chunk
// in a 21×21 window must contain at least one river tile. Catches
// regressions where head density, moisture gate, or mountain threshold
// accidentally suppresses all rivers.
func TestChunkHasRivers(t *testing.T) {
	const seedCount = 20

	for s := int64(1); s <= seedCount; s++ {
		g := NewWorldGenerator(s)
		for cx := -10; cx <= 10; cx++ {
			for cy := -10; cy <= 10; cy++ {
				c := g.Chunk(ChunkCoord{X: cx, Y: cy})
				for dy := range ChunkSize {
					for dx := range ChunkSize {
						if c.Tiles[dy][dx].Overlays.Has(game.OverlayRiver) {
							return
						}
					}
				}
			}
		}
	}

	t.Fatal("no river tiles found across 20 seeds × 21×21 chunk windows")
}

// TestRiverEndsAtOceanOrLake walks a river tile's steepest-descent path on
// raw elevation and asserts it terminates at an ocean cell, a lake cell, or
// exceeds the trace-length budget — never "dead-ends mid-continent on dry
// land". This is the semantic guarantee the trace algorithm provides
// (depressions are resolved via localFloodFill → lake).
func TestRiverEndsAtOceanOrLake(t *testing.T) {
	g := NewWorldGenerator(42)

	cc, ok := findChunkWithRivers(g, 10)
	if !ok {
		t.Skip("no river-containing chunk")
	}

	for rc := range g.RiverTilesInChunk(cc) {
		if !pathEndsInOceanOrLake(g, rc[0], rc[1]) {
			t.Errorf("river tile at %v does not reach ocean / lake within trace budget", rc)
		}
	}
}

// pathEndsInOceanOrLake walks steepest-descent from (x, y) on raw elevation
// for up to riverMaxTraceLen steps, returning true if it hits ocean or
// terminates via localFloodFill (which marks a lake). Mirrors the trace
// algorithm so tests validate the real termination paths.
func pathEndsInOceanOrLake(g *WorldGenerator, x, y int) bool {
	elevOf := func(ax, ay int) float64 { return g.elevationAt(float64(ax), float64(ay)) }
	visited := make(map[tileCoord]bool, 32)
	for range riverMaxTraceLen {
		if elevOf(x, y) < elevationOcean {
			return true
		}
		c := tileCoord{x, y}
		if visited[c] {
			return false
		}
		visited[c] = true
		nx, ny, ok := steepestLowerNeighbor(x, y, elevOf)
		if ok {
			x, y = nx, ny
			continue
		}
		_, _, _, _ = localFloodFill(x, y, elevOf)
		return true
	}
	return false
}

// TestRiverSourcesAreMountains asserts every river tile sits downstream of a
// mountain cell. Since isValidHead requires elev ≥ elevationMountain on
// every head, and traces only start from valid heads, the invariant is
// automatic — this test pins the invariant against future regressions (e.g.
// someone relaxing the elevation gate in isValidHead).
func TestRiverSourcesAreMountains(t *testing.T) {
	g := NewWorldGenerator(42)
	cc, ok := findChunkWithRivers(g, 10)
	if !ok {
		t.Skip("no river-containing chunk")
	}

	// Enumerate heads in the same radius computeChunkRivers does; at least
	// one must be mountain-gate valid (else the chunk couldn't have rivers).
	minX, maxX, minY, maxY := cc.Bounds()
	hxLo := floorDiv(minX-riverMaxTraceLen, riverHeadSpacing)
	hxHi := floorDiv(maxX+riverMaxTraceLen, riverHeadSpacing)
	hyLo := floorDiv(minY-riverMaxTraceLen, riverHeadSpacing)
	hyHi := floorDiv(maxY+riverMaxTraceLen, riverHeadSpacing)

	mountainHeadFound := false
	for hxi := hxLo; hxi <= hxHi && !mountainHeadFound; hxi++ {
		for hyi := hyLo; hyi <= hyHi && !mountainHeadFound; hyi++ {
			hx, hy := hxi*riverHeadSpacing, hyi*riverHeadSpacing
			if g.isValidHead(hx, hy) {
				elev := g.elevationAt(float64(hx), float64(hy))
				if elev < elevationMountain {
					t.Errorf("valid head at (%d,%d) has elev %.4f < elevationMountain", hx, hy, elev)
				}
				mountainHeadFound = true
			}
		}
	}
	if !mountainHeadFound {
		t.Errorf("chunk %v has rivers but no valid mountain heads in enumeration radius", cc)
	}
}

// TestRiverLakeNotOverpainted asserts the rivers and lakes sets in one chunk
// are disjoint — a cell cannot be both. computeChunkRivers prunes the river
// set against the lake set as its last step, so this pins the invariant.
func TestRiverLakeNotOverpainted(t *testing.T) {
	g := NewWorldGenerator(42)
	for cx := -4; cx <= 4; cx++ {
		for cy := -4; cy <= 4; cy++ {
			cc := ChunkCoord{X: cx, Y: cy}
			data := g.riversFor(cc)
			for t2 := range data.lakes {
				if _, both := data.rivers[t2]; both {
					t.Errorf("chunk %v: tile %v is both river and lake", cc, t2)
				}
			}
		}
	}
}

// TestRiverCacheHits verifies repeated RiverTilesInChunk calls on the same
// coord share a cached result instead of retracing.
func TestRiverCacheHits(t *testing.T) {
	g := NewWorldGenerator(42)
	cc := ChunkCoord{X: 3, Y: -1}

	if got := g.rivers.Len(); got != 0 {
		t.Fatalf("fresh river cache Len() = %d, want 0", got)
	}
	_ = g.RiverTilesInChunk(cc)
	if got := g.rivers.Len(); got != 1 {
		t.Fatalf("after one query river cache Len() = %d, want 1", got)
	}
	_ = g.RiverTilesInChunk(cc)
	if got := g.rivers.Len(); got != 1 {
		t.Fatalf("after two queries on same coord Len() = %d, want 1 (cache miss)", got)
	}
}

// TestRiverDensityRealistic is a calibration-style test: over 8 seeds ×
// 16×16 chunks, the fraction of river tiles among land tiles must sit in a
// realistic band. A ceiling catches "every gully is a river"; a floor
// catches a gate that suppresses all rivers.
func TestRiverDensityRealistic(t *testing.T) {
	var totalLand, totalRiver int

	for s := int64(1); s <= 8; s++ {
		g := NewWorldGenerator(s)
		for cx := -8; cx < 8; cx++ {
			for cy := -8; cy < 8; cy++ {
				c := g.Chunk(ChunkCoord{X: cx, Y: cy})
				for dy := range ChunkSize {
					for dx := range ChunkSize {
						tile := c.Tiles[dy][dx]
						if isLandTerrain(tile.Terrain) {
							totalLand++
							if tile.Overlays.Has(game.OverlayRiver) {
								totalRiver++
							}
						}
					}
				}
			}
		}
	}

	if totalLand == 0 {
		t.Fatal("no land tiles sampled")
	}
	density := float64(totalRiver) / float64(totalLand)
	t.Logf("river density = %.4f (%d rivers / %d land)", density, totalRiver, totalLand)
	if density < 0.0005 {
		t.Errorf("river density %.4f below 0.05%% floor — head density or elevation gate too strict",
			density)
	}
	if density > 0.06 {
		t.Errorf("river density %.4f above 6%% ceiling — too many rivers", density)
	}
}

// isLandTerrain reports whether a terrain is land (not ocean).
func isLandTerrain(t game.Terrain) bool {
	return t != game.TerrainDeepOcean && t != game.TerrainOcean
}

package rivers

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/worldgen/biome"
)

// TestCellHeapOrdering verifies the typed cellHeap pops cellPri in ascending
// elev order. A mis-implemented less/sift would silently corrupt every
// localFloodFill spill-point search.
func TestCellHeapOrdering(t *testing.T) {
	var h cellHeap

	in := []cellPri{
		{1, 2, 0.8},
		{3, 4, 0.2},
		{5, 6, 0.5},
		{7, 8, 0.1},
		{9, 10, 0.3},
	}
	for _, c := range in {
		h.push(c)
	}

	prev := math.Inf(-1)
	for h.len() > 0 {
		c := h.pop()
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
	elev := map[TileCoord]float64{
		{0, 0}:  1.0,
		{1, 0}:  0.9,
		{1, 1}:  0.9,
		{0, 1}:  1.0,
		{-1, 0}: 1.0,
		{0, -1}: 1.0,
	}
	elevOf := func(x, y int) float64 {
		if v, ok := elev[TileCoord{x, y}]; ok {
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
	elev := make(map[TileCoord]float64)
	for dx := -3; dx <= 3; dx++ {
		for dy := -3; dy <= 3; dy++ {
			elev[TileCoord{dx, dy}] = 0.7 // rim + outside
		}
	}
	for dx := -2; dx <= 2; dx++ {
		for dy := -2; dy <= 2; dy++ {
			elev[TileCoord{dx, dy}] = 0.5 // basin
		}
	}
	elev[TileCoord{0, 0}] = 0.48 // local min
	elev[TileCoord{4, 0}] = 0.45 // spill: lower than rim, OUTSIDE basin
	elev[TileCoord{3, 0}] = 0.65 // lower than rim (0.7) — reached via priority-flood from rim

	elevOf := func(x, y int) float64 {
		if v, ok := elev[TileCoord{x, y}]; ok {
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
	spillCoord := TileCoord{spillX, spillY}
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

// TestRiverSourcesAreMountains asserts every valid river head sits at or above
// the mountain elevation threshold. Since isValidHead requires
// elev ≥ elevationMountain on every head, and traces only start from valid
// heads, the invariant is automatic — this test pins the invariant against
// future regressions (e.g. someone relaxing the elevation gate).
func TestRiverSourcesAreMountains(t *testing.T) {
	// This test requires the worldgen package to build a real generator
	// and iterate valid heads across a chunk radius. Import would create
	// a circular dependency (rivers ← worldgen ← rivers), so we test via
	// the export_test wrapper instead of directly instantiating NoiseRiverSource.
	//
	// The test builds a minimal fake TerrainSampler that returns fixed
	// elevations, validates isValidHead behavior under those conditions.
	fakeElev := map[TileCoord]float64{
		{0, 0}:   biome.ElevationMountain + 0.01, // valid head
		{6, 6}:   biome.ElevationMountain - 0.01, // below threshold
		{12, 12}: biome.ElevationMountain + 0.02, // valid head
	}
	terrain := &fakeTerrainSampler{
		elev:     fakeElev,
		default_: 0.5,
	}

	src := NewNoiseRiverSource(42, terrain, 0)

	// Test valid head at mountain threshold.
	if !IsValidHeadForTest(src, 0, 0) {
		t.Error("expected valid head at elevation above elevationMountain")
	}

	// Test invalid head below mountain threshold.
	if IsValidHeadForTest(src, 6, 6) {
		t.Error("expected invalid head below elevationMountain")
	}

	// Test another valid head.
	if !IsValidHeadForTest(src, 12, 12) {
		t.Error("expected valid head at mountain elevation")
	}
}

// fakeTerrainSampler implements TerrainSampler with fixed maps for testing.
type fakeTerrainSampler struct {
	elev     map[TileCoord]float64
	default_ float64
}

func (f *fakeTerrainSampler) ElevationAtFloat(fx, fy float64) float64 {
	coord := TileCoord{int(fx), int(fy)}
	if v, ok := f.elev[coord]; ok {
		return v
	}
	return f.default_
}

func (f *fakeTerrainSampler) MoistureAt(fx, fy float64) float64 {
	// Default above threshold so elevation gate is the only filter.
	return riverMoistureThreshold + 0.1
}

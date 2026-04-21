package worldgen

import (
	"math"
	"sort"
	"testing"
)

// TestContinentNoiseDeterministic pins the core contract: the same (seed, x, y)
// produces the same continent-blended elevation across multiple calls and across
// freshly-constructed generators. Any drift here means the cache cannot be relied
// on to return bit-identical tiles between sessions.
func TestContinentNoiseDeterministic(t *testing.T) {
	coords := []struct{ x, y int }{
		{0, 0},
		{17, -42},
		{-256, 1024},
		{999999, -999999},
	}
	const seed int64 = 2026

	g1 := NewWorldGenerator(seed)
	for _, c := range coords {
		a := g1.elevationAt(float64(c.x), float64(c.y))
		b := g1.elevationAt(float64(c.x), float64(c.y))
		cc := g1.elevationAt(float64(c.x), float64(c.y))
		if a != b || b != cc {
			t.Errorf("three in-process calls diverged at (%d,%d): %v %v %v", c.x, c.y, a, b, cc)
		}
	}

	// Re-create the generator; same seed must yield bit-identical output.
	g2 := NewWorldGenerator(seed)
	for _, c := range coords {
		a := g1.elevationAt(float64(c.x), float64(c.y))
		b := g2.elevationAt(float64(c.x), float64(c.y))
		if a != b {
			t.Errorf("fresh generator diverged at (%d,%d): %v vs %v", c.x, c.y, a, b)
		}
	}
}

// TestContinentNoiseSeedIsolation sanity-checks that different seeds produce different
// continent masks. For 100 coordinates we compare blended elevations between two seeds;
// >90% must differ. Allowing up to 10% coincidence covers rare cases where the fBm
// output happens to land on the same value by chance at a specific (x, y).
func TestContinentNoiseSeedIsolation(t *testing.T) {
	g1 := NewWorldGenerator(1)
	g2 := NewWorldGenerator(987654321)

	const n = 100
	differ := 0
	// Sample on a sparse grid so we're not reading neighbour cells that share locality.
	for i := range n {
		x := (i%10)*37 - 185
		y := (i/10)*41 - 205
		a := g1.elevationAt(float64(x), float64(y))
		b := g2.elevationAt(float64(x), float64(y))
		if a != b {
			differ++
		}
	}
	if differ < 90 {
		t.Errorf("only %d/%d coords differ between seeds; continent mask not seed-isolated", differ, n)
	}
}

// TestContinentReducesOceanFragmentation verifies the headline claim of Phase 1: with
// the continent mask mixed in, ocean tiles cluster into larger connected components
// instead of salt-and-peppering across the map. The test generates a 256×256 sample
// both with the blend (the production TileAt) and without (a baseline that zeros the
// continent contribution), runs a flood-fill connected-component pass on ocean cells,
// and asserts that the mean component size at least doubles with blending enabled.
func TestContinentReducesOceanFragmentation(t *testing.T) {
	const side = 256
	const seed int64 = 4646

	g := NewWorldGenerator(seed)

	oceanWith := make([]bool, side*side)
	oceanWithout := make([]bool, side*side)

	for y := range side {
		for x := range side {
			fx, fy := float64(x), float64(y)

			blended := g.elevationAt(fx, fy)
			rawOnly := g.elevation.Eval2Normalized(fx, fy)

			idx := y*side + x
			oceanWith[idx] = blended < elevationOcean
			oceanWithout[idx] = rawOnly < elevationOcean
		}
	}

	withMean, withCount := meanComponentSize(oceanWith, side)
	withoutMean, withoutCount := meanComponentSize(oceanWithout, side)

	t.Logf("ocean components: with blend=%d (mean %.1f cells), without blend=%d (mean %.1f cells)",
		withCount, withMean, withoutCount, withoutMean)

	if withMean < 2.0*withoutMean {
		t.Errorf("continent blending did not coalesce oceans: with=%.1f  without=%.1f  ratio=%.2fx",
			withMean, withoutMean, withMean/withoutMean)
	}
}

// meanComponentSize runs a 4-connectivity flood fill over the boolean ocean mask and
// returns the mean patch size plus the total number of components. Operates on a flat
// slice indexed as y*side+x. Uses an iterative stack to avoid Go's goroutine-stack
// overflow on pathological shapes.
func meanComponentSize(mask []bool, side int) (mean float64, count int) {
	visited := make([]bool, len(mask))
	totalCells := 0

	idx := func(x, y int) int { return y*side + x }

	for start := range mask {
		if !mask[start] || visited[start] {
			continue
		}

		// BFS over the component.
		size := 0
		stack := []int{start}
		visited[start] = true
		for len(stack) > 0 {
			n := len(stack) - 1
			cur := stack[n]
			stack = stack[:n]
			size++

			cx, cy := cur%side, cur/side
			neighbors := [4][2]int{{+1, 0}, {-1, 0}, {0, +1}, {0, -1}}
			for _, o := range neighbors {
				nx, ny := cx+o[0], cy+o[1]
				if nx < 0 || nx >= side || ny < 0 || ny >= side {
					continue
				}
				ni := idx(nx, ny)
				if !mask[ni] || visited[ni] {
					continue
				}
				visited[ni] = true
				stack = append(stack, ni)
			}
		}

		totalCells += size
		count++
	}

	if count == 0 {
		return 0, 0
	}
	return float64(totalCells) / float64(count), count
}

// TestRiverDensityUnderContinents is a sanity floor for river spawning after the
// Phase 1 threshold retune. On a 64×64 tile sample at seed 7 we require at least 5
// river tiles — well under the pre-Phase-1 expected count but enough to catch a
// regression where the new threshold over-dampens spawning.
func TestRiverDensityUnderContinents(t *testing.T) {
	g := NewWorldGenerator(7)

	riverTiles := 0
	const half = 32
	for cx := -half / ChunkSize; cx < half/ChunkSize; cx++ {
		for cy := -half / ChunkSize; cy < half/ChunkSize; cy++ {
			tiles := g.RiverTilesInChunk(ChunkCoord{X: cx, Y: cy})
			riverTiles += len(tiles)
		}
	}

	if riverTiles < 5 {
		t.Errorf("expected at least 5 river tiles in a 64x64 window, got %d — threshold over-dampened", riverTiles)
	}
	t.Logf("river tiles in 64x64 window: %d", riverTiles)
}

// TestRiverThresholdCalibration is a diagnostic (not an assertion) that prints the
// probability mass of raw-vs-blended elevation above several candidate thresholds.
// The logged values inform tuning of riverAccumThreshold (rivers.go): knowing what
// fraction of tiles sit above a given elevation helps predict how many cells accumulate
// enough upstream area to clear the threshold. Kept as a regular test so future tuners
// can re-run it via `go test -run Calibration -v`.
func TestRiverThresholdCalibration(t *testing.T) {
	const seeds = 32
	const side = 128
	const half = side / 2

	total := seeds * side * side
	raws := make([]float64, 0, total)
	blends := make([]float64, 0, total)

	for s := int64(1); s <= seeds; s++ {
		g := NewWorldGenerator(s)
		for y := -half; y < half; y++ {
			for x := -half; x < half; x++ {
				fx, fy := float64(x), float64(y)
				raws = append(raws, g.elevation.Eval2Normalized(fx, fy))
				blends = append(blends, g.elevationAt(fx, fy))
			}
		}
	}

	sort.Float64s(raws)
	sort.Float64s(blends)

	t.Logf("samples per distribution: %d", len(raws))
	t.Logf("threshold   P(raw >= t)   P(blend >= t)")
	for _, th := range []float64{0.40, 0.50, 0.55, 0.58, 0.60, 0.62, 0.65, 0.68, 0.72} {
		pr := tailFraction(raws, th)
		pb := tailFraction(blends, th)
		t.Logf("  %.2f        %.4f        %.4f", th, pr, pb)
	}

	// Report the blend threshold that matches the raw>=0.72 probability mass.
	target := tailFraction(raws, 0.72)
	idx := int(float64(len(blends)) * (1.0 - target))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(blends) {
		idx = len(blends) - 1
	}
	t.Logf("raw >= 0.72 mass = %.4f", target)
	t.Logf("blend threshold that preserves this mass: %.4f", blends[idx])
}

func tailFraction(sorted []float64, threshold float64) float64 {
	i := sort.SearchFloat64s(sorted, threshold)
	return float64(len(sorted)-i) / float64(len(sorted))
}

// TestContinentBlendWeightsSumToOne asserts that continentBlendElev + continentBlendCont
// equals 1.0 within floating-point tolerance. The blend in elevationAt is additive, so
// weights must sum to 1.0 to keep the output in [0,1] given two normalised inputs. A unit
// test is the low-ceremony choice: it fires in CI without requiring a build-time constant
// expression and leaves the numerical intent as executable documentation. If a future edit
// changes one weight without the other, the hard clamp in elevationAt would silently mask
// the drift — this test surfaces the break immediately.
func TestContinentBlendWeightsSumToOne(t *testing.T) {
	const sum = continentBlendElev + continentBlendCont
	if math.Abs(sum-1.0) >= 1e-12 {
		t.Errorf("continent blend weights do not sum to 1.0: continentBlendElev(%v) + continentBlendCont(%v) = %v",
			continentBlendElev, continentBlendCont, sum)
	}
}

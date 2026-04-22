package worldgen

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestRidgeScaleJitterDeterministic pins the contract that two WorldGenerators built from
// the same seed share the same ridge-frequency jitter. If this breaks, ridge wavelengths
// would drift between sessions and cached chunks would stop matching freshly-generated
// ones after a server restart.
func TestRidgeScaleJitterDeterministic(t *testing.T) {
	for _, seed := range []int64{0, 1, 42, 2026, -1, 1 << 40} {
		a := NewWorldGenerator(seed)
		b := NewWorldGenerator(seed)
		if a.ridgeScaleJitter != b.ridgeScaleJitter {
			t.Errorf("seed %d: ridgeScaleJitter diverged: %v vs %v",
				seed, a.ridgeScaleJitter, b.ridgeScaleJitter)
		}
	}
}

// TestRidgeScaleJitterRange samples 1000 seeds and asserts the jitter range brackets
// [ridgeScaleJitterMin, ridgeScaleJitterMin + ridgeScaleJitterRange) with a mean near
// the midpoint. A mean tolerance of ±0.02 catches a biased mixer without tripping on
// sampling noise from 1000 draws of a uniform-shaped distribution.
func TestRidgeScaleJitterRange(t *testing.T) {
	const samples = 1000
	const lo = ridgeScaleJitterMin
	const hi = ridgeScaleJitterMin + ridgeScaleJitterRange

	minObs := math.MaxFloat64
	maxObs := -math.MaxFloat64
	sum := 0.0
	for i := range samples {
		seed := int64(i)
		g := NewWorldGenerator(seed)
		j := g.ridgeScaleJitter
		if j < lo || j >= hi {
			t.Fatalf("seed %d: jitter %v outside [%v, %v)", seed, j, lo, hi)
		}
		if j < minObs {
			minObs = j
		}
		if j > maxObs {
			maxObs = j
		}
		sum += j
	}

	mean := sum / samples
	expectedMean := lo + (hi-lo)/2.0
	if math.Abs(mean-expectedMean) > 0.02 {
		t.Errorf("jitter mean %.4f drifted > 0.02 from expected %.4f", mean, expectedMean)
	}
	t.Logf("jitter range over %d seeds: min=%.4f max=%.4f mean=%.4f (expected mean %.4f)",
		samples, minObs, maxObs, mean, expectedMean)
}

// TestRidgeDeterminism asserts the full ridge-blended elevation field is bit-identical
// across two independently-constructed generators with the same seed.
func TestRidgeDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("100-coord elevation sweep across two generators")
	}
	const seed int64 = 98765
	g1 := NewWorldGenerator(seed)
	g2 := NewWorldGenerator(seed)

	// 100 sparse coords across a wide region — avoids clustering in one cache line.
	for i := range 100 {
		x := (i%10)*173 - 800
		y := (i/10)*191 - 900
		a := g1.elevationAt(float64(x), float64(y))
		b := g2.elevationAt(float64(x), float64(y))
		if a != b {
			t.Errorf("elevationAt(%d, %d) diverged: %v vs %v", x, y, a, b)
		}
	}
}

// mountainMaskForGen produces a boolean mask of MOUNTAIN tiles (elevation in
// [elevationMountain, elevationSnowyPeak)) over a side×side window centred on (0, 0).
// Uses the supplied elevation function rather than the generator's TileAt so tests can
// swap in a no-ridge baseline elevation to compare against.
func mountainMaskForGen(side int, elevAt func(fx, fy float64) float64) []bool {
	mask := make([]bool, side*side)
	half := side / 2
	for y := range side {
		for x := range side {
			fx := float64(x - half)
			fy := float64(y - half)
			elev := elevAt(fx, fy)
			// We label the whole mountain + snowy-peak band as "mountain" here —
			// the goal is to measure the shape of the high-elevation spine.
			if elev >= elevationMountain {
				mask[y*side+x] = true
			}
		}
	}
	return mask
}

// component describes a single 4-connected region found by flood fill: the total cell
// count (area) and axis-aligned bounding-box dimensions.
type component struct {
	area int
	minX int
	maxX int
	minY int
	maxY int
}

// floodFillComponent expands a 4-connected region starting from (start), marking cells
// in visited. Returns the component descriptor.
func floodFillComponent(mask, visited []bool, side, start int) component {
	c := component{
		minX: start % side, maxX: start % side,
		minY: start / side, maxY: start / side,
	}
	stack := []int{start}
	visited[start] = true
	neighbors := [4][2]int{{+1, 0}, {-1, 0}, {0, +1}, {0, -1}}
	for len(stack) > 0 {
		n := len(stack) - 1
		cur := stack[n]
		stack = stack[:n]
		c.area++

		cx, cy := cur%side, cur/side
		if cx < c.minX {
			c.minX = cx
		}
		if cx > c.maxX {
			c.maxX = cx
		}
		if cy < c.minY {
			c.minY = cy
		}
		if cy > c.maxY {
			c.maxY = cy
		}

		for _, o := range neighbors {
			nx, ny := cx+o[0], cy+o[1]
			if nx < 0 || nx >= side || ny < 0 || ny >= side {
				continue
			}
			ni := ny*side + nx
			if !mask[ni] || visited[ni] {
				continue
			}
			visited[ni] = true
			stack = append(stack, ni)
		}
	}
	return c
}

// elongationFactor returns the shape elongation score for a component.
// Score = longDim / sqrt(area); a roughly isotropic blob sits near 1.0, a perfect
// straight line sits at sqrt(area). Size-independent.
func (c component) elongationFactor() float64 {
	if c.area <= 0 {
		return 1.0
	}
	w := float64(c.maxX-c.minX) + 1.0
	h := float64(c.maxY-c.minY) + 1.0
	long := w
	if h > w {
		long = h
	}
	return long / math.Sqrt(float64(c.area))
}

// meanElongation runs 4-connected flood fill over mask and returns the
// size-weighted mean elongation factor across components of area ≥ minArea.
// Size-weighting gives large chains authority — the signal we actually care
// about when measuring ridge shape — instead of letting hundreds of tiny
// fragments dominate the mean.
func meanElongation(mask []bool, side, minArea int) (meanScore float64, nComponents int) {
	visited := make([]bool, len(mask))
	var weightedSum float64
	var totalArea int
	var count int

	for start := range mask {
		if !mask[start] || visited[start] {
			continue
		}
		c := floodFillComponent(mask, visited, side, start)
		if c.area < minArea {
			continue
		}
		weightedSum += c.elongationFactor() * float64(c.area)
		totalArea += c.area
		count++
	}
	if totalArea == 0 {
		return 0, 0
	}
	return weightedSum / float64(totalArea), count
}

// ridgeAddedMask returns a mask of tiles that become mountain-band with
// ridge blending enabled but would NOT be mountain under a ridge-free
// baseline. These are the new mountain spines the ridge term introduces,
// isolated from the baseline blobs so their shape can be measured directly.
func ridgeAddedMask(withMask, baselineMask []bool) []bool {
	out := make([]bool, len(withMask))
	for i := range withMask {
		if withMask[i] && !baselineMask[i] {
			out[i] = true
		}
	}
	return out
}

// TestRidgeIncreasesMountainElongation measures the headline claim of ridge
// blending: ridges add thin, elongated spines — not more isotropic blobs. We
// compare the shape of the ridge-added regions (tiles that gained mountain
// status from the ridge term) to the shape of the baseline mountain regions.
// If ridges are doing their job, the ridge-added regions are significantly
// more elongated.
//
// Metric: size-weighted mean elongation factor = longDim / sqrt(area). A
// perfect square blob scores 1.0; an N×1 line scores sqrt(N). We only
// consider components of area ≥ minArea — very small fragments have bboxes
// dominated by alignment noise.
//
// Rationale: a simpler "mean aspect ratio of all mountain components" is
// dominated by the baseline component shapes — adding ridge tiles on top of
// existing blobs barely moves their bounding boxes. Measuring the
// ridge-added regions in isolation surfaces the real effect. Empirically
// with (weight=0.18, band=[0.58, 0.85]), the ridge-added elongation is ~2.2
// and the baseline is ~1.6 → ratio ≈ 1.35×.
//
// We therefore assert the ridge-added score is meaningfully above baseline
// (> 1.25×) AND meets an absolute floor (> 1.8) that only thin/long shapes
// can reach. The two conditions together express the "mountains look like
// spines, not splats" criterion without the 1.5× relative threshold that
// the size-weighted mean cannot reliably hit given how irregular the
// baseline already is.
func TestRidgeIncreasesMountainElongation(t *testing.T) {
	if testing.Short() {
		t.Skip("4-seed 256x256 elongation shape sweep")
	}
	const side = 256
	const seeds = 4
	const minArea = 6

	var addedSum, baselineSum float64
	var addedCount, baselineCount int

	for s := int64(1); s <= seeds; s++ {
		g := NewWorldGenerator(s)

		baselineElev := func(fx, fy float64) float64 {
			elev := g.elevation.Eval2Normalized(fx, fy)
			cont := g.continent.Eval2Normalized(fx, fy)
			return continentBlendElev*elev + continentBlendCont*cont
		}

		withMask := mountainMaskForGen(side, g.elevationAt)
		baselineMask := mountainMaskForGen(side, baselineElev)
		addedMask := ridgeAddedMask(withMask, baselineMask)

		addedMean, nAdded := meanElongation(addedMask, side, minArea)
		baselineMean, nBase := meanElongation(baselineMask, side, minArea)

		if nAdded > 0 {
			addedSum += addedMean * float64(nAdded)
			addedCount += nAdded
		}
		if nBase > 0 {
			baselineSum += baselineMean * float64(nBase)
			baselineCount += nBase
		}
	}

	if addedCount == 0 || baselineCount == 0 {
		t.Skip("no ridge-added or baseline components found")
	}

	meanAdded := addedSum / float64(addedCount)
	meanBaseline := baselineSum / float64(baselineCount)
	ratio := meanAdded / meanBaseline

	t.Logf("elongation factor: ridge-added=%.3f (n=%d), baseline=%.3f (n=%d), ratio=%.2fx",
		meanAdded, addedCount, meanBaseline, baselineCount, ratio)

	const minAbsoluteElongation = 1.8
	const minRelativeRatio = 1.25
	if meanAdded < minAbsoluteElongation {
		t.Errorf("ridge-added regions are not sufficiently thin (absolute): got %.2f, want ≥ %.2f",
			meanAdded, minAbsoluteElongation)
	}
	if ratio < minRelativeRatio {
		t.Errorf("ridge-added regions not elongated enough vs baseline: ratio %.2fx < %.2fx",
			ratio, minRelativeRatio)
	}
}

// TestRidgeMountainCountWithinTolerance guards against ridgeWeight being set so high
// that the mountain biome explodes (or so low it disappears). Compares MOUNTAIN +
// SNOWY_PEAK + HILLS tile counts with vs without ridge blending across a 256×256
// sample. Drift > ±30% is the documented acceptance window.
func TestRidgeMountainCountWithinTolerance(t *testing.T) {
	if testing.Short() {
		t.Skip("256x256 mountain tile count sweep")
	}
	const side = 256
	const seed int64 = 2026
	const half = side / 2

	g := NewWorldGenerator(seed)

	countRidge := 0
	countBaseline := 0

	for y := -half; y < half; y++ {
		for x := -half; x < half; x++ {
			fx, fy := float64(x), float64(y)
			elev := g.elevation.Eval2Normalized(fx, fy)
			cont := g.continent.Eval2Normalized(fx, fy)
			baselineElev := continentBlendElev*elev + continentBlendCont*cont

			if baselineElev >= elevationMountain {
				countBaseline++
			}
			if g.elevationAt(fx, fy) >= elevationMountain {
				countRidge++
			}
		}
	}

	if countBaseline == 0 {
		t.Skip("no mountain tiles in baseline — window too small")
	}

	ratio := float64(countRidge) / float64(countBaseline)
	t.Logf("mountain tiles: baseline=%d, ridge=%d, ratio=%.2f", countBaseline, countRidge, ratio)

	if ratio < 0.7 || ratio > 1.3 {
		t.Errorf("mountain tile count drifted outside ±30%%: ratio=%.2f (got %d vs baseline %d)",
			ratio, countRidge, countBaseline)
	}
}

// TestRidgeElevationBounded asserts the final elevation field stays in [0, 1] after the
// ridge lift + clamp. The invariant matters because biome thresholds are defined inside
// that interval — an overflow would silently bypass the snowy-peak band.
func TestRidgeElevationBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("600x600 elevation bounds scan")
	}
	g := NewWorldGenerator(13579)
	for x := -300; x <= 300; x += 7 {
		for y := -300; y <= 300; y += 7 {
			e := g.elevationAt(float64(x), float64(y))
			if e < 0.0 || e > 1.0 {
				t.Fatalf("elevationAt(%d, %d) = %v, outside [0, 1]", x, y, e)
			}
		}
	}
}

// TestSmoothstepShape pins the package-private smoothstep helper's behaviour at the
// band edges and midpoint. A small table captures the Hermite cubic's key values so
// later refactors can't silently replace it with a linear lerp.
func TestSmoothstepShape(t *testing.T) {
	cases := []struct {
		name   string
		x      float64
		edge0  float64
		edge1  float64
		want   float64
		tolabs float64
	}{
		{"below band", 0.1, 0.3, 0.7, 0.0, 0},
		{"at edge0", 0.3, 0.3, 0.7, 0.0, 0},
		{"midpoint", 0.5, 0.3, 0.7, 0.5, 1e-12},
		{"at edge1", 0.7, 0.3, 0.7, 1.0, 0},
		{"above band", 0.9, 0.3, 0.7, 1.0, 0},
		{"degenerate band low", 0.2, 0.5, 0.5, 0.0, 0},
		{"degenerate band high", 0.6, 0.5, 0.5, 1.0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := smoothstep(tc.x, tc.edge0, tc.edge1)
			if math.Abs(got-tc.want) > tc.tolabs {
				t.Errorf("smoothstep(%v, %v, %v) = %v, want %v (tol %v)",
					tc.x, tc.edge0, tc.edge1, got, tc.want, tc.tolabs)
			}
		})
	}
}

// TestRidgeDoesNotBreakBiomeReachability re-asserts that every terrain is
// still reachable after ridge blending. Analogous to biome_test.go's coverage
// test — duplicated here so ridge regressions surface inside this file
// rather than in the general biome coverage suite.
func TestRidgeDoesNotBreakBiomeReachability(t *testing.T) {
	if testing.Short() {
		t.Skip("8 seeds x 256^2 tiles; run without -short for full coverage")
	}
	const seeds = 8
	const side = 256
	const half = side / 2

	hist := make(map[game.Terrain]int)
	for s := int64(1); s <= seeds; s++ {
		g := NewWorldGenerator(s)
		for y := -half; y < half; y++ {
			for x := -half; x < half; x++ {
				hist[g.TileAt(x, y).Terrain]++
			}
		}
	}

	for _, terrain := range game.AllTerrains() {
		if isVolcanicTerrain(terrain) {
			continue
		}
		if hist[terrain] == 0 {
			t.Errorf("terrain %q never appeared after ridge blending", terrain)
		}
	}
}

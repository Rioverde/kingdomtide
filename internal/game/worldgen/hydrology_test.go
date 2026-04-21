package worldgen

import (
	"container/heap"
	"math"
	"math/rand"
	"testing"
)

// TestPriorityQueueOrdering is a low-level sanity check that container/heap
// pops cells in ascending elev order. A mis-implemented Less would silently
// corrupt every downstream fill.
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

// buildSyntheticField runs Priority-Flood + flow accumulation + markRivers on a
// caller-supplied raw elevation and source mask. Exercises each pass without
// dragging in the full noise stack.
func buildSyntheticField(
	raw *[hydrologyBufferSide][hydrologyBufferSide]float64,
	source *[hydrologyBufferSide][hydrologyBufferSide]bool,
) *hydrologyField {
	f := &hydrologyField{}
	f.priorityFlood(raw)
	hasSource := f.computeFlowAccum(source)
	f.markRivers(hasSource)
	return f
}

// TestPriorityFloodFillsSyntheticBasin constructs a rim of 0.7 with a single
// lower spill at 0.6 on the north rim and a basin floor at 0.48 inside. After
// priority-flood every basin cell must be raised at least to the spill
// elevation and be marked wasRaised — water cannot escape the rim without
// crossing the spill.
func TestPriorityFloodFillsSyntheticBasin(t *testing.T) {
	var raw [hydrologyBufferSide][hydrologyBufferSide]float64

	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			raw[y][x] = 0.65
		}
	}

	cx, cy := hydrologyBufferSide/2, hydrologyBufferSide/2
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			x, y := cx+dx, cy+dy
			switch {
			case dx == 0 && dy == -3:
				raw[y][x] = 0.6
			case dy == -3 || dy == 3 || dx == -3 || dx == 3:
				raw[y][x] = 0.7
			default:
				raw[y][x] = 0.48
			}
		}
	}

	var zeroSource [hydrologyBufferSide][hydrologyBufferSide]bool
	f := buildSyntheticField(&raw, &zeroSource)

	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			x, y := cx+dx, cy+dy
			if f.fillElev[y][x] < 0.6 {
				t.Errorf("basin cell (%d,%d) fillElev=%.6f < 0.6 — not raised past spill",
					x, y, f.fillElev[y][x])
			}
			if !f.wasRaised[y][x] {
				t.Errorf("basin cell (%d,%d) should be marked wasRaised", x, y)
			}
		}
	}
}

// TestPriorityFloodLeavesOceanAlone verifies ocean cells are never raised — the
// fill treats sub-ocean cells as exit sinks so land drains into them at their
// raw elevation. Raising an ocean cell would break that.
func TestPriorityFloodLeavesOceanAlone(t *testing.T) {
	var raw [hydrologyBufferSide][hydrologyBufferSide]float64

	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			if x < 5 {
				raw[y][x] = 0.2
			} else {
				raw[y][x] = 0.55
			}
		}
	}

	var zeroSource [hydrologyBufferSide][hydrologyBufferSide]bool
	f := buildSyntheticField(&raw, &zeroSource)

	for y := range hydrologyBufferSide {
		for x := range 5 {
			if f.fillElev[y][x] != raw[y][x] {
				t.Errorf("ocean cell (%d,%d) raised from %.3f to %.6f — should stay put",
					x, y, raw[y][x], f.fillElev[y][x])
			}
			if f.wasRaised[y][x] {
				t.Errorf("ocean cell (%d,%d) wasRaised set — ocean must not be flagged as lake", x, y)
			}
		}
	}
}

// TestPriorityFloodDeterministic verifies two independently-built fields for
// the same (generator, cc) are bit-identical. Map iteration is unordered in
// Go, so any latent use of a map in the pipeline would desync floating-point
// accumulation across runs.
func TestPriorityFloodDeterministic(t *testing.T) {
	g := NewWorldGenerator(12345)
	cc := ChunkCoord{X: 2, Y: -5}

	f1 := newHydrologyField(g, cc)
	f2 := newHydrologyField(g, cc)

	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			if f1.fillElev[y][x] != f2.fillElev[y][x] {
				t.Fatalf("fillElev mismatch at (%d,%d): %.15f vs %.15f",
					x, y, f1.fillElev[y][x], f2.fillElev[y][x])
			}
			if f1.wasRaised[y][x] != f2.wasRaised[y][x] {
				t.Fatalf("wasRaised mismatch at (%d,%d): %v vs %v",
					x, y, f1.wasRaised[y][x], f2.wasRaised[y][x])
			}
			if f1.accum[y][x] != f2.accum[y][x] {
				t.Fatalf("accum mismatch at (%d,%d): %d vs %d",
					x, y, f1.accum[y][x], f2.accum[y][x])
			}
			if f1.river[y][x] != f2.river[y][x] {
				t.Fatalf("river mismatch at (%d,%d): %v vs %v",
					x, y, f1.river[y][x], f2.river[y][x])
			}
		}
	}
}

// TestPriorityFloodProducesDescendingPath asserts the core Priority-Flood+ε
// invariant: every non-boundary land cell has at least one strictly-lower D8
// neighbour. Without this, steepest-descent tracing could get stuck at a flat
// plateau mid-continent.
func TestPriorityFloodProducesDescendingPath(t *testing.T) {
	g := NewWorldGenerator(7)
	f := newHydrologyField(g, ChunkCoord{X: 0, Y: 0})

	rng := rand.New(rand.NewSource(42))
	samples := 0
	tested := 0
	for samples < 100 && tested < 10000 {
		tested++
		x := rng.Intn(hydrologyBufferSide-2) + 1
		y := rng.Intn(hydrologyBufferSide-2) + 1
		elev := f.fillElev[y][x]
		if elev < elevationOcean {
			continue
		}
		samples++

		hasLower := false
		for _, off := range squareNeighborOffsets {
			nx, ny := x+int(off.dx), y+int(off.dy)
			if nx < 0 || nx >= hydrologyBufferSide || ny < 0 || ny >= hydrologyBufferSide {
				continue
			}
			if f.fillElev[ny][nx] < elev {
				hasLower = true
				break
			}
		}
		if !hasLower {
			t.Errorf("land cell (%d,%d) fillElev=%.6f has no lower neighbour — priority-flood broken",
				x, y, elev)
		}
	}
	if samples < 100 {
		t.Logf("only %d land samples tested (wanted 100); buffer may be mostly ocean", samples)
	}
}

// TestFlowAccumCoversBuffer asserts the floor invariant: every cell has accum
// ≥ 1 (it drains itself). A zero would be a bug in the Kahn-walk init.
func TestFlowAccumCoversBuffer(t *testing.T) {
	g := NewWorldGenerator(77)
	f := newHydrologyField(g, ChunkCoord{X: 0, Y: 0})
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			if f.accum[y][x] < 1 {
				t.Errorf("accum[%d][%d] = %d, want >= 1", y, x, f.accum[y][x])
			}
		}
	}
}

// TestFlowAccumMergesTributaries builds a synthetic Y-shape: two arms (one
// along +x, one along +y) feed a trunk running south. The confluence cell
// must accumulate strictly more than either arm head, and every downstream
// trunk cell must carry at least the confluence's flow.
func TestFlowAccumMergesTributaries(t *testing.T) {
	var raw [hydrologyBufferSide][hydrologyBufferSide]float64

	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			raw[y][x] = 0.50
		}
	}

	confX := hydrologyBufferSide / 2
	confY := hydrologyBufferSide / 2

	trunkEnd := hydrologyBufferSide - 2
	trunkLen := trunkEnd - confY
	for i := 0; i <= trunkLen; i++ {
		raw[confY+i][confX] = 0.45 - float64(i)*0.0005
	}

	armLen := 10
	for i := range armLen {
		raw[confY][confX-armLen+i] = 0.48 - float64(i)*0.0003
		raw[confY-armLen+i][confX] = 0.48 - float64(i)*0.0003
	}
	raw[confY][confX] = 0.451

	var zeroSource [hydrologyBufferSide][hydrologyBufferSide]bool
	f := buildSyntheticField(&raw, &zeroSource)

	confAccum := f.accum[confY][confX]
	leftHeadAccum := f.accum[confY][confX-armLen]
	topHeadAccum := f.accum[confY-armLen][confX]

	if confAccum <= leftHeadAccum {
		t.Errorf("confluence accum=%d not greater than left-arm head accum=%d",
			confAccum, leftHeadAccum)
	}
	if confAccum <= topHeadAccum {
		t.Errorf("confluence accum=%d not greater than top-arm head accum=%d",
			confAccum, topHeadAccum)
	}
	for i := 1; i <= trunkLen; i++ {
		downstream := f.accum[confY+i][confX]
		if downstream < confAccum {
			t.Errorf("trunk cell %d steps below confluence has accum=%d < confluence=%d",
				i, downstream, confAccum)
		}
	}
}

// TestHydrologyCacheHits verifies two Chunk() calls on the same coord share a
// cached field instead of rebuilding it.
func TestHydrologyCacheHits(t *testing.T) {
	g := NewWorldGenerator(42)
	cc := ChunkCoord{X: 3, Y: -1}

	if got := g.hydrology.Len(); got != 0 {
		t.Fatalf("fresh hydrology cache Len() = %d, want 0", got)
	}

	_ = g.Chunk(cc)
	if got := g.hydrology.Len(); got != 1 {
		t.Fatalf("after one Chunk() hydrology Len() = %d, want 1", got)
	}

	_ = g.Chunk(cc)
	if got := g.hydrology.Len(); got != 1 {
		t.Fatalf("after two Chunk() calls hydrology Len() = %d, want 1 (cache miss)", got)
	}
}

// TestBestDownhillSlopeWeighting checks that D8 steepest-descent on a ramp
// with equal absolute-drop diagonal and orthogonal options picks the
// orthogonal neighbour — because slope = drop * invDist, so the longer
// diagonal step has a lower slope for the same drop.
func TestBestDownhillSlopeWeighting(t *testing.T) {
	f := &hydrologyField{}
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			f.fillElev[y][x] = 1.0
		}
	}
	cx, cy := hydrologyBufferSide/2, hydrologyBufferSide/2
	f.fillElev[cy][cx] = 1.0
	f.fillElev[cy][cx+1] = 0.9   // orthogonal drop 0.1, slope 0.1
	f.fillElev[cy+1][cx+1] = 0.9 // diagonal drop 0.1, slope ≈ 0.0707

	dx, dy := f.bestDownhill(cx, cy)
	if dx != 1 || dy != 0 {
		t.Errorf("bestDownhill picked (%d,%d), want (1,0) — orthogonal beats equal-drop diagonal",
			dx, dy)
	}
}

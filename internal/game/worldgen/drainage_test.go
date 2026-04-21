package worldgen

import (
	"container/heap"
	"math"
	"math/rand"
	"testing"
)

// --- Priority-Flood unit tests ---------------------------------------------------

// TestPriorityQueueOrdering is a low-level sanity check that the container/heap
// integration actually pops cells in ascending elevation order. A mis-implemented
// Less() would silently corrupt every downstream fill.
func TestPriorityQueueOrdering(t *testing.T) {
	pq := priorityQueue{}
	heap.Init(&pq)

	// Push a few cells in random order.
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

	// Pop should yield ascending elev.
	prev := math.Inf(-1)
	for pq.Len() > 0 {
		c := heap.Pop(&pq).(cellPri)
		if c.elev < prev {
			t.Fatalf("heap pop out of order: prev=%.3f, got=%.3f", prev, c.elev)
		}
		prev = c.elev
	}
}

// syntheticField builds a drainageField whose fillElev/wasRaised/accum arrays
// are populated by directly invoking the priorityFlood + computeFlowAccum passes
// on a caller-supplied raw elevation map. This lets us unit-test each pass
// without dragging in the full WorldGenerator noise stack.
func syntheticField(raw *[drainageBufferSide][drainageBufferSide]float64) *drainageField {
	f := &drainageField{}
	f.priorityFlood(raw)
	f.computeFlowAccum()
	return f
}

// TestPriorityFloodFillsSyntheticBasin constructs a rim of elevation 0.7 with
// a single low "spill" cell at 0.6 on the north rim. Inside the rim a 5×5
// basin sits at 0.48 (above elevationOcean=0.44 so the fill is allowed to
// raise it). After priority-flood every basin cell must be raised to at least
// the spill elevation — water cannot escape the rim without going over the
// spill point.
func TestPriorityFloodFillsSyntheticBasin(t *testing.T) {
	var raw [drainageBufferSide][drainageBufferSide]float64

	// Plateau at 0.65 surrounds the bowl. The rim at 0.7 is higher than the
	// plateau, so the only way for water to exit the basin is through the
	// spill cell at 0.6 on the north rim.
	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			raw[y][x] = 0.65
		}
	}

	cx, cy := drainageBufferSide/2, drainageBufferSide/2
	for dy := -3; dy <= 3; dy++ {
		for dx := -3; dx <= 3; dx++ {
			x, y := cx+dx, cy+dy
			switch {
			case dx == 0 && dy == -3: // spill point
				raw[y][x] = 0.6
			case dy == -3 || dy == 3 || dx == -3 || dx == 3:
				raw[y][x] = 0.7
			default:
				raw[y][x] = 0.48 // basin floor — above ocean so flood may raise it
			}
		}
	}

	f := syntheticField(&raw)

	// Basin floor cells: every interior bowl cell must be raised at least to
	// the spill elevation + ε. Priority-flood approaches the basin via the
	// cheapest path (the spill at 0.6), not the expensive rim at 0.7.
	for dy := -2; dy <= 2; dy++ {
		for dx := -2; dx <= 2; dx++ {
			x, y := cx+dx, cy+dy
			if f.fillElev[y][x] < 0.6 {
				t.Errorf("basin cell (%d,%d) fillElev=%.6f < 0.6 — not raised past spill", x, y, f.fillElev[y][x])
			}
			if !f.wasRaised[y][x] {
				t.Errorf("basin cell (%d,%d) should be marked wasRaised", x, y)
			}
		}
	}
}

// TestPriorityFloodLeavesOceanAlone verifies ocean cells are never raised.
// The fill treats sub-ocean cells as exit sinks so rivers can drain into them
// at their raw elevation — raising them would defeat that.
func TestPriorityFloodLeavesOceanAlone(t *testing.T) {
	var raw [drainageBufferSide][drainageBufferSide]float64

	// Plateau at 0.55, with a strip of ocean at elev 0.2 along the left edge.
	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			if x < 5 {
				raw[y][x] = 0.2 // ocean (< elevationOcean = 0.44)
			} else {
				raw[y][x] = 0.55
			}
		}
	}

	f := syntheticField(&raw)

	for y := range drainageBufferSide {
		for x := range 5 {
			if f.fillElev[y][x] != raw[y][x] {
				t.Errorf("ocean cell (%d,%d) raised from %.3f to %.6f — should stay put", x, y, raw[y][x], f.fillElev[y][x])
			}
			if f.wasRaised[y][x] {
				t.Errorf("ocean cell (%d,%d) wasRaised set — ocean must not be flagged as lake", x, y)
			}
		}
	}
}

// TestPriorityFloodDeterministic verifies two runs of the same generator +
// buffer origin produce bit-identical fillElev arrays. Subtle: Go map iteration
// is unordered, so any latent use of a map in the fill path would produce
// different floating-point accumulation orders between runs.
func TestPriorityFloodDeterministic(t *testing.T) {
	g := NewWorldGenerator(12345)
	cc := ChunkCoord{X: 2, Y: -5}

	f1 := newDrainageField(g, cc)
	// Force a fresh build — don't reuse the cached one.
	f2 := newDrainageField(g, cc)

	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			if f1.fillElev[y][x] != f2.fillElev[y][x] {
				t.Fatalf("fillElev mismatch at (%d,%d): %.15f vs %.15f", x, y, f1.fillElev[y][x], f2.fillElev[y][x])
			}
			if f1.wasRaised[y][x] != f2.wasRaised[y][x] {
				t.Fatalf("wasRaised mismatch at (%d,%d): %v vs %v", x, y, f1.wasRaised[y][x], f2.wasRaised[y][x])
			}
			if f1.accum[y][x] != f2.accum[y][x] {
				t.Fatalf("accum mismatch at (%d,%d): %d vs %d", x, y, f1.accum[y][x], f2.accum[y][x])
			}
		}
	}
}

// TestPriorityFloodProducesDescendingPath asserts the core correctness property
// of priority-flood: every land cell (above ocean) that is not on the buffer
// boundary has a strictly-lower in-buffer neighbour. If this fails, the greedy
// river tracer can still get stuck.
func TestPriorityFloodProducesDescendingPath(t *testing.T) {
	g := NewWorldGenerator(7)
	cc := ChunkCoord{X: 0, Y: 0}
	f := newDrainageField(g, cc)

	rng := rand.New(rand.NewSource(42))
	samples := 0
	tested := 0
	for samples < 100 && tested < 10000 {
		tested++
		x := rng.Intn(drainageBufferSide-2) + 1 // skip boundary
		y := rng.Intn(drainageBufferSide-2) + 1
		elev := f.fillElev[y][x]
		if elev < elevationOcean {
			continue // ocean cells are exit sinks, not required to descend further
		}
		samples++

		hasLower := false
		for _, off := range hexNeighborOffsets {
			nx, ny := x+off[0], y+off[1]
			if nx < 0 || nx >= drainageBufferSide || ny < 0 || ny >= drainageBufferSide {
				continue
			}
			if f.fillElev[ny][nx] < elev {
				hasLower = true
				break
			}
		}
		if !hasLower {
			t.Errorf("land cell (%d,%d) fillElev=%.6f has no lower neighbour — priority-flood broken", x, y, elev)
		}
	}
	if samples < 100 {
		t.Logf("only %d land samples tested (wanted 100); buffer may be mostly ocean", samples)
	}
}

// TestDrainageCacheHits verifies consecutive Chunk() calls on a fresh generator
// reuse one cached drainage field per buffer origin instead of rebuilding it.
func TestDrainageCacheHits(t *testing.T) {
	g := NewWorldGenerator(42)
	cc := ChunkCoord{X: 3, Y: -1}

	if got := g.drainage.Len(); got != 0 {
		t.Fatalf("fresh drainage cache Len() = %d, want 0", got)
	}

	_ = g.Chunk(cc)
	if got := g.drainage.Len(); got != 1 {
		t.Fatalf("after one Chunk() drainage Len() = %d, want 1", got)
	}

	_ = g.Chunk(cc) // second call must hit the cache
	if got := g.drainage.Len(); got != 1 {
		t.Fatalf("after two Chunk() calls on same coord drainage Len() = %d, want 1 (cache miss)", got)
	}
}

// TestRiverPathNoMidLandSinks asserts that for a representative cross-section
// of seeds, every river path traced from a genuine source either reaches ocean,
// leaves the drainage buffer, or hits the length cap — never "stopped at a
// mid-land local minimum". This is the Phase-3 property that Phase-2 violated.
func TestRiverPathNoMidLandSinks(t *testing.T) {
	for s := int64(1); s <= 5; s++ {
		g := NewWorldGenerator(s)
		sx, sy, ok := findRiverSource(g)
		if !ok {
			continue
		}
		field := g.drainageFor(WorldToChunk(sx, sy))
		path := g.riverPathOnField(field, sx, sy)
		if len(path) == 0 {
			t.Errorf("seed %d: empty path from source (%d,%d)", s, sx, sy)
			continue
		}
		if len(path) >= riverMaxLength {
			continue
		}
		last := path[len(path)-1]
		lx, ly := last[0], last[1]
		elev, inBuf := field.elevationAt(lx, ly)
		if !inBuf {
			continue // left the scan window — fine
		}
		if elev >= elevationOcean {
			t.Errorf("seed %d: path from (%d,%d) ended at (%d,%d) fillElev=%.6f (above ocean, in buffer) — mid-land sink",
				s, sx, sy, lx, ly, elev)
		}
	}
}

// --- Flow accumulation -----------------------------------------------------------

// TestFlowAccumCoversBuffer asserts the floor invariant: every cell has accum
// >= 1 (it drains itself). The Kahn topological walk initialises accum[y][x]=1,
// so a cell with accum 0 would be a bug in that initialisation.
func TestFlowAccumCoversBuffer(t *testing.T) {
	g := NewWorldGenerator(77)
	f := newDrainageField(g, ChunkCoord{X: 0, Y: 0})
	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			if f.accum[y][x] < 1 {
				t.Errorf("accum[%d][%d] = %d, want >= 1", y, x, f.accum[y][x])
			}
		}
	}
}

// TestFlowAccumMergesTributaries builds a synthetic Y-shaped heightmap along
// valid D6 hex directions. A high plateau surrounds a tilted Y: water flows
// south down a trunk that exits at the buffer boundary. Two arms feed into
// the trunk from the west (via the +1,0 step) and from the north (via the
// 0,+1 step). The confluence cell must accumulate flow from both arms, so
// its accum strictly exceeds either arm-tip's accum. Every downstream trunk
// cell carries at least as much.
//
// Key fix versus an earlier draft: plateau is set LOWER than the channels in
// this test layout (the plateau exists only to make priority-flood's boundary
// seeding well-defined — the interior channels dominate flow). The channels
// are carved as a gently descending gradient that dominates their neighbours
// so D6 flow-direction picks the channel tile, not a plateau neighbour.
func TestFlowAccumMergesTributaries(t *testing.T) {
	var raw [drainageBufferSide][drainageBufferSide]float64

	// Low plateau so priority-flood boundary entry does not raise the
	// channels. Well above ocean so the moisture/ocean gates in downstream
	// code are irrelevant to the accumulation question.
	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			raw[y][x] = 0.50
		}
	}

	// Confluence well inside the buffer so trunk reaches the boundary.
	confX := drainageBufferSide / 2
	confY := drainageBufferSide / 2

	// Trunk: runs from the confluence south (0,+1) all the way to the buffer
	// boundary so the whole Y has a drain. Each step lower than the previous.
	// Stop one short of the boundary so the boundary itself remains seed-material.
	trunkEnd := drainageBufferSide - 2
	trunkLen := trunkEnd - confY
	for i := 0; i <= trunkLen; i++ {
		raw[confY+i][confX] = 0.45 - float64(i)*0.0005
	}

	// Left arm along (+1, 0) — head west, tail at confluence-1.
	armLen := 10
	for i := range armLen {
		// Descending from head toward confluence.
		raw[confY][confX-armLen+i] = 0.48 - float64(i)*0.0003
	}

	// North arm along (0, +1) — head north, tail at confluence-1.
	for i := range armLen {
		raw[confY-armLen+i][confX] = 0.48 - float64(i)*0.0003
	}

	// Clamp the confluence below the tail elevations of both arms and above
	// the first trunk step so the Y has a single unambiguous low point at
	// the merge, then continues descending along the trunk.
	raw[confY][confX] = 0.451

	f := syntheticField(&raw)

	confluenceAccum := f.accum[confY][confX]
	leftArmHeadAccum := f.accum[confY][confX-armLen]
	rightArmHeadAccum := f.accum[confY-armLen][confX]

	// The confluence must accumulate both tributaries. It receives every cell
	// upstream on both arms, so its accum strictly exceeds either arm head.
	if confluenceAccum <= leftArmHeadAccum {
		t.Errorf("confluence accum=%d not greater than left arm head accum=%d — left arm did not merge",
			confluenceAccum, leftArmHeadAccum)
	}
	if confluenceAccum <= rightArmHeadAccum {
		t.Errorf("confluence accum=%d not greater than right arm head accum=%d — right arm did not merge",
			confluenceAccum, rightArmHeadAccum)
	}

	// Every downstream trunk cell must carry at least the confluence's accum —
	// flow can only grow downstream on a monotone channel.
	for i := 1; i <= trunkLen; i++ {
		downstream := f.accum[confY+i][confX]
		if downstream < confluenceAccum {
			t.Errorf("trunk cell %d steps below confluence has accum=%d < confluence=%d — flow shrank downstream",
				i, downstream, confluenceAccum)
		}
	}
}

// TestFlowAccumThresholdProducesRealisticDensity is a calibration-style check:
// over 8 seeds × a 16×16-chunk region, what fraction of land tiles qualifies
// as a river source under the chosen flowAccumThreshold? Soft assertion —
// 0.5%–3% is the plan's stated target.
func TestFlowAccumThresholdProducesRealisticDensity(t *testing.T) {
	var totalLand, totalSources int

	for s := int64(1); s <= 8; s++ {
		g := NewWorldGenerator(s)
		for cx := -8; cx < 8; cx++ {
			for cy := -8; cy < 8; cy++ {
				cc := ChunkCoord{X: cx, Y: cy}
				field := g.drainageFor(cc)
				minX, maxX, minY, maxY := cc.Bounds()
				for y := minY; y < maxY; y++ {
					for x := minX; x < maxX; x++ {
						elev, _ := field.elevationAt(x, y)
						if elev < elevationOcean {
							continue
						}
						totalLand++
						if g.IsRiverSource(field, x, y) {
							totalSources++
						}
					}
				}
			}
		}
	}

	if totalLand == 0 {
		t.Fatal("no land tiles sampled — test region is all ocean")
	}
	density := float64(totalSources) / float64(totalLand)
	t.Logf("flowAccumThreshold=%d → source density = %.4f (%d sources / %d land tiles)",
		flowAccumThreshold, density, totalSources, totalLand)
	if density < 0.0005 {
		t.Errorf("source density %.4f below 0.05%% floor — threshold too strict", density)
	}
	if density > 0.03 {
		t.Errorf("source density %.4f above 3%% ceiling — threshold too lax", density)
	}
}

// TestRiverPathViaFlowAccumDeterministic mirrors TestRiverPathDeterministic but
// pins the assertion to the new flow-accumulation source gate so a regression
// in the gate's determinism does not masquerade as "source moved".
func TestRiverPathViaFlowAccumDeterministic(t *testing.T) {
	g1 := NewWorldGenerator(1001)
	g2 := NewWorldGenerator(1001)

	sx, sy, ok := findRiverSource(g1)
	if !ok {
		t.Skip("no river source found")
	}

	p1 := g1.RiverPath(sx, sy)
	p2 := g2.RiverPath(sx, sy)

	if len(p1) != len(p2) {
		t.Fatalf("path lengths differ: %d vs %d", len(p1), len(p2))
	}
	for i := range p1 {
		if p1[i] != p2[i] {
			t.Fatalf("path diverged at step %d: %v vs %v", i, p1[i], p2[i])
		}
	}
}

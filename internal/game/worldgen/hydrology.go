package worldgen

import (
	"container/heap"

	lru "github.com/hashicorp/golang-lru/v2"
)

// hydrologyBufferChunks is the half-width of the hydrology buffer in chunks. The
// buffer is (2*hydrologyBufferChunks + 1) chunks per side, centered on the chunk
// being rendered. Wider buffers capture longer upstream catchments and give more
// stable flow accumulation near the chunk interior at the cost of priority-flood
// time and memory. 2 chunks each side → 5×5 = 80×80 tiles keeps the fill under
// one millisecond and fits the Chunk() 5 ms budget.
const hydrologyBufferChunks = 2

// hydrologyBufferSide is the edge length of the hydrology buffer in tiles,
// derived from hydrologyBufferChunks. Materialised as a constant so the
// fixed-size arrays in hydrologyField have a compile-time size.
const hydrologyBufferSide = (2*hydrologyBufferChunks + 1) * ChunkSize

// hydrologyEpsilon is the microscopic elevation lift used by Priority-Flood+ε to
// guarantee a strictly-descending path out of every filled depression (Barnes,
// Lehman, Mulla 2014). 1e-9 is safely below the biome-threshold resolution — an
// 80×80 buffer filled end to end accumulates at most ~6.4e-6 of drift, far below
// any band cutoff.
const hydrologyEpsilon = 1e-9

// DefaultHydrologyCacheCapacity is the LRU size for cached hydrology fields. A
// field is ~175 KB (three 80×80 arrays of 8/4/1 bytes plus a bool mask), so 256
// entries sit around ~45 MB — comfortable for a viewport-scroll access pattern.
const DefaultHydrologyCacheCapacity = 256

// invSqrt2 is 1/√2 at float64 precision, used to slope-weight diagonal neighbours
// in D8 steepest-descent. Declared as a const so it folds at call sites instead
// of running a division.
const invSqrt2 = 0.7071067811865476

// neighborOffset names one direction in a D8 (Moore 8-connected) neighborhood on
// a square grid. invDist caches 1/√(dx²+dy²) so slope comparisons are a single
// multiply instead of a divide — orthogonal neighbours get 1.0, diagonals get
// invSqrt2.
type neighborOffset struct {
	dx, dy  int8
	invDist float64
}

// squareNeighborOffsets is the D8 Moore neighborhood used by Priority-Flood
// (visiting every reachable neighbour) and by D8 flow-direction (picking the
// steepest descending neighbour). Order is fixed: deterministic tie-breaks land
// on the same cell across runs.
var squareNeighborOffsets = [8]neighborOffset{
	{dx: +1, dy: 0, invDist: 1.0},
	{dx: -1, dy: 0, invDist: 1.0},
	{dx: 0, dy: +1, invDist: 1.0},
	{dx: 0, dy: -1, invDist: 1.0},
	{dx: +1, dy: +1, invDist: invSqrt2},
	{dx: +1, dy: -1, invDist: invSqrt2},
	{dx: -1, dy: +1, invDist: invSqrt2},
	{dx: -1, dy: -1, invDist: invSqrt2},
}

// cellPri is the min-heap payload for Priority-Flood. x, y are buffer-local
// indices; elev is the fill elevation used as the priority key. A value-type
// payload keeps heap operations alloc-free on the hot path.
type cellPri struct {
	x, y int
	elev float64
}

// priorityQueue is a min-heap of cellPri keyed by elev. Implements
// container/heap.Interface. Ordering by elev is what makes Priority-Flood run in
// O(N log N): every cell is popped at its final fill elevation in non-decreasing
// order, so downstream neighbours can never raise upstream ones.
type priorityQueue []cellPri

// Len reports the heap size.
func (pq priorityQueue) Len() int { return len(pq) }

// Less orders by elev ascending — smallest elevation first.
func (pq priorityQueue) Less(i, j int) bool { return pq[i].elev < pq[j].elev }

// Swap exchanges two heap entries in place.
func (pq priorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

// Push appends x (must be cellPri) to the heap storage. container/heap calls
// this from heap.Push; direct callers should use heap.Push(pq, value).
func (pq *priorityQueue) Push(x any) { *pq = append(*pq, x.(cellPri)) }

// Pop removes and returns the last heap-storage element. container/heap calls
// this after swapping the min to the end; direct callers should use heap.Pop.
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	v := old[n-1]
	*pq = old[:n-1]
	return v
}

// hydrologyField holds the Priority-Flood+ε filled surface, D8 flow accumulation,
// lake marks, and the per-cell river mask for one chunk-centered buffer. Every
// array is materialised up-front in newHydrologyField so the read APIs are O(1)
// lookups with a single bounds check.
//
// Coordinate system:
//   - origin is the center chunk the buffer is built around.
//   - originX, originY are the world-coord of buffer[0][0] (top-left tile).
//   - buffer indices are [row = y-originY][col = x-originX], dy-major to match
//     Chunk.Tiles.
type hydrologyField struct {
	origin  ChunkCoord
	originX int
	originY int

	// fillElev is the depression-filled elevation. Cells below elevationOcean
	// (ocean sinks) keep their raw elevation; every land cell has fillElev ≥
	// rawElev, strictly greater where Priority-Flood lifted it over a spill
	// point.
	fillElev [hydrologyBufferSide][hydrologyBufferSide]float64

	// wasRaised marks cells whose fillElev is strictly greater than their raw
	// elevation by more than ε/2 — the lake tiles. The tolerance rejects pure
	// ε-lift propagation (which touches every cell) and keeps the mask honest.
	wasRaised [hydrologyBufferSide][hydrologyBufferSide]bool

	// accum[y][x] is the D8 upstream-area count at (x, y): the number of cells
	// (including self) whose single-flow-direction path eventually passes
	// through (x, y). Large accum → large river channel.
	accum [hydrologyBufferSide][hydrologyBufferSide]int32

	// river marks the cells classified as river tiles. Computed by markRivers
	// from the accum + source propagation pass.
	river [hydrologyBufferSide][hydrologyBufferSide]bool
}

// newHydrologyField builds a hydrologyField for the buffer centered on cc.
// Deterministic given (g.seed, cc). Three passes:
//  1. Sample raw elevation and the per-cell mountain-and-moist source flag.
//  2. Priority-Flood+ε fills depressions and marks lakes.
//  3. D8 flow direction + Kahn reverse-topological walk accumulates upstream
//     area AND OR-propagates the source flag downstream.
//
// A final markRivers pass writes the per-cell river mask so RiverTilesInChunk
// is a pure scan.
func newHydrologyField(g *WorldGenerator, cc ChunkCoord) *hydrologyField {
	minX, _, minY, _ := cc.Bounds()
	originX := minX - hydrologyBufferChunks*ChunkSize
	originY := minY - hydrologyBufferChunks*ChunkSize

	f := &hydrologyField{
		origin:  cc,
		originX: originX,
		originY: originY,
	}

	var raw [hydrologyBufferSide][hydrologyBufferSide]float64
	var source [hydrologyBufferSide][hydrologyBufferSide]bool
	for dy := range hydrologyBufferSide {
		for dx := range hydrologyBufferSide {
			fx := float64(originX + dx)
			fy := float64(originY + dy)
			elev := g.elevationAt(fx, fy)
			raw[dy][dx] = elev
			if elev >= elevationMountain {
				if g.moisture.Eval2Normalized(fx, fy) >= riverMoistureThreshold {
					source[dy][dx] = true
				}
			}
		}
	}

	f.priorityFlood(&raw)
	hasSource := f.computeFlowAccum(&source)
	f.markRivers(hasSource)

	return f
}

// priorityFlood implements Barnes/Lehman/Mulla (2014) Priority-Flood+ε on the
// buffer. Seed the min-heap with every boundary cell at its raw elevation,
// marked processed; then repeatedly pop the lowest cell and, for each
// unprocessed in-bounds D8 neighbour, set fillElev[n] = max(rawElev,
// fillElev[popped]+ε) and push. Ocean cells (raw < elevationOcean) are never
// raised — they are exit sinks where water leaves the buffer.
//
// Complexity is O(N log N) for N=hydrologyBufferSide². On 80×80 = 6400 cells
// with a binary heap this is a few hundred microseconds.
func (f *hydrologyField) priorityFlood(raw *[hydrologyBufferSide][hydrologyBufferSide]float64) {
	const n = hydrologyBufferSide
	var processed [n][n]bool
	pq := make(priorityQueue, 0, 4*n)

	for i := range n {
		seed := func(x, y int) {
			f.fillElev[y][x] = raw[y][x]
			processed[y][x] = true
			heap.Push(&pq, cellPri{x: x, y: y, elev: raw[y][x]})
		}
		seed(i, 0)
		seed(i, n-1)
		if i != 0 && i != n-1 {
			seed(0, i)
			seed(n-1, i)
		}
	}

	for pq.Len() > 0 {
		c := heap.Pop(&pq).(cellPri)
		for _, off := range squareNeighborOffsets {
			nx := c.x + int(off.dx)
			ny := c.y + int(off.dy)
			if nx < 0 || nx >= n || ny < 0 || ny >= n {
				continue
			}
			if processed[ny][nx] {
				continue
			}
			processed[ny][nx] = true

			rawElev := raw[ny][nx]
			var filled float64
			if rawElev < elevationOcean {
				filled = rawElev
			} else {
				filled = max(rawElev, c.elev+hydrologyEpsilon)
			}
			f.fillElev[ny][nx] = filled
			if filled > rawElev+hydrologyEpsilon/2 {
				f.wasRaised[ny][nx] = true
			}
			heap.Push(&pq, cellPri{x: nx, y: ny, elev: filled})
		}
	}
}

// computeFlowAccum does the D8 flow-direction and reverse-topological
// accumulation pass. Returns hasSource: a bool array where hasSource[y][x] is
// true iff the catchment upstream of (x, y) contains at least one
// mountain-and-moist cell. OR-propagation piggybacks on the same Kahn walk that
// counts accum, so the whole hydrology pass stays O(N).
//
// After priority-flood the fillElev field has a strictly-descending path from
// every interior cell to the buffer boundary, so "downhill neighbour" is
// well-defined on interior cells; boundary cells with no strictly-lower
// neighbour terminate their flow (dx = dy = 0).
func (f *hydrologyField) computeFlowAccum(
	source *[hydrologyBufferSide][hydrologyBufferSide]bool,
) [hydrologyBufferSide][hydrologyBufferSide]bool {
	const n = hydrologyBufferSide

	var downX, downY [n][n]int8
	var indeg [n][n]int32
	for y := range n {
		for x := range n {
			f.accum[y][x] = 1
			dx, dy := f.bestDownhill(x, y)
			downX[y][x] = dx
			downY[y][x] = dy
			if dx != 0 || dy != 0 {
				indeg[y+int(dy)][x+int(dx)]++
			}
		}
	}

	var hasSource [n][n]bool
	for y := range n {
		for x := range n {
			hasSource[y][x] = source[y][x]
		}
	}

	queue := make([][2]int, 0, n)
	for y := range n {
		for x := range n {
			if indeg[y][x] == 0 {
				queue = append(queue, [2]int{x, y})
			}
		}
	}
	for head := 0; head < len(queue); head++ {
		x, y := queue[head][0], queue[head][1]
		dx, dy := int(downX[y][x]), int(downY[y][x])
		if dx == 0 && dy == 0 {
			continue
		}
		nx, ny := x+dx, y+dy
		f.accum[ny][nx] += f.accum[y][x]
		if hasSource[y][x] {
			hasSource[ny][nx] = true
		}
		indeg[ny][nx]--
		if indeg[ny][nx] == 0 {
			queue = append(queue, [2]int{nx, ny})
		}
	}
	return hasSource
}

// bestDownhill returns the D8 offset giving the steepest descent from (x, y) on
// the filled surface. "Steepest" is (fillElev[center] - fillElev[n]) * invDist,
// maximised — so a 1-unit orthogonal drop beats a 1-unit diagonal drop (which
// is gentler per travelled distance). Returns (0, 0) when no neighbour is
// strictly lower, which after Priority-Flood can only happen on the buffer
// boundary.
func (f *hydrologyField) bestDownhill(x, y int) (int8, int8) {
	const n = hydrologyBufferSide
	center := f.fillElev[y][x]
	var bestDX, bestDY int8
	var bestSlope float64
	for _, off := range squareNeighborOffsets {
		nx := x + int(off.dx)
		ny := y + int(off.dy)
		if nx < 0 || nx >= n || ny < 0 || ny >= n {
			continue
		}
		drop := center - f.fillElev[ny][nx]
		if drop <= 0 {
			continue
		}
		slope := drop * off.invDist
		if slope > bestSlope {
			bestSlope = slope
			bestDX = off.dx
			bestDY = off.dy
		}
	}
	return bestDX, bestDY
}

// markRivers fills the per-cell river mask. A cell is a river iff:
//   - above ocean on the filled surface (rivers live on land),
//   - not a lake (lakes render as OverlayLake; a river mark on a lake cell would
//     double-paint),
//   - at least riverAccumThreshold upstream cells flow through it (sparsity),
//   - the catchment contains a mountain-and-moist cell (physical headwater).
//
// The fourth condition is the "rivers start in mountains" rule: dry lowland
// catchments and cells whose entire catchment is below the mountain band never
// become rivers, no matter how much flow they accumulate.
func (f *hydrologyField) markRivers(hasSource [hydrologyBufferSide][hydrologyBufferSide]bool) {
	const n = hydrologyBufferSide
	for y := range n {
		for x := range n {
			if f.fillElev[y][x] < elevationOcean {
				continue
			}
			if f.wasRaised[y][x] {
				continue
			}
			if f.accum[y][x] < riverAccumThreshold {
				continue
			}
			if !hasSource[y][x] {
				continue
			}
			f.river[y][x] = true
		}
	}
}

// elevationAt returns the depression-filled elevation at world coord (x, y).
// The second return is false when (x, y) is outside the hydrology buffer.
func (f *hydrologyField) elevationAt(x, y int) (float64, bool) {
	dx, dy, ok := f.localOffset(x, y)
	if !ok {
		return 0, false
	}
	return f.fillElev[dy][dx], true
}

// isLakeAt reports whether the cell at world coord (x, y) was raised by
// depression filling — i.e. it is a lake tile. Returns false for cells outside
// the buffer.
func (f *hydrologyField) isLakeAt(x, y int) bool {
	dx, dy, ok := f.localOffset(x, y)
	if !ok {
		return false
	}
	return f.wasRaised[dy][dx]
}

// isRiverAt reports whether the cell at world coord (x, y) is a river tile.
// Returns false for cells outside the buffer.
func (f *hydrologyField) isRiverAt(x, y int) bool {
	dx, dy, ok := f.localOffset(x, y)
	if !ok {
		return false
	}
	return f.river[dy][dx]
}

// accumAt returns the upstream-area count at world coord (x, y). The second
// return is false when (x, y) is outside the buffer.
func (f *hydrologyField) accumAt(x, y int) (int32, bool) {
	dx, dy, ok := f.localOffset(x, y)
	if !ok {
		return 0, false
	}
	return f.accum[dy][dx], true
}

// localOffset converts a world coord into buffer-local indices. ok=false when
// (x, y) lies outside the buffer rectangle.
func (f *hydrologyField) localOffset(x, y int) (dx, dy int, ok bool) {
	dx = x - f.originX
	dy = y - f.originY
	if dx < 0 || dx >= hydrologyBufferSide || dy < 0 || dy >= hydrologyBufferSide {
		return 0, 0, false
	}
	return dx, dy, true
}

// hydrologyCache is a bounded LRU of *hydrologyField keyed by the
// buffer-origin chunk coord. Thread-safety comes from
// hashicorp/golang-lru/v2's built-in locking.
type hydrologyCache struct {
	capacity int
	c        *lru.Cache[ChunkCoord, *hydrologyField]
}

// newHydrologyCache builds a hydrologyCache with the requested capacity.
// Non-positive values fall back to DefaultHydrologyCacheCapacity.
func newHydrologyCache(capacity int) *hydrologyCache {
	if capacity <= 0 {
		capacity = DefaultHydrologyCacheCapacity
	}
	c, err := lru.New[ChunkCoord, *hydrologyField](capacity)
	if err != nil {
		panic("worldgen: hydrology cache init: " + err.Error())
	}
	return &hydrologyCache{capacity: capacity, c: c}
}

// Capacity returns the configured LRU capacity. Useful for tests.
func (c *hydrologyCache) Capacity() int { return c.capacity }

// Len returns the current number of cached fields. Useful for tests.
func (c *hydrologyCache) Len() int { return c.c.Len() }

// get returns the cached field for origin, promoting it to MRU.
func (c *hydrologyCache) get(origin ChunkCoord) (*hydrologyField, bool) {
	return c.c.Get(origin)
}

// put inserts or replaces the cached field for origin.
func (c *hydrologyCache) put(origin ChunkCoord, f *hydrologyField) {
	c.c.Add(origin, f)
}

// hydrologyFor returns the cached hydrologyField for cc, building it on miss.
// Callers share one field per chunk under the typical viewport-scroll pattern,
// so the cache hit rate is near 100% after warmup.
func (g *WorldGenerator) hydrologyFor(cc ChunkCoord) *hydrologyField {
	if cached, ok := g.hydrology.get(cc); ok {
		return cached
	}
	f := newHydrologyField(g, cc)
	g.hydrology.put(cc, f)
	return f
}

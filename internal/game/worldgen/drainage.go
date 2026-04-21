package worldgen

import (
	"container/heap"

	lru "github.com/hashicorp/golang-lru/v2"
)

// drainageEpsilon is the microscopic elevation increment added to every cell a priority-flood
// fill raises. The Priority-Flood+Epsilon variant of Barnes, Lehman, Mulla (2014) uses this
// tiny gradient so that the filled basin still has a strictly-descending path toward the spill
// point — without ε, the basin would be a flat plateau that traps greedy steepest-descent
// river tracing. 1e-9 is conservatively double-precision safe: a 80×80 buffer filled end-to-end
// can accumulate at most 6400*ε ≈ 6.4e-6 of drift, far below the elevation band resolution.
const drainageEpsilon = 1e-9

// drainageBufferChunks matches riverBufferChunks — the drainage field is computed over the
// same 5×5 chunk window that rivers.go scans for sources. Sharing the buffer keeps the fill
// and the path tracer looking at identical arrays, so the "reached ocean" and "left the
// scan window" termination conditions are consistent.
const drainageBufferChunks = riverBufferChunks

// drainageBufferSide is the edge length (in tiles) of the 5×5 chunk buffer. Derived constant,
// materialised for the fixed-size arrays in drainageField below so we never allocate per-tile
// slices on the hot path.
const drainageBufferSide = (2*drainageBufferChunks + 1) * ChunkSize

// flowAccumThreshold is the minimum upstream-area count a tile must have to spawn a river.
// Phase-3b replaces the old elevation+moisture+sparsity triple gate with a physics gate:
// rivers exist only where flow accumulation says water actually collects. The moisture gate
// is retained (deserts stay dry) but the hash-based sparsity is gone — accumulation produces
// natural sparsity.
//
// Calibration: TestFlowAccumThresholdProducesRealisticDensity (drainage_test.go) sweeps
// thresholds across 8 seeds × 32×32-chunk regions and records river-source density as a
// fraction of land tiles. The chosen value lands between ~0.5% and ~3% density across every
// seed tested, matching the empirical spawn mass of the pre-Phase-3 gate (~2.2%).
const flowAccumThreshold int32 = 40

// DefaultDrainageCacheCapacity is the LRU size for drainageCache. 256 entries is smaller than
// the chunk cache because each drainage field is larger (two 80×80 float64 arrays plus an
// int32 accumulator array — roughly 6400*(8+8+4) ≈ 160 KB per entry). 256 still covers a
// generous viewport + pre-fetch ring without letting memory creep unboundedly.
const DefaultDrainageCacheCapacity = 256

// cellPri is the min-heap payload for Priority-Flood. x, y are grid indices inside the
// drainage buffer; elev is the fill elevation used as the priority key. A value-type payload
// keeps heap operations alloc-free on the hot path (no boxed interface{} assignments).
type cellPri struct {
	x, y int
	elev float64
}

// priorityQueue is a min-heap of cellPri keyed by elev. Implements container/heap.Interface.
// Ordering by elev (not by insertion) is what makes priority-flood O(N log N): every cell is
// popped at its final fill elevation, in non-decreasing order, so downstream neighbours can
// never raise upstream ones.
type priorityQueue []cellPri

// Len reports the heap size.
func (pq priorityQueue) Len() int { return len(pq) }

// Less orders by elev ascending — smallest elevation first, as priority-flood requires.
func (pq priorityQueue) Less(i, j int) bool { return pq[i].elev < pq[j].elev }

// Swap exchanges two heap entries in place.
func (pq priorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

// Push appends x (must be cellPri) to the heap storage. container/heap calls this from
// heap.Push; direct callers should use heap.Push(pq, value) instead.
func (pq *priorityQueue) Push(x any) { *pq = append(*pq, x.(cellPri)) }

// Pop removes and returns the last element of the heap storage. container/heap calls this
// after it has swapped the min to the end; direct callers should use heap.Pop(pq).
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	v := old[n-1]
	*pq = old[:n-1]
	return v
}

// drainageField holds the Priority-Flood+Epsilon fill and the D6 flow accumulation for a
// single 5×5 chunk buffer. All state is materialised up-front in NewDrainageField so the
// read APIs (elevationAt, isLake, accumAt) are branch-light O(1) lookups.
//
// Coordinate system:
//   - bufferOrigin is the center chunk the buffer is built around.
//   - originX, originY are the world-coord of buffer[0][0] (top-left tile).
//   - buffer indices are [row=y-originY][col=x-originX] — dy-major, matching Chunk.Tiles.
type drainageField struct {
	bufferOrigin ChunkCoord
	originX      int
	originY      int

	// fillElev is the depression-filled elevation. Cells below elevationOcean (ocean sinks)
	// keep their raw elevation; every land cell has fillElev >= rawElev + k*ε where k is the
	// distance to the spill point (loosely — priority-flood guarantees strictly-descending
	// paths, not uniform gradient).
	fillElev [drainageBufferSide][drainageBufferSide]float64

	// wasRaised marks cells whose fillElev is strictly greater than their raw elevation
	// (by more than ε to tolerate floating-point noise). These are the lake tiles — water
	// cannot escape their depression without being raised by the fill.
	wasRaised [drainageBufferSide][drainageBufferSide]bool

	// accum[y][x] is the upstream-area count at (x, y): the number of cells (including self)
	// whose D6 downhill flow eventually passes through (x, y). Large accum → large river.
	accum [drainageBufferSide][drainageBufferSide]int32
}

// newDrainageField builds a drainageField for the 5×5 buffer centered on bufferOrigin.
// It runs three passes:
//  1. Sample raw elevation for every cell in the buffer.
//  2. Priority-Flood+Epsilon to fill depressions.
//  3. D6 flow accumulation via reverse-topological walk.
//
// All passes are deterministic given (g.seed, bufferOrigin). The heap seeds on the buffer
// boundary, so shrinking or shifting the buffer changes the fill — callers must keep the
// buffer shape stable across runs.
func newDrainageField(g *WorldGenerator, bufferOrigin ChunkCoord) *drainageField {
	minX, _, minY, _ := bufferOrigin.Bounds()
	originX := minX - drainageBufferChunks*ChunkSize
	originY := minY - drainageBufferChunks*ChunkSize

	field := &drainageField{
		bufferOrigin: bufferOrigin,
		originX:      originX,
		originY:      originY,
	}

	// Pass 1: sample raw elevation for every tile in the buffer.
	var raw [drainageBufferSide][drainageBufferSide]float64
	for dy := range drainageBufferSide {
		for dx := range drainageBufferSide {
			fx := float64(originX + dx)
			fy := float64(originY + dy)
			raw[dy][dx] = g.elevationAt(fx, fy)
		}
	}

	// Pass 2: Priority-Flood+Epsilon fill. Populates field.fillElev and field.wasRaised.
	field.priorityFlood(&raw)

	// Pass 3: D6 flow accumulation on the filled field. Populates field.accum.
	field.computeFlowAccum()

	return field
}

// priorityFlood implements Barnes/Lehman/Mulla (2014) Priority-Flood+Epsilon on the buffer.
// Seed the min-heap with every boundary cell at its raw elevation; then repeatedly pop the
// lowest cell and, for each unprocessed in-bounds neighbour, set fillElev[n] = max(raw[n],
// fillElev[popped] + ε) and push. Ocean cells (raw < elevationOcean) are never raised —
// they are treated as sinks where water exits.
//
// The algorithm is O(N log N) for N=drainageBufferSide². On 80×80 = 6400 cells with a binary
// heap this is a few hundred microseconds — negligible per chunk.
func (f *drainageField) priorityFlood(raw *[drainageBufferSide][drainageBufferSide]float64) {
	const n = drainageBufferSide

	var processed [n][n]bool
	pq := make(priorityQueue, 0, 4*n)

	// Seed: every cell on the buffer boundary goes in at its raw elevation, marked processed.
	// Seeding with the raw boundary elevation (not fillElev) is correct because boundary
	// cells are not raised — they are the exits where water leaves the buffer.
	for i := range n {
		seedCell := func(x, y int) {
			f.fillElev[y][x] = raw[y][x]
			processed[y][x] = true
			heap.Push(&pq, cellPri{x: x, y: y, elev: raw[y][x]})
		}
		seedCell(i, 0)
		seedCell(i, n-1)
		if i != 0 && i != n-1 {
			seedCell(0, i)
			seedCell(n-1, i)
		}
	}

	// Process: pop the lowest unprocessed cell and flood its neighbours at max(raw, c+ε).
	for pq.Len() > 0 {
		c := heap.Pop(&pq).(cellPri)
		for _, off := range hexNeighborOffsets {
			nx := c.x + off[0]
			ny := c.y + off[1]
			if nx < 0 || nx >= n || ny < 0 || ny >= n {
				continue
			}
			if processed[ny][nx] {
				continue
			}
			processed[ny][nx] = true

			rawElev := raw[ny][nx]
			// Ocean cells are water-exit sinks — never raise them. Treat them as if they
			// pass to the boundary at their own elevation so downstream cells can still
			// drain into them at the raw ocean level.
			var filled float64
			if rawElev < elevationOcean {
				filled = rawElev
			} else {
				filled = max(rawElev, c.elev+drainageEpsilon)
			}
			f.fillElev[ny][nx] = filled
			// wasRaised tolerates floating-point noise: require a strictly-positive lift
			// at least one ε above raw. Using >0 alone would flag every land cell touched
			// by the ε bump even when it was not inside a real depression.
			if filled > rawElev+drainageEpsilon/2 {
				f.wasRaised[ny][nx] = true
			}
			heap.Push(&pq, cellPri{x: nx, y: ny, elev: filled})
		}
	}
}

// computeFlowAccum performs the D6 flow-direction + reverse-topological accumulation pass.
// After priorityFlood the fillElev field has a strictly-descending path from every cell to
// the buffer boundary, so "downhill neighbour" is always well-defined on interior cells.
//
// Implementation: first compute each cell's indegree (number of neighbours whose downhill is
// this cell). Then Kahn-style: start with cells of indegree 0 (headwater tiles), accumulate
// self + incoming into downhill neighbour, decrement indegree, push to queue when it hits 0.
// O(N) total.
func (f *drainageField) computeFlowAccum() {
	downX, downY, indeg := f.computeFlowDirections()
	f.accumulateFlow(downX, downY, indeg)
}

// computeFlowDirections walks every cell in the buffer and picks its D6 downhill neighbour
// offset. Returned arrays hold the chosen offset (zero if none lower) per cell, plus the
// indegree histogram — how many neighbours point at each cell. Split out of computeFlowAccum
// to keep each function's cognitive complexity manageable.
//
// The three named-return arrays (downX, downY, indeg) are fixed-size value types
// ([80][80]int8 / [80][80]int32). Verified with -gcflags='-m=2' on Go 1.24: none of them
// are "moved to heap" — they live on the stack frame of computeFlowDirections and are copied
// out to the caller by value. No scratch-field promotion is needed.
func (f *drainageField) computeFlowDirections() (downX, downY [drainageBufferSide][drainageBufferSide]int8, indeg [drainageBufferSide][drainageBufferSide]int32) {
	const n = drainageBufferSide
	for y := range n {
		for x := range n {
			// Initialise accum: every cell drains at least itself.
			f.accum[y][x] = 1
			bestDX, bestDY := f.bestDownhill(x, y)
			downX[y][x] = bestDX
			downY[y][x] = bestDY
			if bestDX != 0 || bestDY != 0 {
				indeg[y+int(bestDY)][x+int(bestDX)]++
			}
		}
	}
	return downX, downY, indeg
}

// bestDownhill returns the D6 neighbour offset of the cell with the strictly-lowest fillElev
// relative to (x, y). Returns (0, 0) when no neighbour is strictly lower — this happens
// exclusively on the buffer boundary after priority-flood.
func (f *drainageField) bestDownhill(x, y int) (int8, int8) {
	const n = drainageBufferSide
	bestDX, bestDY := int8(0), int8(0)
	bestElev := f.fillElev[y][x]
	for _, off := range hexNeighborOffsets {
		nx, ny := x+off[0], y+off[1]
		if nx < 0 || nx >= n || ny < 0 || ny >= n {
			continue
		}
		ne := f.fillElev[ny][nx]
		if ne < bestElev {
			bestElev = ne
			bestDX = int8(off[0])
			bestDY = int8(off[1])
		}
	}
	return bestDX, bestDY
}

// accumulateFlow runs the Kahn-style topological drain. All cells with indegree 0 seed the
// queue; popping a cell adds its accum to the downhill neighbour's accum and decrements that
// neighbour's indegree, enqueueing it when it hits 0.
func (f *drainageField) accumulateFlow(
	downX, downY [drainageBufferSide][drainageBufferSide]int8,
	indeg [drainageBufferSide][drainageBufferSide]int32,
) {
	const n = drainageBufferSide
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
		indeg[ny][nx]--
		if indeg[ny][nx] == 0 {
			queue = append(queue, [2]int{nx, ny})
		}
	}
}

// elevationAt returns the depression-filled elevation at world coord (x, y). The second
// return value is false when (x, y) is outside the drainage buffer — callers must handle
// that case explicitly (e.g. river tracer terminates with "left the scan window").
func (f *drainageField) elevationAt(x, y int) (float64, bool) {
	dx := x - f.originX
	dy := y - f.originY
	if dx < 0 || dx >= drainageBufferSide || dy < 0 || dy >= drainageBufferSide {
		return 0, false
	}
	return f.fillElev[dy][dx], true
}

// isLake reports whether the cell at world coord (x, y) was raised by depression filling.
// The second return value is false when (x, y) is outside the drainage buffer — callers
// then have no information and should not paint a lake overlay.
func (f *drainageField) isLake(x, y int) (raised, inBuffer bool) {
	dx := x - f.originX
	dy := y - f.originY
	if dx < 0 || dx >= drainageBufferSide || dy < 0 || dy >= drainageBufferSide {
		return false, false
	}
	return f.wasRaised[dy][dx], true
}

// accumAt returns the upstream-area count at world coord (x, y). The second return value
// is false when (x, y) is outside the drainage buffer.
func (f *drainageField) accumAt(x, y int) (int32, bool) {
	dx := x - f.originX
	dy := y - f.originY
	if dx < 0 || dx >= drainageBufferSide || dy < 0 || dy >= drainageBufferSide {
		return 0, false
	}
	return f.accum[dy][dx], true
}

// drainageCache is a bounded LRU of drainageField keyed by the buffer-origin chunk coord.
// Thread safety comes from the underlying hashicorp/golang-lru cache, which is itself
// goroutine-safe.
type drainageCache struct {
	capacity int
	c        *lru.Cache[ChunkCoord, *drainageField]
}

// newDrainageCache builds a drainageCache with the requested capacity. Non-positive values
// fall back to DefaultDrainageCacheCapacity so callers that pass 0 get a sane default.
func newDrainageCache(capacity int) *drainageCache {
	if capacity <= 0 {
		capacity = DefaultDrainageCacheCapacity
	}
	c, err := lru.New[ChunkCoord, *drainageField](capacity)
	if err != nil {
		panic("worldgen: drainage cache init: " + err.Error())
	}
	return &drainageCache{capacity: capacity, c: c}
}

// Capacity returns the configured LRU capacity. Primarily useful for tests.
func (c *drainageCache) Capacity() int { return c.capacity }

// Len returns the current number of cached fields. Primarily useful for tests.
func (c *drainageCache) Len() int { return c.c.Len() }

// get returns the cached drainageField for bufferOrigin if present and promotes it to MRU.
func (c *drainageCache) get(bufferOrigin ChunkCoord) (*drainageField, bool) {
	return c.c.Get(bufferOrigin)
}

// put inserts or replaces the drainageField for bufferOrigin.
func (c *drainageCache) put(bufferOrigin ChunkCoord, field *drainageField) {
	c.c.Add(bufferOrigin, field)
}

// drainageFor returns the cached drainageField for the 5×5 buffer centered on cc, building
// it on miss. Callers (river tracer, Chunk()) share one field per chunk, so the cache hit
// rate is essentially "did any neighbour already request this chunk" — near 100% under the
// typical viewport-scroll access pattern.
func (g *WorldGenerator) drainageFor(cc ChunkCoord) *drainageField {
	if cached, ok := g.drainage.get(cc); ok {
		return cached
	}
	field := newDrainageField(g, cc)
	g.drainage.put(cc, field)
	return field
}

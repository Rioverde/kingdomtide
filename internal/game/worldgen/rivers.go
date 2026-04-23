package worldgen

import (
	"container/heap"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
)

// floorDiv mirrors the chunk package's unexported helper — duplicated here to
// keep the river placement math in the same package as the river-head loop
// without widening the chunk package's exported surface.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// Elevation thresholds mirrored from the biome package so the river gating
// logic can run without importing biome just for two constants. These must
// track biome/biome.go — any retune there requires an update here too.
const (
	elevationOcean    = 0.44
	elevationMountain = 0.63
)

// riverHeadSpacing is the edge length in tiles of the fixed world-grid used to
// place river-head candidates. Every integer multiple of riverHeadSpacing on
// both axes is a candidate; isValidHead then hash-gates and elevation-gates
// the candidate down to a sparse actual-head set. Smaller values produce more
// candidates (denser rivers) at higher enumeration cost per chunk. 6 tiles
// trades cheaper enumeration (vs spacing 4) for slightly coarser head
// placement; traces merge so the visible river density is barely affected.
const riverHeadSpacing = 6

// riverHeadDensity is the probability a mountain-and-moist head candidate is
// actually a river head. A uniform hash of (seed, hx, hy) below this value
// accepts the candidate; this keeps river heads sparse without requiring any
// global structure. 0.40 lands at ~1% river-tile land-coverage, which reads
// as "rivers are a noticeable feature" without saturating the map.
const riverHeadDensity = 0.40

// riverMoistureThreshold gates a head candidate by local moisture. Deserts
// don't birth rivers even when they have mountain peaks.
const riverMoistureThreshold = 0.50

// riverMaxTraceLen caps the number of D8 steps a single trace may take. Any
// river that cannot reach the sea (or a terminal basin) within this budget is
// truncated. Bounds per-chunk work and stops pathological cycles in corner
// cases of the raw-elevation field. 192 tiles is ~12 chunks of reach — enough
// for a continental river to reach a coast in the seeds tested.
const riverMaxTraceLen = 192

// riverMaxBasinCells caps the number of cells a local flood-fill may explore
// when searching for a depression's spill point. Beyond this the depression
// is declared endorheic and the trace terminates; the accumulated cells are
// still marked as lake so the map shows a genuine inland sea rather than a
// broken river.
const riverMaxBasinCells = 512

// DefaultRiverCacheCapacity is the LRU size for cached per-chunk river/lake
// sets. Each entry is a pair of small maps (typical rivers in a chunk: 0–30
// tiles), so even 256 entries take well under 1 MB.
const DefaultRiverCacheCapacity = 256

// invSqrt2 is 1/√2 at float64 precision, used to slope-weight diagonal D8
// neighbours so steepest-descent is computed in slope units (drop / distance)
// rather than raw drop.
const invSqrt2 = 0.7071067811865476

// neighborOffset names one direction in a D8 Moore neighborhood on a square
// grid. invDist caches 1/√(dx²+dy²) so slope comparisons are a single
// multiply instead of a divide.
type neighborOffset struct {
	dx, dy  int8
	invDist float64
}

// squareNeighborOffsets is the D8 Moore neighborhood used everywhere in this
// file: steepest-descent picks, local flood-fill expansion, tie-break order.
// Fixed order means deterministic tie-breaks across runs.
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

// tileCoord is a compact (x, y) tile identifier used as map keys.
type tileCoord = [2]int

// cellPri is the min-heap payload for the local flood-fill priority queue.
type cellPri struct {
	x, y int
	elev float64
}

// priorityQueue is a min-heap of cellPri ordered by elev ascending. Exactly
// the shape container/heap expects; used by localFloodFill to pop rim cells
// in ascending elevation order so the first neighbour below the current rim
// is the true spill point.
type priorityQueue []cellPri

// Len reports the heap size.
func (pq priorityQueue) Len() int { return len(pq) }

// Less orders by elev ascending — smallest elevation first.
func (pq priorityQueue) Less(i, j int) bool { return pq[i].elev < pq[j].elev }

// Swap exchanges two heap entries.
func (pq priorityQueue) Swap(i, j int) { pq[i], pq[j] = pq[j], pq[i] }

// Push appends x (must be cellPri) to the heap storage. container/heap
// invokes this from heap.Push; direct callers should use heap.Push.
func (pq *priorityQueue) Push(x any) { *pq = append(*pq, x.(cellPri)) }

// Pop removes and returns the last element. container/heap invokes this
// after swapping the min to the end; direct callers should use heap.Pop.
func (pq *priorityQueue) Pop() any {
	old := *pq
	n := len(old)
	v := old[n-1]
	*pq = old[:n-1]
	return v
}

// chunkRiverData is the precomputed per-chunk river/lake overlay sets.
// Stored in the LRU so repeated queries for the same chunk skip re-tracing.
type chunkRiverData struct {
	rivers map[tileCoord]struct{}
	lakes  map[tileCoord]struct{}
}

// riverCache is a bounded LRU of per-chunk river/lake sets keyed by the
// chunk coord. All determinism lives upstream in the trace computation;
// the cache is a pure performance layer and can be dropped without changing
// outputs.
type riverCache struct {
	capacity int
	c        *lru.Cache[chunk.ChunkCoord, *chunkRiverData]
}

// newRiverCache builds a riverCache with the requested capacity. Non-positive
// values fall back to DefaultRiverCacheCapacity.
func newRiverCache(capacity int) *riverCache {
	if capacity <= 0 {
		capacity = DefaultRiverCacheCapacity
	}
	c, err := lru.New[chunk.ChunkCoord, *chunkRiverData](capacity)
	if err != nil {
		panic("worldgen: river cache init: " + err.Error())
	}
	return &riverCache{capacity: capacity, c: c}
}

// Capacity returns the configured LRU capacity. Useful for tests.
func (c *riverCache) Capacity() int { return c.capacity }

// Len returns the current number of cached chunk-river sets. Useful for tests.
func (c *riverCache) Len() int { return c.c.Len() }

func (c *riverCache) get(cc chunk.ChunkCoord) (*chunkRiverData, bool) {
	return c.c.Get(cc)
}

func (c *riverCache) put(cc chunk.ChunkCoord, d *chunkRiverData) {
	c.c.Add(cc, d)
}

// riversFor returns the cached per-chunk river/lake sets for cc, computing
// them on miss.
func (g *WorldGenerator) riversFor(cc chunk.ChunkCoord) *chunkRiverData {
	if cached, ok := g.rivers.get(cc); ok {
		return cached
	}
	d := g.computeChunkRivers(cc)
	g.rivers.put(cc, d)
	return d
}

// RiverTilesInChunk returns the set of world-space grid coordinates inside cc
// that are river tiles. Deterministic per (seed, cc); the classification is
// a pure function of world coord, so two adjacent chunks always agree on
// river membership at their shared boundary. The returned map is a read-only
// view of cached state — callers must not mutate it.
func (g *WorldGenerator) RiverTilesInChunk(cc chunk.ChunkCoord) map[tileCoord]struct{} {
	return g.riversFor(cc).rivers
}

// LakeTilesInChunk returns the set of world-space grid coordinates inside cc
// that are lake tiles (depression cells marked during a trace's local
// flood-fill). Deterministic per (seed, cc).
func (g *WorldGenerator) LakeTilesInChunk(cc chunk.ChunkCoord) map[tileCoord]struct{} {
	return g.riversFor(cc).lakes
}

// computeChunkRivers enumerates every river-head grid point within
// riverMaxTraceLen of cc, validates each as a head, traces those that pass,
// and collects the path + flood-fill cells that fall inside cc's bounds into
// the returned chunkRiverData. Deterministic: no buffer, no macro, no shared
// state between chunks — each chunk's classification is a pure function of
// (seed, cc).
func (g *WorldGenerator) computeChunkRivers(cc chunk.ChunkCoord) *chunkRiverData {
	out := &chunkRiverData{
		rivers: make(map[tileCoord]struct{}),
		lakes:  make(map[tileCoord]struct{}),
	}
	minX, maxX, minY, maxY := cc.Bounds()

	hxLo := floorDiv(minX-riverMaxTraceLen, riverHeadSpacing)
	hxHi := floorDiv(maxX+riverMaxTraceLen, riverHeadSpacing)
	hyLo := floorDiv(minY-riverMaxTraceLen, riverHeadSpacing)
	hyHi := floorDiv(maxY+riverMaxTraceLen, riverHeadSpacing)

	for hxi := hxLo; hxi <= hxHi; hxi++ {
		for hyi := hyLo; hyi <= hyHi; hyi++ {
			hx := hxi * riverHeadSpacing
			hy := hyi * riverHeadSpacing
			if !g.isValidHead(hx, hy) {
				continue
			}

			path, lakes := g.traceRiver(hx, hy)
			for _, t := range path {
				if t[0] >= minX && t[0] < maxX && t[1] >= minY && t[1] < maxY {
					out.rivers[t] = struct{}{}
				}
			}
			for _, t := range lakes {
				if t[0] >= minX && t[0] < maxX && t[1] >= minY && t[1] < maxY {
					out.lakes[t] = struct{}{}
				}
			}
		}
	}

	// A cell can be flagged as both river (from the trace cell just before
	// entering a depression) and lake (from the flood-fill that followed).
	// Lakes win on the rendering layer (see runes.go precedence), but we
	// prune the river set here so tests and downstream consumers see a
	// clean, disjoint classification.
	for t := range out.lakes {
		delete(out.rivers, t)
	}

	return out
}

// isValidHead reports whether (hx, hy) spawns a river. Three gates:
//  1. Hash-based density: only a riverHeadDensity fraction of candidates pass.
//  2. Elevation: at least elevationMountain on the final elevation field.
//  3. Moisture: at least riverMoistureThreshold on the moisture field.
//
// Each gate is a pure function of (seed, hx, hy), so two queries for the same
// coord from different chunks always agree.
func (g *WorldGenerator) isValidHead(hx, hy int) bool {
	h := splitMix64(uint64(hx), uint64(hy), uint64(g.seed)^0x9e3779b97f4a7c15)
	normalised := float64(h>>11) / (1 << 53)
	if normalised >= riverHeadDensity {
		return false
	}
	fx, fy := float64(hx), float64(hy)
	if g.elevationAt(fx, fy) < elevationMountain {
		return false
	}
	if g.moisture.Eval2Normalized(fx, fy) < riverMoistureThreshold {
		return false
	}
	return true
}

// traceRiver walks D8 steepest-descent from (startX, startY) on the raw
// (blended + ridge) elevation field, handling depressions via a local
// priority-flood, and returns (path, lakes). Path is the sequence of land
// tiles the river occupies; lakes is the set of cells that were filled while
// resolving a depression along the way. Both are empty if the trace
// immediately terminates (head is below ocean, cycle, etc.). Deterministic:
// same (startX, startY) on the same generator always produces identical
// output.
func (g *WorldGenerator) traceRiver(startX, startY int) (path, lakes []tileCoord) {
	path = make([]tileCoord, 0, 64)
	lakes = make([]tileCoord, 0, 4)
	visited := make(map[tileCoord]bool, 64)
	cache := make(map[tileCoord]float64, 64)

	elevOf := func(x, y int) float64 {
		c := tileCoord{x, y}
		if v, ok := cache[c]; ok {
			return v
		}
		v := g.elevationAt(float64(x), float64(y))
		cache[c] = v
		return v
	}

	x, y := startX, startY
	for range riverMaxTraceLen {
		if elevOf(x, y) < elevationOcean {
			return path, lakes
		}
		cur := tileCoord{x, y}
		if visited[cur] {
			return path, lakes
		}
		visited[cur] = true
		path = append(path, cur)

		nx, ny, ok := steepestLowerNeighbor(x, y, elevOf)
		if ok {
			x, y = nx, ny
			continue
		}

		spillX, spillY, basin, found := localFloodFill(x, y, elevOf)
		lakes = append(lakes, basin...)
		if !found {
			return path, lakes
		}
		x, y = spillX, spillY
	}
	return path, lakes
}

// steepestLowerNeighbor returns the D8 neighbour of (x, y) with the steepest
// negative slope — slope = (elev[center] - elev[n]) * invDist, maximised over
// neighbours with a strictly positive drop. Returns (_, _, false) when no
// neighbour is strictly lower (the cell is a local minimum or plateau).
func steepestLowerNeighbor(x, y int, elevOf func(int, int) float64) (int, int, bool) {
	center := elevOf(x, y)
	var bestX, bestY int
	var bestSlope float64
	found := false
	for _, off := range squareNeighborOffsets {
		nx, ny := x+int(off.dx), y+int(off.dy)
		drop := center - elevOf(nx, ny)
		if drop <= 0 {
			continue
		}
		slope := drop * off.invDist
		if slope > bestSlope {
			bestSlope = slope
			bestX, bestY = nx, ny
			found = true
		}
	}
	return bestX, bestY, found
}

// localFloodFill resolves a depression starting at the local minimum (sx, sy).
// It runs a bounded Priority-Flood: the heap holds rim-candidate cells keyed
// by elev; popping in ascending order, we expand D8 until we find a neighbour
// whose elev is strictly below the currently-popped cell — that neighbour is
// the spill, i.e. the first land cell outside the basin on the descending
// slope.
//
// The basin slice collects every cell that entered the heap before the spill
// was found. These are the depression cells (the lake); the spill itself is
// returned separately and is NOT in basin — the trace continues from the
// spill.
//
// If the basin grows past riverMaxBasinCells without finding a spill, the
// depression is endorheic (no outflow within the budget) and found=false is
// returned. Callers treat this as a terminal lake.
func localFloodFill(sx, sy int, elevOf func(int, int) float64) (spillX, spillY int, basin []tileCoord, found bool) {
	seedElev := elevOf(sx, sy)
	pq := make(priorityQueue, 0, 16)
	heap.Push(&pq, cellPri{x: sx, y: sy, elev: seedElev})

	processed := make(map[tileCoord]bool, 32)
	processed[tileCoord{sx, sy}] = true
	basin = append(basin, tileCoord{sx, sy})

	for pq.Len() > 0 {
		if len(basin) > riverMaxBasinCells {
			return 0, 0, basin, false
		}
		c := heap.Pop(&pq).(cellPri)
		for _, off := range squareNeighborOffsets {
			nx := c.x + int(off.dx)
			ny := c.y + int(off.dy)
			nc := tileCoord{nx, ny}
			if processed[nc] {
				continue
			}
			processed[nc] = true

			ne := elevOf(nx, ny)
			if ne < c.elev {
				return nx, ny, basin, true
			}
			basin = append(basin, nc)
			heap.Push(&pq, cellPri{x: nx, y: ny, elev: ne})
		}
	}
	return 0, 0, basin, false
}

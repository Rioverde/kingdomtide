// Package rivers hosts the deterministic river + lake overlay pipeline.
// Placement enumerates fixed-grid head candidates near each queried chunk,
// hash-gates them down to a sparse accepted set, and traces D8
// steepest-descent on the composite elevation field until the trace
// reaches ocean, exceeds a trace-length budget, or falls into a
// depression resolved by a local priority-flood (producing a lake).
//
// The package holds no references to the concrete worldgen.WorldGenerator;
// it consumes a narrow TerrainSampler interface for sub-tile elevation
// lookups. Callers outside worldgen should interact through the
// forwarding methods on WorldGenerator; the package is exposed here so
// tests can exercise the trace internals directly.
package rivers

import (
	"sync"

	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Rioverde/gongeons/internal/game/worldgen/biome"
	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
)

// TerrainSampler is the minimal consumer-side interface NoiseRiverSource
// needs from a world generator: a sub-tile-resolution elevation lookup
// for head validation and steepest-descent tracing, and a moisture
// lookup used only during head validation (deserts don't birth rivers).
// Declared here so the rivers package does not depend on the concrete
// *worldgen.WorldGenerator.
type TerrainSampler interface {
	ElevationAtFloat(fx, fy float64) float64
	MoistureAt(fx, fy float64) float64
}

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

// TileCoord is a compact (x, y) tile identifier used as map keys. Exported
// so the overlay-materialisation layer on *WorldGenerator can iterate the
// returned maps without redeclaring a local alias.
type TileCoord = [2]int

// cellPri is the min-heap payload for the local flood-fill priority queue.
type cellPri struct {
	x, y int
	elev float64
}

// cellHeap is a typed min-heap of cellPri ordered by elev ascending. It
// replaces a container/heap-backed priorityQueue to avoid the any-box per
// Push/Pop (~11 MB / 150 cold chunks in the diagnostic profile). The
// sift-up / sift-down algorithms mirror container/heap's implementation
// (parent = (i-1)/2, children = 2i+1/2i+2, and the right-child preference
// rule when children are equal) so the pop order on equal-elev ties is
// identical to the previous container/heap path — determinism is preserved
// bit-for-bit against the existing rivers tests.
type cellHeap struct {
	data []cellPri
}

// less orders by elev ascending — smallest elevation first. Matches the
// comparator the old priorityQueue.Less used.
func (h *cellHeap) less(i, j int) bool { return h.data[i].elev < h.data[j].elev }

// swap exchanges two heap entries.
func (h *cellHeap) swap(i, j int) { h.data[i], h.data[j] = h.data[j], h.data[i] }

// push appends c and sifts it up to restore the heap invariant.
func (h *cellHeap) push(c cellPri) {
	h.data = append(h.data, c)
	h.up(len(h.data) - 1)
}

// pop removes and returns the minimum. The standard container/heap trick:
// swap root with last, shrink, sift the new root down.
func (h *cellHeap) pop() cellPri {
	n := len(h.data) - 1
	h.swap(0, n)
	v := h.data[n]
	h.data = h.data[:n]
	h.down(0, n)
	return v
}

// len reports the heap size.
func (h *cellHeap) len() int { return len(h.data) }

// up sifts the element at j toward the root.
func (h *cellHeap) up(j int) {
	for {
		i := (j - 1) / 2 // parent
		if i == j || !h.less(j, i) {
			break
		}
		h.swap(i, j)
		j = i
	}
}

// down sifts the element at i0 toward the leaves over the first n entries.
// Matches container/heap's tie-break: prefers the left child when it is
// not greater than the right child.
func (h *cellHeap) down(i0, n int) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n || j1 < 0 { // j1 < 0 guards against int overflow.
			break
		}
		j := j1 // left child
		if j2 := j1 + 1; j2 < n && h.less(j2, j1) {
			j = j2 // right child strictly smaller wins
		}
		if !h.less(j, i) {
			break
		}
		h.swap(i, j)
		i = j
	}
	return i > i0
}

// chunkRiverData is the precomputed per-chunk river/lake overlay sets.
// Stored in the LRU so repeated queries for the same chunk skip re-tracing.
type chunkRiverData struct {
	rivers map[TileCoord]struct{}
	lakes  map[TileCoord]struct{}
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
		panic("rivers: river cache init: " + err.Error())
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

// NoiseRiverSource generates deterministic river + lake overlays on top of
// a TerrainSampler. Placement and tracing are pure functions of
// (seed, world coord); the LRU cache memoises the per-chunk enumeration +
// trace work without affecting results.
//
// The source is safe for concurrent read once constructed — the cache is
// backed by hashicorp/golang-lru/v2 which handles its own locking.
type NoiseRiverSource struct {
	seed    int64
	terrain TerrainSampler
	cache   *riverCache
}

// NewNoiseRiverSource wires a river source to a TerrainSampler (for
// composite-elevation sampling during head validation and steepest-
// descent tracing). capacity sets the LRU cache size; non-positive values
// fall back to DefaultRiverCacheCapacity.
func NewNoiseRiverSource(seed int64, terrain TerrainSampler, capacity int) *NoiseRiverSource {
	if capacity <= 0 {
		capacity = DefaultRiverCacheCapacity
	}
	return &NoiseRiverSource{
		seed:    seed,
		terrain: terrain,
		cache:   newRiverCache(capacity),
	}
}

// CacheLen returns the current number of cached chunk-river sets. Useful
// for tests that want to observe cache-hit behaviour on repeat queries.
func (r *NoiseRiverSource) CacheLen() int { return r.cache.Len() }

// RiverTilesInChunk returns the set of world-space grid coordinates inside cc
// that are river tiles. Deterministic per (seed, cc); the classification is
// a pure function of world coord, so two adjacent chunks always agree on
// river membership at their shared boundary. The returned map is a read-only
// view of cached state — callers must not mutate it.
func (r *NoiseRiverSource) RiverTilesInChunk(cc chunk.ChunkCoord) map[TileCoord]struct{} {
	return r.chunkData(cc).rivers
}

// LakeTilesInChunk returns the set of world-space grid coordinates inside cc
// that are lake tiles (depression cells marked during a trace's local
// flood-fill). Deterministic per (seed, cc).
func (r *NoiseRiverSource) LakeTilesInChunk(cc chunk.ChunkCoord) map[TileCoord]struct{} {
	return r.chunkData(cc).lakes
}

// chunkData returns the cached per-chunk river/lake sets for cc, computing
// them on miss.
func (r *NoiseRiverSource) chunkData(cc chunk.ChunkCoord) *chunkRiverData {
	if cached, ok := r.cache.get(cc); ok {
		return cached
	}
	d := r.computeChunkRivers(cc)
	r.cache.put(cc, d)
	return d
}

// computeChunkRivers enumerates every river-head grid point within
// riverMaxTraceLen of cc, validates each as a head, traces those that pass,
// and collects the path + flood-fill cells that fall inside cc's bounds into
// the returned chunkRiverData. Deterministic: no buffer, no macro, no shared
// state between chunks — each chunk's classification is a pure function of
// (seed, cc).
func (r *NoiseRiverSource) computeChunkRivers(cc chunk.ChunkCoord) *chunkRiverData {
	out := &chunkRiverData{
		rivers: make(map[TileCoord]struct{}),
		lakes:  make(map[TileCoord]struct{}),
	}
	minX, maxX, minY, maxY := cc.Bounds()

	hxLo := chunk.FloorDiv(minX-riverMaxTraceLen, riverHeadSpacing)
	hxHi := chunk.FloorDiv(maxX+riverMaxTraceLen, riverHeadSpacing)
	hyLo := chunk.FloorDiv(minY-riverMaxTraceLen, riverHeadSpacing)
	hyHi := chunk.FloorDiv(maxY+riverMaxTraceLen, riverHeadSpacing)

	for hxi := hxLo; hxi <= hxHi; hxi++ {
		for hyi := hyLo; hyi <= hyHi; hyi++ {
			hx := hxi * riverHeadSpacing
			hy := hyi * riverHeadSpacing
			if !r.isValidHead(hx, hy) {
				continue
			}

			path, lakes := r.traceRiver(hx, hy)
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
func (r *NoiseRiverSource) isValidHead(hx, hy int) bool {
	h := genprim.SplitMix64(uint64(hx), uint64(hy), uint64(r.seed)^0x9e3779b97f4a7c15)
	normalised := float64(h>>11) / (1 << 53)
	if normalised >= riverHeadDensity {
		return false
	}
	fx, fy := float64(hx), float64(hy)
	if r.terrain.ElevationAtFloat(fx, fy) < biome.ElevationMountain {
		return false
	}
	if r.terrain.MoistureAt(fx, fy) < riverMoistureThreshold {
		return false
	}
	return true
}

// traceScratch groups trace-local maps pooled via sync.Pool to cut GC churn across hundreds of traces per super-region.
type traceScratch struct {
	visited map[TileCoord]struct{}
	cache   map[TileCoord]float64
}

// traceScratchPool reuses visited + cache maps across traceRiver calls.
// New() seeds the maps at the same capacities traceRiver used pre-pool
// so the first-touch allocation shape is preserved.
var traceScratchPool = sync.Pool{
	New: func() any {
		return &traceScratch{
			visited: make(map[TileCoord]struct{}, 64),
			cache:   make(map[TileCoord]float64, 64),
		}
	},
}

// floodScratch groups localFloodFill's heap + processed map for pooling.
// Eliminates the fresh priorityQueue slice and processed map allocated on
// every call (78.8% of alloc_objects in the cold-chunk profile).
type floodScratch struct {
	pq        cellHeap
	processed map[TileCoord]struct{}
}

// floodScratchPool reuses heap + processed map across localFloodFill calls.
// Sized to riverMaxBasinCells so the backing slice / map never resize in
// a single call. localFloodFill is not recursive nor concurrent within a
// trace: traceRiver owns one scratch from traceScratchPool and makes
// serial localFloodFill calls, so pool reuse is safe.
var floodScratchPool = sync.Pool{
	New: func() any {
		return &floodScratch{
			pq:        cellHeap{data: make([]cellPri, 0, riverMaxBasinCells)},
			processed: make(map[TileCoord]struct{}, riverMaxBasinCells),
		}
	},
}

// traceRiver walks D8 steepest-descent from (startX, startY) on the raw
// (blended + ridge) elevation field, handling depressions via a local
// priority-flood, and returns (path, lakes). Path is the sequence of land
// tiles the river occupies; lakes is the set of cells that were filled while
// resolving a depression along the way. Both are empty if the trace
// immediately terminates (head is below ocean, cycle, etc.). Deterministic:
// same (startX, startY) on the same generator always produces identical
// output.
func (r *NoiseRiverSource) traceRiver(startX, startY int) (path, lakes []TileCoord) {
	path = make([]TileCoord, 0, 64)
	lakes = make([]TileCoord, 0, 4)

	scratch := traceScratchPool.Get().(*traceScratch)
	visited := scratch.visited
	cache := scratch.cache
	clear(visited)
	clear(cache)
	defer traceScratchPool.Put(scratch)

	elevOf := func(x, y int) float64 {
		c := TileCoord{x, y}
		if v, ok := cache[c]; ok {
			return v
		}
		v := r.terrain.ElevationAtFloat(float64(x), float64(y))
		cache[c] = v
		return v
	}

	x, y := startX, startY
	for range riverMaxTraceLen {
		if elevOf(x, y) < biome.ElevationOcean {
			return path, lakes
		}
		cur := TileCoord{x, y}
		if _, seen := visited[cur]; seen {
			return path, lakes
		}
		visited[cur] = struct{}{}
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
func localFloodFill(sx, sy int, elevOf func(int, int) float64) (spillX, spillY int, basin []TileCoord, found bool) {
	scratch := floodScratchPool.Get().(*floodScratch)
	defer floodScratchPool.Put(scratch)
	scratch.pq.data = scratch.pq.data[:0]
	clear(scratch.processed)
	pq := &scratch.pq
	processed := scratch.processed

	seedElev := elevOf(sx, sy)
	pq.push(cellPri{x: sx, y: sy, elev: seedElev})
	processed[TileCoord{sx, sy}] = struct{}{}
	basin = append(basin, TileCoord{sx, sy})

	for pq.len() > 0 {
		if len(basin) > riverMaxBasinCells {
			return 0, 0, basin, false
		}
		c := pq.pop()
		for _, off := range squareNeighborOffsets {
			nx := c.x + int(off.dx)
			ny := c.y + int(off.dy)
			nc := TileCoord{nx, ny}
			if _, ok := processed[nc]; ok {
				continue
			}
			processed[nc] = struct{}{}

			ne := elevOf(nx, ny)
			if ne < c.elev {
				return nx, ny, basin, true
			}
			basin = append(basin, nc)
			pq.push(cellPri{x: nx, y: ny, elev: ne})
		}
	}
	return 0, 0, basin, false
}

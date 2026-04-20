package game

import (
	"container/heap"
	"container/list"
	"sort"
	"sync"
)

// Road generation constants. roadMaxEdgesPerPOI limits the k-nearest-neighbour graph
// built over POIs so that densely-packed settlements do not create a fully-connected
// clique. roadLoopFraction controls how many edges beyond the MST are added to create
// cycles — a pure tree looks unnaturally sparse. roadSearchBudget caps A* node
// expansions per edge so that a pair of POIs separated by a large impassable region
// (ocean, mountain wall) fails fast rather than exhausting memory.
const (
	roadMaxEdgesPerPOI = 3
	roadLoopFraction   = 0.2
	roadSearchBudget   = 2048
)

// roadCost returns the movement cost for a tile with the given terrain. A cost of zero
// signals that the terrain is impassable and the tile must not be traversed. River tiles
// get cost 2 regardless of their underlying biome (MVP bridge semantics: roads cross
// rivers at the cost of slightly rough ground).
func roadCost(t Terrain, river bool) int {
	if river {
		return 2
	}
	switch t {
	case TerrainPlains, TerrainGrassland, TerrainMeadow, TerrainSavanna, TerrainBeach:
		return 1
	case TerrainTundra, TerrainHills:
		return 2
	case TerrainForest, TerrainTaiga, TerrainJungle:
		return 3
	case TerrainDesert, TerrainSnow:
		return 4
	default:
		// Mountain, SnowyPeak, Ocean, DeepOcean, CursedForest — impassable.
		return 0
	}
}

// roadPOI is a world-space axial coordinate of a POI used during road graph construction.
type roadPOI struct{ q, r int }

// roadEdge connects two POIs (by index in a slice) with a hex-distance weight used to
// build the MST and the k-NN candidate list.
type roadEdge struct{ a, b int; dist int }

// roadAStarNode is an entry in the A* open set priority queue.
type roadAStarNode struct {
	q, r     int
	gCost    int // exact cost from the source
	fCost    int // gCost + heuristic
	index    int // position in the heap (required by container/heap)
}

// roadAStarHeap is a min-heap of *roadAStarNode ordered by fCost. The heap interface
// methods are defined on a pointer to the slice so Push/Pop can mutate it in place.
type roadAStarHeap []*roadAStarNode

func (h roadAStarHeap) Len() int            { return len(h) }
func (h roadAStarHeap) Less(i, j int) bool  { return h[i].fCost < h[j].fCost }
func (h roadAStarHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *roadAStarHeap) Push(x any) {
	n := x.(*roadAStarNode)
	n.index = len(*h)
	*h = append(*h, n)
}

func (h *roadAStarHeap) Pop() any {
	old := *h
	last := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return last
}

// roadAStar finds the cheapest path between two axial hex coordinates using A* with
// terrain-weighted movement costs. It returns the path as a slice of [q, r] coords
// (including both endpoints) or nil if no path was found within roadSearchBudget
// node expansions. The heuristic is hex-distance × 1 (minimum possible per-step cost),
// which is admissible and makes the search consistent.
func (g *WorldGenerator) roadAStar(srcQ, srcR, dstQ, dstR int) [][2]int {
	type key struct{ q, r int }

	open := make(roadAStarHeap, 0, 64)
	heap.Init(&open)

	gScore := make(map[key]int, 128)
	cameFrom := make(map[key]key, 128)

	start := key{srcQ, srcR}
	gScore[start] = 0
	heap.Push(&open, &roadAStarNode{
		q:     srcQ,
		r:     srcR,
		gCost: 0,
		fCost: hexDistance(srcQ, srcR, dstQ, dstR),
	})

	expanded := 0

	for open.Len() > 0 {
		if expanded >= roadSearchBudget {
			return nil
		}
		cur := heap.Pop(&open).(*roadAStarNode)
		expanded++

		if cur.q == dstQ && cur.r == dstR {
			// Reconstruct the path by following cameFrom back to the start.
			path := make([][2]int, 0, 32)
			k := key{dstQ, dstR}
			for k != start {
				path = append(path, [2]int{k.q, k.r})
				k = cameFrom[k]
			}
			path = append(path, [2]int{start.q, start.r})
			// Reverse so path runs from source to destination.
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}

		curKey := key{cur.q, cur.r}
		curG, ok := gScore[curKey]
		if !ok || cur.gCost > curG {
			// A stale entry — skip it.
			continue
		}

		for _, off := range hexNeighborOffsets {
			nq, nr := cur.q+off[0], cur.r+off[1]
			tile := g.TileAt(nq, nr)
			cost := roadCost(tile.Terrain, tile.River)
			if cost == 0 {
				continue // impassable
			}
			ng := curG + cost
			nk := key{nq, nr}
			if prev, exists := gScore[nk]; exists && ng >= prev {
				continue
			}
			gScore[nk] = ng
			cameFrom[nk] = curKey
			heap.Push(&open, &roadAStarNode{
				q:     nq,
				r:     nr,
				gCost: ng,
				fCost: ng + hexDistance(nq, nr, dstQ, dstR),
			})
		}
	}

	return nil // no path found within budget
}

// roadMSTPrim builds a minimum spanning tree over pois using Prim's algorithm with
// hex-distance edge weights. It returns the set of edges in the MST. The result is
// deterministic because we break ties by (a, b) index — no random element is
// introduced. Prim is O(V²) here which is fine because the POI count per super-chunk
// region is small (typically < 30).
func roadMSTPrim(pois []roadPOI) []roadEdge {
	n := len(pois)
	if n < 2 {
		return nil
	}

	inMST := make([]bool, n)
	// minEdge[i] holds the cheapest known edge connecting node i to the MST so far.
	type candidate struct {
		dist int
		from int
	}
	minEdge := make([]candidate, n)
	for i := range minEdge {
		minEdge[i] = candidate{dist: 1<<62, from: -1}
	}
	minEdge[0].dist = 0

	result := make([]roadEdge, 0, n-1)

	for iter := 0; iter < n; iter++ {
		// Pick the not-yet-in-MST node with the smallest connecting edge.
		// Stable tie-break by index keeps results deterministic.
		u := -1
		for v := 0; v < n; v++ {
			if !inMST[v] {
				if u == -1 || minEdge[v].dist < minEdge[u].dist ||
					(minEdge[v].dist == minEdge[u].dist && v < u) {
					u = v
				}
			}
		}
		inMST[u] = true
		if minEdge[u].from != -1 {
			a, b := minEdge[u].from, u
			if a > b {
				a, b = b, a
			}
			result = append(result, roadEdge{a: a, b: b, dist: minEdge[u].dist})
		}

		// Relax neighbours.
		pu := pois[u]
		for v := 0; v < n; v++ {
			if inMST[v] {
				continue
			}
			pv := pois[v]
			d := hexDistance(pu.q, pu.r, pv.q, pv.r)
			if d < minEdge[v].dist {
				minEdge[v] = candidate{dist: d, from: u}
			}
		}
	}

	return result
}

// roadCandidateEdges returns all pairwise edges between pois, each POI limited to its
// roadMaxEdgesPerPOI nearest neighbours. The returned slice is sorted by distance then
// by (a, b) for determinism.
func roadCandidateEdges(pois []roadPOI) []roadEdge {
	n := len(pois)
	edges := make([]roadEdge, 0, n*roadMaxEdgesPerPOI)
	seen := make(map[[2]int]struct{}, n*roadMaxEdgesPerPOI)

	for i := 0; i < n; i++ {
		// Collect distances from i to every other POI.
		type neighbour struct {
			j    int
			dist int
		}
		nbrs := make([]neighbour, 0, n-1)
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			nbrs = append(nbrs, neighbour{j: j, dist: hexDistance(pois[i].q, pois[i].r, pois[j].q, pois[j].r)})
		}
		sort.Slice(nbrs, func(x, y int) bool {
			if nbrs[x].dist != nbrs[y].dist {
				return nbrs[x].dist < nbrs[y].dist
			}
			return nbrs[x].j < nbrs[y].j
		})
		limit := roadMaxEdgesPerPOI
		if limit > len(nbrs) {
			limit = len(nbrs)
		}
		for _, nb := range nbrs[:limit] {
			a, b := i, nb.j
			if a > b {
				a, b = b, a
			}
			k := [2]int{a, b}
			if _, dup := seen[k]; dup {
				continue
			}
			seen[k] = struct{}{}
			edges = append(edges, roadEdge{a: a, b: b, dist: nb.dist})
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		if edges[i].dist != edges[j].dist {
			return edges[i].dist < edges[j].dist
		}
		if edges[i].a != edges[j].a {
			return edges[i].a < edges[j].a
		}
		return edges[i].b < edges[j].b
	})
	return edges
}

// roadSuperChunkCacheCapacity is the maximum number of super-chunk road results kept in
// memory per WorldGenerator. 256 super-chunks covers a very large explored area while
// bounding memory use — old entries are evicted LRU when the limit is exceeded.
const roadSuperChunkCacheCapacity = 256

// roadSuperChunkCacheEntry is the value stored in each list element of superChunkRoadCache.
type roadSuperChunkCacheEntry struct {
	key   SuperChunkCoord
	value map[[2]int]struct{}
}

// superChunkRoadCache is a bounded LRU cache keyed by SuperChunkCoord whose values are
// the road-tile sets computed by RoadTilesInSuperChunk. It has the same shape as
// ChunkCache: container/list + map + sync.Mutex.
type superChunkRoadCache struct {
	capacity int

	mu      sync.Mutex
	order   *list.List
	entries map[SuperChunkCoord]*list.Element
}

// newSuperChunkRoadCache builds a superChunkRoadCache with the given capacity.
func newSuperChunkRoadCache(capacity int) *superChunkRoadCache {
	return &superChunkRoadCache{
		capacity: capacity,
		order:    list.New(),
		entries:  make(map[SuperChunkCoord]*list.Element, capacity),
	}
}

// Get returns the cached road tile set for sc if present and promotes it to MRU.
func (c *superChunkRoadCache) Get(sc SuperChunkCoord) (map[[2]int]struct{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[sc]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*roadSuperChunkCacheEntry).value, true
}

// Put inserts or replaces the road tile set for sc, evicting the LRU entry if at capacity.
func (c *superChunkRoadCache) Put(sc SuperChunkCoord, tiles map[[2]int]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[sc]; ok {
		elem.Value.(*roadSuperChunkCacheEntry).value = tiles
		c.order.MoveToFront(elem)
		return
	}
	elem := c.order.PushFront(&roadSuperChunkCacheEntry{key: sc, value: tiles})
	c.entries[sc] = elem
	if c.order.Len() > c.capacity {
		tail := c.order.Back()
		if tail != nil {
			entry := tail.Value.(*roadSuperChunkCacheEntry)
			c.order.Remove(tail)
			delete(c.entries, entry.key)
		}
	}
}

// RoadTilesInSuperChunk returns the set of world-space axial coordinates that carry a
// road inside super-chunk sc. Coordinates outside sc.ChunkBounds() are excluded even if
// the A* path passes through them — each super-chunk only owns its own road tiles.
//
// The algorithm:
//  1. Gather POIs from sc plus a 1-super-chunk buffer ring so that roads reach across
//     super-chunk borders without dangling ends.
//  2. Build a k-NN candidate edge list and compute a minimum spanning tree (Prim).
//  3. Augment the MST with a fraction of non-MST edges to introduce loops.
//  4. For each kept edge, run A* on the hex grid with terrain-weighted costs.
//  5. Return only the coords that fall inside sc's axial bounding box.
func (g *WorldGenerator) RoadTilesInSuperChunk(sc SuperChunkCoord) map[[2]int]struct{} {
	if v, ok := g.roadCache.Get(sc); ok {
		return v
	}
	result := g.computeRoadTilesInSuperChunk(sc)
	g.roadCache.Put(sc, result)
	return result
}

// computeRoadTilesInSuperChunk is the uncached implementation called by
// RoadTilesInSuperChunk. Keeping the computation separate makes the memoisation
// wrapper easy to read and verify.
func (g *WorldGenerator) computeRoadTilesInSuperChunk(sc SuperChunkCoord) map[[2]int]struct{} {
	// Determine the axial bounds of sc so we know which road tiles to keep.
	minCX, maxCX, minCY, maxCY := sc.ChunkBounds()
	scMinQ := minCX * ChunkSize
	scMaxQ := maxCX * ChunkSize // exclusive
	scMinR := minCY * ChunkSize
	scMaxR := maxCY * ChunkSize // exclusive

	// Scan the 3×3 super-chunk neighbourhood (sc plus the 8 surrounding super-chunks)
	// and collect all POI world coordinates. Using a 1-super-chunk buffer ensures that
	// roads can connect from a POI just outside sc's boundary into sc itself.
	var pois []roadPOI

	for dsy := -1; dsy <= 1; dsy++ {
		for dsx := -1; dsx <= 1; dsx++ {
			nsc := SuperChunkCoord{X: sc.X + dsx, Y: sc.Y + dsy}
			nMinCX, nMaxCX, nMinCY, nMaxCY := nsc.ChunkBounds()
			for cy := nMinCY; cy < nMaxCY; cy++ {
				for cx := nMinCX; cx < nMaxCX; cx++ {
					cc := ChunkCoord{X: cx, Y: cy}
					objects := g.ObjectsInChunk(cc)
					chMinQ, _, chMinR, _ := cc.Bounds()
					for key := range objects {
						pois = append(pois, roadPOI{
							q: chMinQ + key[0],
							r: chMinR + key[1],
						})
					}
				}
			}
		}
	}

	if len(pois) < 2 {
		return map[[2]int]struct{}{}
	}

	// Deduplicate POIs (shouldn't happen normally, but be safe).
	sort.Slice(pois, func(i, j int) bool {
		if pois[i].q != pois[j].q {
			return pois[i].q < pois[j].q
		}
		return pois[i].r < pois[j].r
	})
	unique := pois[:1]
	for _, p := range pois[1:] {
		if p != unique[len(unique)-1] {
			unique = append(unique, p)
		}
	}
	pois = unique

	// Build candidate edges (k-NN graph) and compute MST.
	candidates := roadCandidateEdges(pois)
	mstEdges := roadMSTPrim(pois)

	// Build a set of MST edge keys so we can identify non-MST candidates.
	mstSet := make(map[[2]int]struct{}, len(mstEdges))
	for _, e := range mstEdges {
		mstSet[[2]int{e.a, e.b}] = struct{}{}
	}

	// Compute how many extra loop edges to add.
	extra := int(float64(len(mstEdges))*roadLoopFraction+0.9999) // ceil
	if extra < 0 {
		extra = 0
	}

	// Select extra non-MST edges from the sorted candidate list (already sorted by
	// distance, so we pick shortest first which is both greedy and deterministic).
	kept := make([]roadEdge, 0, len(mstEdges)+extra)
	kept = append(kept, mstEdges...)
	added := 0
	for _, e := range candidates {
		if added >= extra {
			break
		}
		k := [2]int{e.a, e.b}
		if _, inMST := mstSet[k]; inMST {
			continue
		}
		kept = append(kept, e)
		added++
	}

	// Run A* for each kept edge and collect road tiles that fall inside sc.
	result := make(map[[2]int]struct{})
	for _, e := range kept {
		src := pois[e.a]
		dst := pois[e.b]
		path := g.roadAStar(src.q, src.r, dst.q, dst.r)
		for _, coord := range path {
			q, r := coord[0], coord[1]
			if q >= scMinQ && q < scMaxQ && r >= scMinR && r < scMaxR {
				result[[2]int{q, r}] = struct{}{}
			}
		}
	}

	return result
}

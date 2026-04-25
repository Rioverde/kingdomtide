package worldgen

import (
	"math/rand/v2"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/voronoi"
)

// saltRiver decorrelates river-head selection from every other RNG
// stream so two worlds with the same elevation field still get
// different river layouts.
const saltRiver int64 = 0x1c8ebc3a42cdd0e9

// corner is one Voronoi vertex enriched with the data the river
// pipeline needs.
type corner struct {
	pos        voronoi.Vertex
	elev       float32
	downslope  int      // index into corners; -1 if local minimum
	neighbors  []int    // adjacent corner indices
	adjCells   []uint16 // unique cells touching this corner
	touchOcean bool     // true if any adjacent cell is ocean
}

// computeRivers traces rivers from random sources in the [0.30, 0.90]
// elevation band downslope to the coast and rasterises the resulting
// edges into a packed bitset. Volume on shared edges accumulates as
// branches merge — wider trunks downstream. Returns also the corner
// indices where rivers terminated at inland local minima — caller
// uses these to form lakes. Caller owns the corner graph; this
// function only consumes it.
func computeRivers(w *World, corners []corner, seed int64) (*bitset, []int) {
	if len(corners) == 0 {
		return newBitset(w.Width * w.Height), nil
	}
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^uint64(saltRiver)))
	heads := pickRiverHeads(corners, rng)
	edges, lakes := traceRivers(corners, heads)
	return rasterizeRivers(corners, edges, w.Width, w.Height), lakes
}

// applyRiverLakes turns every land cell adjacent to a river-terminated
// local minimum corner into a lake (TerrainOcean — our convention for
// any standing water body). The isOcean tracker is updated alongside
// so downstream features (watersheds) treat the new lakes as drainage
// barriers.
func applyRiverLakes(w *World, corners []corner, lakes []int, isOcean []bool) {
	for _, ci := range lakes {
		for _, adj := range corners[ci].adjCells {
			if isOcean[adj] {
				continue
			}
			w.Terrain[adj] = gworld.TerrainOcean
			isOcean[adj] = true
		}
	}
}

// buildCorners hydrates a per-vertex corner slice from the raster-
// based diagram. Each Vertex in the diagram becomes one corner;
// Edges feed the neighbour-and-adjacent-cell tables. Corner
// elevations are the mean of their adjacent cells' elevations.
//
// Voronoi corners are degree-3 (three cells meet at each vertex), so
// we preallocate cap=4 on every per-corner slice to land each
// corner's growth in a single tiny allocation instead of the
// 0→1→2→4 ramp that would dominate the buildCorners memory profile.
func buildCorners(w *World, isOcean []bool) []corner {
	d := w.Voronoi
	if d == nil || len(d.Vertices) == 0 {
		return nil
	}

	const cornerArity = 4
	corners := make([]corner, len(d.Vertices))
	for i, v := range d.Vertices {
		corners[i] = corner{
			pos:       v,
			downslope: -1,
			neighbors: make([]int, 0, cornerArity),
			adjCells:  make([]uint16, 0, cornerArity),
		}
	}

	for _, e := range d.Edges {
		a := int(e.Va)
		b := int(e.Vb)
		corners[a].neighbors = appendUnique(corners[a].neighbors, b)
		corners[b].neighbors = appendUnique(corners[b].neighbors, a)
		corners[a].adjCells = appendUniqueU16(corners[a].adjCells, e.CellL)
		corners[a].adjCells = appendUniqueU16(corners[a].adjCells, e.CellR)
		corners[b].adjCells = appendUniqueU16(corners[b].adjCells, e.CellL)
		corners[b].adjCells = appendUniqueU16(corners[b].adjCells, e.CellR)
	}

	for i := range corners {
		var sum float32
		for _, cid := range corners[i].adjCells {
			sum += w.Elevation[cid]
			if isOcean[cid] {
				corners[i].touchOcean = true
			}
		}
		if n := len(corners[i].adjCells); n > 0 {
			corners[i].elev = sum / float32(n)
		}
	}
	return corners
}

// assignDownslope sets each corner's downslope to the neighbour with
// the lowest elevation if and only if that neighbour sits strictly
// below the corner. Corners at local minima keep downslope = -1.
func assignDownslope(corners []corner) {
	for i := range corners {
		best := corners[i].elev
		bestN := -1
		for _, n := range corners[i].neighbors {
			if corners[n].elev < best {
				best = corners[n].elev
				bestN = n
			}
		}
		corners[i].downslope = bestN
	}
}

// pickRiverHeads samples corners whose elevation falls inside the
// [riverHeadElevMin, riverHeadElevMax] band, have a downslope, and do
// not already touch ocean. Sample size is a fraction of total corners.
func pickRiverHeads(corners []corner, rng *rand.Rand) []int {
	target := int(float64(len(corners)) * riverHeadFraction)
	if target < 1 {
		target = 1
	}
	heads := make([]int, 0, target)
	for attempts := 0; len(heads) < target && attempts < target*40; attempts++ {
		i := rng.IntN(len(corners))
		c := &corners[i]
		if c.elev < riverHeadElevMin || c.elev > riverHeadElevMax {
			continue
		}
		if c.downslope < 0 || c.touchOcean {
			continue
		}
		heads = append(heads, i)
	}
	return heads
}

// edgeKey is the canonical pair of corner indices for a river edge.
type edgeKey [2]int

// traceRivers follows each head's downslope chain to the coast,
// recording every edge crossed and accumulating per-edge volume.
// When two rivers share a downstream segment, the shared edges' volumes
// add — exactly as Patel's mapgen2 specifies, and the foundation for
// width-by-volume rasterisation. Returns also the set of corners where
// a river terminated at an inland local minimum — those become lakes.
// Loops are guarded by a step cap.
func traceRivers(corners []corner, heads []int) (map[edgeKey]int, []int) {
	edges := make(map[edgeKey]int)
	lakeSet := make(map[int]struct{})
	const maxSteps = 4000
	for _, head := range heads {
		cur := head
		for step := 0; step < maxSteps; step++ {
			next := corners[cur].downslope
			if next < 0 {
				if !corners[cur].touchOcean {
					lakeSet[cur] = struct{}{}
				}
				break
			}
			a, b := cur, next
			if a > b {
				a, b = b, a
			}
			edges[edgeKey{a, b}]++
			if corners[next].touchOcean {
				break
			}
			cur = next
		}
	}
	lakes := make([]int, 0, len(lakeSet))
	for k := range lakeSet {
		lakes = append(lakes, k)
	}
	return edges, lakes
}

// rasterizeRivers walks every river edge with Bresenham's line
// algorithm, marking each tile the line crosses. Edge width is
// proportional to sqrt(volume) per Patel's mapgen2 — small streams
// stay 1 tile wide, busy trunk rivers fan out to a 3×3 brush so the
// hierarchy reads at a glance. Returns a packed W*H bitset (8×
// smaller than []bool).
func rasterizeRivers(corners []corner, edges map[edgeKey]int, w, h int) *bitset {
	out := newBitset(w * h)
	brush := func(x, y, radius int) {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				px, py := x+dx, y+dy
				if px < 0 || py < 0 || px >= w || py >= h {
					continue
				}
				out.Set(py*w + px)
			}
		}
	}
	for k, vol := range edges {
		a := corners[k[0]].pos
		b := corners[k[1]].pos
		radius := riverBrushRadius(vol)
		plot := func(x, y int) { brush(x, y, radius) }
		bresenham(int(a.X), int(a.Y), int(b.X), int(b.Y), plot)
	}
	return out
}

// riverBrushRadius maps accumulated edge volume to a half-width brush
// radius. width ≈ sqrt(volume) per Patel, bucketed to integer radii so
// the raster produces clean 1, 3, or 5-tile-wide channels. Anything
// past the head's first confluence (vol ≥ 2) widens to 3 tiles so the
// hierarchy reads at a glance — pure 1-tile rivers everywhere are
// indistinguishable from terrain noise on the tile grid.
func riverBrushRadius(vol int) int {
	switch {
	case vol >= 6:
		return 2 // 5-tile trunk
	case vol >= 2:
		return 1 // 3-tile mid-river
	default:
		return 0 // 1-tile stream / source
	}
}

// bresenham draws a 1-pixel-wide line between two integer endpoints,
// invoking plot for every tile crossed.
func bresenham(x0, y0, x1, y1 int, plot func(x, y int)) {
	dx := absInt(x1 - x0)
	dy := -absInt(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		plot(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// appendUnique appends v to s only if it is not already present.
func appendUnique(s []int, v int) []int {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// appendUniqueU16 is the uint16 specialisation.
func appendUniqueU16(s []uint16, v uint16) []uint16 {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

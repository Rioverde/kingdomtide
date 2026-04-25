// Package voronoi builds a raster-based Voronoi diagram for the
// mapgen2-style worldgen pipeline. Sites are placed by bucket-
// accelerated rejection sampling, then nearest-site rasterisation
// produces a per-tile cell-membership grid (CellID). Lloyd's
// relaxation moves each site to the centroid of its rasterised
// region; cell adjacency is read off the raster boundaries; corners
// (Voronoi vertices) are detected by scanning 2×2 tile windows for
// 3+ distinct cell IDs; edges connect corners that share a pair of
// cells.
//
// We do NOT run Fortune's sweepline or compute exact polygon
// geometry. Every downstream consumer in this game writes to a tile
// grid anyway, so integer raster corners feed Bresenham river
// rasterisation and downslope corner graphs as cleanly as float
// vertices would. Skipping Fortune's gets us out of pzsz/voronoi's
// 4M+ small allocations per build, which used to dominate GC.
package voronoi

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"
	"time"
)

// SubStageHook is called after each sub-stage of Generate when
// non-nil. Used by diagnostic tooling to time individual passes
// inside the Voronoi build (placeSeeds, Lloyd's, rasterise…).
var SubStageHook func(stage string, dur time.Duration)

func substage(stage string, t0 time.Time) {
	if SubStageHook != nil {
		SubStageHook(stage, time.Since(t0))
	}
}

// Vertex is a 2D point — a site, a centroid, or a polygon corner.
// We keep it in float64 so site coordinates carry the post-Lloyd
// fractional centroid; Vertex values used as polygon corners come
// out integer-valued (raster-derived) but we still store as float64
// for uniform interop with site math.
type Vertex struct {
	X, Y float64
}

// Cell is a Voronoi region: its site, its centre coordinate (post
// Lloyd's the site IS the raster centroid), and the list of cell
// IDs whose regions it borders.
type Cell struct {
	ID        uint16
	CenterX   float64
	CenterY   float64
	Neighbors []uint16
}

// Edge separates two adjacent cells; its two endpoints are corner
// indices into Diagram.Vertices.
type Edge struct {
	Va    uint32
	Vb    uint32
	CellL uint16
	CellR uint16
}

// Diagram is the complete raster-based Voronoi layout.
type Diagram struct {
	W, H   int
	Cells  []Cell
	CellID []uint16 // W*H, per-tile cell assignment

	// Vertices are polygon corners — points where 3+ cells meet.
	// Integer raster positions stored as float64 for math interop.
	Vertices []Vertex

	// Edges connect adjacent vertices; each edge knows its two
	// flanking cells.
	Edges []Edge

	// borderCells flags cells touching the outer border (any tile in
	// row 0, row H-1, col 0, col W-1). O(1) lookup via TouchesEdge.
	borderCells []bool
}

const saltSeeds int64 = 0x1c8ebc3a42cdd0e9

// Generate builds a diagram with the requested cell count. Lloyd's
// relaxation runs lloydIterations passes, each one re-rasterising
// from updated raster centroids. The trailing float64 parameter is
// kept for API compatibility with the previous pzsz wrapper but is
// no longer used.
func Generate(seed int64, w, h, cellCount, lloydIterations int, _ float64) *Diagram {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^uint64(saltSeeds)))

	t0 := time.Now()
	sites := placeSeeds(rng, w, h, cellCount)
	substage("place_seeds", t0)

	t0 = time.Now()
	cellID := rasterizeNearest(w, h, sites)
	substage("rasterize_initial", t0)

	t0 = time.Now()
	for iter := 0; iter < lloydIterations; iter++ {
		sites = computeRasterCentroids(cellID, sites, w, h)
		cellID = rasterizeNearest(w, h, sites)
	}
	substage("lloyd_relax", t0)

	t0 = time.Now()
	cells := make([]Cell, len(sites))
	for i, s := range sites {
		cells[i] = Cell{
			ID:      uint16(i),
			CenterX: s.X,
			CenterY: s.Y,
		}
	}
	buildAdjacency(cells, cellID, w, h)
	substage("build_cells", t0)

	t0 = time.Now()
	vertices, vertCells := findCorners(cellID, w, h)
	edges := buildEdges(vertCells)
	substage("corners_edges", t0)

	border := computeBorderCells(cellID, w, h, len(cells))

	return &Diagram{
		W:           w,
		H:           h,
		Cells:       cells,
		CellID:      cellID,
		Vertices:    vertices,
		Edges:       edges,
		borderCells: border,
	}
}

// computeBorderCells walks the four outer rows/columns of the
// rasterised CellID grid once and flags every cell ID seen. Replaces
// the per-call O(W+H) TouchesEdge scan with an O(1) lookup.
func computeBorderCells(cellID []uint16, w, h, cellCount int) []bool {
	out := make([]bool, cellCount)
	for x := 0; x < w; x++ {
		out[cellID[x]] = true
		out[cellID[(h-1)*w+x]] = true
	}
	for y := 0; y < h; y++ {
		out[cellID[y*w]] = true
		out[cellID[y*w+w-1]] = true
	}
	return out
}

// RefreshBorderCells rebuilds the border-cell index against the
// current CellID array. Worldgen calls this after applyNoisyEdges
// because the warp can change which cells own border tiles.
func (d *Diagram) RefreshBorderCells() {
	d.borderCells = computeBorderCells(d.CellID, d.W, d.H, len(d.Cells))
}

// CellAt returns the cell at a tile.
func (d *Diagram) CellAt(x, y int) *Cell {
	return &d.Cells[d.CellID[y*d.W+x]]
}

// CellIDAt returns the cell ID at a tile.
func (d *Diagram) CellIDAt(x, y int) uint16 {
	return d.CellID[y*d.W+x]
}

// TouchesEdge reports whether any tile of the cell lies on the outer
// border of the diagram. O(1) lookup against the precomputed border
// cell set built in Generate.
func (d *Diagram) TouchesEdge(cellID uint16) bool {
	if int(cellID) >= len(d.borderCells) {
		return false
	}
	return d.borderCells[cellID]
}

// placeSeeds scatters count points via rejection sampling with a
// minimum inter-point distance. Lloyd's relaxation downstream smooths
// the layout; placeSeeds just seeds a reasonable starting
// configuration.
//
// The per-attempt neighbour check is bucket-accelerated: every placed
// seed lives in a uniform grid whose cell side is minDist/√2 (so each
// grid cell holds at most one seed). A candidate's neighbour test
// scans a 5×5 grid neighbourhood — O(1) — instead of the entire seed
// list.
func placeSeeds(rng *rand.Rand, w, h, count int) []Vertex {
	if count < 1 {
		return nil
	}
	minDist := math.Sqrt(float64(w*h)/float64(count)) * 0.65
	minDist2 := minDist * minDist
	margin := minDist * 0.5

	cellSize := minDist / math.Sqrt2
	cols := int(math.Ceil(float64(w) / cellSize))
	rows := int(math.Ceil(float64(h) / cellSize))
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	grid := make([]int32, cols*rows)
	for i := range grid {
		grid[i] = -1
	}

	seeds := make([]Vertex, 0, count)
	for i := 0; i < count; i++ {
		placed := false
		for attempts := 0; attempts < 2000; attempts++ {
			x := margin + rng.Float64()*(float64(w)-2*margin)
			y := margin + rng.Float64()*(float64(h)-2*margin)

			c := int(x / cellSize)
			r := int(y / cellSize)
			ok := true
			for dr := -2; dr <= 2 && ok; dr++ {
				rr := r + dr
				if rr < 0 || rr >= rows {
					continue
				}
				for dc := -2; dc <= 2 && ok; dc++ {
					cc := c + dc
					if cc < 0 || cc >= cols {
						continue
					}
					idx := grid[rr*cols+cc]
					if idx < 0 {
						continue
					}
					s := seeds[idx]
					dx := s.X - x
					dy := s.Y - y
					if dx*dx+dy*dy < minDist2 {
						ok = false
					}
				}
			}
			if ok {
				idx := int32(len(seeds))
				seeds = append(seeds, Vertex{X: x, Y: y})
				if c >= 0 && c < cols && r >= 0 && r < rows {
					grid[r*cols+c] = idx
				}
				placed = true
				break
			}
		}
		if !placed {
			break
		}
	}
	return seeds
}

// computeRasterCentroids returns a fresh site slice where each entry
// is the centroid of its cell's tiles in the raster — the average
// (x, y) of every tile carrying that cell's ID. Drives Lloyd's
// relaxation directly off the rasterised geometry.
//
// Parallelised over row bands; per-worker accumulators are merged at
// the end so the centroid math stays lock-free.
func computeRasterCentroids(cellID []uint16, sites []Vertex, w, h int) []Vertex {
	n := len(sites)
	if n == 0 {
		return sites
	}

	workers := runtime.GOMAXPROCS(0)
	band := (h + workers - 1) / workers
	type acc struct {
		sumX  []float64
		sumY  []float64
		count []uint32
	}
	parts := make([]acc, workers)
	for i := range parts {
		parts[i] = acc{
			sumX:  make([]float64, n),
			sumY:  make([]float64, n),
			count: make([]uint32, n),
		}
	}

	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		yLo := worker * band
		yHi := yLo + band
		if yHi > h {
			yHi = h
		}
		if yLo >= yHi {
			continue
		}
		p := &parts[worker]
		wg.Add(1)
		go func(yLo, yHi int, p *acc) {
			defer wg.Done()
			for y := yLo; y < yHi; y++ {
				fy := float64(y)
				row := y * w
				for x := 0; x < w; x++ {
					c := cellID[row+x]
					p.sumX[c] += float64(x)
					p.sumY[c] += fy
					p.count[c]++
				}
			}
		}(yLo, yHi, p)
	}
	wg.Wait()

	out := make([]Vertex, n)
	copy(out, sites)
	for i := 0; i < n; i++ {
		var sx, sy float64
		var c uint32
		for w := range parts {
			sx += parts[w].sumX[i]
			sy += parts[w].sumY[i]
			c += parts[w].count[i]
		}
		if c > 0 {
			out[i] = Vertex{X: sx / float64(c), Y: sy / float64(c)}
		}
	}
	return out
}

// buildAdjacency reads the raster row-by-row and registers an
// adjacency between any two cells that share a tile boundary
// (horizontal or vertical). Each unordered pair is added at most
// once; cells receive a neighbour list of cell IDs in arbitrary
// order. O(W·H) plus O(N·avg-neighbours) inserts.
func buildAdjacency(cells []Cell, cellID []uint16, w, h int) {
	seen := make(map[uint32]struct{}, len(cells)*6)
	for y := 0; y < h; y++ {
		row := y * w
		for x := 0; x < w; x++ {
			c := cellID[row+x]
			if x+1 < w {
				n := cellID[row+x+1]
				if c != n {
					addAdjacency(cells, seen, c, n)
				}
			}
			if y+1 < h {
				n := cellID[row+w+x]
				if c != n {
					addAdjacency(cells, seen, c, n)
				}
			}
		}
	}
}

func addAdjacency(cells []Cell, seen map[uint32]struct{}, a, b uint16) {
	if a > b {
		a, b = b, a
	}
	key := uint32(a)<<16 | uint32(b)
	if _, dup := seen[key]; dup {
		return
	}
	seen[key] = struct{}{}
	cells[a].Neighbors = append(cells[a].Neighbors, b)
	cells[b].Neighbors = append(cells[b].Neighbors, a)
}

// findCorners scans every 2×2 tile window in the raster. A window
// holding 3+ distinct cell IDs marks a Voronoi corner: the place
// where three (or four) cells meet. The corner's position is
// (x+0.5, y+0.5) — the centre of the 2×2 window. Returns the corner
// vertices and, for each corner, the deduplicated list of cells
// touching it (≤4, typically 3).
func findCorners(cellID []uint16, w, h int) ([]Vertex, [][]uint16) {
	if w < 2 || h < 2 {
		return nil, nil
	}
	// Pre-size to ~2× cell count heuristic; corners count typically
	// runs around 2× sites for a Voronoi diagram.
	hint := (w * h) / 100
	verts := make([]Vertex, 0, hint)
	cellsPerVert := make([][]uint16, 0, hint)

	for y := 0; y < h-1; y++ {
		row := y * w
		nextRow := row + w
		for x := 0; x < w-1; x++ {
			a := cellID[row+x]
			b := cellID[row+x+1]
			c := cellID[nextRow+x]
			d := cellID[nextRow+x+1]
			// Fast reject: all four equal.
			if a == b && a == c && a == d {
				continue
			}
			// Dedup the four IDs into a stack-allocated 4-slot array.
			var arr [4]uint16
			n := 1
			arr[0] = a
			if b != arr[0] {
				arr[n] = b
				n++
			}
			cIn := false
			for i := 0; i < n; i++ {
				if arr[i] == c {
					cIn = true
					break
				}
			}
			if !cIn {
				arr[n] = c
				n++
			}
			dIn := false
			for i := 0; i < n; i++ {
				if arr[i] == d {
					dIn = true
					break
				}
			}
			if !dIn {
				arr[n] = d
				n++
			}
			if n < 3 {
				continue
			}
			// 3+ distinct cells around this 2×2 — corner.
			verts = append(verts, Vertex{X: float64(x) + 0.5, Y: float64(y) + 0.5})
			adj := make([]uint16, n)
			copy(adj, arr[:n])
			cellsPerVert = append(cellsPerVert, adj)
		}
	}
	return verts, cellsPerVert
}

// buildEdges constructs the edge list from per-corner cell adjacency.
// Two corners that both touch the same pair of cells (a, b) lie at
// the endpoints of the (a, b) Voronoi edge. We bucket vertices by
// cell-pair, then connect them.
//
// Most pairs see exactly 2 vertices (a single edge between two
// cells). Boundary cells whose edge runs into the map border show
// only one vertex — we skip those (the open boundary). Raster noise
// can occasionally produce 3+ vertices for a pair; we connect every
// vertex to a chosen anchor so the corner graph stays connected
// without duplicate worry (rivers/watersheds dedup neighbours
// downstream).
func buildEdges(vertCells [][]uint16) []Edge {
	type pair struct {
		a, b uint16
	}
	pairToVerts := make(map[pair][]uint32, len(vertCells))
	for vi, cells := range vertCells {
		for i := 0; i < len(cells); i++ {
			for j := i + 1; j < len(cells); j++ {
				a, b := cells[i], cells[j]
				if a > b {
					a, b = b, a
				}
				k := pair{a, b}
				pairToVerts[k] = append(pairToVerts[k], uint32(vi))
			}
		}
	}

	edges := make([]Edge, 0, len(pairToVerts))
	for k, vs := range pairToVerts {
		if len(vs) < 2 {
			continue
		}
		anchor := vs[0]
		for i := 1; i < len(vs); i++ {
			edges = append(edges, Edge{
				Va:    anchor,
				Vb:    vs[i],
				CellL: k.a,
				CellR: k.b,
			})
		}
	}
	return edges
}

// siteGrid is a uniform spatial hash of sites used by rasterizeNearest
// for O(1) nearest-neighbour queries. A Lloyd-relaxed layout never
// places a site farther than ~2×bucket from any tile, so a fixed
// 5×5 bucket sweep is correct without the expanding-ring complexity.
type siteGrid struct {
	bucketSize float64
	cols, rows int
	buckets    [][]int
}

// newSiteGrid buckets sites into a grid whose cell side is the
// average cell side of the requested layout — typically one site
// per bucket.
func newSiteGrid(sites []Vertex, w, h int) siteGrid {
	bucketSize := math.Sqrt(float64(w*h) / float64(len(sites)))
	if bucketSize < 4 {
		bucketSize = 4
	}
	cols := max(1, int(math.Ceil(float64(w)/bucketSize)))
	rows := max(1, int(math.Ceil(float64(h)/bucketSize)))
	buckets := make([][]int, cols*rows)
	for i, s := range sites {
		c := clampInt(int(s.X/bucketSize), 0, cols-1)
		r := clampInt(int(s.Y/bucketSize), 0, rows-1)
		buckets[r*cols+c] = append(buckets[r*cols+c], i)
	}
	return siteGrid{bucketSize: bucketSize, cols: cols, rows: rows, buckets: buckets}
}

// nearest returns the site index closest to (fx, fy). Searches the
// home bucket first, then expands one ring at a time until the next
// ring's minimum possible distance exceeds the current best — for a
// Lloyd-relaxed layout this almost always terminates after the 3×3
// home ring (9 buckets, ~9 sites checked).
func (g *siteGrid) nearest(sites []Vertex, fx, fy float64) uint16 {
	cTile := clampInt(int(fx/g.bucketSize), 0, g.cols-1)
	rTile := clampInt(int(fy/g.bucketSize), 0, g.rows-1)

	bestDist := math.MaxFloat64
	bestID := uint16(0)
	for ring := 0; ; ring++ {
		bestDist, bestID = g.scanRing(sites, fx, fy, cTile, rTile, ring, bestDist, bestID)
		if bestDist == math.MaxFloat64 {
			if !g.hasMoreRings(cTile, rTile, ring) {
				return bestID
			}
			continue
		}
		// Any unsearched bucket is at least `ring*bucketSize` away
		// (worst case: query at corner of its bucket). If our current
		// best beats that bound, we're done.
		margin := float64(ring) * g.bucketSize
		if margin*margin > bestDist {
			return bestID
		}
		if !g.hasMoreRings(cTile, rTile, ring) {
			return bestID
		}
	}
}

// scanRing visits every bucket at chebyshev distance == ring from
// (cTile, rTile) and updates (bestDist, bestID) against the sites it
// holds. Returns the new best.
func (g *siteGrid) scanRing(sites []Vertex, fx, fy float64, cTile, rTile, ring int, bestDist float64, bestID uint16) (float64, uint16) {
	rLo := rTile - ring
	rHi := rTile + ring
	cLo := cTile - ring
	cHi := cTile + ring
	for r := rLo; r <= rHi; r++ {
		if r < 0 || r >= g.rows {
			continue
		}
		base := r * g.cols
		// Only the boundary cells of the ring need checking — the
		// interior was searched on previous iterations.
		ringEdge := r == rLo || r == rHi
		for c := cLo; c <= cHi; c++ {
			if c < 0 || c >= g.cols {
				continue
			}
			if !ringEdge && c != cLo && c != cHi {
				continue
			}
			for _, i := range g.buckets[base+c] {
				s := sites[i]
				dx := s.X - fx
				dy := s.Y - fy
				d := dx*dx + dy*dy
				if d < bestDist {
					bestDist = d
					bestID = uint16(i)
				}
			}
		}
	}
	return bestDist, bestID
}

// hasMoreRings reports whether the next ring contains any in-bounds
// bucket — false means we've covered the whole grid.
func (g *siteGrid) hasMoreRings(cTile, rTile, ring int) bool {
	next := ring + 1
	return rTile-next >= 0 || rTile+next < g.rows ||
		cTile-next >= 0 || cTile+next < g.cols
}

// rasterizeNearest assigns every tile to its nearest seed using a
// uniform spatial grid. Per-tile cost is bounded — 25 buckets × ~1
// site each — collapsing the naive O(W·H·N) work to O(W·H · const).
// Parallelised over row bands; the grid is read-only after build.
func rasterizeNearest(w, h int, sites []Vertex) []uint16 {
	out := make([]uint16, w*h)
	if len(sites) == 0 {
		return out
	}
	g := newSiteGrid(sites, w, h)
	parallelRows(h, func(yLo, yHi int) {
		for y := yLo; y < yHi; y++ {
			fy := float64(y)
			row := y * w
			for x := 0; x < w; x++ {
				out[row+x] = g.nearest(sites, float64(x), fy)
			}
		}
	})
	return out
}

// parallelRows splits [0, rows) across GOMAXPROCS workers and calls
// fn(yLo, yHi) per worker, blocking until all complete.
func parallelRows(rows int, fn func(yLo, yHi int)) {
	workers := runtime.GOMAXPROCS(0)
	band := (rows + workers - 1) / workers
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		yLo := worker * band
		yHi := min(yLo+band, rows)
		if yLo >= yHi {
			continue
		}
		wg.Add(1)
		go func(yLo, yHi int) {
			defer wg.Done()
			fn(yLo, yHi)
		}(yLo, yHi)
	}
	wg.Wait()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Package voronoi builds the Voronoi diagram that the mapgen2-style
// worldgen runs on. Underneath it uses github.com/pzsz/voronoi —
// Fortune's algorithm in Go — which gives us exact polygon geometry
// (cells, halfedges, edges, corners) for the downstream passes that
// need it (noisy edges, rivers along polygon boundaries, proper
// cell adjacency). A raster layer (CellID slice) is still computed
// via nearest-seed so per-tile queries stay O(1).
package voronoi

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"

	pzsz "github.com/pzsz/voronoi"
)

// Cell is our lightweight wrapper over pzsz's Cell. We store only the
// data the mapgen pipeline actually reads; the full polygon structure
// is kept on Diagram.PZSZ for passes that need edges and halfedges.
type Cell struct {
	ID        uint16
	CenterX   float64
	CenterY   float64
	Neighbors []uint16
}

// Diagram is the rasterised Voronoi layout: per-tile cell assignment
// plus the cell list. PZSZ holds the original polygon structure for
// downstream geometry-driven passes.
type Diagram struct {
	W, H   int
	Cells  []Cell
	CellID []uint16 // W*H, per-tile cell assignment

	// PZSZ is the full Fortune's-algorithm diagram. Exposed so
	// later stages (noisy edges, rivers along polygon boundaries)
	// can walk the cells' halfedges and polygon corners.
	PZSZ *pzsz.Diagram
}

const (
	saltSeeds int64 = 0x1c8ebc3a42cdd0e9
)

// Generate builds a diagram with the requested cell count. Lloyd's
// relaxation smooths the site distribution into uniform blue noise
// across lloydIterations passes (2-3 is a good default). The warp
// parameter is kept for API compatibility but no longer used — with
// proper polygon geometry, organic edges come from a dedicated
// noisy-edges pass, not from domain-warping the rasteriser.
func Generate(seed int64, w, h, cellCount, lloydIterations int, _ float64) *Diagram {
	rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed)^uint64(saltSeeds)))

	sites := placeSeeds(rng, w, h, cellCount)
	bbox := pzsz.NewBBox(0, float64(w), 0, float64(h))

	// Lloyd's relaxation — recompute diagram, move each seed to its
	// polygon centroid, repeat.
	for iter := 0; iter < lloydIterations; iter++ {
		d := pzsz.ComputeDiagram(sites, bbox, true)
		sites = polygonCentroids(d, sites)
	}

	// Final diagram with polygon geometry closed.
	d := pzsz.ComputeDiagram(sites, bbox, true)

	// Extract sites in pzsz's cell order so our Cell slice indexes
	// line up with PZSZ.Cells[i].
	finalSites := make([]pzsz.Vertex, len(d.Cells))
	for i, c := range d.Cells {
		finalSites[i] = c.Site
	}

	cells := make([]Cell, len(d.Cells))
	siteToID := make(map[pzsz.Vertex]uint16, len(d.Cells))
	for i, c := range d.Cells {
		cells[i] = Cell{
			ID:      uint16(i),
			CenterX: c.Site.X,
			CenterY: c.Site.Y,
		}
		siteToID[c.Site] = uint16(i)
	}

	// Neighbours from the actual Voronoi edges — each edge borders
	// exactly two cells, so the edge list gives us the adjacency
	// graph without scanning tiles.
	for _, e := range d.Edges {
		if e.LeftCell == nil || e.RightCell == nil {
			continue
		}
		a := siteToID[e.LeftCell.Site]
		b := siteToID[e.RightCell.Site]
		cells[a].Neighbors = append(cells[a].Neighbors, b)
		cells[b].Neighbors = append(cells[b].Neighbors, a)
	}

	cellID := rasterizeNearest(w, h, finalSites)

	return &Diagram{
		W:      w,
		H:      h,
		Cells:  cells,
		CellID: cellID,
		PZSZ:   d,
	}
}

// CellAt returns the cell at a tile.
func (d *Diagram) CellAt(x, y int) *Cell {
	return &d.Cells[d.CellID[y*d.W+x]]
}

// CellIDAt returns the cell ID at a tile.
func (d *Diagram) CellIDAt(x, y int) uint16 {
	return d.CellID[y*d.W+x]
}

// placeSeeds scatters count points via rejection sampling with a
// minimum inter-point distance. Lloyd's relaxation downstream
// smooths the layout; placeSeeds just seeds a reasonable starting
// configuration.
func placeSeeds(rng *rand.Rand, w, h, count int) []pzsz.Vertex {
	if count < 1 {
		return nil
	}
	minDist := math.Sqrt(float64(w*h)/float64(count)) * 0.65
	minDist2 := minDist * minDist
	margin := minDist * 0.5

	seeds := make([]pzsz.Vertex, 0, count)
	for i := 0; i < count; i++ {
		placed := false
		for attempts := 0; attempts < 2000; attempts++ {
			x := margin + rng.Float64()*(float64(w)-2*margin)
			y := margin + rng.Float64()*(float64(h)-2*margin)
			ok := true
			for _, s := range seeds {
				dx := s.X - x
				dy := s.Y - y
				if dx*dx+dy*dy < minDist2 {
					ok = false
					break
				}
			}
			if ok {
				seeds = append(seeds, pzsz.Vertex{X: x, Y: y})
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

// polygonCentroids walks every cell in the diagram and computes the
// mean of its polygon vertices. A Lloyd's-relaxation iteration moves
// each site to its centroid, pulling seeds into blue-noise uniform
// spacing after a few rounds.
//
// Returns a fresh slice of sites in the same order as the input so
// the next ComputeDiagram call produces a diagram whose cell order
// aligns.
func polygonCentroids(d *pzsz.Diagram, prev []pzsz.Vertex) []pzsz.Vertex {
	// Map pzsz cell's site to a position in the caller's site slice
	// so we can return centroids in the caller's order.
	indexBySite := make(map[pzsz.Vertex]int, len(prev))
	for i, s := range prev {
		indexBySite[s] = i
	}
	out := make([]pzsz.Vertex, len(prev))
	copy(out, prev)

	for _, c := range d.Cells {
		i, ok := indexBySite[c.Site]
		if !ok {
			continue
		}
		var sx, sy float64
		n := 0
		for _, he := range c.Halfedges {
			sx += he.Edge.Va.X
			sy += he.Edge.Va.Y
			n++
		}
		if n > 0 {
			out[i] = pzsz.Vertex{X: sx / float64(n), Y: sy / float64(n)}
		}
	}
	return out
}

// rasterizeNearest assigns every tile to its nearest seed. O(W*H*N)
// naive but parallelised over row bands — fast enough for the cell
// counts we use (~1500 on Standard).
func rasterizeNearest(w, h int, sites []pzsz.Vertex) []uint16 {
	out := make([]uint16, w*h)
	workers := runtime.GOMAXPROCS(0)
	band := (h + workers - 1) / workers
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
		wg.Add(1)
		go func(yLo, yHi int) {
			defer wg.Done()
			for y := yLo; y < yHi; y++ {
				fy := float64(y)
				for x := 0; x < w; x++ {
					fx := float64(x)
					bestDist := math.MaxFloat64
					bestID := uint16(0)
					for i, s := range sites {
						dx := s.X - fx
						dy := s.Y - fy
						d := dx*dx + dy*dy
						if d < bestDist {
							bestDist = d
							bestID = uint16(i)
						}
					}
					out[y*w+x] = bestID
				}
			}
		}(yLo, yHi)
	}
	wg.Wait()
	return out
}

// TouchesEdge reports whether any tile of the cell lies on the outer
// border of the diagram.
func (d *Diagram) TouchesEdge(cellID uint16) bool {
	w, h := d.W, d.H
	for x := 0; x < w; x++ {
		if d.CellID[x] == cellID || d.CellID[(h-1)*w+x] == cellID {
			return true
		}
	}
	for y := 0; y < h; y++ {
		if d.CellID[y*w] == cellID || d.CellID[y*w+w-1] == cellID {
			return true
		}
	}
	return false
}

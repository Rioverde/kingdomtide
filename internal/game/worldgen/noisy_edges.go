package worldgen

import (
	"runtime"
	"sync"

	opensimplex "github.com/ojrac/opensimplex-go"
)

// saltNoisyEdgesX / saltNoisyEdgesY decorrelate the two perpendicular
// displacement noise fields so the warp vector covers the full 2D
// plane rather than collapsing to a diagonal.
const (
	saltNoisyEdgesX int64 = 0x4f9c2c8b1e5d7a93
	saltNoisyEdgesY int64 = 0x1b2a3c5e7d8f1062
)

// applyNoisyEdges rebakes the per-tile CellID array with a low-
// frequency noise warp applied to the lookup. Patel's mapgen2 makes
// boundaries organic via recursive midpoint displacement — a vector
// technique that emits wavy polylines per Voronoi edge. On a tile
// grid the same visual outcome is reached more directly: shift each
// tile's cell-membership query by a smooth 2D noise vector. Cells
// reclaim slivers of their neighbour's territory in wavy patterns,
// producing organic coastlines and biome transitions while the
// underlying cell graph (centres, neighbours, polygon corners) stays
// untouched and downstream lookups remain O(1).
//
// Parallelised across rows since each output tile is independent
// (read-only on src, write to disjoint dst slots).
func applyNoisyEdges(w *World, seed int64) {
	nx := opensimplex.New(seed ^ saltNoisyEdgesX)
	ny := opensimplex.New(seed ^ saltNoisyEdgesY)

	src := w.Voronoi.CellID
	dst := make([]uint16, len(src))
	width := w.Width
	height := w.Height

	workers := runtime.GOMAXPROCS(0)
	band := (height + workers - 1) / workers
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		yLo := worker * band
		yHi := yLo + band
		if yHi > height {
			yHi = height
		}
		if yLo >= yHi {
			continue
		}
		wg.Add(1)
		go func(yLo, yHi int) {
			defer wg.Done()
			for y := yLo; y < yHi; y++ {
				fy := float64(y) * noisyEdgesFreq
				for x := 0; x < width; x++ {
					fx := float64(x) * noisyEdgesFreq
					dx := nx.Eval2(fx, fy) * noisyEdgesAmplitude
					dy := ny.Eval2(fx, fy) * noisyEdgesAmplitude
					sx := x + int(dx)
					sy := y + int(dy)
					if sx < 0 {
						sx = 0
					} else if sx >= width {
						sx = width - 1
					}
					if sy < 0 {
						sy = 0
					} else if sy >= height {
						sy = height - 1
					}
					dst[y*width+x] = src[sy*width+sx]
				}
			}
		}(yLo, yHi)
	}
	wg.Wait()
	w.Voronoi.CellID = dst
	w.Voronoi.RefreshBorderCells()
}

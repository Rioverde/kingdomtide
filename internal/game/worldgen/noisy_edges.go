package worldgen

import (
	"math"
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

// applyNoisyEdges rebakes the per-tile CellID array with a multi-
// octave fBm noise warp applied to the lookup. Patel's mapgen2 makes
// boundaries organic via recursive midpoint displacement; the same
// visual outcome on a tile grid is reached by shifting each tile's
// cell-membership query by a smooth 2D noise vector.
//
// Performance — hybrid coarse/per-tile sampling:
//
//   - LOW octaves (long wavelength, big organic curves) are sampled
//     once per coarse-grid cell (1 per noisyEdgesCoarseFactor² tiles)
//     and bilinearly interpolated at every tile. fBm is smooth at
//     small scales so this is visually lossless.
//   - HIGH octaves (short wavelength, pixel-scale jitter) ARE
//     evaluated per tile. Without them, bilinear interp on the
//     coarse grid produces visible 8×8 blocky regions where every
//     tile reads the same source — the cell polygons re-emerge as
//     blocks. Per-tile high-octave noise scatters individual tiles
//     across boundaries, dissolving those blocks.
//
// Total noise evals per Standard build:
//   - Old all-per-tile (4 oct × 2 axes × 2.6M tiles) = 21M
//   - All-coarse (lost pixel jitter, polygons returned) = 0.3M
//   - Hybrid (2 coarse + 2 per-tile) = 10.4M ≈ half the original cost
//     while keeping full visual quality.
//
// Parallelised across rows; the coarse grid is read-only after build.
func applyNoisyEdges(w *World, seed int64) {
	nx := opensimplex.New(seed ^ saltNoisyEdgesX)
	ny := opensimplex.New(seed ^ saltNoisyEdgesY)

	src := w.Voronoi.CellID
	dst := make([]uint32, len(src))
	width := w.Width
	height := w.Height
	cf := noisyEdgesCoarseFactor
	coarseOct := noisyEdgesCoarseOctaves
	totalOct := noisyEdgesOctaves

	// Dynamic amplitude and base frequency, both keyed off the
	// average cell side. Amplitude controls how far cells intrude
	// into neighbours; base frequency controls how long the lowest-
	// octave wave is. Both scale with cellSide so visual quality
	// stays consistent from Tiny to Gigantic.
	avgCellSide := math.Sqrt(float64(width*height) / float64(len(w.Voronoi.Cells)))
	amplitude := avgCellSide * noisyEdgesAmplitudeFactor
	baseFreq := noisyEdgesFreqFactor / avgCellSide

	// Total normalisation across all octaves so coarse + per-tile
	// contributions sum to a unit-amplitude fBm.
	var totalNorm float64
	{
		amp := 1.0
		for oct := 0; oct < totalOct; oct++ {
			totalNorm += amp
			amp *= noisyEdgesGain
		}
	}

	// Starting amp/freq for the per-tile (high) octaves — picks up
	// where the coarse octaves left off.
	tileStartAmp := 1.0
	tileStartFreq := 1.0
	for oct := 0; oct < coarseOct; oct++ {
		tileStartAmp *= noisyEdgesGain
		tileStartFreq *= noisyEdgesLacunarity
	}

	// Coarse grid dimensions — one extra cell on each axis so every
	// tile's bilinear quad (gx, gy)→(gx+1, gy+1) is in-bounds.
	cw := (width+cf-1)/cf + 1
	ch := (height+cf-1)/cf + 1

	dxGrid := make([]float64, cw*ch)
	dyGrid := make([]float64, cw*ch)

	workers := runtime.GOMAXPROCS(0)

	// Phase 1: build coarse grid with LOW octaves only. The grid
	// stores the un-normalised sum of those octaves; per-tile phase
	// will add high-octave contributions and divide by totalNorm
	// once at the end.
	band := (ch + workers - 1) / workers
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		cyLo := worker * band
		cyHi := cyLo + band
		if cyHi > ch {
			cyHi = ch
		}
		if cyLo >= cyHi {
			continue
		}
		wg.Add(1)
		go func(cyLo, cyHi int) {
			defer wg.Done()
			for cy := cyLo; cy < cyHi; cy++ {
				fyBase := float64(cy*cf) * baseFreq
				for cx := 0; cx < cw; cx++ {
					fxBase := float64(cx*cf) * baseFreq

					var sumDx, sumDy float64
					amp := 1.0
					freq := 1.0
					for oct := 0; oct < coarseOct; oct++ {
						sumDx += amp * nx.Eval2(fxBase*freq, fyBase*freq)
						sumDy += amp * ny.Eval2(fxBase*freq, fyBase*freq)
						amp *= noisyEdgesGain
						freq *= noisyEdgesLacunarity
					}
					dxGrid[cy*cw+cx] = sumDx
					dyGrid[cy*cw+cx] = sumDy
				}
			}
		}(cyLo, cyHi)
	}
	wg.Wait()

	// Phase 2: per-tile bilinear lookup of the coarse grid + direct
	// per-tile evaluation of the HIGH octaves. Combine and divide by
	// totalNorm to get unit-amplitude fBm, then scale by amplitude.
	band = (height + workers - 1) / workers
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
			fcf := float64(cf)
			for y := yLo; y < yHi; y++ {
				gy := y / cf
				ty := float64(y%cf) / fcf
				ty1 := 1.0 - ty
				row00 := gy * cw
				row10 := (gy + 1) * cw
				fyTile := float64(y) * baseFreq
				for x := 0; x < width; x++ {
					gx := x / cf
					tx := float64(x%cf) / fcf
					tx1 := 1.0 - tx

					i00 := row00 + gx
					i10 := row00 + gx + 1
					i01 := row10 + gx
					i11 := row10 + gx + 1

					// Bilinear-interpolated low-octave contribution.
					coarseDx := dxGrid[i00]*tx1*ty1 + dxGrid[i10]*tx*ty1 +
						dxGrid[i01]*tx1*ty + dxGrid[i11]*tx*ty
					coarseDy := dyGrid[i00]*tx1*ty1 + dyGrid[i10]*tx*ty1 +
						dyGrid[i01]*tx1*ty + dyGrid[i11]*tx*ty

					// Per-tile high-octave contribution — picks up
					// the spatial frequencies the coarse grid loses.
					sumDx := coarseDx
					sumDy := coarseDy
					amp := tileStartAmp
					freq := tileStartFreq
					fxTile := float64(x) * baseFreq
					for oct := coarseOct; oct < totalOct; oct++ {
						sumDx += amp * nx.Eval2(fxTile*freq, fyTile*freq)
						sumDy += amp * ny.Eval2(fxTile*freq, fyTile*freq)
						amp *= noisyEdgesGain
						freq *= noisyEdgesLacunarity
					}

					dx := (sumDx / totalNorm) * amplitude
					dy := (sumDy / totalNorm) * amplitude

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

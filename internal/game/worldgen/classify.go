package worldgen

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"

	opensimplex "github.com/ojrac/opensimplex-go"
)

// Per-field salts for water / continent placement.
const (
	saltClass  int64 = 0x243f6a8885a308d3
	saltCenter int64 = 0x13198a2e03707344
)

// classifyWater runs the multi-centre Amit Patel perlin radial
// island shape:
//
//	isLand = noise(q) > 0.3 + 0.3 * length²
//
// length = distance from cell to nearest continent centre, normalised
// by continentRadius. Edge-touching cells are forced to water so the
// flood-fill in classifyOceanLake finds a contiguous border ocean.
//
// Parallelised over cells — each per-cell decision is independent
// (reads cell centre + continent centres, samples noise, writes its
// own isWater slot). opensimplex.Noise.Eval2 is concurrent-safe
// after construction, so the noise instance is shared across workers.
func classifyWater(w *World, seed int64) []bool {
	cells := w.Voronoi.Cells
	isWater := make([]bool, len(cells))
	noise := opensimplex.New(seed ^ saltClass)
	halfH := float64(w.Height) / 2
	continentRadius := halfH * continentRadiusFraction
	centres := placeContinentCentres(
		rand.New(rand.NewPCG(uint64(seed), uint64(seed)^uint64(saltCenter))),
		w.Width, w.Height,
		w.Size.ContinentCount(),
		continentRadius*continentSpacingFactor,
	)
	halfW := float64(w.Width) / 2

	workers := runtime.GOMAXPROCS(0)
	band := (len(cells) + workers - 1) / workers
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		lo := worker * band
		hi := lo + band
		if hi > len(cells) {
			hi = len(cells)
		}
		if lo >= hi {
			continue
		}
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				if w.Voronoi.TouchesEdge(uint16(i)) {
					isWater[i] = true
					continue
				}
				cell := cells[i]
				minDist := math.MaxFloat64
				for _, c := range centres {
					dx := c[0] - cell.CenterX
					dy := c[1] - cell.CenterY
					d := dx*dx + dy*dy
					if d < minDist {
						minDist = d
					}
				}
				length := math.Sqrt(minDist) / continentRadius

				nx := (cell.CenterX - halfW) / halfH
				ny := (cell.CenterY - halfH) / halfH
				v := 0.0
				amp := 1.0
				freq := 1.0
				norm := 0.0
				for j := 0; j < classifyOctaves; j++ {
					v += amp * noise.Eval2(nx*2*freq, ny*2*freq)
					norm += amp
					amp *= 0.5
					freq *= 2.0
				}
				v = (v/norm + 1) * 0.5

				threshold := classifyBaseThreshold + classifySlopeThreshold*length*length
				isWater[i] = v <= threshold
			}
		}(lo, hi)
	}
	wg.Wait()
	return isWater
}

// placeContinentCentres — rejection-sampled centres with minimum
// inter-centre spacing and a margin proportional to minSep.
func placeContinentCentres(rng *rand.Rand, width, height, count int, minSep float64) [][2]float64 {
	margin := minSep * 0.4
	minSep2 := minSep * minSep
	out := make([][2]float64, 0, count)
	for i := 0; i < count; i++ {
		for attempts := 0; attempts < 4000; attempts++ {
			x := margin + rng.Float64()*(float64(width)-2*margin)
			y := margin + rng.Float64()*(float64(height)-2*margin)
			ok := true
			for _, c := range out {
				dx := c[0] - x
				dy := c[1] - y
				if dx*dx+dy*dy < minSep2 {
					ok = false
					break
				}
			}
			if ok {
				out = append(out, [2]float64{x, y})
				break
			}
		}
	}
	return out
}

// classifyOceanLake — flood-fill from border water cells. Water
// reachable from the border is ocean; the rest is lake.
func classifyOceanLake(w *World, isWater []bool) (isOcean, isLake []bool) {
	isOcean = make([]bool, len(isWater))
	isLake = make([]bool, len(isWater))
	queue := make([]uint16, 0, len(isWater))
	for id := range w.Voronoi.Cells {
		if isWater[id] && w.Voronoi.TouchesEdge(uint16(id)) {
			isOcean[id] = true
			queue = append(queue, uint16(id))
		}
	}
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		for _, n := range w.Voronoi.Cells[id].Neighbors {
			if isWater[n] && !isOcean[n] {
				isOcean[n] = true
				queue = append(queue, n)
			}
		}
	}
	for i := range isWater {
		if isWater[i] && !isOcean[i] {
			isLake[i] = true
		}
	}
	return isOcean, isLake
}

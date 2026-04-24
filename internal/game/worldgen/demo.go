package worldgen

import (
	"runtime"
	"sync"

	opensimplex "github.com/ojrac/opensimplex-go"
)

// DemoWorld is a throw-away visualisation fixture that feeds the
// worldgen-explorer TUI before the real staged pipeline lands. It
// evaluates multi-octave OpenSimplex noise over the full grid so the
// explorer has something visibly interesting to scroll through while
// we iterate on tools.
//
// Every field is a flat []float32 laid out row-major: index = y*Width + x.
// This matches the layout every future pipeline stage will use, so
// explorer rendering code does not have to change when the fake layers
// are replaced with tectonics / orogeny / climate / etc.
//
// DemoWorld is not part of the game runtime; it exists solely inside
// cmd/worldgen-explorer. The stub worldgen.go keeps producing the
// all-ocean *world.World the server expects.
type DemoWorld struct {
	Size   WorldSize
	Seed   int64
	Width  int
	Height int

	// Elevation is a multi-octave fractal Brownian noise field in [0, 1].
	Elevation []float32

	// Temperature is a latitude-gradient minus elevation, in [0, 1].
	Temperature []float32

	// Moisture is a second noise field in [0, 1], lightly attenuated
	// by elevation to give very rough "dry mountaintop" visual.
	Moisture []float32
}

// GenerateDemoWorld fills the three float32 grids in parallel row
// bands. Salts are fixed so the same (seed, size) pair is deterministic
// across runs and across goroutine counts.
func GenerateDemoWorld(seed int64, size WorldSize) *DemoWorld {
	w, h := size.Dimensions()
	total := w * h

	out := &DemoWorld{
		Size:        size,
		Seed:        seed,
		Width:       w,
		Height:      h,
		Elevation:   make([]float32, total),
		Temperature: make([]float32, total),
		Moisture:    make([]float32, total),
	}

	const (
		saltElev  int64 = 0x243f6a8885a308d3
		saltMoist int64 = 0x13198a2e03707344
	)
	elevNoise := opensimplex.New(seed ^ saltElev)
	moistNoise := opensimplex.New(seed ^ saltMoist)

	workers := runtime.GOMAXPROCS(0)
	bandHeight := (h + workers - 1) / workers
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		yLo := worker * bandHeight
		yHi := yLo + bandHeight
		if yHi > h {
			yHi = h
		}
		if yLo >= yHi {
			continue
		}
		wg.Add(1)
		go func(yLo, yHi int) {
			defer wg.Done()
			fillDemoBand(out, yLo, yHi, elevNoise, moistNoise)
		}(yLo, yHi)
	}
	wg.Wait()
	return out
}

// fillDemoBand populates one horizontal band of the demo world. Factored
// out so every worker goroutine sees the same loop body; the bands do
// not overlap, so parallel writes to Elevation / Temperature / Moisture
// are safe without locks.
func fillDemoBand(w *DemoWorld, yLo, yHi int, elevNoise, moistNoise opensimplex.Noise) {
	const (
		elevScale  = 256.0
		moistScale = 192.0
	)
	halfH := float64(w.Height) / 2.0

	for y := yLo; y < yHi; y++ {
		fy := float64(y)
		// Latitude: equator at Height/2 -> 1.0, poles -> 0.0.
		lat := 1.0 - float64(absInt(y-w.Height/2))/halfH
		if lat < 0 {
			lat = 0
		}

		for x := 0; x < w.Width; x++ {
			fx := float64(x)

			elev := fbm(elevNoise, fx/elevScale, fy/elevScale, 4, 0.5, 2.0)
			elev = (elev + 1) * 0.5 // -> [0, 1]

			moist := fbm(moistNoise, fx/moistScale, fy/moistScale, 3, 0.5, 2.0)
			moist = (moist + 1) * 0.5
			// Slight cooling of wet peaks: multiplicative attenuation
			// by (1 - elev) gives dry mountaintops without touching the
			// raw noise shape.
			moist *= 1.0 - 0.4*elev

			// Temperature: latitude dominant, elevation cools.
			temp := lat - 0.4*elev
			if temp < 0 {
				temp = 0
			}
			if temp > 1 {
				temp = 1
			}

			idx := y*w.Width + x
			w.Elevation[idx] = float32(elev)
			w.Moisture[idx] = float32(moist)
			w.Temperature[idx] = float32(temp)
		}
	}
}

// fbm is a minimal fractal-Brownian-motion accumulator over an
// opensimplex.Noise source. Kept inline here rather than depending on a
// separate noise helper package — the explorer is throw-away, the real
// pipeline will own its own noise primitives.
func fbm(n opensimplex.Noise, x, y float64, octaves int, persistence, lacunarity float64) float64 {
	sum := 0.0
	amp := 1.0
	freq := 1.0
	norm := 0.0
	for i := 0; i < octaves; i++ {
		sum += amp * n.Eval2(x*freq, y*freq)
		norm += amp
		amp *= persistence
		freq *= lacunarity
	}
	if norm == 0 {
		return 0
	}
	return sum / norm
}

// absInt returns the absolute value of a signed int. Used for latitude
// distance from the equator; stdlib math.Abs operates on float64 and
// would require casts at every call site.
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

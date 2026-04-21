package worldgen

import (
	opensimplex "github.com/ojrac/opensimplex-go"
)

// OctaveOpts controls fractional Brownian motion (fBm) over OpenSimplex noise.
//
// Octaves is the number of noise layers summed together. Each layer samples at a higher
// frequency and lower amplitude than the previous one, producing fractal detail.
//
// Lacunarity is the frequency multiplier per octave (classic value: 2.0).
// Persistence is the amplitude multiplier per octave (classic value: 0.5).
// Scale is the base world-coordinate scale; larger Scale stretches features out.
type OctaveOpts struct {
	Octaves     int
	Lacunarity  float64
	Persistence float64
	Scale       float64
}

// DefaultOctaveOpts are the starting parameters for terrain-style fBm noise.
var DefaultOctaveOpts = OctaveOpts{
	Octaves:     4,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       48.0,
}

// OctaveNoise samples multi-octave OpenSimplex noise in 2D.
// It is safe for concurrent use because the underlying opensimplex.Noise is stateless after construction.
type OctaveNoise struct {
	src  opensimplex.Noise
	opts OctaveOpts
	// norm is the maximum possible magnitude of the sum — used to rescale Eval2 output to [-1, 1].
	norm float64
}

// NewOctaveNoise creates an fBm noise field seeded by seed with the given options.
// Pass distinct (seed, layerSalt) combinations to decorrelate independent layers such as
// elevation, temperature, and moisture — computed by the caller as, for example,
// seed ^ 0x1234 for temperature, seed ^ 0x5678 for moisture.
//
// Preconditions: Octaves >= 1, Lacunarity > 0, Persistence > 0, Scale > 0. Violating
// any of these is a programming error and causes an immediate panic.
func NewOctaveNoise(seed int64, opts OctaveOpts) OctaveNoise {
	if opts.Octaves < 1 {
		panic("invalid OctaveOpts: Octaves must be >= 1")
	}
	if opts.Lacunarity <= 0 {
		panic("invalid OctaveOpts: Lacunarity must be > 0")
	}
	if opts.Persistence <= 0 {
		panic("invalid OctaveOpts: Persistence must be > 0")
	}
	if opts.Scale <= 0 {
		panic("invalid OctaveOpts: Scale must be > 0")
	}

	norm := 0.0
	amp := 1.0
	for range opts.Octaves {
		norm += amp
		amp *= opts.Persistence
	}

	return OctaveNoise{
		src:  opensimplex.New(seed),
		opts: opts,
		norm: norm,
	}
}

// Eval2 returns the noise value at (x, y) in [-1, 1].
func (n OctaveNoise) Eval2(x, y float64) float64 {
	sum := 0.0
	amp := 1.0
	freq := 1.0 / n.opts.Scale
	for range n.opts.Octaves {
		sum += amp * n.src.Eval2(x*freq, y*freq)
		freq *= n.opts.Lacunarity
		amp *= n.opts.Persistence
	}
	return sum / n.norm
}

// Eval2Normalized returns the noise value at (x, y) remapped to [0, 1].
func (n OctaveNoise) Eval2Normalized(x, y float64) float64 {
	return (n.Eval2(x, y) + 1.0) * 0.5
}

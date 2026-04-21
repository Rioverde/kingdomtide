package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// Seed salts for per-layer noise decorrelation. Independent layers must see different
// underlying noise fields, otherwise elevation and temperature would look visually
// correlated across the map. XOR-ing the user seed with a fixed salt is cheap and keeps
// determinism — two runs with the same base seed produce the same per-layer seeds.
// The constants are the fractional digits of π and e in hex (Knuth-style nothing-up-my-sleeve
// numbers), so small user seeds like 0, 1, 2 cannot accidentally cancel the salt.
const (
	seedSaltTemperature int64 = 0x243f6a8885a308d3
	seedSaltMoisture    int64 = 0x13198a2e03707344
	// seedSaltContinent decorrelates the low-frequency continent mask from the other layers.
	// Picked as a distinct 64-bit constant (different bit pattern from the other salts) so
	// small user seeds cannot collapse two layers into the same noise field. Value is kept
	// under 2^63 so it fits a non-negative int64 literal without an explicit conversion.
	seedSaltContinent int64 = 0x5a308d313198a2e0
	// seedSaltRidge decorrelates the ridge noise from every other layer. Distinct 64-bit
	// pattern (Phase 2). Kept under 2^63 to fit a non-negative int64 literal.
	seedSaltRidge int64 = 0x452821e638d01377
)

// seedSaltRidgeFreq is the mixer salt used to derive the per-world ridge-frequency jitter
// from worldSeed. Not a noise salt — it only feeds the scalar jitter formula. Kept distinct
// from seedSaltRidge so that two worlds with identical ridge-noise fields would still
// usually differ on ridge frequency.
const seedSaltRidgeFreq uint64 = 0xbe5466cf34e90c6c

// Continent-mask blending weights. The plan (Phase 1) specifies additive — not
// multiplicative — mixing to keep the combined value in [0, 1] and preserve the biome
// threshold meanings. Weights sum to 1.0 so the output stays in [0, 1] given two
// normalised inputs. The 0.6 / 0.4 split keeps local elevation variation dominant
// while giving the continent mask enough authority to cluster oceans into seas.
const (
	continentBlendElev = 0.6
	continentBlendCont = 0.4
)

// Phase 2 ridge-blend tuning constants. Ridges only lift elevation inside the mountain
// band — valleys stay smooth. The band is a Hermite smoothstep over raw continent-blended
// elevation. ridgeWeight caps the added elevation so peaks rise a fraction of one band,
// keeping the mountain footprint inside the generator's [0,1] contract even before the
// hard clamp in TileAt.
//
//   - ridgeBandLow: elev at which ridge contribution begins to ramp in.
//   - ridgeBandHigh: elev at which ridge contribution saturates.
//   - ridgeWeight: maximum additive contribution (ridge ∈ [0,1] is multiplied by this).
//
// Values picked empirically by TestSweepRidgeParams (removed after tuning). The plan
// suggested [0.55, 0.85] × 0.18 as a starting point; the sweep showed that band-low
// = 0.55 pushes the MOUNTAIN tile count up by > 30% because it promotes high hills into
// mountains far below the actual ridge spine. Shifting band-low to 0.58 keeps the
// count drift at ~1.24× baseline (within the ±30% gate) while still lifting genuine
// mountain tiles.
const (
	ridgeBandLow  = 0.58
	ridgeBandHigh = 0.85
	ridgeWeight   = 0.18
)

// Phase 2 ridge-frequency jitter. The ridge noise scale is multiplied by a per-world
// jitter in [ridgeScaleJitterMin, ridgeScaleJitterMin + ridgeScaleJitterRange) so every
// seed grows mountain chains with a slightly different wavelength (Himalayan to
// Appalachian). Named constants — not magic numbers — so the range is discoverable.
const (
	ridgeBaseScale        = 96.0
	ridgeScaleJitterMin   = 0.7
	ridgeScaleJitterRange = 0.6
)

// temperatureOpts is a lower-frequency two-octave fBm field. Temperature varies over large
// distances (continents worth of terrain) so a bigger scale and fewer octaves feel right.
var temperatureOpts = OctaveOpts{
	Octaves:     2,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       80.0,
}

// moistureOpts adds a touch more detail than temperature — rain shadows feel local — but
// still coarser than elevation.
var moistureOpts = OctaveOpts{
	Octaves:     3,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       64.0,
}

// continentOpts is the low-frequency landmass mask. Scale=512 means a single continent
// or ocean basin spans roughly 500 tiles (~32 chunks) — an order of magnitude larger
// than the elevation field, so oceans cluster into seas/gulfs rather than salt-and-
// peppering the map. Three octaves give continents an irregular coastline without
// introducing mid-frequency noise that would fight the main elevation field.
var continentOpts = OctaveOpts{
	Octaves:     3,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       512.0,
}

// ridgeOpts is the base fBm configuration for ridge noise. Three octaves is enough to
// add tributary-ridge detail without drowning the chain-like main axis produced by
// Eval2Ridge. Scale is a base value — NewWorldGenerator multiplies it by a per-world
// jitter in [ridgeScaleJitterMin, ridgeScaleJitterMin + ridgeScaleJitterRange).
var ridgeOpts = OctaveOpts{
	Octaves:     3,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       ridgeBaseScale,
}

// WorldGenerator is the deterministic pure function layer: given an (x, y) coordinate (or a
// whole chunk coord) it returns the tile that would live there for this seed. It owns five
// independent noise fields — elevation, temperature, moisture, continent, ridge — sampled on
// global world coordinates so that chunk borders stitch seamlessly.
//
// ridgeScaleJitter is the per-world jitter applied to ridgeBaseScale at construction time.
// It is stored on the struct so TileAt can re-derive the effective scale factor at every
// sample site without re-hashing the seed. The noise field itself already bakes the scale
// into its freq computation (see NewOctaveNoise); ridgeScaleJitter is retained purely for
// tests and diagnostics that want to assert the per-world ridge wavelength.
type WorldGenerator struct {
	seed             int64
	elevation        OctaveNoise
	temperature      OctaveNoise
	moisture         OctaveNoise
	continent        OctaveNoise
	ridge            OctaveNoise
	ridgeScaleJitter float64

	// rivers caches the per-chunk river + lake tile sets. The classification
	// itself is a pure function of (seed, x, y) computed by rivers.go; the
	// cache only memoises the enumeration + trace work. Thread-safety comes
	// from hashicorp/golang-lru/v2.
	rivers *riverCache
}

// NewWorldGenerator builds the five noise fields off the supplied base seed. The per-layer
// seeds are derived by XOR-ing with fixed salts so callers that care about determinism only
// need to remember one number.
//
// The ridge noise gets a per-world frequency jitter — deterministic from worldSeed — so
// every seed grows mountains with a slightly different wavelength without a config knob.
func NewWorldGenerator(seed int64) *WorldGenerator {
	jitter := ridgeScaleJitterMin + seedJitter01(seed, seedSaltRidgeFreq)*ridgeScaleJitterRange
	jitteredRidgeOpts := ridgeOpts
	jitteredRidgeOpts.Scale = ridgeBaseScale * jitter

	return &WorldGenerator{
		seed:             seed,
		elevation:        NewOctaveNoise(seed, DefaultOctaveOpts),
		temperature:      NewOctaveNoise(seed^seedSaltTemperature, temperatureOpts),
		moisture:         NewOctaveNoise(seed^seedSaltMoisture, moistureOpts),
		continent:        NewOctaveNoise(seed^seedSaltContinent, continentOpts),
		ridge:            NewOctaveNoise(seed^seedSaltRidge, jitteredRidgeOpts),
		ridgeScaleJitter: jitter,
		rivers:           newRiverCache(DefaultRiverCacheCapacity),
	}
}

// Seed returns the base seed used to construct the generator. Useful for
// reproducing a world elsewhere (seed + same version of gongeons yields the
// same map).
func (g *WorldGenerator) Seed() int64 {
	return g.seed
}

// elevationAt returns the final elevation at (fx, fy) — continent-blended (Phase 1) and
// ridge-lifted inside the mountain band (Phase 2). Every downstream consumer (biome lookup,
// river sources, river tracing) must use this helper so the whole pipeline sees the same
// elevation field.
//
// Stages:
//  1. Continent blend: 0.6*elev + 0.4*cont — both inputs live in [0, 1] and the weights
//     sum to 1.0, so the intermediate is in [0, 1] too.
//  2. Ridge lift: inside [ridgeBandLow, ridgeBandHigh] a Hermite smoothstep ramps in
//     ridge = (1 - |raw|)² times ridgeWeight. Ridges are added only in the mountain
//     band so valleys stay smooth.
//  3. Clamp to [0, 1] so the biome invariant holds even if the raw sum overshoots.
func (g *WorldGenerator) elevationAt(fx, fy float64) float64 {
	elev := g.elevation.Eval2Normalized(fx, fy)
	cont := g.continent.Eval2Normalized(fx, fy)
	blended := continentBlendElev*elev + continentBlendCont*cont

	ridge := g.ridge.Eval2Ridge(fx, fy)
	band := smoothstep(blended, ridgeBandLow, ridgeBandHigh)
	final := blended + band*ridgeWeight*ridge

	return max(0.0, min(final, 1.0))
}

// TileAt is the canonical per-coord lookup. It samples the noise fields at the given
// global grid coordinate and hands them to the biome matrix. Same (seed, x, y) always
// yields the same tile, with or without the chunk cache in front.
func (g *WorldGenerator) TileAt(x, y int) game.Tile {
	fx, fy := float64(x), float64(y)
	elev := g.elevationAt(fx, fy)
	temp := g.temperature.Eval2Normalized(fx, fy)
	moist := g.moisture.Eval2Normalized(fx, fy)
	return game.Tile{Terrain: Biome(elev, temp, moist)}
}

// smoothstep is the Hermite cubic smoothstep: 0 below edge0, 1 above edge1, and a
// C1-continuous t*t*(3-2*t) in between. Package-private helper for ridge-band gating
// (Phase 2). When edge1 <= edge0 the function falls through to a 0-or-1 step at edge0
// rather than panicking — the degenerate-range case is benign and silent.
func smoothstep(x, edge0, edge1 float64) float64 {
	if edge1 <= edge0 {
		// Degenerate band — treat as a step function at edge0.
		if x < edge0 {
			return 0.0
		}
		return 1.0
	}
	t := max(0.0, min((x-edge0)/(edge1-edge0), 1.0))
	return t * t * (3.0 - 2.0*t)
}

// seedJitter01 returns a deterministic float in [0, 1) derived from (seed, salt).
// Splits salt across two int inputs to the SplitMix64-style mixer so all three
// slots stay non-zero (the mixer XORs three multiplies; a zero slot collapses
// one input). Takes the top 53 bits of the 64-bit output and divides by 2^53 —
// the standard construction for a uniform [0, 1) float from a 64-bit PRNG,
// matching math/rand's Float64.
func seedJitter01(seed int64, salt uint64) float64 {
	saltLo := int(salt & 0xffffffff)
	saltHi := int(salt >> 32)
	u := splitMix64(uint64(saltLo), uint64(saltHi), uint64(seed))
	return float64(u>>11) / (1 << 53)
}

// splitMix64 mixes three 64-bit inputs into a single well-diffused 64-bit value
// using large-prime multiplication and the SplitMix64 finalizer. Used for
// deterministic seeded jittering; not a cryptographic hash.
func splitMix64(a, b, c uint64) uint64 {
	x := a*0x9e3779b97f4a7c15 ^ b*0x6c62272e07bb0142 ^ c*0xbf58476d1ce4e5b9
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// Chunk fills an entire Chunk worth of tiles in a single pass: for each coord
// it calls TileAt for the base biome and overlays the hydrology layer (rivers
// + lakes). Biome is intentionally not altered here — river-adjacent biome
// blending is a later tuning pass. Settlements (villages, castles) are NOT
// placed at this layer — they are civilization artifacts and live on Layer 3,
// generated by the history/simulation pipeline.
//
// Chunk fills the per-chunk river and lake overlays using the trace-based
// algorithm in rivers.go. Rivers and lakes are pure functions of (seed, x, y),
// so neighbouring chunks always agree on overlays at their shared edges — no
// buffer-boundary seams.
func (g *WorldGenerator) Chunk(cc ChunkCoord) Chunk {
	chunk := Chunk{Coord: cc}
	minX, _, minY, _ := cc.Bounds()

	overlays := g.riversFor(cc)

	for dy := range ChunkSize {
		for dx := range ChunkSize {
			x, y := minX+dx, minY+dy
			t := g.TileAt(x, y)
			coord := tileCoord{x, y}
			if _, ok := overlays.rivers[coord]; ok {
				t.Overlays |= game.OverlayRiver
			}
			if _, ok := overlays.lakes[coord]; ok {
				t.Overlays |= game.OverlayLake
			}
			chunk.Tiles[dy][dx] = t
		}
	}

	return chunk
}

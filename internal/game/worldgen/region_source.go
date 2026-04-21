package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// Six low-frequency fBm fields drive the per-character region influence.
// Scale values sit in the 320-512 tile band so influence gradients span
// several super-chunks and regions pick up coherent thematic zones rather
// than salt-and-pepper noise. Blight and Savage get a third octave for a
// slightly rougher feel; the calmer characters stay at two octaves.
var (
	regionBlightOpts  = OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionFaeOpts     = OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 384}
	regionAncientOpts = OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionSavageOpts  = OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 448}
	regionHolyOpts    = OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionWildOpts    = OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 320}
)

// Per-character seed salts. The top bit is set in most of these values, so
// the Go constant system refuses them as untyped int64 literals — routing
// through regionToInt64 turns the conversion into a runtime step that
// preserves the full 64-bit pattern. This mirrors the Sub-phase 1a pattern
// in superchunk.go.
var (
	seedSaltRegionBlight  = regionToInt64(0x7f4a7c15be5466cf)
	seedSaltRegionFae     = regionToInt64(0x34e90c6c85a308d3)
	seedSaltRegionAncient = regionToInt64(0x13198a2e03707344)
	seedSaltRegionSavage  = regionToInt64(0x82efa98ec4eec6a9)
	seedSaltRegionHoly    = regionToInt64(0xc0ac29b7c97c50dd)
	seedSaltRegionWild    = regionToInt64(0x3f84d5b5b5470917)
)

// regionToInt64 reinterprets a uint64 bit pattern as int64 at runtime. The
// game package ships its own toInt64 for the same purpose but keeps it
// unexported; duplicating the two-line helper here is cheaper than widening
// the game package's exported surface for a purely internal mechanic.
func regionToInt64(u uint64) int64 { return int64(u) }

// regionInfluenceFloor is the per-component magnitude below which the raw
// [0, 1] noise value is clipped to zero before rescaling. The floor carves
// out a "no influence" background so a character only shows up where its
// noise genuinely peaks, rather than painting a weak wash across the whole
// world.
const regionInfluenceFloor float32 = 0.35

// InfluenceSampler is the narrow interface the client needs for per-tile tint
// sampling. It exposes only InfluenceAt so callers do not need to depend on
// the full NoiseRegionSource (which carries an LRU WorldGenerator for server-
// side terrain sampling that the client never uses).
type InfluenceSampler interface {
	InfluenceAt(worldX, worldY int) game.RegionInfluence
}

// influenceSampler is the lightweight client-side implementation of
// InfluenceSampler. It owns only the six noise fields required for tint
// sampling and skips the WorldGenerator that NoiseRegionSource carries for
// server-side terrain lookup.
type influenceSampler struct {
	blight  OctaveNoise
	fae     OctaveNoise
	ancient OctaveNoise
	savage  OctaveNoise
	holy    OctaveNoise
	wild    OctaveNoise
}

// NewInfluenceSampler returns an InfluenceSampler seeded from seed. The
// returned sampler is safe for concurrent read; it is allocation-free per
// call once constructed. Use this on the client where only per-tile tint
// sampling is required — it skips the WorldGenerator and its LRU river
// cache that NoiseRegionSource builds for the server-side terrain pipeline.
func NewInfluenceSampler(seed int64) InfluenceSampler {
	return &influenceSampler{
		blight:  NewOctaveNoise(seed^seedSaltRegionBlight, regionBlightOpts),
		fae:     NewOctaveNoise(seed^seedSaltRegionFae, regionFaeOpts),
		ancient: NewOctaveNoise(seed^seedSaltRegionAncient, regionAncientOpts),
		savage:  NewOctaveNoise(seed^seedSaltRegionSavage, regionSavageOpts),
		holy:    NewOctaveNoise(seed^seedSaltRegionHoly, regionHolyOpts),
		wild:    NewOctaveNoise(seed^seedSaltRegionWild, regionWildOpts),
	}
}

// InfluenceAt implements InfluenceSampler for the lightweight client sampler.
func (s *influenceSampler) InfluenceAt(worldX, worldY int) game.RegionInfluence {
	fx, fy := float64(worldX), float64(worldY)
	return game.RegionInfluence{
		Blight:  rescaleInfluence(s.blight.Eval2Normalized(fx, fy)),
		Fae:     rescaleInfluence(s.fae.Eval2Normalized(fx, fy)),
		Ancient: rescaleInfluence(s.ancient.Eval2Normalized(fx, fy)),
		Savage:  rescaleInfluence(s.savage.Eval2Normalized(fx, fy)),
		Holy:    rescaleInfluence(s.holy.Eval2Normalized(fx, fy)),
		Wild:    rescaleInfluence(s.wild.Eval2Normalized(fx, fy)),
	}
}

// Compile-time assertion: influenceSampler satisfies InfluenceSampler.
var _ InfluenceSampler = (*influenceSampler)(nil)

// NoiseRegionSource implements game.RegionSource on top of six independent
// fBm noise fields. Determined entirely by seed.
//
// The struct also owns a WorldGenerator so that RegionAt can consult the
// terrain pipeline at the anchor position — the chosen biome family feeds
// the name generator's geographical-term picker.
type NoiseRegionSource struct {
	seed int64

	blight  OctaveNoise
	fae     OctaveNoise
	ancient OctaveNoise
	savage  OctaveNoise
	holy    OctaveNoise
	wild    OctaveNoise

	// worldgen is used for dominantBiomeNear sampling. Held as a pointer
	// because WorldGenerator carries an LRU river cache internally; we want
	// the source to share that cache across calls rather than rebuild it.
	worldgen *WorldGenerator
}

// NewNoiseRegionSource wires the six region noise fields and an accompanying
// WorldGenerator from a single seed. Each field gets an independent salt so
// the underlying noise streams are decorrelated — otherwise Blight and Holy
// (for example) would peak in visually identical places.
func NewNoiseRegionSource(seed int64) *NoiseRegionSource {
	return &NoiseRegionSource{
		seed:     seed,
		blight:   NewOctaveNoise(seed^seedSaltRegionBlight, regionBlightOpts),
		fae:      NewOctaveNoise(seed^seedSaltRegionFae, regionFaeOpts),
		ancient:  NewOctaveNoise(seed^seedSaltRegionAncient, regionAncientOpts),
		savage:   NewOctaveNoise(seed^seedSaltRegionSavage, regionSavageOpts),
		holy:     NewOctaveNoise(seed^seedSaltRegionHoly, regionHolyOpts),
		wild:     NewOctaveNoise(seed^seedSaltRegionWild, regionWildOpts),
		worldgen: NewWorldGenerator(seed),
	}
}

// RegionAt returns the canonical Region for the super-chunk sc. The sample
// site is the jittered anchor of sc — keeping the sample point deterministic
// on sc (not on the calling tile) ensures every tile inside the region sees
// the same Region record.
func (s *NoiseRegionSource) RegionAt(sc game.SuperChunkCoord) game.Region {
	anchor := game.AnchorOf(s.seed, sc)
	influence := s.InfluenceAt(anchor.X, anchor.Y)
	character := influence.Dominant()
	biome := s.dominantBiomeNear(anchor)

	return game.Region{
		Coord:     sc,
		Anchor:    anchor,
		Influence: influence,
		Character: character,
		Name:      RegionName(character, biome, s.seed, sc),
	}
}

// InfluenceAt samples the six fields at (worldX, worldY), clips anything at
// or below regionInfluenceFloor to zero, and rescales the remainder to
// [0, 1]. This is a hot path in two senses:
//
//   - The client tint pipeline calls it for every visible tile.
//   - The server calls it to populate RegionAt at every super-chunk anchor.
//
// Avoids heap allocations: all arithmetic happens on stack-resident floats,
// and the returned value is a plain struct (not a pointer).
func (s *NoiseRegionSource) InfluenceAt(worldX, worldY int) game.RegionInfluence {
	fx, fy := float64(worldX), float64(worldY)

	return game.RegionInfluence{
		Blight:  rescaleInfluence(s.blight.Eval2Normalized(fx, fy)),
		Fae:     rescaleInfluence(s.fae.Eval2Normalized(fx, fy)),
		Ancient: rescaleInfluence(s.ancient.Eval2Normalized(fx, fy)),
		Savage:  rescaleInfluence(s.savage.Eval2Normalized(fx, fy)),
		Holy:    rescaleInfluence(s.holy.Eval2Normalized(fx, fy)),
		Wild:    rescaleInfluence(s.wild.Eval2Normalized(fx, fy)),
	}
}

// rescaleInfluence maps a raw [0, 1] noise sample to the post-floor
// [0, 1] output band. Values at or below the floor clip to zero; the
// remainder stretches linearly so the peak (1.0) stays reachable. The cast
// from float64 to float32 happens once here so the noise field stays in
// its native double precision and only the stored component narrows.
func rescaleInfluence(raw float64) float32 {
	v := float32(raw)
	if v <= regionInfluenceFloor {
		return 0
	}
	return (v - regionInfluenceFloor) / (1.0 - regionInfluenceFloor)
}

// dominantBiomeNear returns the BiomeFamily of the tile at the anchor
// position. Sampling a single tile (rather than, say, a 3x3 neighbourhood
// average) is intentional: regions are tens of tiles wide and their
// anchor sits well inside the cell, so the anchor tile is representative
// enough for name-picking. Broader sampling can come later once the
// naming pass starts looking lopsided in specific terrains.
func (s *NoiseRegionSource) dominantBiomeNear(anchor game.Position) BiomeFamily {
	t := s.worldgen.TileAt(anchor.X, anchor.Y)
	return FamilyOf(t.Terrain)
}

// Compile-time assertion that NoiseRegionSource implements the consumer-side
// interface. Mirrors the pattern in source.go for ChunkedSource /
// game.TileSource.
var _ game.RegionSource = (*NoiseRegionSource)(nil)

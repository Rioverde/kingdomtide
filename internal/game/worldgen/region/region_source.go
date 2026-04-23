package region

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/biome"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/noise"
)

// Six low-frequency fBm fields drive the per-character region influence.
// Scale values sit in the 320-512 tile band so influence gradients span
// several super-chunks and regions pick up coherent thematic zones rather
// than salt-and-pepper noise. Blight and Savage get a third octave for a
// slightly rougher feel; the calmer characters stay at two octaves.
var (
	regionBlightOpts  = noise.OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionFaeOpts     = noise.OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 384}
	regionAncientOpts = noise.OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionSavageOpts  = noise.OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 448}
	regionHolyOpts    = noise.OctaveOpts{Octaves: 2, Persistence: 0.5, Lacunarity: 2.0, Scale: 512}
	regionWildOpts    = noise.OctaveOpts{Octaves: 3, Persistence: 0.5, Lacunarity: 2.0, Scale: 320}
)

// Per-character seed salts. The top bit is set in most of these values, so
// the Go constant system refuses them as untyped int64 literals — routing
// through genprim.ToInt64 turns the conversion into a runtime step that
// preserves the full 64-bit pattern.
var (
	seedSaltRegionBlight  = genprim.ToInt64(0x7f4a7c15be5466cf)
	seedSaltRegionFae     = genprim.ToInt64(0x34e90c6c85a308d3)
	seedSaltRegionAncient = genprim.ToInt64(0x13198a2e03707344)
	seedSaltRegionSavage  = genprim.ToInt64(0x82efa98ec4eec6a9)
	seedSaltRegionHoly    = genprim.ToInt64(0xc0ac29b7c97c50dd)
	seedSaltRegionWild    = genprim.ToInt64(0x3f84d5b5b5470917)
)

// regionInfluenceFloor is the per-component magnitude below which the raw
// [0, 1] noise value is clipped to zero before rescaling. The floor carves
// out a "no influence" background so a character only shows up where its
// noise genuinely peaks, rather than painting a weak wash across the whole
// world.
const regionInfluenceFloor float32 = 0.35

// TerrainSampler is the minimal consumer-side interface NoiseRegionSource
// needs from a world generator: a single-tile terrain lookup for the
// dominant-biome-at-anchor pass. Declared here at the consumer so the
// region package does not depend on the concrete *worldgen.WorldGenerator.
type TerrainSampler interface {
	TileAt(x, y int) world.Tile
}

// InfluenceSampler is the narrow interface the client needs for per-tile
// tint sampling. It exposes only InfluenceAt so callers do not need to
// depend on the full NoiseRegionSource (which carries a TerrainSampler for
// server-side biome sampling that the client never uses).
type InfluenceSampler interface {
	InfluenceAt(worldX, worldY int) world.RegionInfluence
}

// influenceSampler is the lightweight client-side implementation of
// InfluenceSampler. It owns only the six noise fields required for tint
// sampling and skips the TerrainSampler that NoiseRegionSource carries for
// server-side terrain lookup.
type influenceSampler struct {
	blight  noise.OctaveNoise
	fae     noise.OctaveNoise
	ancient noise.OctaveNoise
	savage  noise.OctaveNoise
	holy    noise.OctaveNoise
	wild    noise.OctaveNoise
}

// NewInfluenceSampler returns an InfluenceSampler seeded from seed. The
// returned sampler is safe for concurrent read; it is allocation-free per
// call once constructed. Use this on the client where only per-tile tint
// sampling is required — it skips the TerrainSampler that the server-side
// NoiseRegionSource builds for biome lookup.
func NewInfluenceSampler(seed int64) InfluenceSampler {
	return &influenceSampler{
		blight:  noise.NewOctaveNoise(seed^seedSaltRegionBlight, regionBlightOpts),
		fae:     noise.NewOctaveNoise(seed^seedSaltRegionFae, regionFaeOpts),
		ancient: noise.NewOctaveNoise(seed^seedSaltRegionAncient, regionAncientOpts),
		savage:  noise.NewOctaveNoise(seed^seedSaltRegionSavage, regionSavageOpts),
		holy:    noise.NewOctaveNoise(seed^seedSaltRegionHoly, regionHolyOpts),
		wild:    noise.NewOctaveNoise(seed^seedSaltRegionWild, regionWildOpts),
	}
}

// InfluenceAt implements InfluenceSampler for the lightweight client sampler.
func (s *influenceSampler) InfluenceAt(worldX, worldY int) world.RegionInfluence {
	fx, fy := float64(worldX), float64(worldY)
	return world.RegionInfluence{
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

// NoiseRegionSource implements world.RegionSource on top of six independent
// fBm noise fields. Determined entirely by seed.
//
// The struct also holds a TerrainSampler so RegionAt can consult the terrain
// pipeline at the anchor position — the chosen biome family feeds the name
// generator's geographical-term picker. terrain is the consumer-side
// abstraction; production wiring passes a *worldgen.WorldGenerator, which
// satisfies the interface structurally.
type NoiseRegionSource struct {
	seed int64

	blight  noise.OctaveNoise
	fae     noise.OctaveNoise
	ancient noise.OctaveNoise
	savage  noise.OctaveNoise
	holy    noise.OctaveNoise
	wild    noise.OctaveNoise

	terrain TerrainSampler
}

// NewNoiseRegionSource wires the six region noise fields and a
// TerrainSampler for biome lookup. Each field gets an independent salt so
// the underlying noise streams are decorrelated — otherwise Blight and
// Holy (for example) would peak in visually identical places. terrain may
// be nil; callers that only need InfluenceAt can skip it, but RegionAt
// requires a non-nil sampler.
func NewNoiseRegionSource(seed int64, terrain TerrainSampler) *NoiseRegionSource {
	return &NoiseRegionSource{
		seed:    seed,
		blight:  noise.NewOctaveNoise(seed^seedSaltRegionBlight, regionBlightOpts),
		fae:     noise.NewOctaveNoise(seed^seedSaltRegionFae, regionFaeOpts),
		ancient: noise.NewOctaveNoise(seed^seedSaltRegionAncient, regionAncientOpts),
		savage:  noise.NewOctaveNoise(seed^seedSaltRegionSavage, regionSavageOpts),
		holy:    noise.NewOctaveNoise(seed^seedSaltRegionHoly, regionHolyOpts),
		wild:    noise.NewOctaveNoise(seed^seedSaltRegionWild, regionWildOpts),
		terrain: terrain,
	}
}

// Terrain returns the TerrainSampler wired into this source. Exposed so
// callers that already have a NoiseRegionSource can reuse its underlying
// sampler for landmark / volcano plumbing without building a second one.
func (s *NoiseRegionSource) Terrain() TerrainSampler {
	return s.terrain
}

// RegionAt returns the canonical Region for the super-chunk sc. The
// sample site is the jittered anchor of sc — keeping the sample point
// deterministic on sc (not on the calling tile) ensures every tile
// inside the region sees the same Region record. The returned
// Region.Name is a language-agnostic Parts record; the client composes
// the display string locally.
func (s *NoiseRegionSource) RegionAt(sc geom.SuperChunkCoord) world.Region {
	anchor := geom.AnchorOf(s.seed, sc)
	influence := s.InfluenceAt(anchor.X, anchor.Y)
	character := influence.Dominant()
	family := s.dominantBiomeNear(anchor)

	return world.Region{
		Coord:     sc,
		Anchor:    anchor,
		Influence: influence,
		Character: character,
		Name:      Name(character, family, s.seed, sc),
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
func (s *NoiseRegionSource) InfluenceAt(worldX, worldY int) world.RegionInfluence {
	fx, fy := float64(worldX), float64(worldY)

	return world.RegionInfluence{
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
func (s *NoiseRegionSource) dominantBiomeNear(anchor geom.Position) biome.BiomeFamily {
	t := s.terrain.TileAt(anchor.X, anchor.Y)
	return biome.FamilyOf(t.Terrain)
}

// Compile-time assertion that NoiseRegionSource implements the consumer-side
// world.RegionSource interface.
var _ world.RegionSource = (*NoiseRegionSource)(nil)

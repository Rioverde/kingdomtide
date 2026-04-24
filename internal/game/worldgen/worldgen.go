// Package worldgen is a transitional stub during the worldgen rewrite.
//
// Every query returns an empty ocean world: no land, no landmarks, no
// volcanoes, no regions, no deposits. The types and constructors are
// preserved so existing consumers (cmd/server, internal/ui) keep
// compiling while the new bounded, staged pipeline is built stage by
// stage and replaces these stubs piece by piece.
//
// When the real pipeline lands, this file shrinks to thin constructors
// that wire a *World into the same public symbols. Consumers require
// no source changes across the cutover.
package worldgen

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// NewWorld returns a *world.World whose TileSource reports deep ocean
// for every coordinate. Used by cmd/server to boot; the server runs,
// the UI paints blue everywhere, no gameplay is possible until the new
// pipeline populates the tiles.
func NewWorld(seed int64) *world.World {
	return world.NewWorld(NewChunkedSource(seed))
}

// WorldGenerator is retained as the legacy entry point. Post-rewrite it
// will orchestrate the staged geological pipeline; today it only
// carries the seed so downstream source constructors still type-check.
type WorldGenerator struct {
	seed int64
}

// NewWorldGenerator returns a WorldGenerator whose TileAt always
// reports TerrainDeepOcean.
func NewWorldGenerator(seed int64) *WorldGenerator {
	return &WorldGenerator{seed: seed}
}

// Seed returns the base seed passed at construction. Preserved so
// callers that reproduce a world elsewhere can read the original value.
func (g *WorldGenerator) Seed() int64 { return g.seed }

// TileAt returns deep ocean for every coordinate during the stub phase.
func (g *WorldGenerator) TileAt(_, _ int) world.Tile {
	return world.Tile{Terrain: world.TerrainDeepOcean}
}

// ChunkedSource is the stub-era world.TileSource. It owns a
// *WorldGenerator so callers that need the generator (e.g. to pass to
// region/landmark/volcano/deposit sources) can still fetch one via
// Generator().
type ChunkedSource struct {
	gen *WorldGenerator
}

// NewChunkedSource returns an ocean-everywhere TileSource.
func NewChunkedSource(seed int64) *ChunkedSource {
	return &ChunkedSource{gen: NewWorldGenerator(seed)}
}

// TileAt delegates to the wrapped generator.
func (s *ChunkedSource) TileAt(x, y int) world.Tile { return s.gen.TileAt(x, y) }

// Generator returns the underlying *WorldGenerator. Preserved for
// cmd/server's wiring: downstream source constructors take a
// *WorldGenerator to share a single terrain reader.
func (s *ChunkedSource) Generator() *WorldGenerator { return s.gen }

var _ world.TileSource = (*ChunkedSource)(nil)

// InfluenceSampler is the per-tile thematic-influence lookup the client
// UI consumes for region tinting. During the stub phase every tile
// reports zero influence across all six characters, so tints render as
// neutral.
type InfluenceSampler interface {
	InfluenceAt(x, y int) world.RegionInfluence
}

type zeroInfluenceSampler struct{}

// NewInfluenceSampler returns a sampler that reports zero influence
// everywhere — every tile is plain Normal until the real region layer
// returns.
func NewInfluenceSampler(_ int64) InfluenceSampler { return zeroInfluenceSampler{} }

// InfluenceAt always returns the zero RegionInfluence (all fields 0).
func (zeroInfluenceSampler) InfluenceAt(_, _ int) world.RegionInfluence {
	return world.RegionInfluence{}
}

// NoiseRegionSource is the stub world.RegionSource. Every super-chunk
// resolves to a blank Normal region with zero influence.
type NoiseRegionSource struct{}

// NewNoiseRegionSource returns the stub region source. The terrain
// argument is retained for signature compatibility; it is unused until
// the rewrite wires real regions back in.
func NewNoiseRegionSource(_ int64, _ *WorldGenerator) *NoiseRegionSource {
	return &NoiseRegionSource{}
}

// RegionAt returns a blank Region anchored at the super-chunk's origin.
// Character is RegionNormal by zero value; Name.Parts is empty so the
// client does not attempt to compose a display string.
func (*NoiseRegionSource) RegionAt(sc geom.SuperChunkCoord) world.Region {
	return world.Region{Coord: sc}
}

// InfluenceAt returns zero influence for every coordinate.
func (*NoiseRegionSource) InfluenceAt(_, _ int) world.RegionInfluence {
	return world.RegionInfluence{}
}

var (
	_ world.RegionSource = (*NoiseRegionSource)(nil)
	_ InfluenceSampler   = (*NoiseRegionSource)(nil)
)

// NoiseLandmarkSource is the stub world.LandmarkSource. No landmarks
// exist anywhere.
type NoiseLandmarkSource struct{}

// NewNoiseLandmarkSource returns the stub landmark source. Arguments
// are retained for signature compatibility.
func NewNoiseLandmarkSource(_ int64, _ world.RegionSource, _ *WorldGenerator) *NoiseLandmarkSource {
	return &NoiseLandmarkSource{}
}

// LandmarksIn always returns nil — there are no landmarks on an empty
// ocean world.
func (*NoiseLandmarkSource) LandmarksIn(_ geom.SuperChunkCoord) []world.Landmark { return nil }

var _ world.LandmarkSource = (*NoiseLandmarkSource)(nil)

// NoiseVolcanoSource is the stub world.VolcanoSource. No volcanoes
// exist anywhere.
type NoiseVolcanoSource struct{}

// NewNoiseVolcanoSource returns the stub volcano source. Arguments are
// retained for signature compatibility.
func NewNoiseVolcanoSource(_ int64, _ *WorldGenerator, _ world.LandmarkSource) *NoiseVolcanoSource {
	return &NoiseVolcanoSource{}
}

// VolcanoAt returns nil — no volcanoes in stub world.
func (*NoiseVolcanoSource) VolcanoAt(_ geom.SuperChunkCoord) []world.Volcano { return nil }

// TerrainOverrideAt returns (empty, false) — no volcano footprint
// overrides base terrain.
func (*NoiseVolcanoSource) TerrainOverrideAt(_ geom.Position) (world.Terrain, bool) {
	return "", false
}

var _ world.VolcanoSource = (*NoiseVolcanoSource)(nil)

// NoiseDepositSource is the stub world.DepositSource. No deposits exist
// anywhere.
type NoiseDepositSource struct{}

// NewNoiseDepositSource returns the stub deposit source. Arguments are
// retained for signature compatibility.
func NewNoiseDepositSource(_ int64, _ *WorldGenerator, _ world.LandmarkSource, _ world.VolcanoSource) *NoiseDepositSource {
	return &NoiseDepositSource{}
}

// DepositAt returns the empty deposit for every tile.
func (*NoiseDepositSource) DepositAt(_ geom.Position) (world.Deposit, bool) {
	return world.Deposit{}, false
}

// DepositsIn returns nil for every rect.
func (*NoiseDepositSource) DepositsIn(_ geom.Rect) []world.Deposit { return nil }

// DepositsNear returns nil for every query.
func (*NoiseDepositSource) DepositsNear(_ geom.Position, _ int) []world.Deposit { return nil }

var _ world.DepositSource = (*NoiseDepositSource)(nil)

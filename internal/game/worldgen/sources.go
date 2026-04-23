package worldgen

// sources.go re-exports the constructors and types that callers outside
// the worldgen tree depend on. The actual implementations live in the
// cities, landmark, region, resource, and volcano sub-packages; these
// thin wrappers keep the worldgen package's public surface stable so
// importers (ui, server, devtools) need not change their import paths.

import (
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/cities"
	"github.com/Rioverde/gongeons/internal/game/worldgen/landmark"
	"github.com/Rioverde/gongeons/internal/game/worldgen/region"
	"github.com/Rioverde/gongeons/internal/game/worldgen/resource"
	"github.com/Rioverde/gongeons/internal/game/worldgen/volcano"
)

// InfluenceSampler is the narrow interface callers use for per-tile tint
// sampling. Aliased from region.InfluenceSampler so the ui package does
// not need to import the region sub-package directly.
type InfluenceSampler = region.InfluenceSampler

// NewInfluenceSampler returns a lightweight client-side InfluenceSampler
// seeded from seed. It owns only the six noise fields required for tint
// sampling and skips the TerrainSampler that NoiseRegionSource carries.
func NewInfluenceSampler(seed int64) InfluenceSampler {
	return region.NewInfluenceSampler(seed)
}

// NewNoiseRegionSource wires the six region noise fields. terrain must be
// non-nil; RegionAt dereferences it for biome sampling. For tint-only
// client-side use where RegionAt is never called, use NewInfluenceSampler instead.
func NewNoiseRegionSource(seed int64, terrain *WorldGenerator) *region.NoiseRegionSource {
	return region.NewNoiseRegionSource(seed, terrain)
}

// NewNoiseLandmarkSource wires the landmark pipeline from seed, a
// RegionSource for character biasing, and a WorldGenerator for terrain
// sampling (water rejection and elevation).
func NewNoiseLandmarkSource(seed int64, regions world.RegionSource, terrain *WorldGenerator) *landmark.NoiseLandmarkSource {
	return landmark.NewNoiseLandmarkSource(seed, regions, terrain)
}

// NoiseVolcanoSource is a type alias for volcano.NoiseVolcanoSource so
// callers in the worldgen package's own test files can reference the
// concrete type without importing the sub-package.
type NoiseVolcanoSource = volcano.NoiseVolcanoSource

// NewNoiseVolcanoSource wires the volcano placement pipeline from seed, a
// WorldGenerator for biome acceptance, and an optional LandmarkSource for
// collision rejection (may be nil).
func NewNoiseVolcanoSource(seed int64, terrain *WorldGenerator, lm world.LandmarkSource) *NoiseVolcanoSource {
	return volcano.NewNoiseVolcanoSource(seed, terrain, lm)
}

// NoiseDepositSource is an alias for resource.NoiseDepositSource so
// external callers can keep referencing the type through the worldgen
// package without importing the resource sub-package directly.
type NoiseDepositSource = resource.NoiseDepositSource

// NewNoiseDepositSource wires a deposit source to a WorldGenerator (for
// biome sampling), a LandmarkSource (used by point-like collision
// rejection), and a VolcanoSource (used by volcanic structural obsidian
// and sulfur). The landmark and volcano sources may be nil; callers that
// only need zonal + fish can pass nil without special casing.
func NewNoiseDepositSource(
	seed int64,
	wg *WorldGenerator,
	lm world.LandmarkSource,
	vs world.VolcanoSource,
) *NoiseDepositSource {
	return resource.NewNoiseDepositSource(seed, wg, lm, vs)
}

// SettlementKind is an alias for cities.SettlementKind so callers can
// keep using the type name through the worldgen package.
type SettlementKind = cities.SettlementKind

// Re-exports of the enum values so call sites read as worldgen.SettlementVillage.
const (
	SettlementVillage = cities.SettlementVillage
	SettlementTown    = cities.SettlementTown
	SettlementCity    = cities.SettlementCity
	SettlementKeep    = cities.SettlementKeep
	SettlementRuin    = cities.SettlementRuin
)

// Culture is an alias for cities.Culture.
type Culture = cities.Culture

// Re-exports of the Culture values so call sites read as worldgen.CultureDrevan.
const (
	CultureDrevan = cities.CultureDrevan
	CultureWild   = cities.CultureWild
	CultureFallen = cities.CultureFallen
	CulturePlain  = cities.CulturePlain
)


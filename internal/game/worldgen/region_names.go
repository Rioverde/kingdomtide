package worldgen

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/biome"
)

// regionBounds caps the PrefixIndex and PatternIndex draws to the
// number of catalog entries present in active.en.toml / active.ru.toml.
// Both locales carry the same counts (enforced by TestRegionCatalogCoverage)
// so a single Bounds record suffices for every language. Pattern keys
// use the "<domain>.<sub_kind>" shape that naming.Generate expects.
var regionBounds = naming.Bounds{
	PatternCount: map[string]int{
		"region.forest":   5,
		"region.plain":    5,
		"region.mountain": 4,
		"region.water":    4,
		"region.desert":   4,
		"region.tundra":   3,
		"region.unknown":  1,
	},
	PrefixCount: map[string]int{
		"normal":   5,
		"blighted": 5,
		"fey":      5,
		"ancient":  5,
		"savage":   5,
		"holy":     5,
		"wild":     5,
	},
}

// RegionBounds exposes regionBounds for the catalog-coverage tests in
// other packages. The return value is a snapshot; callers must not
// mutate the underlying maps.
func RegionBounds() naming.Bounds {
	return regionBounds
}

// RegionName produces a deterministic structured name for a region.
// Same (character, biome, seed, sc) inputs always return the same
// Parts — the naming package owns body-seed derivation, so the client
// can reproduce any language's body locally from Name.BodySeed.
//
// The returned Parts is stored on world.Region and composed into a
// display string by the client via the locale catalog under
// "region.name.*" and "region.prefix.*" keys.
func RegionName(
	character world.RegionCharacter,
	family biome.BiomeFamily,
	seed int64,
	sc geom.SuperChunkCoord,
) naming.Parts {
	return naming.Generate(
		naming.Input{
			Domain:    naming.DomainRegion,
			Character: character.Key(),
			SubKind:   family.Key(),
			Seed:      seed,
			CoordX:    sc.X,
			CoordY:    sc.Y,
		},
		regionBounds,
	)
}

// hashCoordPrimeX and hashCoordPrimeY are large odd primes preserved
// from the Phase 1 name generator. They remain here only because
// TestHashCoordDistribution exercises the exact mixing used by the
// old non-naming-package region-name RNG; the naming package itself
// uses a different pair of primes declared in
// internal/game/naming/naming.go so its coord stream is decorrelated
// from this one.
const (
	hashCoordPrimeX uint64 = 0x9e3779b185ebca87
	hashCoordPrimeY uint64 = 0xc2b2ae3d27d4eb4f
)

// hashCoord mixes the two components of a SuperChunkCoord into a
// single uint64 suitable for seeding rand.NewPCG's second input.
// Exposed for the distribution regression test; the production naming
// path uses the identical-shaped mixer inside internal/game/naming
// with distinct primes.
func hashCoord(sc geom.SuperChunkCoord) uint64 {
	return uint64(int64(sc.X))*hashCoordPrimeX ^ uint64(int64(sc.Y))*hashCoordPrimeY
}

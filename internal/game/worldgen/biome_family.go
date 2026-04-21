package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// BiomeFamily is a coarse grouping of Terrain values used by region naming.
// The Whittaker biome matrix in biome.go has 16 specific cells; the naming
// code only needs a handful of buckets ("forest-ish", "water-ish") to pick
// plausible geographical terms, so this family type collapses the matrix
// down to seven buckets and keeps the name generation decoupled from any
// single biome-table shape. Phase 1d will route these buckets through the
// locale catalog.
type BiomeFamily int

// Family constants. FamilyUnknown is the catch-all for terrain values the
// mapping has not yet been taught about; it lets FamilyOf remain total
// without panicking on a future Terrain addition.
const (
	FamilyPlain BiomeFamily = iota
	FamilyForest
	FamilyMountain
	FamilyWater
	FamilyDesert
	FamilyTundra
	FamilyUnknown
)

// biomeFamilyNames backs the String method. The array is indexed by the
// family constant; keeping it as a slice avoids a map allocation for what
// is effectively an enum-to-name lookup.
var biomeFamilyNames = [...]string{
	FamilyPlain:    "plain",
	FamilyForest:   "forest",
	FamilyMountain: "mountain",
	FamilyWater:    "water",
	FamilyDesert:   "desert",
	FamilyTundra:   "tundra",
	FamilyUnknown:  "unknown",
}

// String returns the lowercase key of the family. Implements fmt.Stringer
// for debug/logging use. Out-of-range values return "unknown" instead of
// panicking so a corrupted value still logs usefully.
func (f BiomeFamily) String() string {
	if int(f) < 0 || int(f) >= len(biomeFamilyNames) {
		return "unknown"
	}
	return biomeFamilyNames[f]
}

// FamilyOf maps a domain Terrain to its family bucket. Unknown Terrain
// values collapse to FamilyUnknown rather than panicking — region naming
// can still pick a plausible name from the "generic" geo-term list in that
// case, so there is no benefit to a hard failure at the mapping boundary.
func FamilyOf(t game.Terrain) BiomeFamily {
	switch t {
	case game.TerrainPlains, game.TerrainGrassland, game.TerrainMeadow, game.TerrainSavanna:
		return FamilyPlain
	case game.TerrainForest, game.TerrainJungle, game.TerrainTaiga:
		return FamilyForest
	case game.TerrainHills, game.TerrainMountain, game.TerrainSnowyPeak:
		return FamilyMountain
	case game.TerrainDeepOcean, game.TerrainOcean, game.TerrainBeach:
		return FamilyWater
	case game.TerrainDesert:
		return FamilyDesert
	case game.TerrainTundra, game.TerrainSnow:
		return FamilyTundra
	default:
		return FamilyUnknown
	}
}

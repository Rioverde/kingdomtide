package game

// Slot identifies an equipment slot on a combatant's body (head, body, legs).
type Slot string

// Terrain is the biome identifier for a map tile, used to drive passability,
// rendering, and world-generation classification.
type Terrain string

// StructureKind identifies a single built structure that can occupy a tile —
// villages, castles and similar. Structures are mutually exclusive: a tile
// holds at most one. The zero value StructureNone signals "no structure on
// this tile".
type StructureKind string

const (
	StructureNone    StructureKind = ""
	StructureVillage StructureKind = "village"
	StructureCastle  StructureKind = "castle"
)

const (
	// Equipment slots.

	SlotHead Slot = "head"
	SlotBody Slot = "body"
	SlotLegs Slot = "legs"

	// Terrain values. The set is tuned for a Whittaker-style biome matrix: water at low
	// elevation, mountain/snow at high elevation, and a climate grid in between that mixes
	// temperature and moisture.

	TerrainDeepOcean Terrain = "deep_ocean"
	TerrainOcean     Terrain = "ocean"
	TerrainBeach     Terrain = "beach"
	TerrainDesert    Terrain = "desert"
	TerrainSavanna   Terrain = "savanna"
	TerrainPlains    Terrain = "plains"
	TerrainGrassland Terrain = "grass"
	TerrainMeadow    Terrain = "meadow"
	TerrainForest    Terrain = "forest"
	TerrainJungle    Terrain = "jungle"
	TerrainTaiga     Terrain = "taiga"
	TerrainTundra    Terrain = "tundra"
	TerrainSnow      Terrain = "snow"
	TerrainHills     Terrain = "hills"
	TerrainMountain  Terrain = "mountain"
	TerrainSnowyPeak Terrain = "snowy_peak"

	// Damage multipliers for different body parts.
	HeadDamageMultiplier = 2.0
	BodyDamageMultiplier = 1.0
	LegsDamageMultiplier = 0.5
	numberOfSlots        = 3
)

// Speed scale values applied to Player and Monster entities. NetHack-style
// units: 12 is the baseline, doubling is "very fast", halving is "very slow".
// The absolute numbers matter only relative to baseActionCost.
const (
	SpeedVerySlow = 6
	SpeedSlow     = 9
	SpeedNormal   = 12
	SpeedFast     = 18
	SpeedVeryFast = 24
)

// baseActionCost is the Energy consumed by a standard gameplay action
// (move, basic attack). Exotic actions override this via Intent.Cost.
const baseActionCost = 12

// AllStructureKinds returns every StructureKind except StructureNone in a
// stable order. Useful when a client needs to enumerate all structures
// (for example, to pre-load sprite assets).
func AllStructureKinds() []StructureKind {
	return []StructureKind{StructureVillage, StructureCastle}
}

// AllTerrains lists every Terrain value in a stable order. Useful when a client
// needs to enumerate the full biome set (for example, to pre-load sprite assets).
func AllTerrains() []Terrain {
	return []Terrain{
		TerrainDeepOcean,
		TerrainOcean,
		TerrainBeach,
		TerrainDesert,
		TerrainSavanna,
		TerrainPlains,
		TerrainGrassland,
		TerrainMeadow,
		TerrainForest,
		TerrainJungle,
		TerrainTaiga,
		TerrainTundra,
		TerrainSnow,
		TerrainHills,
		TerrainMountain,
		TerrainSnowyPeak,
	}
}

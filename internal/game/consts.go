package game

type Slot string
type Terrain string

// ObjectKind identifies a point-of-interest structure that can appear on a tile as an
// overlay. The zero value ObjectNone signals "no POI on this tile" and is omitted from
// JSON thanks to the omitempty tag on Tile.Object.
type ObjectKind string

const (
	ObjectNone    ObjectKind = ""
	ObjectVillage ObjectKind = "village"
	ObjectCastle  ObjectKind = "castle"
)

const (
	// Equipment slots.

	SlotHead Slot = "head"
	SlotBody Slot = "body"
	SlotLegs Slot = "legs"

	// Terrain values. The set is tuned for a Whittaker-style biome matrix: water at low
	// elevation, mountain/snow at high elevation, and a climate grid in between that mixes
	// temperature and moisture. CursedForest is kept as a rare special biome that the
	// generator may sprinkle on top of the base matrix.

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

	TerrainCursedForest Terrain = "cursed_forest"

	// Damage multipliers for different body parts.
	HeadDamageMultiplier = 2.0
	BodyDamageMultiplier = 1.0
	LegsDamageMultiplier = 0.5
	numberOfSlots        = 3
)

// AllObjectKinds returns every ObjectKind except ObjectNone in a stable order. Used by the
// /api/meta endpoint to let the client pre-cache POI sprites.
func AllObjectKinds() []ObjectKind {
	return []ObjectKind{ObjectVillage, ObjectCastle}
}

// AllTerrains lists every Terrain value in a stable order. Used by the /api/meta endpoint
// and anywhere that needs to enumerate the full biome set.
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
		TerrainCursedForest,
	}
}

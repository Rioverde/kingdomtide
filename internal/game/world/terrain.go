package world

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

	// Volcanic terrains — multi-tile volcano footprints overwrite base biomes.
	// Core and CoreDormant are impassable; CraterLake behaves as inland water;
	// Slope and Ashland are land-passable.
	TerrainVolcanoCore        Terrain = "volcano_core"
	TerrainVolcanoCoreDormant Terrain = "volcano_core_dormant"
	TerrainCraterLake         Terrain = "crater_lake"
	TerrainVolcanoSlope       Terrain = "volcano_slope"
	TerrainAshland            Terrain = "ashland"
)

// AllStructureKinds returns every StructureKind except StructureNone in a
// stable order. Useful when a client needs to enumerate all structures
// (for example, to pre-load sprite assets).
func AllStructureKinds() []StructureKind {
	return []StructureKind{StructureVillage, StructureCastle}
}

// Passable reports whether an entity can stand on a tile of this terrain.
// Water and high peaks block movement; the empty string and unknown values
// are treated as impassable so buggy map data fails closed rather than open.
func (t Terrain) Passable() bool {
	switch t {
	case TerrainPlains,
		TerrainGrassland,
		TerrainMeadow,
		TerrainBeach,
		TerrainSavanna,
		TerrainDesert,
		TerrainSnow,
		TerrainTundra,
		TerrainTaiga,
		TerrainForest,
		TerrainJungle,
		TerrainHills,
		TerrainVolcanoSlope,
		TerrainAshland:
		return true
	default:
		return false
	}
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
		TerrainVolcanoCore,
		TerrainVolcanoCoreDormant,
		TerrainCraterLake,
		TerrainVolcanoSlope,
		TerrainAshland,
	}
}

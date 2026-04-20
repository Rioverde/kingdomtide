package game

// ObjectSprite returns the PNG filename for a point-of-interest overlay. The filename is
// relative to the tiles asset directory, matching the convention used by TileAsset.
// ObjectNone (and any unknown kind) returns an empty string so callers can skip rendering.
func ObjectSprite(k ObjectKind) string {
	switch k {
	case ObjectVillage:
		return "village.png"
	case ObjectCastle:
		return "castle.png"
	default:
		return ""
	}
}

// TileAsset returns the filename of the PNG tile that represents the given terrain.
// Filenames are relative to the tiles asset directory (e.g. "assets/tiles/water.png").
// An unknown terrain falls back to dirt so the caller never gets an empty string.
func TileAsset(t Terrain) string {
	switch t {
	case TerrainDeepOcean:
		return "deep_ocean.png"
	case TerrainOcean:
		return "ocean.png"
	case TerrainBeach:
		return "beach.png"
	case TerrainDesert:
		return "desert.png"
	case TerrainSavanna:
		return "savanna.png"
	case TerrainPlains:
		return "plains.png"
	case TerrainGrassland:
		return "grass.png"
	case TerrainMeadow:
		return "meadow.png"
	case TerrainForest:
		return "forest.png"
	case TerrainJungle:
		return "jungle.png"
	case TerrainTaiga:
		return "taiga.png"
	case TerrainTundra:
		return "tundra.png"
	case TerrainSnow:
		return "snow.png"
	case TerrainHills:
		return "hills.png"
	case TerrainMountain:
		return "mountain.png"
	case TerrainSnowyPeak:
		return "snowy_peak.png"
	default:
		// Defensive fallback — no live Terrain value should reach here. hills.png is used
		// because it exists in assets/tiles and is visually neutral enough to signal
		// "something went wrong" without crashing the renderer.
		return "hills.png"
	}
}

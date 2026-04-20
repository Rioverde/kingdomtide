package game

// Biome thresholds. The elevation bands are the water/land/mountain layout; within the land
// band temperature and moisture pick a Whittaker cell. All three inputs live in [0, 1] — the
// generator normalises the noise output before calling us. Edge values collapse into the
// nearest band via the `<` comparisons.
const (
	// Elevation bands. Tuned to the empirical fBm output range (~[0.28, 0.72]) so all
	// biome cells are reachable with the multi-octave Simplex noise used by the generator.

	elevationDeepOcean = 0.38
	elevationOcean     = 0.44
	elevationBeach     = 0.46
	elevationHills     = 0.58
	elevationMountain  = 0.63
	elevationSnowyPeak = 0.68

	// Temperature bands: cold / temperate / hot.

	temperatureCold = 0.44
	temperatureHot  = 0.56

	// Moisture bands: dry / mid / wet.

	moistureDry = 0.44
	moistureWet = 0.56
)

// Biome returns the Terrain for a tile given normalised elevation, temperature and moisture
// (each in [0, 1]).
//
// The function is deliberately pure and table-like so it can be unit-tested with hand-chosen
// samples and swapped without touching the generator or cache. Elevation decides water /
// lowland / highland / peak; inside the lowland band a 3x3 grid of temperature × moisture
// picks the specific biome. A final hills band sits between the plains biomes and the bare
// rock mountains.
func Biome(elevation, temperature, moisture float64) Terrain {
	if elevation < elevationDeepOcean {
		return TerrainDeepOcean
	}
	if elevation < elevationOcean {
		return TerrainOcean
	}
	if elevation < elevationBeach {
		// Cold coasts freeze into tundra, hot coasts are desert shoreline, everything else
		// is sandy beach.
		if temperature < temperatureCold {
			return TerrainTundra
		}
		if temperature > temperatureHot && moisture < moistureDry {
			return TerrainDesert
		}
		return TerrainBeach
	}
	if elevation < elevationHills {
		return lowlandBiome(temperature, moisture)
	}
	if elevation < elevationMountain {
		// Hill band: cold and wet hills turn into pine taiga, other hills keep their
		// generic rocky look. Hot dry hills read as desert mesa.
		if temperature < temperatureCold && moisture > moistureDry {
			return TerrainTaiga
		}
		if temperature > temperatureHot && moisture < moistureDry {
			return TerrainDesert
		}
		return TerrainHills
	}
	if elevation < elevationSnowyPeak {
		if temperature < temperatureCold {
			return TerrainSnow
		}
		return TerrainMountain
	}
	return TerrainSnowyPeak
}

// lowlandBiome picks the Whittaker cell for tiles inside the main land band. Split into a
// helper so the outer elevation ladder stays readable.
func lowlandBiome(temperature, moisture float64) Terrain {
	switch {
	case temperature < temperatureCold:
		// Cold lowlands: dry → tundra, mid → taiga, wet → snow fields.
		if moisture < moistureDry {
			return TerrainTundra
		}
		if moisture < moistureWet {
			return TerrainTaiga
		}
		return TerrainSnow
	case temperature > temperatureHot:
		// Hot lowlands: dry → desert, mid → savanna, wet → jungle.
		if moisture < moistureDry {
			return TerrainDesert
		}
		if moisture < moistureWet {
			return TerrainSavanna
		}
		return TerrainJungle
	default:
		// Temperate lowlands: dry → plains, mid → grassland, wet → meadow or forest.
		if moisture < moistureDry {
			return TerrainPlains
		}
		if moisture < moistureWet {
			return TerrainGrassland
		}
		if moisture < 0.60 { // compressed from 0.85 to fit the new moistureWet=0.56 band
			return TerrainMeadow
		}
		return TerrainForest
	}
}

package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestBiomeMatrix pins the most visible decisions of the Whittaker lookup so accidental
// threshold drift gets caught. Inputs live in [0, 1]; the chosen values sit comfortably in
// the middle of each band to avoid being sensitive to one-off threshold tweaks.
func TestBiomeMatrix(t *testing.T) {
	cases := []struct {
		name                          string
		elevation, temperature, moist float64
		want                          game.Terrain
	}{
		{"deep ocean", 0.05, 0.5, 0.5, game.TerrainDeepOcean},
		// Updated: 0.33 < elevationDeepOcean(0.38) → deep ocean; use 0.41 for shallow ocean
		{"shallow ocean", 0.41, 0.5, 0.5, game.TerrainOcean}, // was 0.33; now 0.41 lands in [0.38,0.44)
		// Updated: 0.40 < elevationOcean(0.44) → ocean; use 0.45 for beach
		{"temperate beach", 0.45, 0.5, 0.5, game.TerrainBeach}, // was 0.40; now 0.45 lands in [0.44,0.46)
		// Lowland cases: elev=0.55 still in [0.46, 0.58) → OK
		// temp=0.9 > temperatureHot(0.56), moist=0.1 < moistureDry(0.44)
		{"hot dry lowland is desert", 0.55, 0.9, 0.1, game.TerrainDesert},
		// temp=0.9 > hot, moist=0.5 in [0.44,0.56)
		{"hot mid lowland is savanna", 0.55, 0.9, 0.5, game.TerrainSavanna},
		// temp=0.9 > hot, moist=0.9 > moistureWet(0.56)
		{"hot wet lowland is jungle", 0.55, 0.9, 0.9, game.TerrainJungle},
		// temp=0.1 < temperatureCold(0.44), moist=0.1 < dry
		{"cold dry lowland is tundra", 0.55, 0.1, 0.1, game.TerrainTundra},
		// temp=0.1 < cold, moist=0.5 in [0.44,0.56)
		{"cold mid lowland is taiga", 0.55, 0.1, 0.5, game.TerrainTaiga},
		// temp=0.1 < cold, moist=0.9 > wet
		{"cold wet lowland is snow", 0.55, 0.1, 0.9, game.TerrainSnow},
		// temp=0.5 in [0.44,0.56) (temperate), moist=0.1 < dry
		{"temperate dry lowland is plains", 0.55, 0.5, 0.1, game.TerrainPlains},
		// temp=0.5 temperate, moist=0.5 in [0.44,0.56)
		{"temperate mid lowland is grassland", 0.55, 0.5, 0.5, game.TerrainGrassland},
		// Updated: temp=0.5 temperate, moist must be in (moistureWet=0.56, 0.60) for Meadow
		// was 0.75 which now exceeds the 0.60 sub-threshold → Forest; use 0.58 instead
		{"temperate wet lowland is meadow", 0.55, 0.5, 0.58, game.TerrainMeadow}, // was moist=0.75
		// temp=0.5 temperate, moist=0.95 > 0.60 sub-threshold → Forest (unchanged)
		{"temperate very wet lowland is forest", 0.55, 0.5, 0.95, game.TerrainForest},
		// Updated: elevationHills=[0.58,0.63); use 0.60 instead of 0.78
		{"hill band is hills", 0.60, 0.5, 0.5, game.TerrainHills}, // was 0.78
		// Updated: elevationMountain=[0.63,0.68); use 0.65 instead of 0.88
		{"mountain band is mountain", 0.65, 0.5, 0.5, game.TerrainMountain}, // was 0.88
		// Updated: elev=0.65 in mountain band, temp=0.1 < cold → Snow
		{"mountain band cold is snow", 0.65, 0.1, 0.5, game.TerrainSnow}, // was elev=0.88
		// elevationSnowyPeak=0.68; elev=0.98 still > 0.68 → SnowyPeak (unchanged)
		{"top band is snowy peak", 0.98, 0.5, 0.5, game.TerrainSnowyPeak},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Biome(tc.elevation, tc.temperature, tc.moist)
			if got != tc.want {
				t.Errorf("Biome(%v, %v, %v) = %q, want %q",
					tc.elevation, tc.temperature, tc.moist, got, tc.want)
			}
		})
	}
}

// TestBiomeDistributionCoverage verifies that every terrain produced by the
// full pipeline (TileAt → Biome) is reachable across a broad sample of world
// coordinates. Continent blending shifts the elevation distribution, so
// driving Biome() with raw noise samples in a fixed [0.30, 0.70] range would
// not reflect what players see. Instead we sample the live generator across
// multiple seeds — this is a stronger guarantee: not just "biome thresholds
// cover the [0, 1] interval" but "the generator actually produces every
// biome on a real map".
func TestBiomeDistributionCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("524K tile coverage scan across 8 seeds")
	}
	const seeds = 8
	const side = 256
	const half = side / 2

	histogram := make(map[game.Terrain]int)
	total := 0
	for s := int64(1); s <= seeds; s++ {
		g := NewWorldGenerator(s)
		for y := -half; y < half; y++ {
			for x := -half; x < half; x++ {
				tile := g.TileAt(x, y)
				histogram[tile.Terrain]++
				total++
			}
		}
	}

	t.Logf("Biome distribution over %d tiles (%d seeds × %d² window):", total, seeds, side)
	for _, terrain := range game.AllTerrains() {
		count := histogram[terrain]
		pct := float64(count) / float64(total) * 100
		t.Logf("  %-14s %6d  (%.2f%%)", terrain, count, pct)
	}

	// Every terrain in AllTerrains must appear at least once across the sample. Rare
	// terrains (SnowyPeak, Jungle) can need a wide sample to surface — 8 seeds over
	// 256² tiles (~524k samples total) is generous and gives the test a stable pass.
	// Volcanic terrains are skipped: they are placed by the VolcanoSource override
	// pipeline rather than emitted by the base noise generator.
	for _, terrain := range game.AllTerrains() {
		if isVolcanicTerrain(terrain) {
			continue
		}
		if histogram[terrain] == 0 {
			t.Errorf("terrain %q never appeared in %d tiles — thresholds may be miscalibrated", terrain, total)
		}
	}
}

// isVolcanicTerrain reports whether t is one of the volcanic terrains placed by
// the VolcanoSource override pipeline rather than emitted by the base noise
// generator. Tests that sample base-noise output must skip these so they do not
// fail the "every terrain appears" invariant before volcano placement ships.
func isVolcanicTerrain(t game.Terrain) bool {
	switch t {
	case game.TerrainVolcanoCore,
		game.TerrainVolcanoCoreDormant,
		game.TerrainCraterLake,
		game.TerrainVolcanoSlope,
		game.TerrainAshland:
		return true
	default:
		return false
	}
}

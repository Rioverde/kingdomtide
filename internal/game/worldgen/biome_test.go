package worldgen

import (
	"math/rand"
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

// TestBiomeDistributionCoverage verifies that every terrain produced by Biome() is reachable
// when inputs are drawn from the realistic fBm output range (~[0.30, 0.70]).
func TestBiomeDistributionCoverage(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	const iterations = 5000
	const lo, hi = 0.30, 0.70

	histogram := make(map[game.Terrain]int)
	for range iterations {
		elev := lo + rng.Float64()*(hi-lo)
		temp := lo + rng.Float64()*(hi-lo)
		moist := lo + rng.Float64()*(hi-lo)
		terrain := Biome(elev, temp, moist)
		histogram[terrain]++
	}

	t.Logf("Biome distribution over %d iterations (inputs in [%.2f, %.2f]):", iterations, lo, hi)
	for _, terrain := range game.AllTerrains() {
		count := histogram[terrain]
		pct := float64(count) / float64(iterations) * 100
		t.Logf("  %-14s %4d  (%.1f%%)", terrain, count, pct)
	}

	// Every terrain in AllTerrains must appear at least once across the sample.
	for _, terrain := range game.AllTerrains() {
		if histogram[terrain] == 0 {
			t.Errorf("terrain %q never appeared in %d iterations — thresholds may be miscalibrated", terrain, iterations)
		}
	}
}

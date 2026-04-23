package biome

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/world"
)

// TestBiomeMatrix pins the most visible decisions of the Whittaker lookup so accidental
// threshold drift gets caught. Inputs live in [0, 1]; the chosen values sit comfortably in
// the middle of each band to avoid being sensitive to one-off threshold tweaks.
func TestBiomeMatrix(t *testing.T) {
	cases := []struct {
		name                          string
		elevation, temperature, moist float64
		want                          world.Terrain
	}{
		{"deep ocean", 0.05, 0.5, 0.5, world.TerrainDeepOcean},
		// Updated: 0.33 < elevationDeepOcean(0.38) → deep ocean; use 0.41 for shallow ocean
		{"shallow ocean", 0.41, 0.5, 0.5, world.TerrainOcean}, // was 0.33; now 0.41 lands in [0.38,0.44)
		// Updated: 0.40 < elevationOcean(0.44) → ocean; use 0.45 for beach
		{"temperate beach", 0.45, 0.5, 0.5, world.TerrainBeach}, // was 0.40; now 0.45 lands in [0.44,0.46)
		// Lowland cases: elev=0.55 still in [0.46, 0.58) → OK
		// temp=0.9 > temperatureHot(0.56), moist=0.1 < moistureDry(0.44)
		{"hot dry lowland is desert", 0.55, 0.9, 0.1, world.TerrainDesert},
		// temp=0.9 > hot, moist=0.5 in [0.44,0.56)
		{"hot mid lowland is savanna", 0.55, 0.9, 0.5, world.TerrainSavanna},
		// temp=0.9 > hot, moist=0.9 > moistureWet(0.56)
		{"hot wet lowland is jungle", 0.55, 0.9, 0.9, world.TerrainJungle},
		// temp=0.1 < temperatureCold(0.44), moist=0.1 < dry
		{"cold dry lowland is tundra", 0.55, 0.1, 0.1, world.TerrainTundra},
		// temp=0.1 < cold, moist=0.5 in [0.44,0.56)
		{"cold mid lowland is taiga", 0.55, 0.1, 0.5, world.TerrainTaiga},
		// temp=0.1 < cold, moist=0.9 > wet
		{"cold wet lowland is snow", 0.55, 0.1, 0.9, world.TerrainSnow},
		// temp=0.5 in [0.44,0.56) (temperate), moist=0.1 < dry
		{"temperate dry lowland is plains", 0.55, 0.5, 0.1, world.TerrainPlains},
		// temp=0.5 temperate, moist=0.5 in [0.44,0.56)
		{"temperate mid lowland is grassland", 0.55, 0.5, 0.5, world.TerrainGrassland},
		// Updated: temp=0.5 temperate, moist must be in (moistureWet=0.56, 0.60) for Meadow
		// was 0.75 which now exceeds the 0.60 sub-threshold → Forest; use 0.58 instead
		{"temperate wet lowland is meadow", 0.55, 0.5, 0.58, world.TerrainMeadow}, // was moist=0.75
		// temp=0.5 temperate, moist=0.95 > 0.60 sub-threshold → Forest (unchanged)
		{"temperate very wet lowland is forest", 0.55, 0.5, 0.95, world.TerrainForest},
		// Updated: elevationHills=[0.58,0.63); use 0.60 instead of 0.78
		{"hill band is hills", 0.60, 0.5, 0.5, world.TerrainHills}, // was 0.78
		// Updated: elevationMountain=[0.63,0.68); use 0.65 instead of 0.88
		{"mountain band is mountain", 0.65, 0.5, 0.5, world.TerrainMountain}, // was 0.88
		// Updated: elev=0.65 in mountain band, temp=0.1 < cold → Snow
		{"mountain band cold is snow", 0.65, 0.1, 0.5, world.TerrainSnow}, // was elev=0.88
		// elevationSnowyPeak=0.68; elev=0.98 still > 0.68 → SnowyPeak (unchanged)
		{"top band is snowy peak", 0.98, 0.5, 0.5, world.TerrainSnowyPeak},
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

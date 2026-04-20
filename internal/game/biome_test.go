package game

import "testing"

// TestBiomeMatrix pins the most visible decisions of the Whittaker lookup so accidental
// threshold drift gets caught. Inputs live in [0, 1]; the chosen values sit comfortably in
// the middle of each band to avoid being sensitive to one-off threshold tweaks.
func TestBiomeMatrix(t *testing.T) {
	cases := []struct {
		name                          string
		elevation, temperature, moist float64
		want                          Terrain
	}{
		{"deep ocean", 0.05, 0.5, 0.5, TerrainDeepOcean},
		{"shallow ocean", 0.33, 0.5, 0.5, TerrainOcean},
		{"temperate beach", 0.40, 0.5, 0.5, TerrainBeach},
		{"hot dry lowland is desert", 0.55, 0.9, 0.1, TerrainDesert},
		{"hot mid lowland is savanna", 0.55, 0.9, 0.5, TerrainSavanna},
		{"hot wet lowland is jungle", 0.55, 0.9, 0.9, TerrainJungle},
		{"cold dry lowland is tundra", 0.55, 0.1, 0.1, TerrainTundra},
		{"cold mid lowland is taiga", 0.55, 0.1, 0.5, TerrainTaiga},
		{"cold wet lowland is snow", 0.55, 0.1, 0.9, TerrainSnow},
		{"temperate dry lowland is plains", 0.55, 0.5, 0.1, TerrainPlains},
		{"temperate mid lowland is grassland", 0.55, 0.5, 0.5, TerrainGrassland},
		{"temperate wet lowland is meadow", 0.55, 0.5, 0.75, TerrainMeadow},
		{"temperate very wet lowland is forest", 0.55, 0.5, 0.95, TerrainForest},
		{"hill band is hills", 0.78, 0.5, 0.5, TerrainHills},
		{"mountain band is mountain", 0.88, 0.5, 0.5, TerrainMountain},
		{"mountain band cold is snow", 0.88, 0.1, 0.5, TerrainSnow},
		{"top band is snowy peak", 0.98, 0.5, 0.5, TerrainSnowyPeak},
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

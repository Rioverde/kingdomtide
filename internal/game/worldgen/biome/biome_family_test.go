package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestFamilyOfAllTerrains iterates every Terrain value exposed by the domain
// package and asserts the family mapping is total (no panic, no unknown).
// Keeping it table-driven over game.AllTerrains guarantees that a new Terrain
// added to the domain shows up here and fails loudly until FamilyOf learns
// about it — the whole point of the catch-all bucket is to flag that gap.
func TestFamilyOfAllTerrains(t *testing.T) {
	cases := []struct {
		terrain game.Terrain
		want    BiomeFamily
	}{
		{game.TerrainDeepOcean, FamilyWater},
		{game.TerrainOcean, FamilyWater},
		{game.TerrainBeach, FamilyWater},
		{game.TerrainDesert, FamilyDesert},
		{game.TerrainSavanna, FamilyPlain},
		{game.TerrainPlains, FamilyPlain},
		{game.TerrainGrassland, FamilyPlain},
		{game.TerrainMeadow, FamilyPlain},
		{game.TerrainForest, FamilyForest},
		{game.TerrainJungle, FamilyForest},
		{game.TerrainTaiga, FamilyForest},
		{game.TerrainTundra, FamilyTundra},
		{game.TerrainSnow, FamilyTundra},
		{game.TerrainHills, FamilyMountain},
		{game.TerrainMountain, FamilyMountain},
		{game.TerrainSnowyPeak, FamilyMountain},
		{game.TerrainVolcanoCore, FamilyMountain},
		{game.TerrainVolcanoCoreDormant, FamilyMountain},
		{game.TerrainCraterLake, FamilyMountain},
		{game.TerrainVolcanoSlope, FamilyMountain},
		{game.TerrainAshland, FamilyMountain},
	}

	// Sanity: AllTerrains must be fully covered by the cases above. If the
	// domain package grows a new Terrain, the length check below fails and
	// the developer is forced to extend both this table and FamilyOf.
	if len(cases) != len(game.AllTerrains()) {
		t.Fatalf("case table length %d does not match game.AllTerrains length %d — "+
			"new Terrain value probably added without updating FamilyOf",
			len(cases), len(game.AllTerrains()))
	}

	for _, tc := range cases {
		got := FamilyOf(tc.terrain)
		if got != tc.want {
			t.Errorf("FamilyOf(%q) = %s, want %s", tc.terrain, got, tc.want)
		}
	}
}

// TestBiomeFamilyString verifies every declared family has a non-empty
// stringification. A missing name would leak an empty log line on debug
// output, which is harder to notice than a compile-time gap.
func TestBiomeFamilyString(t *testing.T) {
	families := []BiomeFamily{
		FamilyPlain,
		FamilyForest,
		FamilyMountain,
		FamilyWater,
		FamilyDesert,
		FamilyTundra,
		FamilyUnknown,
	}
	for _, f := range families {
		if s := f.String(); s == "" {
			t.Errorf("BiomeFamily(%d).String() returned empty", int(f))
		}
	}

	// Out-of-range values fall back to "unknown" rather than panic.
	if got := BiomeFamily(-1).String(); got != "unknown" {
		t.Errorf("BiomeFamily(-1).String() = %q, want %q", got, "unknown")
	}
	if got := BiomeFamily(999).String(); got != "unknown" {
		t.Errorf("BiomeFamily(999).String() = %q, want %q", got, "unknown")
	}
}

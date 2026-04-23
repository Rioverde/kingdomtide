package biome

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/world"
)

// TestFamilyOfAllTerrains iterates every Terrain value exposed by the domain
// package and asserts the family mapping is total (no panic, no unknown).
// Keeping it table-driven over world.AllTerrains guarantees that a new Terrain
// added to the domain shows up here and fails loudly until FamilyOf learns
// about it — the whole point of the catch-all bucket is to flag that gap.
func TestFamilyOfAllTerrains(t *testing.T) {
	cases := []struct {
		terrain world.Terrain
		want    BiomeFamily
	}{
		{world.TerrainDeepOcean, FamilyWater},
		{world.TerrainOcean, FamilyWater},
		{world.TerrainBeach, FamilyWater},
		{world.TerrainDesert, FamilyDesert},
		{world.TerrainSavanna, FamilyPlain},
		{world.TerrainPlains, FamilyPlain},
		{world.TerrainGrassland, FamilyPlain},
		{world.TerrainMeadow, FamilyPlain},
		{world.TerrainForest, FamilyForest},
		{world.TerrainJungle, FamilyForest},
		{world.TerrainTaiga, FamilyForest},
		{world.TerrainTundra, FamilyTundra},
		{world.TerrainSnow, FamilyTundra},
		{world.TerrainHills, FamilyMountain},
		{world.TerrainMountain, FamilyMountain},
		{world.TerrainSnowyPeak, FamilyMountain},
		{world.TerrainVolcanoCore, FamilyMountain},
		{world.TerrainVolcanoCoreDormant, FamilyMountain},
		{world.TerrainCraterLake, FamilyMountain},
		{world.TerrainVolcanoSlope, FamilyMountain},
		{world.TerrainAshland, FamilyMountain},
	}

	// Sanity: AllTerrains must be fully covered by the cases above. If the
	// domain package grows a new Terrain, the length check below fails and
	// the developer is forced to extend both this table and FamilyOf.
	if len(cases) != len(world.AllTerrains()) {
		t.Fatalf("case table length %d does not match world.AllTerrains length %d — "+
			"new Terrain value probably added without updating FamilyOf",
			len(cases), len(world.AllTerrains()))
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

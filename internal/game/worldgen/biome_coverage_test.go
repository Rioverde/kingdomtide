package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/world"
)

// TestBiomeDistributionCoverage verifies that every terrain produced by the
// full pipeline (TileAt → Biome) is reachable across a broad sample of world
// coordinates. Continent blending shifts the elevation distribution, so
// driving Biome() with raw noise samples in a fixed [0.30, 0.70] range would
// not reflect what players see. Instead we sample the live generator across
// multiple seeds — this is a stronger guarantee: not just "biome thresholds
// cover the [0, 1] interval" but "the generator actually produces every
// biome on a real map".
//
// Lives in the worldgen root package rather than the biome sub-package so it
// can exercise NewWorldGenerator without an import cycle.
func TestBiomeDistributionCoverage(t *testing.T) {
	if testing.Short() {
		t.Skip("524K tile coverage scan across 8 seeds")
	}
	const seeds = 8
	const side = 256
	const half = side / 2

	histogram := make(map[world.Terrain]int)
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
	for _, terrain := range world.AllTerrains() {
		count := histogram[terrain]
		pct := float64(count) / float64(total) * 100
		t.Logf("  %-14s %6d  (%.2f%%)", terrain, count, pct)
	}

	// Every terrain in AllTerrains must appear at least once across the sample. Rare
	// terrains (SnowyPeak, Jungle) can need a wide sample to surface — 8 seeds over
	// 256² tiles (~524k samples total) is generous and gives the test a stable pass.
	// Volcanic terrains are skipped: they are placed by the VolcanoSource override
	// pipeline rather than emitted by the base noise generator.
	for _, terrain := range world.AllTerrains() {
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
func isVolcanicTerrain(t world.Terrain) bool {
	switch t {
	case world.TerrainVolcanoCore,
		world.TerrainVolcanoCoreDormant,
		world.TerrainCraterLake,
		world.TerrainVolcanoSlope,
		world.TerrainAshland:
		return true
	default:
		return false
	}
}

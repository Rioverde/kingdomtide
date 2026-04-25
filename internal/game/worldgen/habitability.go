package worldgen

// Pure functions for scoring per-cell habitability for camp placement.
// Called once per Bridson candidate during NewCampSource construction;
// must be allocation-light because it runs O(candidates) ≈ O(map area).

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// biomeBaseScore is the per-terrain habitability prior. Zero means hard
// rejection. Terrains absent from the map return 0 via the zero-value
// of the map lookup — water, volcanic cores, and snowy peaks all fall
// through to 0 without an explicit entry.
//
// Score table per camps.md §4:
//
//	Plains, Meadow            0.95
//	Grassland                 0.85
//	Hills                     0.70
//	Beach                     0.70
//	Forest                    0.65
//	Savanna                   0.55
//	Jungle                    0.45
//	Taiga                     0.40
//	Desert                    0.25
//	Tundra                    0.20
//	Mountain                  0.20
//	Snow                      0.10
//	Ashland                   0.05
//	Everything else           0   (hard reject)
var biomeBaseScore = map[gworld.Terrain]float32{
	gworld.TerrainPlains:    0.95,
	gworld.TerrainMeadow:    0.95,
	gworld.TerrainGrassland: 0.85,
	gworld.TerrainHills:     0.70,
	gworld.TerrainBeach:     0.70,
	gworld.TerrainForest:    0.65,
	gworld.TerrainSavanna:   0.55,
	gworld.TerrainJungle:    0.45,
	gworld.TerrainTaiga:     0.40,
	gworld.TerrainDesert:    0.25,
	gworld.TerrainTundra:    0.20,
	gworld.TerrainMountain:  0.20,
	gworld.TerrainSnow:      0.10,
	gworld.TerrainAshland:   0.05,
}

// isFoodDeposit reports whether kind contributes to camp food-supply
// scoring. Fertile, Fish, and Game are food; Iron, Stone, Timber, and
// the rest are generic (helpful but lower weight).
func isFoodDeposit(kind gworld.DepositKind) bool {
	switch kind {
	case gworld.DepositFertile, gworld.DepositFish, gworld.DepositGame:
		return true
	}
	return false
}

// nearVolcano reports whether any tile within Chebyshev radius of p
// carries a volcanic terrain override. Used to detect proximity to a
// volcano beyond the hard-reject footprint gates, which filter core and
// crater-lake tiles before habitabilityAt is reached.
//
// The VolcanoSource interface exposes TerrainOverrideAt(geom.Position),
// so we scan the bounding square and probe each tile. The bounding box
// is at most (2r+1)² = 289 tiles at the default radius of 8 — cheap on
// the stack; no allocation.
func nearVolcano(p geom.Position, vs gworld.VolcanoSource, radius int) bool {
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if _, ok := vs.TerrainOverrideAt(geom.Position{X: p.X + dx, Y: p.Y + dy}); ok {
				return true
			}
		}
	}
	return false
}

// habitabilityAt returns a [0, 1]-bounded score for placing a camp at p.
// Zero means hard rejection (water, snowy peak, volcano interior).
//
// Scoring:
//
//	base                     biomeBaseScore[terrain]
//	+ campCoastBonus         if the cell is coastal
//	+ campRiverBonus         if a river runs through this tile
//	+ campFoodDepositBonus   per food deposit within campDepositSearchRadius
//	+ campGenericDepositBonus per non-food deposit within radius
//	* campVolcanoPenaltyMult if a volcano footprint tile is within campVolcanoPenaltyRadius
//
// Returns at most 1.0. The only allocation is inside DepositsNear; the
// rest is arithmetic on stack values.
func habitabilityAt(
	p geom.Position,
	w *Map,
	cellID uint32,
	deposits gworld.DepositSource,
	volcanoes gworld.VolcanoSource,
) float32 {
	base := biomeBaseScore[w.Terrain[cellID]]
	if base <= 0 {
		return 0
	}

	score := base

	if w.IsCoast(cellID) {
		score += campCoastBonus
	}
	if w.IsRiver(p.X, p.Y) {
		score += campRiverBonus
	}

	var nearby []gworld.Deposit
	if deposits != nil {
		nearby = deposits.DepositsNear(p, campDepositSearchRadius)
	}
	for _, d := range nearby {
		if isFoodDeposit(d.Kind) {
			score += campFoodDepositBonus
		} else {
			score += campGenericDepositBonus
		}
	}

	if volcanoes != nil && nearVolcano(p, volcanoes, campVolcanoPenaltyRadius) {
		score *= campVolcanoPenaltyMult
	}

	if score > 1.0 {
		return 1.0
	}
	return score
}

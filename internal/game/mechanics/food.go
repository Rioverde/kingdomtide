package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// harvestLaborDivisor converts raw population into baseline yield. A
// city of 10 people produces 1 food unit — a deliberately loose scale
// that keeps early-game numbers readable in dev logs. The exact ratio
// gets tuned once biome modifiers and trade imports land in later
// milestones.
const harvestLaborDivisor = 10

// ApplyFoodYear rolls this year's harvest outcome for one city and
// writes it into city.FoodBalance. Simplified model: base yield
// scales with population's labor pool, then a D6 weather /
// minor-event variance is applied, then the accumulated soil fatigue
// from prior years cuts the result. Tech effects: Irrigation lifts
// base yield; Calendar averages two variance rolls to shrink the
// weather band without shifting its mean. Biome modifier, trade
// import, and famine-cascade deltas join the formula in later
// milestones.
func ApplyFoodYear(city *polity.City, stream *dice.Stream) {
	baseYield := city.Population / harvestLaborDivisor
	baseYield = int(float64(baseYield) * techFoodYieldMultiplier(city))

	// D6 returns [1, 6]; centering at -3 gives variance [-2, +3].
	variance := stream.D6() - 3
	if techHarvestVarianceReduction(city) > 0 {
		// Average two rolls — same mean, tighter distribution.
		variance = (variance + stream.D6() - 3) / 2
	}
	city.FoodBalance = baseYield + variance

	applySoilFatiguePenaltyInPlace(city)
}

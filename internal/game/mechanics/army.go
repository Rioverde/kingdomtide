package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Army constants. The baseline fraction is a hard-coded 2 % because
// culture multipliers haven't shipped yet; when they do, culture
// becomes a City field and this constant turns into a per-culture
// lookup.
const (
	// armyBaselineFraction is the share of the population that every
	// city raises as a standing garrison each year.
	armyBaselineFraction = 0.02

	// armyAttritionPermille is the per-mille rate at which soldiers
	// desert when the treasury can not cover their upkeep. 30 ‰ means
	// 3 % of the force walks off each year of deficit — a year or two
	// of bankruptcy melts a quarter of the army without wiping it in
	// one step.
	armyAttritionPermille = 30
)

// ApplyArmyYear pulls the city's standing army toward its population
// baseline (2 % of population) and shrinks it when the treasury has
// run dry. Must be called AFTER ApplyEconomicYear — it reads the
// fresh Wealth the economy step produced so attrition tracks this
// year's deficit, not last year's.
//
// Growth toward baseline is asymmetric: the army never grows past the
// 2 % ceiling under this function's control. Decree-driven surges
// ("Raise Army") will add on top in a later milestone — those write
// Army directly and ApplyArmyYear leaves the inflated value alone
// because baseline capped growth can never lift the count.
func ApplyArmyYear(city *polity.City) {
	baseline := int(float64(city.Population) * armyBaselineFraction *
		techArmyBaselineMultiplier(city))
	if greatPersonOf(city, polity.GreatPersonGeneral) {
		baseline = int(float64(baseline) *
			float64(generalArmyMultPermille) / 1000.0)
	}

	// Grow up to the baseline when we have the manpower and treasury.
	if city.Army < baseline && city.Wealth >= 0 {
		city.Army = baseline
	}

	// Shrink when the treasury is in deficit — desertion models the
	// observable historical pattern of unpaid medieval armies
	// melting away between seasons.
	if city.Wealth < 0 && city.Army > 0 {
		// At least one per year, or a 10-man garrison never shrinks.
		loss := max(1, (city.Army*armyAttritionPermille)/1000)
		city.Army = max(0, city.Army-loss)
	}
}

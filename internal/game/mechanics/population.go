package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Population growth constants. The model is logistic growth capped
// at popMaxCap, modulated by the interaction of food, happiness, and
// wealth. Exact numbers are MVP placeholders — rebalanced when the
// economy deepens and plague / famine events become first-class.
const (
	// popMaxCap is the hard ceiling on any single city's headcount.
	// Beyond this, growth pressure will vent into new villages once
	// that subsystem lands.
	popMaxCap = 40000

	// popMin is the viability floor. A city that would drop below
	// this either dies in a famine event or stays stuck; we clamp to
	// this value so no city silently drifts to zero.
	popMin = 80

	// popGrowthBase is the logistic growth rate — as a permille — when
	// food, wealth, and happiness all hit neutral. 30 ‰ ≈ 3 % / year,
	// in the upper band of historical pre-industrial rates
	// (0.5 % – 4 %); the higher baseline buys the recovery headroom
	// disaster cooldowns alone cannot supply.
	popGrowthBasePermille = 30

	// Growth modulators — each knocks the base growth up or down by
	// its permille amount. Applied additively so a city with food
	// surplus AND high happiness grows faster than either alone.
	popFoodSurplusBonusPermille = 10
	popFoodDeficitPenaltyPermille = -30
	popHappinessBonusPermille     = 10
	popHappinessPenaltyPermille   = -15
	popWealthPositiveBonusPermille = 5
	popWealthNegativePenaltyPermille = -10

	// Thresholds that flip "has food surplus / deficit" etc. Kept
	// generous so we don't oscillate on tiny variance.
	foodSurplusThreshold   = 5
	foodDeficitThreshold   = -5
	happinessGoodThreshold = 60
	happinessBadThreshold  = 30
)

// ApplyPopulationYear mutates city.Population according to a
// logistic-growth model modulated by the year's food, happiness, and
// wealth outcomes. Growth is always applied as a permille (tenth of
// a percent) of current population; final result is clamped to
// [popMin, popMaxCap].
//
// Must be called AFTER ApplyFoodYear, ApplyHappinessYear, and
// ApplyEconomicYear — this function reads the fresh values those
// produced. The ordering invariant is enforced by TickCityYear.
func ApplyPopulationYear(city *polity.City) {
	growth := popGrowthBasePermille + growthModifiers(city)

	// Logistic saturation: growth tapers as population approaches cap.
	saturation := max(0, 1.0-float64(city.Population)/float64(popMaxCap))

	delta := int(float64(city.Population*growth) * saturation / 1000)
	city.Population += delta

	// Clamp to viability range.
	city.Population = min(popMaxCap, max(popMin, city.Population))
}

// growthModifiers returns the sum of permille deltas from the three
// growth-influencing factors. Separated from ApplyPopulationYear so
// each contribution is inspectable in tests.
func growthModifiers(city *polity.City) int {
	mod := 0

	switch {
	case city.FoodBalance >= foodSurplusThreshold:
		mod += popFoodSurplusBonusPermille
	case city.FoodBalance <= foodDeficitThreshold:
		mod += popFoodDeficitPenaltyPermille
	}

	switch {
	case city.Happiness >= happinessGoodThreshold:
		mod += popHappinessBonusPermille
	case city.Happiness <= happinessBadThreshold:
		mod += popHappinessPenaltyPermille
	}

	switch {
	case city.Wealth > 0:
		mod += popWealthPositiveBonusPermille
	case city.Wealth < 0:
		mod += popWealthNegativePenaltyPermille
	}

	return mod
}

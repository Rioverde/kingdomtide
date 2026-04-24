package mechanics

import (
	"math"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Prosperity weighting. Weights sum to 1.0 so the resulting
// Prosperity lives in [0, 1]. The "cap" values are what map each raw
// stat into its [0, 1] contribution. We scale Wealth / population so
// a prosperous medium city lands around 0.5 rather than clipping at
// 1.0 immediately — final tuning waits on the economy.
const (
	prosperityWeightWealth    = 0.30
	prosperityWeightTrade     = 0.20
	prosperityWeightFood      = 0.20
	prosperityWeightHappiness = 0.20
	prosperityWeightAge       = 0.10

	// Caps against which each stat is normalized to [0, 1].
	prosperityWealthCap    = 100000 // high-end treasury
	prosperityFoodCap      = 50     // very comfortable surplus
	prosperityHappinessCap = 100    // displayed ceiling
	prosperityAgeCap       = 1500   // upper end of the city-age seed range
)

// Hoisted to package level to avoid recomputing on every ApplyProsperityYear tick.
var prosperityAgeLogDivisor = math.Log10(prosperityAgeCap + 1)

// ApplyProsperityYear recomputes city.Prosperity as the weighted sum
// of its component signals. Must be called AFTER ApplyFoodYear,
// ApplyHappinessYear, and ApplyEconomicYear — it reads the
// freshly-written values.
//
// The TradeScore contribution is stubbed at 0.5 (neutral) until the
// gravity-model trade system lands. currentYear is needed to compute
// Age at call time because Age itself is a computed property on
// Settlement, not a stored field.
func ApplyProsperityYear(city *polity.City, currentYear int) {
	wealthNorm := clamp01(float64(city.Wealth) / prosperityWealthCap)

	// TradeScore is stubbed at neutral 0.5 until the trade subsystem
	// lands.
	tradeNorm := 0.5

	foodNorm := clamp01(float64(city.FoodBalance)/prosperityFoodCap/2 + 0.5)

	happinessNorm := clamp01(float64(city.Happiness) / prosperityHappinessCap)

	// Age is log-scaled to avoid old cities dominating the mix purely
	// by age. log10(0) is -Inf, so add 1 to keep the math sane for
	// freshly-founded settlements. A future-dated Founded value (older
	// than currentYear returning a negative Age) would feed a negative
	// number into Log10 and produce NaN — clamp at zero so the
	// prosperity output stays finite regardless of pathological
	// timestamps.
	age := math.Max(0, float64(city.Age(currentYear)))
	ageNorm := clamp01(math.Log10(age+1) / prosperityAgeLogDivisor)

	city.Prosperity =
		prosperityWeightWealth*wealthNorm +
			prosperityWeightTrade*tradeNorm +
			prosperityWeightFood*foodNorm +
			prosperityWeightHappiness*happinessNorm +
			prosperityWeightAge*ageNorm
}

// clamp01 folds x into [0, 1]. Exported nowhere — internal helper so
// each tick function's clamp intent is obvious without repeating the
// min(max(...)) idiom everywhere.
func clamp01(x float64) float64 {
	return min(1, max(0, x))
}

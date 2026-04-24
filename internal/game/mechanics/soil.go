package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Soil-fatigue constants. SoilFatigue is a [0, 1] scalar that
// accumulates when a city chronically over-works its cropland and
// recovers during surplus years. Beyond the food cutoff it starts
// penalizing next year's harvest.
const (
	// soilFatigueAccumRate is added to SoilFatigue each year the city
	// runs a food deficit (population > agricultural capacity).
	soilFatigueAccumRate = 0.05
	// soilFatigueRecoverRate is subtracted each year of surplus so
	// fallow / crop rotation can restore the land.
	soilFatigueRecoverRate = 0.06

	// Food-balance thresholds used as proxies for "uses > 80 %
	// capacity" (deficit) and "< 60 %" (surplus) until we have a
	// real capacity model tied to biome / deposits.
	soilDeficitThreshold = -5
	soilSurplusThreshold = 5

	// soilFatigueFoodCutoff is the SoilFatigue level at which food
	// output drops by 40 % next year. Values below leave food
	// untouched; values at or above apply the penalty.
	soilFatigueFoodCutoff  = 0.8
	soilFatigueFoodPenalty = 0.4 // 40 % cut to next-year harvest
)

// ApplySoilFatigueYear evolves city.SoilFatigue based on this year's
// FoodBalance. Must be called AFTER ApplyFoodYear — it reads the
// fresh FoodBalance. Writes stay in [0, 1] via the clamp01 helper.
//
// The food penalty itself is applied BY next year's ApplyFoodYear,
// which reads the SoilFatigue value this step wrote.
func ApplySoilFatigueYear(city *polity.City) {
	switch {
	case city.FoodBalance <= soilDeficitThreshold:
		city.SoilFatigue += soilFatigueAccumRate
	case city.FoodBalance >= soilSurplusThreshold:
		city.SoilFatigue -= soilFatigueRecoverRate
	}
	city.SoilFatigue = clamp01(city.SoilFatigue)
}

// applySoilFatiguePenaltyInPlace reduces the city's FoodBalance when
// SoilFatigue has crossed the cutoff. Called by ApplyFoodYear
// immediately after the D6 variance so the penalty applies to this
// year's total yield rather than accumulating over years.
func applySoilFatiguePenaltyInPlace(city *polity.City) {
	if city.SoilFatigue < soilFatigueFoodCutoff {
		return
	}
	city.FoodBalance = int(float64(city.FoodBalance) * (1.0 - soilFatigueFoodPenalty))
}

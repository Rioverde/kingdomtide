package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Tech-effect multipliers and offsets. Applied by the ApplyYear steps
// that consume them — kept centralised so balance tuning has one
// source of truth. Permille-scaled integer constants avoid floating
// magic numbers at call sites; call sites convert with /1000.0 when
// they need a float multiplier.
const (
	// techIrrigationFoodMultPermille lifts final food yield by 15 %
	// when Irrigation is unlocked. MECHANICS.md §6c.
	techIrrigationFoodMultPermille = 150

	// techWritingDecreeDCReduction lowers the D20 execution target of
	// a decree by this many points when Writing is unlocked.
	techWritingDecreeDCReduction = 1

	// techMetallurgyArmyMultPermille lifts the army baseline fraction
	// by 15 % when Metallurgy is unlocked.
	techMetallurgyArmyMultPermille = 150

	// techNavigationTradeBonus is the flat bonus added to TradeScore
	// after tech multipliers when Navigation is unlocked.
	techNavigationTradeBonus = 10

	// techCalendarVarianceReduction averages two harvest-variance
	// rolls when Calendar is unlocked, shrinking the variance band
	// without changing its mean.
	techCalendarVarianceReduction = 1

	// techPrintingRevolutionDCReduction lowers the revolution D20 DC
	// by this many points when Printing is unlocked — revolts fire
	// a little more often in an informed populace.
	techPrintingRevolutionDCReduction = 1

	// techPrintingSchismThresholdReduction lowers the schism
	// innovation gate by this many points when Printing is unlocked —
	// easier religious fragmentation in a literate city.
	techPrintingSchismThresholdReduction = 5

	// techBankingTradeMultPermille lifts the TradeScore multiplier to
	// 1.30 when Banking is unlocked.
	techBankingTradeMultPermille = 1300

	// techBankingTributeMultPermille halves tribute extraction (50 %)
	// when Banking is unlocked in the vassal city.
	techBankingTributeMultPermille = 500
)

// techFoodYieldMultiplier returns the per-year food multiplier from
// unlocked techs. Base 1.0, lifted by Irrigation. Multipliers stack
// multiplicatively over 1.0 so adding a second food-tech multiplies
// cleanly.
func techFoodYieldMultiplier(city *polity.City) float64 {
	mult := 1.0
	if city.Techs.Has(polity.TechIrrigation) {
		mult *= 1.0 + float64(techIrrigationFoodMultPermille)/1000.0
	}
	return mult
}

// techArmyBaselineMultiplier returns the baseline-army multiplier
// from Metallurgy — 1.15 when unlocked, 1.0 otherwise.
func techArmyBaselineMultiplier(city *polity.City) float64 {
	if city.Techs.Has(polity.TechMetallurgy) {
		return 1.0 + float64(techMetallurgyArmyMultPermille)/1000.0
	}
	return 1.0
}

// techTradeMultiplier returns the TradeScore multiplier from Banking
// — 1.30 when unlocked, 1.0 otherwise. Navigation's flat bonus is
// applied separately by the caller.
func techTradeMultiplier(city *polity.City) float64 {
	if city.Techs.Has(polity.TechBanking) {
		return float64(techBankingTradeMultPermille) / 1000.0
	}
	return 1.0
}

// techTradeFlatBonus returns the flat TradeScore bonus from
// Navigation — applied after the trade multiplier.
func techTradeFlatBonus(city *polity.City) int {
	if city.Techs.Has(polity.TechNavigation) {
		return techNavigationTradeBonus
	}
	return 0
}

// techDecreeDCReduction returns the decree execution-DC reduction
// from Writing. Applied as a subtraction against decreeExecutionDC.
func techDecreeDCReduction(city *polity.City) int {
	if city.Techs.Has(polity.TechWriting) {
		return techWritingDecreeDCReduction
	}
	return 0
}

// techHarvestVarianceReduction returns 1 when Calendar is unlocked.
// Callers interpret the non-zero result as "average two variance
// rolls" rather than a point offset, shrinking the band without
// shifting its mean.
func techHarvestVarianceReduction(city *polity.City) int {
	if city.Techs.Has(polity.TechCalendar) {
		return techCalendarVarianceReduction
	}
	return 0
}

// techRevolutionDCReduction returns the revolution-DC reduction from
// Printing. Applied as a subtraction against revolutionDC.
func techRevolutionDCReduction(city *polity.City) int {
	if city.Techs.Has(polity.TechPrinting) {
		return techPrintingRevolutionDCReduction
	}
	return 0
}

// techSchismThresholdReduction returns the innovation-gate reduction
// for schism when Printing is unlocked.
func techSchismThresholdReduction(city *polity.City) int {
	if city.Techs.Has(polity.TechPrinting) {
		return techPrintingSchismThresholdReduction
	}
	return 0
}

// techTributeRate scales baseRate by 0.5 when Banking is unlocked in
// the vassal, otherwise returns baseRate unchanged. Reduced tribute
// rate models the commercial independence Banking brings.
func techTributeRate(city *polity.City, baseRate float64) float64 {
	if city.Techs.Has(polity.TechBanking) {
		return baseRate * float64(techBankingTributeMultPermille) / 1000.0
	}
	return baseRate
}

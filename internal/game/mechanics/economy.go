package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Economic constants. Numbers are deliberately loose for the MVP —
// once we have Trade, Tribute, and Decree-driven modifiers, the
// per-capita income formula becomes a richer composition. For now a
// flat per-capita base keeps the tick testable end-to-end.
const (
	// perCapitaBaseIncome is the wealth a single citizen generates
	// before tax rate is applied. Tuned so a 1 000-person town at
	// TaxNormal (17 %) collects ~170 wealth / year — enough to fund a
	// 20-soldier garrison without dominating the budget.
	perCapitaBaseIncome = 1

	// upkeepPerSoldier is the wealth drain per standing soldier per
	// year. Tuned so an army of 40 costs ~40 wealth — comparable to a
	// mid-town's tax take.
	upkeepPerSoldier = 1

	// tradeIncomePerScore is the wealth a city collects per unit of
	// TradeScore per year. A TradeScore of 100 (maximum) yields 50
	// wealth, keeping the trade contribution around half the size of a
	// mid-tier town's tax intake. The gravity-model inter-city volume
	// calculation lands later; this is the single-city baseline.
	tradeIncomePerScore = 0.5

	// criminalWealthDrainRate is the per-year fraction of Wealth the
	// black-market siphons off per unit of Criminals faction influence.
	// At full 1.0 influence a city loses 2 %/year — a slow bleed that
	// compounds over decades without bankrupting the treasury overnight.
	criminalWealthDrainRate = 0.02
)

// ApplyEconomicYear mutates the city's Wealth by the year's net cash
// flow. Income = tax + trade (from TradeScore) + active Wealth
// historical-mod sum; outflow = standing-army upkeep. Trade gravity
// across cities lands later once the neighbor topology is in place.
//
// TradeScore is read from the prior year's ApplyTradeYear; trade income
// lags by one tick which is consistent with seasonal trade timelines.
//
// currentYear is required so the historical-mod sum can filter
// mods whose decay windows have closed.
//
// No floor is applied: a city in deficit ends the year with negative
// Wealth, which feeds future events (bankruptcy, army attrition). The
// population / happiness steps read Wealth to decide growth.
func ApplyEconomicYear(city *polity.City, currentYear int) {
	income := int(float64(city.Population*perCapitaBaseIncome) * city.TaxRate.Fraction())
	upkeep := city.Army * upkeepPerSoldier
	trade := int(float64(city.TradeScore) * tradeIncomePerScore)
	_, wealthMod, _, _ := HistoricalModSumByKind(city, currentYear)
	modSum := wealthMod
	city.Wealth += income - upkeep + trade + modSum

	// Criminals siphon a fraction of a positive treasury each year.
	// Applied after income so a negative balance does not get
	// "drained" into larger negative territory.
	if city.Wealth > 0 {
		drain := int(float64(city.Wealth) * criminalWealthDrainRate *
			city.Factions.Get(polity.FactionCriminals))
		if drain > 0 {
			city.Wealth -= drain
		}
	}
}

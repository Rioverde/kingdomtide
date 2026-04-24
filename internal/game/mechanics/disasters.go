package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Disaster DCs. Plague and Earthquake sit highest because their
// effects are the most punishing; Famine and Drought are more
// frequent because their damage is recoverable.
const (
	disasterDCPlague     = 18
	disasterDCFamine     = 16
	disasterDCEarthquake = 19
	disasterDCFlood      = 17
	disasterDCDrought    = 16
	disasterDCWildfire   = 18
)

// Effect magnitudes for natural disasters. Population and wealth
// multipliers are expressed as percentages so the integer arithmetic
// stays deterministic.
const (
	plagueMinPopPercent       = 75
	plagueMaxPopPercent       = 90
	plagueEligiblePopulation  = 200
	plagueHappinessPenalty    = -15
	famineFoodDrop            = 25
	faminePopPercent          = 95
	earthquakeWealthPercent   = 80
	earthquakeArmyPercent     = 90
	floodHappinessPenalty     = -5
	droughtFoodPercent        = 70
	droughtSoilFatigueGain    = 0.2
	droughtSoilFatigueCeiling = 1.0
	wildfireWealthPercent     = 85
	wildfirePopPercent        = 95
)

// disasterCooldownYears is the minimum gap between any two natural
// disasters firing on one city. Without it, plagues and famines
// can cascade across consecutive years and wipe populations below
// recovery thresholds.
const disasterCooldownYears = 10

// Decay windows for the historical mods queued by disasters.
// Permanent consequences (population loss, one-shot wealth
// destruction, soil-fatigue accumulation) stay as direct mutations.
// These windows apply only to civic mood and structural recovery
// lags — food-supply shocks, trade disruption, rebuilding time.
const (
	// plagueDecayYears is how long collective grief keeps happiness
	// depressed after a plague. The population loss itself is
	// permanent — this is strictly the civic-mood aftershock.
	plagueDecayYears = 5

	// famineFoodDecayYears is how long the harvest disruption
	// weighs on the next seasons. Population loss is permanent;
	// this is the food-balance drag as stores are replenished.
	famineFoodDecayYears = 3

	// earthquakeArmyLossMagnitude is the decaying army-size drag
	// applied on top of the direct headcount cut. Models broken
	// barracks / wounded garrisons recovering over a decade.
	earthquakeArmyLossMagnitude = -20
	earthquakeArmyDecayYears    = 10

	// floodTradeHappinessPenalty is the trade-disruption mood dent
	// while damaged roads / bridges are repaired.
	floodTradeHappinessPenalty = -3
	floodTradeDecayYears       = 3

	// droughtFoodMagnitude is the lingering food-supply drag after
	// the direct harvest hit. Soil-fatigue accumulation is
	// permanent; the food-balance dip decays on a 3-year horizon
	// as granaries are refilled from surplus years.
	droughtFoodMagnitude = -10
	droughtFoodDecayYears = 3

	// wildfireTradeHappinessPenalty is the commercial disruption
	// mood dent while markets and storehouses are rebuilt.
	wildfireTradeHappinessPenalty = -3
	wildfireTradeDecayYears       = 3
)

// disasters is the ordered natural-disaster table. All entries are
// Natural so the cascade cap is one natural event per year.
var disasters = []Event{
	plagueEvent(),
	famineEvent(),
	earthquakeEvent(),
	floodEvent(),
	droughtEvent(),
	wildfireEvent(),
}

// ApplyNaturalDisastersYear rolls the six natural disasters against
// the city. All entries are Natural so only one may fire per year
// under the natural-cascade cap. Order of the table matters: the
// first disaster whose DC check succeeds consumes the single natural
// slot. currentYear threads through for event handlers that need it.
//
// Honors the multi-year cooldown stored on City.LastDisasterYear —
// cities that suffered a disaster within disasterCooldownYears skip
// the entire table this tick. When a disaster does fire the cooldown
// stamp rolls forward to currentYear.
func ApplyNaturalDisastersYear(city *polity.City, stream *dice.Stream, currentYear int) {
	if city.LastDisasterYear != 0 &&
		currentYear-city.LastDisasterYear < disasterCooldownYears {
		return
	}
	fired := applyEventTable(city, stream, disasters, currentYear)
	if fired > 0 {
		city.LastDisasterYear = currentYear
	}
}

// plagueEvent wipes 20–40 % of the population and tanks happiness.
// Requires a minimum population so tiny settlements don't get
// halved to nothing — the famine path handles that tier.
//
// Permanent mutation: population reduction. Decaying mod: civic
// grief depresses happiness for several years.
func plagueEvent() Event {
	return Event{
		Name:    "Plague",
		DC:      disasterDCPlague,
		Natural: true,
		EligibleFn: func(c *polity.City) bool {
			return c.Population > plagueEligiblePopulation
		},
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			band := plagueMaxPopPercent - plagueMinPopPercent
			// D20 [1, 20] mapped onto [plagueMinPopPercent,
			// plagueMaxPopPercent] via (d-1)*band/19.
			percent := plagueMinPopPercent + (s.D20()-1)*band/19
			c.Population = c.Population * percent / 100
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   plagueHappinessPenalty,
				YearApplied: currentYear,
				DecayYears:  plagueDecayYears,
			})
		},
	}
}

// famineEvent drops food balance and trims population by 10 %. Less
// punishing than plague but no population floor — even small towns
// starve.
//
// Permanent mutation: population reduction. Decaying mod: harvest
// disruption depresses food balance for a few years while granaries
// are replenished.
func famineEvent() Event {
	return Event{
		Name:    "Famine",
		DC:      disasterDCFamine,
		Natural: true,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.Population = c.Population * faminePopPercent / 100
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModFoodBalance,
				Magnitude:   -famineFoodDrop,
				YearApplied: currentYear,
				DecayYears:  famineFoodDecayYears,
			})
		},
	}
}

// earthquakeEvent rubbles 20 % of wealth and 10 % of standing army.
// Infrastructure hit manifests as wealth loss pending a dedicated
// buildings-damaged field. Wealth multiplier is gated behind a
// positive-wealth check — debts do not disappear in earthquakes.
//
// Permanent mutations: wealth and army headcount reductions.
// Decaying mod: rebuild-time army drag on top of the direct hit
// while wounded soldiers convalesce and barracks are repaired.
func earthquakeEvent() Event {
	return Event{
		Name:    "Earthquake",
		DC:      disasterDCEarthquake,
		Natural: true,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			if c.Wealth > 0 {
				c.Wealth = c.Wealth * earthquakeWealthPercent / 100
			}
			c.Army = c.Army * earthquakeArmyPercent / 100
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModArmy,
				Magnitude:   earthquakeArmyLossMagnitude,
				YearApplied: currentYear,
				DecayYears:  earthquakeArmyDecayYears,
			})
		},
	}
}

// floodEvent zeroes food balance and dents happiness. Earlier drafts
// wrote a TradeScore penalty here, but ApplyTradeYear runs at
// tick-end and fully recomputes TradeScore from population and
// placeholder signals — the event delta was wiped each year. Trade
// disruption becomes a transient flag once the trade-route system
// grows a proper damaged-state model; for now flood expresses its
// economic bite through the food and happiness channels.
//
// Permanent mutation: this year's food balance zeroed. Decaying
// mods: happiness dent from the immediate crisis, plus a short-
// lived trade-disruption happiness drag while bridges and roads
// are repaired.
func floodEvent() Event {
	return Event{
		Name:    "Flood",
		DC:      disasterDCFlood,
		Natural: true,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.FoodBalance = 0
			c.HistoricalMods = append(c.HistoricalMods,
				polity.HistoricalMod{
					Kind:        polity.HistoricalModHappiness,
					Magnitude:   floodHappinessPenalty,
					YearApplied: currentYear,
					DecayYears:  floodTradeDecayYears,
				},
				polity.HistoricalMod{
					Kind:        polity.HistoricalModHappiness,
					Magnitude:   floodTradeHappinessPenalty,
					YearApplied: currentYear,
					DecayYears:  floodTradeDecayYears,
				},
			)
		},
	}
}

// droughtEvent halves-plus food balance and slams soil fatigue up
// by 0.2. Soil fatigue persists across years so drought's scar
// lingers — a deliberate multi-year cascade hook.
//
// Permanent mutations: this year's food balance slashed, soil
// fatigue accumulated. Decaying mod: food-supply drag over the
// next few harvests while surplus years refill stores.
func droughtEvent() Event {
	return Event{
		Name:    "Drought",
		DC:      disasterDCDrought,
		Natural: true,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.FoodBalance = c.FoodBalance * droughtFoodPercent / 100
			c.SoilFatigue = min(droughtSoilFatigueCeiling,
				c.SoilFatigue+droughtSoilFatigueGain)
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModFoodBalance,
				Magnitude:   droughtFoodMagnitude,
				YearApplied: currentYear,
				DecayYears:  droughtFoodDecayYears,
			})
		},
	}
}

// wildfireEvent burns 15 % of wealth and kills 5 % of population.
// Lighter touch than earthquake but hits population directly. Wealth
// multiplier is gated behind a positive-wealth check — debts do not
// disappear in wildfires.
//
// Permanent mutations: wealth and population reductions. Decaying
// mod: short-lived mood dent from market disruption while shops
// and storehouses are rebuilt.
func wildfireEvent() Event {
	return Event{
		Name:    "Wildfire",
		DC:      disasterDCWildfire,
		Natural: true,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			if c.Wealth > 0 {
				c.Wealth = c.Wealth * wildfireWealthPercent / 100
			}
			c.Population = c.Population * wildfirePopPercent / 100
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   wildfireTradeHappinessPenalty,
				YearApplied: currentYear,
				DecayYears:  wildfireTradeDecayYears,
			})
		},
	}
}

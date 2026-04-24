package polity

import "github.com/Rioverde/gongeons/internal/game/geom"

// City is a named urban centre — the primary simulation unit of the
// political tier. Embeds Settlement for identity and demographics, adds
// Wealth / Army / Ruler / rank for the tier-aware mechanics. Derived
// attributes (TradeScore, FoodBalance, Happiness, Prosperity) are not
// stored here; they live in the mechanics layer and are recomputed each
// year from City's raw state.
type City struct {
	Settlement

	Ruler Ruler `json:"ruler"`

	Wealth int `json:"wealth"`
	Army   int `json:"army"`

	// FoodBalance is this year's harvest outcome: positive means
	// surplus (feeds population growth, fills granaries), negative
	// means deficit (triggers famine events, reduces growth).
	// Recomputed every simulated year by mechanics.ApplyFoodYear.
	FoodBalance int `json:"food_balance"`

	// TradeScore is the continuous [0, 100] index of mercantile
	// health. Aggregates neighbor-city density, deposit variety,
	// water adjacency, population, and road / river connectivity.
	// Feeds the gravity-model trade volume and contributes to
	// Prosperity. Recomputed every simulated year by
	// mechanics.ApplyTradeYear.
	TradeScore int `json:"trade_score"`

	// Happiness is the raw (un-clamped) civic mood score. The
	// revolution dispatcher reads the raw value so compounding
	// grievances register correctly below zero; UI clamps to
	// [0, 100] for display. Recomputed every year by
	// mechanics.ApplyHappinessYear.
	Happiness int `json:"happiness"`

	// TaxRate is the ruler's chosen fiscal policy. Drives tax income in
	// mechanics.ApplyEconomicYear and a happiness penalty in
	// mechanics.ApplyHappinessYear. Defaults to TaxNormal.
	TaxRate TaxRate `json:"tax_rate"`

	// Prosperity is the weighted composite [0, 1] derived from Wealth,
	// FoodBalance, Happiness, and Age. Recomputed at the end of each
	// year's tick by mechanics.ApplyProsperityYear.
	Prosperity float64 `json:"prosperity"`

	// SoilFatigue tracks accumulated over-cultivation damage in
	// [0, 1]. Grows when population chronically exceeds agricultural
	// capacity, recovers under surplus. Values above
	// soilFatigueFoodCutoff penalize next year's FoodBalance. Updated
	// each year by mechanics.ApplySoilFatigueYear.
	SoilFatigue float64 `json:"soil_fatigue"`

	// RevolutionThisYear is a one-year latch set by the
	// revolution-check step when a revolt fires. Downstream consumers
	// (UI banners, event log, faction reactions) read the flag during
	// the same tick; the next tick's revolution check clears it at
	// the top. Not persisted across save/load, which is why it is a
	// transient bool, not a historical mod.
	RevolutionThisYear bool `json:"revolution_this_year"`

	// LastDisasterYear is the most recent simulated year a natural
	// disaster (plague, famine, earthquake, flood, drought, wildfire)
	// fired in this city. The natural-disaster tick consults it to
	// honor a multi-year cooldown so consecutive Plague events cannot
	// wipe a population in back-to-back years.
	LastDisasterYear int `json:"last_disaster_year"`

	BaseRank      BaseRank      `json:"base_rank"`
	EffectiveRank EffectiveRank `json:"effective_rank"`

	// Deposits holds the city's active mineral deposits. Each deposit
	// drains over time based on mining activity; when RemainingYield
	// falls below the exhaustion threshold the deposit is removed.
	// Recomputed every simulated year by
	// mechanics.ApplyMineralDepletionYear.
	Deposits []Deposit `json:"deposits,omitempty"`

	// Innovation is the city's continuous [0, 100+] technology
	// progress score. Grows each year modulated by INT modifier,
	// Mages faction influence, and Green Sage faith bonus. Crosses
	// tech thresholds unlock entries in Techs. Recomputed yearly by
	// mechanics.ApplyTechnologyYear.
	Innovation float64 `json:"innovation"`

	// Techs is the bitmask of technologies this city has unlocked.
	// Grows monotonically — techs cannot be lost once earned.
	Techs TechMask `json:"techs"`

	// GreatPerson is the currently-hosted great person, nil when the
	// city has none. A city may host at most one great person at a
	// time. Populated / cleared by mechanics.ApplyGreatPeopleYear.
	GreatPerson *GreatPerson `json:"great_person,omitempty"`

	// Factions is the per-faction influence map. Each value is
	// independent in [0, 1]; they do not need to sum to 1. Drifts
	// ±0.05/yr via mechanics.ApplyFactionDriftYear.
	Factions FactionInfluence `json:"factions"`

	// Culture is the civilization archetype of this city — feudal,
	// steppe, celtic, etc. Drives succession-law defaults and the
	// Mulk cultural-gravitation mechanic when the kingdom's culture
	// differs from the city's.
	Culture Culture `json:"culture"`

	// Faiths is the religion-distribution map. Sum of values equals
	// 1.0 within floating tolerance. Majority drives UI display and
	// interacts with the schism four-gate model. Evolves via
	// mechanics.ApplyReligionDiffusionYear.
	Faiths FaithDistribution `json:"faiths"`

	// FaithHistory lists every schism event that has altered the
	// city's faith distribution. Append-only; used by UI for historical
	// flavour and by future variant-faith creation.
	FaithHistory []SchismEvent `json:"faith_history,omitempty"`

	// Fortifications lists the city's defensive structures. Each entry
	// contributes to siege resistance and army effectiveness. Built via
	// decrees or events; persists across ticks until destroyed by
	// disasters or sieges (future work).
	Fortifications []Fortification `json:"fortifications,omitempty"`

	// HistoricalMods is the queue of time-decaying effects from past
	// events. Each mod contributes to one target field (Happiness,
	// Wealth, Army, FoodBalance) while active, then is pruned by
	// ApplyRecrystallizeYear once its decay window closes.
	HistoricalMods []HistoricalMod `json:"historical_mods,omitempty"`
}

// NewCity constructs a City at the given anchor with a fresh Ruler.
// Returns a pointer because the tick loop mutates City fields
// (Wealth after tax, Army after levy, ranks after dominance compute);
// value semantics would silently drop those updates at every call-site
// that forgot to take the address. The minimal "just-founded"
// constructor — Population, Wealth, Army, and ranks default to zero.
// Pass-1 attribute derivation lives in the mechanics layer and seeds
// those fields as the world-generation pipeline runs.
// historicalModsInitialCap is the starting capacity for a fresh
// city's HistoricalMod queue. Steady-state tracks 30-50 mods under
// 50-year cities; 32 keeps the initial NewCity allocation within one
// backing array — avoiding the 3-4 reallocs that the old cap of 16
// triggered as tournament, council, scandal, and marriage events
// collectively push the queue past 16 in the first active decades.
const historicalModsInitialCap = 32

func NewCity(name string, pos geom.Position, founded int, ruler Ruler) *City {
	return &City{
		Settlement: Settlement{
			Name:     name,
			Position: pos,
			Founded:  founded,
		},
		Ruler:          ruler,
		Faiths:         NewFaithDistribution(),
		HistoricalMods: make([]HistoricalMod, 0, historicalModsInitialCap),
	}
}

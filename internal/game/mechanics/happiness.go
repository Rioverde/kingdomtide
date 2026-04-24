package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Happiness-composition constants. Composed of a base civic mood
// plus bounded contributions; we model the subset that our current
// fields can express. Religion / faction / decree / siege deltas are
// added as their source systems land.
const (
	// happinessBase is the civic-mood baseline every year starts
	// from. Positive so cities default to mildly content —
	// compounded negatives are needed to trigger revolt.
	happinessBase = 50

	// happinessFoodWeight converts FoodBalance into a happiness
	// contribution. A surplus of 10 food adds +10 happiness; a deficit
	// of 10 subtracts 10. Clamped by happinessFoodBound below so a
	// single factor can't blow the whole mood up.
	happinessFoodWeight = 1

	// happinessFoodBound caps how much the food factor alone can push
	// happiness in a single year. The ±15 band keeps an outlier
	// harvest from eclipsing every other mood input.
	happinessFoodBound = 15

	// happinessModPositiveBound caps the NET positive contribution
	// from active Happiness HistoricalMods. Multiple subsystems
	// (decrees, life events, great people, disasters) can queue
	// same-kind mods; without a ceiling the sum drifts high enough
	// to float a Brutal-tax city above the revolt ceiling. Negative
	// sums are NOT clamped — compounding grievances must remain
	// visible to the revolution dispatcher.
	happinessModPositiveBound = 10

	// happinessPositiveContribCap caps the SUM of non-food, non-base,
	// non-tax positive contributions (Charisma, religion match,
	// Merchants faction, Great Person, positive historical mods).
	// Without this cap a city with aligned faith + Merchants majority
	// + charismatic ruler + GP + positive decree mods can stack +28
	// happiness, which neutralizes the -20 Brutal-tax penalty and
	// makes brutal-tax revolts vanishingly rare. Cap at +15 keeps the
	// Brutal-Low tax-rate differential meaningful across 100 yr of
	// simulation (revolt-eligible windows stay common under Brutal)
	// while still letting all individual factors stay as designed.
	// Negative contributions are NOT clamped — stacked grievances
	// must remain visible to the revolution dispatcher.
	happinessPositiveContribCap = 15

	// Charisma-happiness thresholds. A ruler below happinessCharismaCommonThreshold
	// is indistinguishable from an average one in civic mood; above the
	// thresholds a goodwill bonus flows through the yearly recompute.
	happinessCharismaCommonThreshold = 12
	happinessCharismaHighThreshold   = 14
	happinessCharismaMaxThreshold    = 18
	happinessCharismaCommonBonus     = 1
	happinessCharismaHighBonus       = 2
	happinessCharismaMaxBonus        = 3

	// Religion alignment: ruler faith matching majority → +8; mismatch → -8.
	happinessReligionMatchBonus      = 8
	happinessReligionMismatchPenalty = -8

	// Faction influence contributions to mood.
	happinessMerchantsBonus  = 6  // mercantile prosperity → civic mood
	happinessMilitaryPenalty = -6 // militant politicking → fatigue

	// Great Person boost — a living notable lifts the spirit.
	happinessGreatPersonBonus = 3
)

// ApplyHappinessYear recomputes the city's raw happiness for the
// year. Factors: base civic mood, food surplus/deficit (clamped to
// ±15), the tax-rate delta, the ruler's charisma bonus (0–3),
// religion alignment (±8), faction influence (Merchants +6,
// Military up to -6), a great-person presence bonus (+3), and the
// sum of currently-active Happiness historical mods queued by past
// events (positive mod contribution capped at +10). The SUM of the
// positive-side extras (CHA + religion match + Merchants + GP +
// positive mods) is further capped at happinessPositiveContribCap
// (+20) so stacked bonuses cannot neutralize the Brutal-tax penalty.
// Negative contributions (mismatched faith, Military faction,
// negative mods) are NOT capped.
//
// currentYear is required so the historical-mod sum can filter
// mods whose decay windows have closed; callers thread the
// simulation year through TickCityYear.
//
// The raw (un-clamped) value is written to city.Happiness so the
// revolution dispatcher can read extreme negative moods. UI callers
// that want a displayable integer in [0, 100] clamp the value
// themselves; see the UI package's rankLabel for the pattern.
func ApplyHappinessYear(city *polity.City, currentYear int) {
	foodDelta := city.FoodBalance * happinessFoodWeight
	foodDelta = min(max(foodDelta, -happinessFoodBound), happinessFoodBound)

	modSum := HistoricalModSum(city, polity.HistoricalModHappiness, currentYear)
	modPositive := max(0, modSum)
	if modPositive > happinessModPositiveBound {
		modPositive = happinessModPositiveBound
	}
	modNegative := min(0, modSum)

	religion := religionAlignmentContribution(city)
	factions := factionInfluenceContribution(city)

	positiveExtras := charismaContribution(city) +
		max(0, religion) +
		max(0, factions) +
		greatPersonContribution(city) +
		modPositive
	negativeExtras := min(0, religion) +
		min(0, factions) +
		modNegative

	positiveExtras = min(happinessPositiveContribCap, positiveExtras)

	city.Happiness = happinessBase +
		foodDelta +
		city.TaxRate.HappinessDelta() +
		positiveExtras +
		negativeExtras
}

// charismaContribution returns a happiness bonus for a ruler whose
// Charisma crosses the charismatic threshold. A charismatic leader
// (CHA ≥ 14) projects goodwill — citizens tolerate more hardship.
// Returns a clean integer in [0, 3] so the mood envelope stays
// predictable across simulation years.
func charismaContribution(city *polity.City) int {
	cha := city.Ruler.Stats.Charisma
	switch {
	case cha >= happinessCharismaMaxThreshold:
		return happinessCharismaMaxBonus
	case cha >= happinessCharismaHighThreshold:
		return happinessCharismaHighBonus
	case cha >= happinessCharismaCommonThreshold:
		return happinessCharismaCommonBonus
	default:
		return 0
	}
}

// religionAlignmentContribution returns happinessReligionMatchBonus
// when the ruler's faith matches the city's majority faith, the
// mismatch penalty when it diverges, and zero when no faiths are
// configured yet.
func religionAlignmentContribution(city *polity.City) int {
	if city.Faiths.IsZero() {
		return 0
	}
	if city.Ruler.Faith == city.Faiths.Majority() {
		return happinessReligionMatchBonus
	}
	return happinessReligionMismatchPenalty
}

// factionInfluenceContribution weights Merchants vs Military.
// Scales by each faction's influence in [0, 1]; a fully
// Merchant-dominated city earns +6, a fully Military-dominated one
// earns -6. Intermediate values scale proportionally.
func factionInfluenceContribution(city *polity.City) int {
	merch := city.Factions.Get(polity.FactionMerchants)
	mil := city.Factions.Get(polity.FactionMilitary)
	return int(merch*float64(happinessMerchantsBonus)) +
		int(mil*float64(happinessMilitaryPenalty))
}

// greatPersonContribution returns happinessGreatPersonBonus when
// the city currently hosts an alive great person of any archetype.
func greatPersonContribution(city *polity.City) int {
	if city.GreatPerson == nil || !city.GreatPerson.Alive() {
		return 0
	}
	return happinessGreatPersonBonus
}

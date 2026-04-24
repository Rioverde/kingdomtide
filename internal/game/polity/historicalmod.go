package polity

// HistoricalModKind tags what a mod adjusts — happiness, wealth, army,
// food. Each concrete event appends a mod of the appropriate kind
// rather than mutating the field in-place, so the effect decays over
// time rather than being silently overwritten by the next yearly
// recompute.
type HistoricalModKind uint8

const (
	HistoricalModHappiness HistoricalModKind = iota
	HistoricalModWealth
	HistoricalModArmy
	HistoricalModFoodBalance
)

// String returns the English identifier for the kind — dev-only,
// player-visible text via the client i18n catalog.
func (k HistoricalModKind) String() string {
	switch k {
	case HistoricalModHappiness:
		return "Happiness"
	case HistoricalModWealth:
		return "Wealth"
	case HistoricalModArmy:
		return "Army"
	case HistoricalModFoodBalance:
		return "FoodBalance"
	default:
		return "UnknownHistoricalModKind"
	}
}

// HistoricalMod is a time-decaying delta queued by a past event.
// Magnitude is the value added to the target field each year the
// mod is active; DecayYears is the lifetime after which the mod is
// pruned by ApplyRecrystallizeYear. YearApplied tracks when the mod
// was created so the pruning step can compare against currentYear.
type HistoricalMod struct {
	Kind        HistoricalModKind
	Magnitude   int
	YearApplied int
	DecayYears  int // active for [YearApplied, YearApplied+DecayYears)
}

// Active reports whether this mod still contributes at the given
// simulation year. Inactive mods are candidates for removal by the
// recrystallize step.
func (m HistoricalMod) Active(currentYear int) bool {
	return currentYear-m.YearApplied < m.DecayYears
}

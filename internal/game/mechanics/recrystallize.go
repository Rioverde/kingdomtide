package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// ApplyRecrystallizeYear prunes historical modifiers whose decay
// windows have closed by currentYear. Writes a fresh, compacted
// slice back to city.HistoricalMods so memory stays bounded across
// millennium-scale runs.
//
// Must run AFTER every step that queues new mods so this year's
// fresh mods are preserved; placed near the end of TickCityYear.
func ApplyRecrystallizeYear(city *polity.City, currentYear int) {
	if len(city.HistoricalMods) == 0 {
		return
	}
	kept := city.HistoricalMods[:0]
	for _, m := range city.HistoricalMods {
		if m.Active(currentYear) {
			kept = append(kept, m)
		}
	}
	city.HistoricalMods = kept
}

// HistoricalModSum totals the currently-active mods of a given kind
// for one city. Called by happiness / wealth / army / food yearly
// recomputes to fold past-event deltas into the current result.
func HistoricalModSum(city *polity.City, kind polity.HistoricalModKind, currentYear int) int {
	total := 0
	for _, m := range city.HistoricalMods {
		if m.Kind == kind && m.Active(currentYear) {
			total += m.Magnitude
		}
	}
	return total
}

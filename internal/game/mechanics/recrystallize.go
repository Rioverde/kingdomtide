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
	// kept shares the current backing array; the compaction loop keeps at most
	// len(city.HistoricalMods) items, which never exceeds cap(kept), so no
	// reallocation can fire here. Allocation pressure lives in the event
	// subsystems that append new mods; historicalModsInitialCap in NewCity
	// covers the steady-state count without a pre-grow here.
	kept := city.HistoricalMods[:0]
	for _, m := range city.HistoricalMods {
		if m.Active(currentYear) {
			kept = append(kept, m)
		}
	}
	city.HistoricalMods = kept
}

// HistoricalModSumByKind returns the currently-active mod totals for each kind in a single pass.
func HistoricalModSumByKind(city *polity.City, currentYear int) (happiness, wealth, army, foodBalance int) {
	for _, m := range city.HistoricalMods {
		if !m.Active(currentYear) {
			continue
		}
		switch m.Kind {
		case polity.HistoricalModHappiness:
			happiness += m.Magnitude
		case polity.HistoricalModWealth:
			wealth += m.Magnitude
		case polity.HistoricalModArmy:
			army += m.Magnitude
		case polity.HistoricalModFoodBalance:
			foodBalance += m.Magnitude
		}
	}
	return
}

// HistoricalModSum totals currently-active mods of one kind on a city.
func HistoricalModSum(city *polity.City, kind polity.HistoricalModKind, currentYear int) int {
	h, w, a, f := HistoricalModSumByKind(city, currentYear)
	switch kind {
	case polity.HistoricalModHappiness:
		return h
	case polity.HistoricalModWealth:
		return w
	case polity.HistoricalModArmy:
		return a
	case polity.HistoricalModFoodBalance:
		return f
	}
	return 0
}

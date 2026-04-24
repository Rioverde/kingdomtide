package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyRecrystallizeYear_PrunesExpired verifies that mods whose
// decay windows have closed are dropped while still-active mods
// survive the compaction pass.
func TestApplyRecrystallizeYear_PrunesExpired(t *testing.T) {
	c := polity.City{
		HistoricalMods: []polity.HistoricalMod{
			{Kind: polity.HistoricalModHappiness, Magnitude: -5, YearApplied: 1490, DecayYears: 5},
			{Kind: polity.HistoricalModHappiness, Magnitude: 3, YearApplied: 1498, DecayYears: 10},
		},
	}
	ApplyRecrystallizeYear(&c, 1500)
	// First mod: 1500 - 1490 = 10, DecayYears 5 → expired. Pruned.
	// Second: 1500 - 1498 = 2, DecayYears 10 → active. Kept.
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 active mod, got %d", len(c.HistoricalMods))
	}
}

// TestHistoricalModSum_OnlyActiveCounted verifies that the summing
// helper honours both the kind filter and the decay window — only
// active mods of the requested kind contribute to the sum.
func TestHistoricalModSum_OnlyActiveCounted(t *testing.T) {
	c := polity.City{
		HistoricalMods: []polity.HistoricalMod{
			{Kind: polity.HistoricalModHappiness, Magnitude: -5, YearApplied: 1490, DecayYears: 5},  // expired
			{Kind: polity.HistoricalModHappiness, Magnitude: 3, YearApplied: 1498, DecayYears: 10},  // active
			{Kind: polity.HistoricalModWealth, Magnitude: 100, YearApplied: 1499, DecayYears: 5},    // different kind
		},
	}
	got := HistoricalModSum(&c, polity.HistoricalModHappiness, 1500)
	if got != 3 {
		t.Errorf("sum = %d, want 3", got)
	}
}

// TestApplyRecrystallizeYear_EmptyIsNoop verifies the helper is
// safe on a freshly-constructed city with no queued mods.
func TestApplyRecrystallizeYear_EmptyIsNoop(t *testing.T) {
	c := polity.City{}
	ApplyRecrystallizeYear(&c, 1500)
	if c.HistoricalMods != nil && len(c.HistoricalMods) != 0 {
		t.Errorf("empty mods should remain empty")
	}
}

// TestHistoricalMod_Active pins the active-window semantics:
// [YearApplied, YearApplied+DecayYears) — inclusive on the start,
// exclusive on the end.
func TestHistoricalMod_Active(t *testing.T) {
	m := polity.HistoricalMod{YearApplied: 1500, DecayYears: 10}
	if !m.Active(1509) {
		t.Error("year 1509 should be active (9 < 10)")
	}
	if m.Active(1510) {
		t.Error("year 1510 should be expired (10 not < 10)")
	}
}

// TestApplyHappinessYear_HistoricalModsApplied verifies the
// happiness recompute folds an active Happiness mod into the
// final value.
func TestApplyHappinessYear_HistoricalModsApplied(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	c.HistoricalMods = []polity.HistoricalMod{
		{Kind: polity.HistoricalModHappiness, Magnitude: -10, YearApplied: 1498, DecayYears: 5},
	}
	ApplyHappinessYear(&c, 1500)
	// base 50 + 0 food + 0 tax + 0 cha + (-10 mod) = 40
	if c.Happiness != 40 {
		t.Errorf("Happiness = %d, want 40 (with -10 mod)", c.Happiness)
	}
}

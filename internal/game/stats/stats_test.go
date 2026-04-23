package stats

import (
	"errors"
	"testing"
)

// TestModifierBoundary probes the D&D 5e ability modifier at every
// interesting inflection point, including the negative-floor edge case
// that exposes the Go truncate-toward-zero quirk.
func TestModifierBoundary(t *testing.T) {
	cases := []struct {
		stat int
		want int
	}{
		{0, -5},
		{1, -5},
		{3, -4},
		{5, -3},
		{8, -1},
		{9, -1},
		{10, 0},
		{11, 0},
		{12, 1},
		{13, 1},
		{14, 2},
		{15, 2},
		{16, 3},
		{18, 4},
		{20, 5},
	}
	for _, tc := range cases {
		if got := Modifier(tc.stat); got != tc.want {
			t.Errorf("Modifier(%d) = %d, want %d", tc.stat, got, tc.want)
		}
	}
}

// TestNewStatsPointBuy_ValidBaseline verifies a simple balanced
// distribution. The PHB "standard array" of (15, 14, 13, 12, 10, 8) maps
// to 9+7+5+4+2+0 = 27 points.
func TestNewStatsPointBuy_ValidBaseline(t *testing.T) {
	cs, err := NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cs.Strength != 15 || cs.Dexterity != 14 || cs.Constitution != 13 ||
		cs.Intelligence != 12 || cs.Wisdom != 10 || cs.Charisma != 8 {
		t.Errorf("unexpected stats: %+v", cs)
	}
}

// TestNewStatsPointBuy_ExactBudget sweeps three distinct valid
// distributions that all sum to 27 to ensure the validator does not
// special-case any single layout.
func TestNewStatsPointBuy_ExactBudget(t *testing.T) {
	cases := []struct {
		name               string
		s, d, c, i, w, cha int
	}{
		{"standard_array", 15, 14, 13, 12, 10, 8},
		{"max_three", 15, 15, 15, 8, 8, 8},
		{"mixed", 15, 13, 13, 12, 10, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewStatsPointBuy(tc.s, tc.d, tc.c, tc.i, tc.w, tc.cha); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestNewStatsPointBuy_RejectsBudgetMismatch verifies that any total
// other than 27 surfaces as ErrPointBuyBudget.
func TestNewStatsPointBuy_RejectsBudgetMismatch(t *testing.T) {
	_, err := NewStatsPointBuy(8, 12, 13, 13, 13, 13)
	if !errors.Is(err, ErrPointBuyBudget) {
		t.Errorf("below budget: got %v, want ErrPointBuyBudget", err)
	}
	_, err = NewStatsPointBuy(15, 15, 15, 11, 8, 8)
	if !errors.Is(err, ErrPointBuyBudget) {
		t.Errorf("above budget: got %v, want ErrPointBuyBudget", err)
	}
}

// TestNewStatsPointBuy_RejectsOutOfRange verifies that scores outside
// [pointBuyMin, pointBuyMax] trip ErrPointBuyRange regardless of how
// the rest of the distribution balances.
func TestNewStatsPointBuy_RejectsOutOfRange(t *testing.T) {
	_, err := NewStatsPointBuy(7, 14, 13, 13, 12, 10)
	if !errors.Is(err, ErrPointBuyRange) {
		t.Errorf("too low: got %v, want ErrPointBuyRange", err)
	}
	_, err = NewStatsPointBuy(16, 8, 8, 8, 8, 8)
	if !errors.Is(err, ErrPointBuyRange) {
		t.Errorf("too high: got %v, want ErrPointBuyRange", err)
	}
}

// TestNewStatsPointBuy_AllEights_Rejects: all eights produces a total of
// zero points, which is not 27. Rejected with ErrPointBuyBudget.
func TestNewStatsPointBuy_AllEights_Rejects(t *testing.T) {
	_, err := NewStatsPointBuy(8, 8, 8, 8, 8, 8)
	if !errors.Is(err, ErrPointBuyBudget) {
		t.Errorf("all eights: got %v, want ErrPointBuyBudget", err)
	}
}

// TestCoreStats_Derivations checks that DefaultCoreStats flows through
// every derived-stat accessor with the zero-modifier baselines.
func TestCoreStats_Derivations(t *testing.T) {
	s := DefaultCoreStats()
	if got := s.MaxHP(); got != baseHP {
		t.Errorf("MaxHP() = %d, want %d", got, baseHP)
	}
	if got := s.Mana(); got != baseMana {
		t.Errorf("Mana() = %d, want %d", got, baseMana)
	}
	if got := s.BaseDamage(); got != weaponDamage {
		t.Errorf("BaseDamage() = %d, want %d", got, weaponDamage)
	}
	if got := s.DerivedSpeed(); got != SpeedNormal {
		t.Errorf("DerivedSpeed() = %d, want %d", got, SpeedNormal)
	}
	if got := s.DerivedInitiative(); got != 0 {
		t.Errorf("DerivedInitiative() = %d, want 0", got)
	}
}

// TestCoreStats_DerivationsWithModifiers exercises the non-zero
// modifier paths so the scale constants are tested beyond the zero case.
// Uses the valid Point Buy distribution (15, 14, 13, 12, 10, 8).
func TestCoreStats_DerivationsWithModifiers(t *testing.T) {
	cs, err := NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	if got, want := cs.MaxHP(), baseHP+hpPerLevel; got != want {
		t.Errorf("MaxHP() = %d, want %d", got, want)
	}
	if got, want := cs.Mana(), baseMana+manaPerLevel; got != want {
		t.Errorf("Mana() = %d, want %d", got, want)
	}
	if got, want := cs.BaseDamage(), weaponDamage+2; got != want {
		t.Errorf("BaseDamage() = %d, want %d", got, want)
	}
	if got, want := cs.DerivedSpeed(), SpeedNormal+2*speedPerDexMod; got != want {
		t.Errorf("DerivedSpeed() = %d, want %d", got, want)
	}
	if got := cs.DerivedInitiative(); got != 2 {
		t.Errorf("DerivedInitiative() = %d, want 2", got)
	}
}

package game

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
		// Standard array — cost 9+7+5+4+2+0 = 27.
		{"standard_array", 15, 14, 13, 12, 10, 8},
		// All 13s — cost 5*6 = 30. Wait: 5+5+5+5+5+5 = 30, not 27.
		// Use the fighter-like spread instead: 14, 14, 13, 10, 10, 10.
		// 7+7+5+2+2+2 = 25. Also not 27.
		// Balanced caster: 14, 14, 14, 8, 8, 12 -> 7+7+7+0+0+4 = 25. Nope.
		// Pure min/max: 15, 15, 15, 8, 8, 8 -> 9+9+9+0+0+0 = 27. ✔
		{"max_three", 15, 15, 15, 8, 8, 8},
		// All 13s + one 12 + two 8s: 13,13,13,13,12,8 ->
		// 5+5+5+5+4+0 = 24. Nope.
		// Try 14,13,13,12,12,8 -> 7+5+5+4+4+0 = 25. Nope.
		// Try 15,13,13,12,10,10 -> 9+5+5+4+2+2 = 27. ✔
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
	// cost 0+4+5+5+5+5 = 24 (below budget).
	_, err := NewStatsPointBuy(8, 12, 13, 13, 13, 13)
	if !errors.Is(err, ErrPointBuyBudget) {
		t.Errorf("below budget: got %v, want ErrPointBuyBudget", err)
	}
	// cost 9+9+9+3+0+0 = 30 (above budget): 15,15,15,11,8,8.
	_, err = NewStatsPointBuy(15, 15, 15, 11, 8, 8)
	if !errors.Is(err, ErrPointBuyBudget) {
		t.Errorf("above budget: got %v, want ErrPointBuyBudget", err)
	}
}

// TestNewStatsPointBuy_RejectsOutOfRange verifies that scores outside
// [pointBuyMin, pointBuyMax] trip ErrPointBuyRange regardless of how
// the rest of the distribution balances.
func TestNewStatsPointBuy_RejectsOutOfRange(t *testing.T) {
	// 7 is below the minimum — range error fires before budget.
	_, err := NewStatsPointBuy(7, 14, 13, 13, 12, 10)
	if !errors.Is(err, ErrPointBuyRange) {
		t.Errorf("too low: got %v, want ErrPointBuyRange", err)
	}
	// 16 is above the maximum.
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
	// CON 13 -> mod +1 -> MaxHP = baseHP + 1*hpPerLevel.
	if got, want := cs.MaxHP(), baseHP+hpPerLevel; got != want {
		t.Errorf("MaxHP() = %d, want %d", got, want)
	}
	// INT 12 -> mod +1 -> Mana = baseMana + 1*manaPerLevel.
	if got, want := cs.Mana(), baseMana+manaPerLevel; got != want {
		t.Errorf("Mana() = %d, want %d", got, want)
	}
	// STR 15 -> mod +2 -> BaseDamage = weaponDamage + 2.
	if got, want := cs.BaseDamage(), weaponDamage+2; got != want {
		t.Errorf("BaseDamage() = %d, want %d", got, want)
	}
	// DEX 14 -> mod +2 -> Speed = SpeedNormal + 2.
	if got, want := cs.DerivedSpeed(), SpeedNormal+2*speedPerDexMod; got != want {
		t.Errorf("DerivedSpeed() = %d, want %d", got, want)
	}
	if got := cs.DerivedInitiative(); got != 2 {
		t.Errorf("DerivedInitiative() = %d, want 2", got)
	}
}

// TestPlayerJoinWithStats verifies that NewPlayer hydrates every derived
// field from the supplied CoreStats so the returned Player is
// tick-ready without further mutation.
func TestPlayerJoinWithStats(t *testing.T) {
	stats, err := NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	pos := Position{X: 3, Y: 7}
	p, err := NewPlayer("p1", "Alice", *stats, pos)
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}
	if p.ID != "p1" || p.Name != "Alice" {
		t.Errorf("id/name: %+v", p)
	}
	if p.Position != pos {
		t.Errorf("Position = %+v, want %+v", p.Position, pos)
	}
	if p.Stats != *stats {
		t.Errorf("Stats = %+v, want %+v", p.Stats, *stats)
	}
	if p.MaxHP != stats.MaxHP() || p.HP != stats.MaxHP() {
		t.Errorf("HP/MaxHP = %d/%d, want %d/%d", p.HP, p.MaxHP, stats.MaxHP(), stats.MaxHP())
	}
	if p.Mana != stats.Mana() {
		t.Errorf("Mana = %d, want %d", p.Mana, stats.Mana())
	}
	if p.Speed != stats.DerivedSpeed() {
		t.Errorf("Speed = %d, want %d", p.Speed, stats.DerivedSpeed())
	}
	if p.Initiative != stats.DerivedInitiative() {
		t.Errorf("Initiative = %d, want %d", p.Initiative, stats.DerivedInitiative())
	}
	if p.Energy != baseActionCost {
		t.Errorf("Energy = %d, want %d", p.Energy, baseActionCost)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil", p.Intent)
	}
}

// TestNewPlayerValidation keeps the constructor's basic empty-id /
// empty-name rejection behaviour covered after the signature change.
func TestNewPlayerValidation(t *testing.T) {
	if _, err := NewPlayer("", "Alice", DefaultCoreStats(), Position{}); err == nil {
		t.Error("empty id: want error, got nil")
	}
	if _, err := NewPlayer("p1", "", DefaultCoreStats(), Position{}); err == nil {
		t.Error("empty name: want error, got nil")
	}
}

// TestNewMonsterDerivations mirrors TestPlayerJoinWithStats for the
// monster constructor.
func TestNewMonsterDerivations(t *testing.T) {
	m, err := NewMonster("m1", "Rat", DefaultCoreStats())
	if err != nil {
		t.Fatalf("NewMonster: %v", err)
	}
	if m.MaxHP != baseHP || m.HP != baseHP {
		t.Errorf("HP/MaxHP = %d/%d, want %d/%d", m.HP, m.MaxHP, baseHP, baseHP)
	}
	if m.Mana != baseMana {
		t.Errorf("Mana = %d, want %d", m.Mana, baseMana)
	}
	if m.Speed != SpeedNormal {
		t.Errorf("Speed = %d, want %d", m.Speed, SpeedNormal)
	}
	if m.Initiative != 0 {
		t.Errorf("Initiative = %d, want 0", m.Initiative)
	}
}

// TestApplyJoinUsesStats end-to-ends a JoinCmd carrying validated stats
// and asserts the world-stored Player reflects every derived field —
// proof that the stats travel through ApplyCommand intact.
func TestApplyJoinUsesStats(t *testing.T) {
	stats, err := NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	w := newTestWorld(testTiles{})
	_, err = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice", Stats: *stats})
	if err != nil {
		t.Fatalf("ApplyCommand: %v", err)
	}
	p, ok := w.PlayerByID("p1")
	if !ok {
		t.Fatalf("player p1 missing after join")
	}
	if p.Stats != *stats {
		t.Errorf("Stats = %+v, want %+v", p.Stats, *stats)
	}
	if p.MaxHP != stats.MaxHP() {
		t.Errorf("MaxHP = %d, want %d", p.MaxHP, stats.MaxHP())
	}
	if p.Speed != stats.DerivedSpeed() {
		t.Errorf("Speed = %d, want %d", p.Speed, stats.DerivedSpeed())
	}
}

// TestApplyJoinDefaultsStatsWhenOmitted asserts the applyJoin fallback
// path: a bare JoinCmd without Stats gets the neutral baseline so
// pre-stats callers and domain tests keep working.
func TestApplyJoinDefaultsStatsWhenOmitted(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if err != nil {
		t.Fatalf("ApplyCommand: %v", err)
	}
	p, _ := w.PlayerByID("p1")
	if p.Stats != DefaultCoreStats() {
		t.Errorf("Stats = %+v, want DefaultCoreStats", p.Stats)
	}
	if p.MaxHP != baseHP {
		t.Errorf("MaxHP = %d, want %d", p.MaxHP, baseHP)
	}
}

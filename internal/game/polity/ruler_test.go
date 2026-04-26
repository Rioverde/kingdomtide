package polity

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestNewRuler_Determinism verifies that two Streams built from the same
// (worldSeed, salt) produce bit-identical Rulers. This is the load-bearing
// guarantee behind the "same WorldSeed yields the same history" contract.
func TestNewRuler_Determinism(t *testing.T) {
	const seed int64 = 42
	a := dice.New(seed, dice.SaltKingdomYear)
	b := dice.New(seed, dice.SaltKingdomYear)
	ra := NewRuler(a, 100, "")
	rb := NewRuler(b, 100, "")
	if ra != rb {
		t.Errorf("rulers diverged despite identical (seed, salt)\n  a=%+v\n  b=%+v", ra, rb)
	}
}

// TestNewRuler_StatsInRange verifies every rolled ability score lands in
// [3, 18] — the output distribution of 4d6-drop-lowest. Catches any
// regression in Stream.Stat4D6DropLowest or in NewRuler's wiring.
func TestNewRuler_StatsInRange(t *testing.T) {
	s := dice.New(42, dice.SaltKingdomYear)
	for i := 0; i < 1000; i++ {
		r := NewRuler(s, 100, "")
		scores := []int{
			r.Stats.Strength, r.Stats.Dexterity, r.Stats.Constitution,
			r.Stats.Intelligence, r.Stats.Wisdom, r.Stats.Charisma,
		}
		for j, v := range scores {
			if v < 3 || v > 18 {
				t.Fatalf("ruler %d stat %d = %d, out of [3, 18]", i, j, v)
			}
		}
	}
}

// TestLifeExpectancy_SpecMatches verifies the MECHANICS.md §4b formula
// (30 + 10 × Modifier(CON)) on every CON value in the valid domain.
// Hand-computed expectations against the spec, not against the code —
// this catches formula drift if anyone "optimizes" the constants.
func TestLifeExpectancy_SpecMatches(t *testing.T) {
	cases := []struct {
		con  int
		want int
	}{
		{3, 0},    // clamped mod -3 → 30 + 10×(-3) = 0, ruler dies at coronation
		{5, 0},    // mod -3 → same
		{8, 20},   // mod -1 → 30 + 10×(-1) = 20
		{10, 30},  // mod 0 → 30 + 0 = 30 (baseline)
		{14, 50},  // mod +2 → 30 + 20 = 50
		{18, 70},  // mod +4 → 30 + 40 = 70
		{20, 80},  // mod +5 → 30 + 50 = 80 (magic-item ceiling)
	}
	for _, c := range cases {
		r := Ruler{Stats: stats.CoreStats{Constitution: c.con}}
		if got := r.LifeExpectancy(); got != c.want {
			t.Errorf("LifeExpectancy(CON=%d) = %d; want %d", c.con, got, c.want)
		}
	}
}

// TestLifeExpectancy_UpperClamp confirms that extreme CON scores beyond
// the D&D upper bound (20) do not produce life expectancies above 80.
// Future magic-item effects or buffs must not bypass the spec cap.
func TestLifeExpectancy_UpperClamp(t *testing.T) {
	r := Ruler{Stats: stats.CoreStats{Constitution: 30}}
	if got := r.LifeExpectancy(); got != 80 {
		t.Errorf("LifeExpectancy(CON=30) = %d; want 80 (mod clamped at +5)", got)
	}
}

// TestAlive_ZeroValue verifies a zero-value Ruler is considered alive.
// This is the natural construction of a ruler that has not yet died —
// callers set DeathYear explicitly when a death event fires.
func TestAlive_ZeroValue(t *testing.T) {
	var r Ruler
	if !r.Alive() {
		t.Error("zero-value Ruler should be Alive")
	}
}

// TestAlive_AfterDeath verifies that setting DeathYear to any non-zero
// year flips Alive to false. Guards against off-by-one bugs where year 0
// is accidentally treated as "dead at birth".
func TestAlive_AfterDeath(t *testing.T) {
	r := Ruler{DeathYear: 999}
	if r.Alive() {
		t.Error("Ruler with DeathYear=999 should not be Alive")
	}
}

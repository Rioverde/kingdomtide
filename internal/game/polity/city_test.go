package polity

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
)

// TestNewCity_FieldsSet verifies the constructor threads all four
// explicit arguments into their destinations and leaves derived
// attributes at zero (awaiting Pass-1 derivation in the mechanics layer).
func TestNewCity_FieldsSet(t *testing.T) {
	ruler := NewRuler(dice.New(42, dice.SaltKingdomYear), 1500)
	c := NewCity("Anglaria", geom.Position{X: 10, Y: 20}, 1500, ruler)

	if c.Name != "Anglaria" {
		t.Errorf("Name = %q, want Anglaria", c.Name)
	}
	if c.Position != (geom.Position{X: 10, Y: 20}) {
		t.Errorf("Position = %+v, want (10, 20)", c.Position)
	}
	if c.Founded != 1500 {
		t.Errorf("Founded = %d, want 1500", c.Founded)
	}
	if c.Ruler != ruler {
		t.Errorf("Ruler mismatch: got %+v, want %+v", c.Ruler, ruler)
	}
	if c.Population != 0 || c.Wealth != 0 || c.Army != 0 {
		t.Errorf("derived attributes should default to zero, got pop=%d wealth=%d army=%d",
			c.Population, c.Wealth, c.Army)
	}
	if c.BaseRank != RankHamlet {
		t.Errorf("BaseRank = %v, want RankHamlet (zero value)", c.BaseRank)
	}
	if c.EffectiveRank != RankIndependent {
		t.Errorf("EffectiveRank = %v, want RankIndependent (zero value)", c.EffectiveRank)
	}
}

// TestCity_Age_InheritsFromSettlement verifies that the Age method
// promoted from Settlement is callable on City and returns the correct
// delta. This is the load-bearing test that confirms the embedding
// works end-to-end.
func TestCity_Age_InheritsFromSettlement(t *testing.T) {
	c := NewCity("Anglaria", geom.Position{}, 1500, Ruler{})
	if got := c.Age(1550); got != 50 {
		t.Errorf("Age(1550) = %d, want 50", got)
	}
	if got := c.Age(1500); got != 0 {
		t.Errorf("Age(1500) = %d, want 0 (same year)", got)
	}
}

// TestBaseRank_String verifies the stringer mapping for every declared
// BaseRank value, plus the unknown-value fallback.
func TestBaseRank_String(t *testing.T) {
	cases := []struct {
		r    BaseRank
		want string
	}{
		{RankHamlet, "Hamlet"},
		{RankTown, "Town"},
		{RankCity, "City"},
		{RankMetropolis, "Metropolis"},
		{BaseRank(99), "UnknownBaseRank"},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.want {
			t.Errorf("BaseRank(%d).String() = %q, want %q", c.r, got, c.want)
		}
	}
}

// TestEffectiveRank_String verifies the stringer mapping for every
// declared EffectiveRank value, plus the unknown-value fallback.
func TestEffectiveRank_String(t *testing.T) {
	cases := []struct {
		r    EffectiveRank
		want string
	}{
		{RankIndependent, "Independent"},
		{RankAutonomous, "Autonomous"},
		{RankVassal, "Vassal"},
		{RankCapital, "Capital"},
		{EffectiveRank(99), "UnknownEffectiveRank"},
	}
	for _, c := range cases {
		if got := c.r.String(); got != c.want {
			t.Errorf("EffectiveRank(%d).String() = %q, want %q", c.r, got, c.want)
		}
	}
}

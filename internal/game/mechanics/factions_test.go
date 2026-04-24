package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyFactionDriftYear_AllFactionsDrift verifies that across
// many ticks every faction sees at least one non-zero influence
// change — none are silently pinned at their starting value.
func TestApplyFactionDriftYear_AllFactionsDrift(t *testing.T) {
	c := &polity.City{}
	c.Factions.Set(polity.FactionMerchants, 0.5)
	c.Factions.Set(polity.FactionMilitary, 0.5)
	c.Factions.Set(polity.FactionMages, 0.5)
	c.Factions.Set(polity.FactionCriminals, 0.5)

	stream := dice.New(42, dice.SaltFactions)
	moved := [4]bool{}
	prev := c.Factions
	for i := 0; i < 100; i++ {
		ApplyFactionDriftYear(c, stream)
		for _, f := range []polity.Faction{
			polity.FactionMerchants, polity.FactionMilitary,
			polity.FactionMages, polity.FactionCriminals,
		} {
			if c.Factions.Get(f) != prev.Get(f) {
				moved[f] = true
			}
		}
		prev = c.Factions
	}
	for i, m := range moved {
		if !m {
			t.Errorf("faction %s never drifted across 100 ticks",
				polity.Faction(i))
		}
	}
}

// TestApplyFactionDriftYear_MilitaryPeacetimePenalty verifies the
// Military faction shows a negative bias across many ticks — even
// with symmetric ±0.05 drift, the −0.03 peacetime nudge dominates.
func TestApplyFactionDriftYear_MilitaryPeacetimePenalty(t *testing.T) {
	c := &polity.City{}
	c.Factions.Set(polity.FactionMilitary, 0.5)
	stream := dice.New(42, dice.SaltFactions)

	for i := 0; i < 1000; i++ {
		ApplyFactionDriftYear(c, stream)
	}
	if c.Factions.Get(polity.FactionMilitary) >= 0.5 {
		t.Errorf("Military faction did not trend down under peacetime: %v",
			c.Factions.Get(polity.FactionMilitary))
	}
}

// TestApplyFactionDriftYear_StaysInRange verifies influence stays
// clamped in [0, 1] across many ticks — Set's clamp must hold.
func TestApplyFactionDriftYear_StaysInRange(t *testing.T) {
	c := &polity.City{}
	c.Factions.Set(polity.FactionMerchants, 0.5)
	stream := dice.New(42, dice.SaltFactions)
	for i := 0; i < 5000; i++ {
		ApplyFactionDriftYear(c, stream)
		for _, f := range []polity.Faction{
			polity.FactionMerchants, polity.FactionMilitary,
			polity.FactionMages, polity.FactionCriminals,
		} {
			v := c.Factions.Get(f)
			if v < 0 || v > 1 {
				t.Fatalf("iter %d: faction %s=%v outside [0,1]", i, f, v)
			}
		}
	}
}

// TestApplyFactionDriftYear_Determinism verifies identical runs
// produce identical final FactionInfluence values.
func TestApplyFactionDriftYear_Determinism(t *testing.T) {
	newCity := func() *polity.City {
		c := &polity.City{}
		c.Factions.Set(polity.FactionMerchants, 0.5)
		c.Factions.Set(polity.FactionMilitary, 0.5)
		c.Factions.Set(polity.FactionMages, 0.5)
		c.Factions.Set(polity.FactionCriminals, 0.5)
		return c
	}
	a := newCity()
	b := newCity()
	streamA := dice.New(42, dice.SaltFactions)
	streamB := dice.New(42, dice.SaltFactions)
	for i := 0; i < 500; i++ {
		ApplyFactionDriftYear(a, streamA)
		ApplyFactionDriftYear(b, streamB)
	}
	if a.Factions != b.Factions {
		t.Errorf("Factions diverged: a=%v b=%v", a.Factions, b.Factions)
	}
}

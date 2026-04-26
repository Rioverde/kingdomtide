package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// happyCity builds a baseline city above the revolution happiness
// ceiling with a fixed Ruler — test helpers should never revolt.
func happyCity() *polity.City {
	return &polity.City{
		Happiness: 80,
		Settlement: polity.Settlement{
			Ruler: polity.Ruler{Stats: stats.CoreStats{
				Strength: 10, Dexterity: 10, Constitution: 10,
				Intelligence: 10, Wisdom: 10, Charisma: 10,
			}},
		},
	}
}

// TestApplyRevolutionCheckYear_HappyCityNeverRevolts verifies a city
// above the happiness ceiling survives even on a natural 20 roll —
// the ceiling gate fires first.
func TestApplyRevolutionCheckYear_HappyCityNeverRevolts(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	for i := 0; i < 100; i++ {
		c := happyCity()
		before := c.Ruler
		ApplyRevolutionCheckYear(c, stream, 1500)
		if c.RevolutionThisYear {
			t.Fatalf("iter %d: happy city revolted", i)
		}
		if c.Ruler != before {
			t.Fatalf("iter %d: happy city's ruler replaced", i)
		}
	}
}

// TestApplyRevolutionCheckYear_UnhappyCityEventuallyRevolts verifies
// sufficiently many years of low happiness eventually trigger a
// revolt — the D20 = 20 natural-20 firing rate over hundreds of
// rolls makes the probability of zero revolts vanishing.
func TestApplyRevolutionCheckYear_UnhappyCityEventuallyRevolts(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	c := &polity.City{Happiness: 30}

	for year := 0; year < 500; year++ {
		ApplyRevolutionCheckYear(c, stream, year)
		if c.RevolutionThisYear {
			// On revolt happiness resets to 55; push back down so
			// subsequent years still qualify under the ceiling.
			c.Happiness = 30
			return // pass on first revolt
		}
	}
	t.Fatal("500 years of Happiness=30 produced no revolts — check DC/ceiling wiring")
}

// TestApplyRevolutionCheckYear_RevolutionResetsState verifies a
// successful revolt replaces the Ruler, sets happiness to 55, and
// raises the one-year flag.
func TestApplyRevolutionCheckYear_RevolutionResetsState(t *testing.T) {
	// Seed + salt chosen empirically so the first D20 for an unhappy
	// city lands on 20; stable across runs thanks to Stream
	// determinism. If the constants change and the test flakes, pick
	// a different seed.
	stream := dice.New(42, dice.SaltRevolutions)
	c := &polity.City{Happiness: 30}

	foundRevolt := false
	originalRuler := c.Ruler
	for year := 0; year < 500; year++ {
		ApplyRevolutionCheckYear(c, stream, year)
		if c.RevolutionThisYear {
			foundRevolt = true
			if c.Happiness != revolutionHappinessReset {
				t.Errorf("after revolt: Happiness = %d, want %d",
					c.Happiness, revolutionHappinessReset)
			}
			if c.Ruler == originalRuler {
				t.Errorf("after revolt: Ruler unchanged, expected replacement")
			}
			return
		}
	}
	if !foundRevolt {
		t.Fatal("no revolt in 500 years — cannot validate post-revolt state")
	}
}

// TestApplyRevolutionCheckYear_FlagResetsEachCall verifies the
// RevolutionThisYear flag only stays true for the single year it
// fired — the next call clears it even if no revolt happens.
func TestApplyRevolutionCheckYear_FlagResetsEachCall(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	c := happyCity() // will never revolt
	c.RevolutionThisYear = true // simulate a prior-year revolt

	ApplyRevolutionCheckYear(c, stream, 1500)

	if c.RevolutionThisYear {
		t.Errorf("RevolutionThisYear should clear on every call when no revolt fires")
	}
}

// TestApplyRevolutionCheckYear_Determinism verifies the same seed
// produces the same revolution pattern across runs — replay invariant.
func TestApplyRevolutionCheckYear_Determinism(t *testing.T) {
	run := func(seed int64) bool {
		stream := dice.New(seed, dice.SaltRevolutions)
		c := &polity.City{Happiness: 30}
		for i := 0; i < 50; i++ {
			ApplyRevolutionCheckYear(c, stream, i)
			if c.RevolutionThisYear {
				return true
			}
		}
		return false
	}
	if run(42) != run(42) {
		t.Fatal("same seed produced different revolution outcomes")
	}
}

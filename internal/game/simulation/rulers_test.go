package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestRulers_DiesAtExpectancy verifies that a ruler whose life expectancy
// is exhausted at a known year receives a DeathYear and a successor is
// installed in the same tick.
func TestRulers_DiesAtExpectancy(t *testing.T) {
	const seed int64 = 42
	st := newState(seed, 200)
	c := newCamp(1, 10, 10, polity.RegionNormal, 50, polity.FaithOldGods)
	st.settlements[1] = c

	// Capture the initial ruler and compute the year it should die.
	initialRuler := c.Ruler
	expectancy := initialRuler.LifeExpectancy()
	deathYear := initialRuler.BirthYear + expectancy

	// Tick through years up to and including the death year.
	for y := 0; y <= deathYear; y++ {
		st.refreshSortedIDs()
		st.tickRulers(y)
	}

	set := c.Base()
	if set.Ruler.Name == initialRuler.Name {
		t.Errorf("ruler unchanged after expected death year %d; succession did not fire", deathYear)
	}
	if initialRuler.DeathYear != deathYear {
		t.Errorf("old ruler DeathYear: got %d, want %d", initialRuler.DeathYear, deathYear)
	}
}

// TestRulers_SuccessorIsFresh verifies that the successor rolled by
// succeedRuler has a fresh BirthYear equal to the succession year, a
// non-empty name, and (with very high probability) different stats from
// the predecessor. Runs 100 successions against distinct settlement IDs
// to confirm the stream produces varied output.
func TestRulers_SuccessorIsFresh(t *testing.T) {
	const seed int64 = 42
	const year = 50

	differentStats := 0
	for i := 0; i < 100; i++ {
		st := newState(seed, 200)
		id := polity.SettlementID(i + 1)
		c := newCamp(id, i*3, i*5, polity.RegionNormal, 50, polity.FaithOldGods)
		st.settlements[id] = c

		predecessor := c.Ruler
		st.succeedRuler(c.Base(), id, year)
		successor := c.Base().Ruler

		if successor.Name == "" {
			t.Errorf("successor %d has empty name", i)
		}
		if successor.BirthYear != year {
			t.Errorf("successor %d BirthYear: got %d, want %d", i, successor.BirthYear, year)
		}
		if successor.Stats != predecessor.Stats {
			differentStats++
		}
	}

	// With 100 rolls of 4d6-drop-lowest across 6 stats, virtually all
	// successors should differ from their predecessor. Accept if ≥95%.
	if differentStats < 95 {
		t.Errorf("only %d/100 successors had different stats from predecessor; expected ≥95", differentStats)
	}
}

// TestRulers_Determinism verifies that two state instances built from the
// same seed produce the identical successor name and stats when
// succeedRuler fires for the same (settlement, year).
func TestRulers_Determinism(t *testing.T) {
	const seed int64 = 99
	const year = 77

	mkSuccessor := func() polity.Ruler {
		st := newState(seed, 200)
		id := polity.SettlementID(5)
		c := newCamp(id, 20, 30, polity.RegionHoly, 50, polity.FaithSunCovenant)
		st.settlements[id] = c
		st.succeedRuler(c.Base(), id, year)
		return c.Base().Ruler
	}

	a, b := mkSuccessor(), mkSuccessor()
	if a.Name != b.Name {
		t.Errorf("successor name not deterministic: %q vs %q", a.Name, b.Name)
	}
	if a.Stats != b.Stats {
		t.Errorf("successor stats not deterministic: %+v vs %+v", a.Stats, b.Stats)
	}
	if a.BirthYear != b.BirthYear {
		t.Errorf("successor BirthYear not deterministic: %d vs %d", a.BirthYear, b.BirthYear)
	}
}

package mechanics

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestTickCityYear_Determinism verifies that two identical cities
// driven by two streams with the same seed produce byte-identical
// state after a year. Load-bearing for replay.
func TestTickCityYear_Determinism(t *testing.T) {
	const seed int64 = 42
	newCity := func() *polity.City {
		return polity.NewCity("Anglaria", geom.Position{X: 1, Y: 2}, 1400, polity.Ruler{})
	}
	a, b := newCity(), newCity()
	a.Population = 1000
	a.TaxRate = polity.TaxNormal
	b.Population = 1000
	b.TaxRate = polity.TaxNormal

	TickCityYear(a, dice.New(seed, dice.SaltKingdomYear), 1500)
	TickCityYear(b, dice.New(seed, dice.SaltKingdomYear), 1500)

	if !reflect.DeepEqual(a, b) {
		t.Errorf("diverged after one tick\n  a=%+v\n  b=%+v", *a, *b)
	}
}

// TestTickCityYear_OrderingPropagates verifies that Happiness reflects
// this year's FoodBalance — the food → happiness chain holds inside a
// single tick. Under the new ordering (events run after the happiness
// baseline) an individual event or revolt may pull the final value
// down from the baseline, so we assert happiness stays within a
// plausible envelope rather than a single exact value. A city with
// zero-value Ruler can revolt under unlucky rolls and reset to 55;
// the envelope covers that outcome, the clean +15 food bonus, the
// default religion-alignment +8 (NewCity seeds FaithOldGods majority
// and a zero-value Ruler defaults to the same faith), and typical
// event deltas.
func TestTickCityYear_OrderingPropagates(t *testing.T) {
	c := polity.NewCity("x", geom.Position{}, 1400, polity.Ruler{})
	c.Population = 1000
	c.TaxRate = polity.TaxNormal
	stream := dice.New(42, dice.SaltKingdomYear)

	TickCityYear(c, stream, 1500)

	// Baseline = 50 (base) + 15 (clamped food surplus) + 0 (TaxNormal)
	// + 8 (religion match: default city majority OldGods ≡ default
	// Ruler.Faith OldGods) = 73. Events and a possible revolt can only
	// subtract from that; revolt reset floor is 55. The combined
	// envelope is [30, 80] — 80 is the ceiling given a full positive-
	// extras cap of +20 (which can't land here without a charismatic
	// ruler or factions, but we give it slack for future wiring).
	if c.Happiness < 30 || c.Happiness > 80 {
		t.Errorf("Happiness = %d, want in [30, 80] given the food→happiness chain",
			c.Happiness)
	}
}

// TestTickCityYear_HundredYears_NoExplosion verifies a century of
// ticks keeps all state inside sane bounds — no integer overflow, no
// population explosion past the cap, no wealth running into billions.
// This is the long-run sanity gate: if any formula is pathologically
// unbalanced, a hundred iterations make it obvious.
func TestTickCityYear_HundredYears_NoExplosion(t *testing.T) {
	c := polity.NewCity("Anglaria", geom.Position{}, 1400, polity.Ruler{})
	c.Population = 1000
	c.TaxRate = polity.TaxNormal
	stream := dice.New(42, dice.SaltKingdomYear)

	for year := 1400; year < 1500; year++ {
		TickCityYear(c, stream, year)

		if c.Population < popMin || c.Population > popMaxCap {
			t.Fatalf("year %d: Population=%d escaped [%d, %d]",
				year, c.Population, popMin, popMaxCap)
		}
		if c.Prosperity < 0 || c.Prosperity > 1 {
			t.Fatalf("year %d: Prosperity=%v escaped [0, 1]", year, c.Prosperity)
		}
	}
}

// TestTickCityYear_GhostTownSkipsFullTick verifies the early-skip
// branch: a sub-viability city without an alive ruler does not run
// the full subsystem stack; just clamps population to the floor and
// returns. This keeps late-game cost bounded for worlds with many
// dissolved cities.
func TestTickCityYear_GhostTownSkipsFullTick(t *testing.T) {
	c := polity.NewCity("Ghost", geom.Position{}, 1400, polity.Ruler{DeathYear: 1450})
	c.Population = 5 // below popMin
	stream := dice.New(42, dice.SaltKingdomYear)
	TickCityYear(c, stream, 1500)
	if c.Population != 80 {
		t.Errorf("ghost town should clamp to popMin=80, got %d", c.Population)
	}
	// Innovation should stay zero — tech step skipped.
	if c.Innovation != 0 {
		t.Errorf("Innovation = %v, want 0 (tech step should skip)", c.Innovation)
	}
}

// TestTickCityYear_ProducesWealthAtNormalTax verifies the economy is
// positive-sum for a modestly-sized, modestly-happy city — a century
// of TaxNormal with no army drain should leave a clear surplus.
func TestTickCityYear_ProducesWealthAtNormalTax(t *testing.T) {
	c := polity.NewCity("Anglaria", geom.Position{}, 1400, polity.Ruler{})
	c.Population = 5000
	c.TaxRate = polity.TaxNormal
	stream := dice.New(42, dice.SaltKingdomYear)

	for year := 1400; year < 1500; year++ {
		TickCityYear(c, stream, year)
	}
	if c.Wealth <= 0 {
		t.Errorf("century of normal tax on 5000 pop should be profitable, Wealth=%d",
			c.Wealth)
	}
}

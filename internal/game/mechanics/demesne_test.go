package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestApplyDemesneYear_GrowsUnderNormalConditions(t *testing.T) {
	d := polity.NewDemesne("test", geom.Position{}, 1200, "parent")
	d.Population = 100
	stream := dice.New(42, dice.SaltKingdomYear)

	initial := d.Population
	for i := 0; i < 50; i++ {
		ApplyDemesneYear(d, stream)
	}
	if d.Population <= initial {
		t.Errorf("demesne should grow over 50 yr, started %d ended %d",
			initial, d.Population)
	}
}

func TestApplyDemesneYear_ClampsAtFloor(t *testing.T) {
	d := polity.NewDemesne("tiny", geom.Position{}, 1200, "parent")
	d.Population = 1 // below floor
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyDemesneYear(d, stream)
	if d.Population < demesnePopMin {
		t.Errorf("demesne clamped to %d, want >= %d", d.Population, demesnePopMin)
	}
}

func TestApplyDemesneYear_ClampsAtCeiling(t *testing.T) {
	d := polity.NewDemesne("huge", geom.Position{}, 1200, "parent")
	d.Population = demesnePopMaxCap * 2
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyDemesneYear(d, stream)
	if d.Population > demesnePopMaxCap {
		t.Errorf("demesne overflow: %d > cap %d", d.Population, demesnePopMaxCap)
	}
}

func TestApplyDemesneYear_Determinism(t *testing.T) {
	a := polity.NewDemesne("a", geom.Position{}, 1200, "parent")
	a.Population = 100
	b := polity.NewDemesne("a", geom.Position{}, 1200, "parent")
	b.Population = 100
	sa := dice.New(42, dice.SaltKingdomYear)
	sb := dice.New(42, dice.SaltKingdomYear)

	for i := 0; i < 20; i++ {
		ApplyDemesneYear(a, sa)
		ApplyDemesneYear(b, sb)
	}
	if a.Population != b.Population {
		t.Errorf("determinism broken: a=%d b=%d", a.Population, b.Population)
	}
}

func TestResolveDemesneToCity_AddsFood(t *testing.T) {
	cityA := polity.NewCity("A", geom.Position{}, 1200, polity.Ruler{})
	cityA.FoodBalance = 100

	demesnes := []*polity.Demesne{
		mkDemesne("d1", 200, "A"),
		mkDemesne("d2", 150, "A"),
		mkDemesne("d3", 100, "nonexistent"), // orphan, skipped
	}
	cities := map[string]*polity.City{"A": cityA}

	ResolveDemesneToCity(demesnes, cities)
	// d1: 200 x 0.1 = 20, d2: 150 x 0.1 = 15. Total +35.
	if cityA.FoodBalance != 135 {
		t.Errorf("FoodBalance = %d, want 135 (100 base + 35 demesne)", cityA.FoodBalance)
	}
}

func mkDemesne(name string, pop int, parent string) *polity.Demesne {
	d := polity.NewDemesne(name, geom.Position{}, 1200, parent)
	d.Population = pop
	return d
}

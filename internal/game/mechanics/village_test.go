package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestApplyVillageYear_GrowsUnderNormalConditions(t *testing.T) {
	v := polity.NewVillage("test", geom.Position{}, 1200, "parent")
	v.Population = 100
	stream := dice.New(42, dice.SaltKingdomYear)

	initial := v.Population
	for i := 0; i < 50; i++ {
		ApplyVillageYear(v, stream)
	}
	if v.Population <= initial {
		t.Errorf("village should grow over 50 yr, started %d ended %d",
			initial, v.Population)
	}
}

func TestApplyVillageYear_ClampsAtFloor(t *testing.T) {
	v := polity.NewVillage("tiny", geom.Position{}, 1200, "parent")
	v.Population = 1 // below floor
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyVillageYear(v, stream)
	if v.Population < villagePopMin {
		t.Errorf("village clamped to %d, want >= %d", v.Population, villagePopMin)
	}
}

func TestApplyVillageYear_ClampsAtCeiling(t *testing.T) {
	v := polity.NewVillage("huge", geom.Position{}, 1200, "parent")
	v.Population = villagePopMaxCap * 2
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyVillageYear(v, stream)
	if v.Population > villagePopMaxCap {
		t.Errorf("village overflow: %d > cap %d", v.Population, villagePopMaxCap)
	}
}

func TestApplyVillageYear_Determinism(t *testing.T) {
	a := polity.NewVillage("a", geom.Position{}, 1200, "parent")
	a.Population = 100
	b := polity.NewVillage("a", geom.Position{}, 1200, "parent")
	b.Population = 100
	sa := dice.New(42, dice.SaltKingdomYear)
	sb := dice.New(42, dice.SaltKingdomYear)

	for i := 0; i < 20; i++ {
		ApplyVillageYear(a, sa)
		ApplyVillageYear(b, sb)
	}
	if a.Population != b.Population {
		t.Errorf("determinism broken: a=%d b=%d", a.Population, b.Population)
	}
}

func TestResolveVillageToCity_AddsFood(t *testing.T) {
	cityA := polity.NewCity("A", geom.Position{}, 1200, polity.Ruler{})
	cityA.FoodBalance = 100

	villages := []*polity.Village{
		mkVillage("v1", 200, "A"),
		mkVillage("v2", 150, "A"),
		mkVillage("v3", 100, "nonexistent"), // orphan, skipped
	}
	cities := map[string]*polity.City{"A": cityA}

	ResolveVillageToCity(villages, cities)
	// v1: 200 x 0.1 = 20, v2: 150 x 0.1 = 15. Total +35.
	if cityA.FoodBalance != 135 {
		t.Errorf("FoodBalance = %d, want 135 (100 base + 35 village)", cityA.FoodBalance)
	}
}

func mkVillage(name string, pop int, parent string) *polity.Village {
	v := polity.NewVillage(name, geom.Position{}, 1200, parent)
	v.Population = pop
	return v
}

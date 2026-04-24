package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestBuildFortification_AppendsToList(t *testing.T) {
	c := &polity.City{}
	f := BuildFortification(c, 1500)
	if len(c.Fortifications) != 1 || c.Fortifications[0] != f {
		t.Errorf("fortification not appended correctly")
	}
}

func TestTotalDefense_ZeroWhenEmpty(t *testing.T) {
	c := polity.City{}
	if TotalDefense(&c) != 0 {
		t.Errorf("empty city should have zero defense")
	}
}

func TestTotalDefense_MasonryMultiplier(t *testing.T) {
	c := polity.City{
		Fortifications: []polity.Fortification{{Defense: 10}},
	}
	base := TotalDefense(&c)
	c.Techs.Set(polity.TechMasonry)
	withMasonry := TotalDefense(&c)
	if withMasonry <= base {
		t.Errorf("Masonry should increase defense: base=%d withMasonry=%d",
			base, withMasonry)
	}
	if withMasonry != 12 { // 10 × 1.2
		t.Errorf("Masonry defense = %d, want 12", withMasonry)
	}
}

func TestTotalDefense_ArchitectBonus(t *testing.T) {
	c := polity.City{
		Fortifications: []polity.Fortification{{Defense: 10}},
		GreatPerson: &polity.GreatPerson{
			Kind:      polity.GreatPersonArchitect,
			DeathYear: 1600,
		},
	}
	if got := TotalDefense(&c); got != 15 { // 10 + 5
		t.Errorf("Architect defense = %d, want 15", got)
	}
}

func TestTotalDefense_MasonryAndArchitectStack(t *testing.T) {
	c := polity.City{
		Fortifications: []polity.Fortification{{Defense: 10}},
		GreatPerson: &polity.GreatPerson{
			Kind:      polity.GreatPersonArchitect,
			DeathYear: 1600,
		},
	}
	c.Techs.Set(polity.TechMasonry)
	// 10 × 1.2 = 12, + 5 architect = 17
	if got := TotalDefense(&c); got != 17 {
		t.Errorf("Masonry+Architect = %d, want 17", got)
	}
}

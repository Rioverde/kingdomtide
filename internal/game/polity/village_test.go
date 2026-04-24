package polity

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// TestNewVillage_FieldsSet verifies the constructor fills every field
// and leaves Population at zero (seeded later by the mechanics layer
// from surrounding tile fertility).
func TestNewVillage_FieldsSet(t *testing.T) {
	v := NewVillage("Millford", geom.Position{X: 3, Y: 7}, 1450, "anglaria")

	if v.Name != "Millford" {
		t.Errorf("Name = %q, want Millford", v.Name)
	}
	if v.Position != (geom.Position{X: 3, Y: 7}) {
		t.Errorf("Position = %+v, want (3, 7)", v.Position)
	}
	if v.Founded != 1450 {
		t.Errorf("Founded = %d, want 1450", v.Founded)
	}
	if v.ParentCityID != "anglaria" {
		t.Errorf("ParentCityID = %q, want anglaria", v.ParentCityID)
	}
	if v.Population != 0 {
		t.Errorf("Population = %d, want 0 (defaults to zero)", v.Population)
	}
}

// TestVillage_Age_InheritsFromSettlement verifies the promoted Age
// method works on Village, confirming the Settlement embedding works
// identically for both polity types.
func TestVillage_Age_InheritsFromSettlement(t *testing.T) {
	v := NewVillage("Millford", geom.Position{}, 1450, "anglaria")
	if got := v.Age(1550); got != 100 {
		t.Errorf("Age(1550) = %d, want 100", got)
	}
}

package polity

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// TestNewDemesne_FieldsSet verifies the constructor fills every field
// and leaves Population at zero (seeded later by the mechanics layer
// from surrounding tile fertility).
func TestNewDemesne_FieldsSet(t *testing.T) {
	d := NewDemesne("Millford", geom.Position{X: 3, Y: 7}, 1450, "anglaria")

	if d.Name != "Millford" {
		t.Errorf("Name = %q, want Millford", d.Name)
	}
	if d.Position != (geom.Position{X: 3, Y: 7}) {
		t.Errorf("Position = %+v, want (3, 7)", d.Position)
	}
	if d.Founded != 1450 {
		t.Errorf("Founded = %d, want 1450", d.Founded)
	}
	if d.ParentCityID != "anglaria" {
		t.Errorf("ParentCityID = %q, want anglaria", d.ParentCityID)
	}
	if d.Population != 0 {
		t.Errorf("Population = %d, want 0 (defaults to zero)", d.Population)
	}
}

// TestDemesne_Age_InheritsFromSettlement verifies the promoted Age
// method works on Demesne, confirming the Settlement embedding works
// identically for both polity types.
func TestDemesne_Age_InheritsFromSettlement(t *testing.T) {
	d := NewDemesne("Millford", geom.Position{}, 1450, "anglaria")
	if got := d.Age(1550); got != 100 {
		t.Errorf("Age(1550) = %d, want 100", got)
	}
}

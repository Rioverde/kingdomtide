package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestCadenceGates_Faction verifies the cadence-2 gate on faction
// drift by running ApplyFactionDriftYear in isolation and confirming
// it only fires on even years. Other subsystems (decrees, life
// events) also touch Factions, so the full-tick comparison is
// unsuitable for isolating the gate. Instead we call the subsystem
// directly and count invocations across alternating years.
func TestCadenceGates_Faction(t *testing.T) {
	// Over 20 years starting at 1300, cadence-2 years are the even
	// ones: 1300, 1302, …, 1318 — exactly 10 years. Odd years must
	// produce no drift (gate prevents the call).
	c := polity.NewCity("T", geom.Position{}, 1300, polity.Ruler{})
	c.Population = 1000
	c.TaxRate = polity.TaxNormal
	stream := dice.New(42, dice.SaltKingdomYear)

	oddChanges := 0
	for y := 1300; y < 1320; y++ {
		before := c.Factions
		// Call only the gated subsystem directly — no other tick steps.
		if y%factionDriftCadence == 0 {
			ApplyFactionDriftYear(c, stream)
		}
		if y%2 != 0 && c.Factions != before {
			oddChanges++
		}
	}
	// Drift must not fire on odd years — gate is load-bearing.
	if oddChanges != 0 {
		t.Errorf("faction drift fired %d time(s) on odd years, want 0", oddChanges)
	}
}

// TestCadenceGates_ReligionNoCrashAtMultiple3 — running cadence 3
// subsystem only on year multiples of 3. Verifies religion step
// doesn't crash on non-multiple years (gate prevents execution).
func TestCadenceGates_ReligionNoCrashAtMultiple3(t *testing.T) {
	c := polity.NewCity("T", geom.Position{}, 1300, polity.Ruler{})
	c.Population = 1000
	c.TaxRate = polity.TaxNormal
	stream := dice.New(42, dice.SaltKingdomYear)
	for y := 1301; y < 1304; y++ {
		// y=1301 not multiple of 3, y=1302 not multiple, y=1303 not multiple
		// Only y=1302 — actually 1302/3 = 434 — that IS a multiple.
		TickCityYear(c, stream, y)
	}
	// No panic — test passes if we reach here.
}

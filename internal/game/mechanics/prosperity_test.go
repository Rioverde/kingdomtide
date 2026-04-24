package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyProsperityYear_InUnitRange is the property-check: no matter
// what stats we throw at the function, Prosperity must land in [0, 1].
// Guards against a future component bypassing the per-stat clamp.
func TestApplyProsperityYear_InUnitRange(t *testing.T) {
	cases := []struct {
		name string
		c    polity.City
	}{
		{"empty zero-value", polity.City{}},
		{"max healthy", polity.City{
			Settlement:  polity.Settlement{Population: 40000, Founded: 1, Name: "Old"},
			Wealth:      prosperityWealthCap,
			FoodBalance: prosperityFoodCap,
			Happiness:   prosperityHappinessCap,
		}},
		{"catastrophic", polity.City{
			Wealth:      -prosperityWealthCap * 10,
			FoodBalance: -prosperityFoodCap * 10,
			Happiness:   -1000,
		}},
	}
	for _, tc := range cases {
		ApplyProsperityYear(&tc.c, 1500)
		if tc.c.Prosperity < 0 || tc.c.Prosperity > 1 {
			t.Errorf("%s: Prosperity = %v, out of [0, 1]", tc.name, tc.c.Prosperity)
		}
	}
}

// TestApplyProsperityYear_MonotoneOnWealth verifies adding wealth
// never decreases prosperity (while other factors stay constant). This
// catches formula inversions — prosperity should be monotone in every
// positive component.
func TestApplyProsperityYear_MonotoneOnWealth(t *testing.T) {
	poor := polity.City{Settlement: polity.Settlement{Founded: 1000}, Wealth: 1000}
	rich := polity.City{Settlement: polity.Settlement{Founded: 1000}, Wealth: 50000}

	ApplyProsperityYear(&poor, 1500)
	ApplyProsperityYear(&rich, 1500)

	if rich.Prosperity < poor.Prosperity {
		t.Errorf("richer city should have >= prosperity: poor=%v rich=%v",
			poor.Prosperity, rich.Prosperity)
	}
}

// TestApplyProsperityYear_UsesAge verifies Age actually contributes — an
// ancient city with the same non-age stats should land higher than a
// fresh founding.
func TestApplyProsperityYear_UsesAge(t *testing.T) {
	fresh := polity.City{Settlement: polity.Settlement{Founded: 1499}, Wealth: 1000}
	ancient := polity.City{Settlement: polity.Settlement{Founded: 0}, Wealth: 1000}

	ApplyProsperityYear(&fresh, 1500)
	ApplyProsperityYear(&ancient, 1500)

	if ancient.Prosperity <= fresh.Prosperity {
		t.Errorf("ancient should outrank fresh: fresh=%v ancient=%v",
			fresh.Prosperity, ancient.Prosperity)
	}
}

// _ keeps the geom import live even if the case tables shrink in the
// future. The package truly depends on geom.Position through City.
var _ = geom.Position{}

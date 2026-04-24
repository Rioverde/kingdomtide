package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyFoodYear_Determinism verifies that the same city state plus
// the same dice stream produces identical FoodBalance — the replay
// guarantee that underpins the simulation's same-seed-same-history
// contract.
func TestApplyFoodYear_Determinism(t *testing.T) {
	const seed int64 = 42
	a := polity.City{Settlement: polity.Settlement{Population: 1000}}
	b := polity.City{Settlement: polity.Settlement{Population: 1000}}

	ApplyFoodYear(&a, dice.New(seed, dice.SaltKingdomYear))
	ApplyFoodYear(&b, dice.New(seed, dice.SaltKingdomYear))

	if a.FoodBalance != b.FoodBalance {
		t.Errorf("FoodBalance diverged: a=%d b=%d", a.FoodBalance, b.FoodBalance)
	}
}

// TestApplyFoodYear_Range verifies the variance always lands in
// [baseYield - 2, baseYield + 3] — the D6-centered variance interval.
// Catches off-by-one regressions in the centering math.
func TestApplyFoodYear_Range(t *testing.T) {
	const pop = 1000
	baseYield := pop / harvestLaborDivisor
	stream := dice.New(42, dice.SaltKingdomYear)

	for i := 0; i < 1000; i++ {
		c := polity.City{Settlement: polity.Settlement{Population: pop}}
		ApplyFoodYear(&c, stream)
		if c.FoodBalance < baseYield-2 || c.FoodBalance > baseYield+3 {
			t.Fatalf("iter %d: FoodBalance=%d, want [%d, %d]",
				i, c.FoodBalance, baseYield-2, baseYield+3)
		}
	}
}

// TestApplyFoodYear_ZeroPopulation verifies a city with no inhabitants
// still produces valid variance output — base yield floors at zero, so
// the result is just the D6 variance in [-2, +3].
func TestApplyFoodYear_ZeroPopulation(t *testing.T) {
	stream := dice.New(42, dice.SaltKingdomYear)
	for i := 0; i < 100; i++ {
		c := polity.City{}
		ApplyFoodYear(&c, stream)
		if c.FoodBalance < -2 || c.FoodBalance > 3 {
			t.Fatalf("iter %d: FoodBalance=%d, want [-2, +3]", i, c.FoodBalance)
		}
	}
}

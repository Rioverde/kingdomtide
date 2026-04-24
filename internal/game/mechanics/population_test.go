package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyPopulationYear_GrowsOnGoodConditions verifies a healthy
// city (food surplus, high happiness, positive wealth) grows — the
// baseline sanity check that growth isn't wired backwards.
func TestApplyPopulationYear_GrowsOnGoodConditions(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: 1000},
		FoodBalance: 20,
		Happiness:   80,
		Wealth:      100,
	}
	before := c.Population
	ApplyPopulationYear(&c)
	if c.Population <= before {
		t.Errorf("healthy city should grow: before=%d after=%d", before, c.Population)
	}
}

// TestApplyPopulationYear_ShrinksOnBadConditions verifies food deficit
// plus low happiness knocks population down — the negative case.
func TestApplyPopulationYear_ShrinksOnBadConditions(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: 10000},
		FoodBalance: -20,
		Happiness:   10,
		Wealth:      -1000,
	}
	before := c.Population
	ApplyPopulationYear(&c)
	if c.Population >= before {
		t.Errorf("starving unhappy broke city should shrink: before=%d after=%d",
			before, c.Population)
	}
}

// TestApplyPopulationYear_RespectsFloor verifies a very small city
// with catastrophic conditions never drops below popMin — this is the
// viability floor per §2a.
func TestApplyPopulationYear_RespectsFloor(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: popMin},
		FoodBalance: -100,
		Happiness:   0,
	}
	for i := 0; i < 10; i++ {
		ApplyPopulationYear(&c)
	}
	if c.Population < popMin {
		t.Errorf("Population = %d, should not drop below popMin=%d", c.Population, popMin)
	}
}

// TestApplyPopulationYear_RespectsCeiling verifies a city already at
// the cap does not grow past it — logistic saturation kicks in.
func TestApplyPopulationYear_RespectsCeiling(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: popMaxCap},
		FoodBalance: 100,
		Happiness:   100,
		Wealth:      1000000,
	}
	ApplyPopulationYear(&c)
	if c.Population > popMaxCap {
		t.Errorf("Population = %d, exceeded popMaxCap=%d", c.Population, popMaxCap)
	}
}

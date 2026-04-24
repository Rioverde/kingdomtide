package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyArmyYear_GrowsToBaseline verifies a freshly-founded city
// with zero Army and a positive treasury raises the §2d baseline
// (2 % of population) on its first tick.
func TestApplyArmyYear_GrowsToBaseline(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		Wealth:     100,
		Army:       0,
	}
	ApplyArmyYear(&c)
	if c.Army != 20 {
		t.Errorf("Army = %d, want 20 (2%% of 1000)", c.Army)
	}
}

// TestApplyArmyYear_DoesNotShrinkDecreeSurge verifies the function
// never pulls a decree-raised army DOWN to baseline. A ruler who
// raised 500 troops on a 1 000-person city (far over 2 %) keeps them
// even after the baseline check — shrinkage only happens via
// attrition on wealth deficit.
func TestApplyArmyYear_DoesNotShrinkDecreeSurge(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		Wealth:     100,
		Army:       500,
	}
	ApplyArmyYear(&c)
	if c.Army != 500 {
		t.Errorf("Army = %d, want 500 (decree surge preserved)", c.Army)
	}
}

// TestApplyArmyYear_AttritionOnDeficit verifies a bankrupt treasury
// shrinks the army by the per-mille attrition rate.
func TestApplyArmyYear_AttritionOnDeficit(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		Wealth:     -50,
		Army:       100,
	}
	ApplyArmyYear(&c)
	// 100 × 30 / 1000 = 3, so Army = 97.
	if c.Army != 97 {
		t.Errorf("Army = %d, want 97 (100 - 3 attrition)", c.Army)
	}
}

// TestApplyArmyYear_AttritionMinOne verifies even a tiny garrison
// loses at least one soldier per bankrupt year — guarantees forward
// progress even when the per-mille would round down to zero.
func TestApplyArmyYear_AttritionMinOne(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 500},
		Wealth:     -10,
		Army:       10,
	}
	ApplyArmyYear(&c)
	if c.Army != 9 {
		t.Errorf("Army = %d, want 9 (10 - 1 min attrition)", c.Army)
	}
}

// TestApplyArmyYear_ClampsAtZero verifies prolonged bankruptcy
// eventually reduces Army to 0 rather than going negative.
func TestApplyArmyYear_ClampsAtZero(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 10},
		Wealth:     -100,
		Army:       1,
	}
	ApplyArmyYear(&c)
	if c.Army != 0 {
		t.Errorf("Army = %d, want 0", c.Army)
	}
}

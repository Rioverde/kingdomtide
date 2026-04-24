package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplySoilFatigueYear_AccumulatesOnDeficit verifies a year of
// food deficit raises SoilFatigue by the canonical accumulation rate.
func TestApplySoilFatigueYear_AccumulatesOnDeficit(t *testing.T) {
	c := polity.City{FoodBalance: -10}
	ApplySoilFatigueYear(&c)
	if c.SoilFatigue != soilFatigueAccumRate {
		t.Errorf("SoilFatigue = %v, want %v (one year of deficit)",
			c.SoilFatigue, soilFatigueAccumRate)
	}
}

// TestApplySoilFatigueYear_RecoversOnSurplus verifies a year of
// surplus reduces SoilFatigue by the canonical recovery rate.
func TestApplySoilFatigueYear_RecoversOnSurplus(t *testing.T) {
	c := polity.City{FoodBalance: 20, SoilFatigue: 0.5}
	ApplySoilFatigueYear(&c)
	want := 0.5 - soilFatigueRecoverRate
	if c.SoilFatigue != want {
		t.Errorf("SoilFatigue = %v, want %v", c.SoilFatigue, want)
	}
}

// TestApplySoilFatigueYear_NeutralRange verifies a food balance
// between the thresholds leaves SoilFatigue unchanged — the land is
// neither over-worked nor fallow.
func TestApplySoilFatigueYear_NeutralRange(t *testing.T) {
	c := polity.City{FoodBalance: 0, SoilFatigue: 0.3}
	ApplySoilFatigueYear(&c)
	if c.SoilFatigue != 0.3 {
		t.Errorf("SoilFatigue = %v, want 0.3 (neutral year, no change)", c.SoilFatigue)
	}
}

// TestApplySoilFatigueYear_ClampsToUnit verifies SoilFatigue never
// escapes [0, 1] even under sustained abuse or recovery.
func TestApplySoilFatigueYear_ClampsToUnit(t *testing.T) {
	maxed := polity.City{FoodBalance: -100, SoilFatigue: 1.0}
	for i := 0; i < 10; i++ {
		ApplySoilFatigueYear(&maxed)
	}
	if maxed.SoilFatigue > 1.0 {
		t.Errorf("max-fatigued: SoilFatigue = %v, escaped ceiling", maxed.SoilFatigue)
	}

	fresh := polity.City{FoodBalance: 100, SoilFatigue: 0}
	for i := 0; i < 10; i++ {
		ApplySoilFatigueYear(&fresh)
	}
	if fresh.SoilFatigue < 0 {
		t.Errorf("fresh-fatigued: SoilFatigue = %v, escaped floor", fresh.SoilFatigue)
	}
}

// TestApplyFoodYear_SoilFatiguePenalty verifies a city with
// SoilFatigue above the cutoff takes the 40 % yield cut.
func TestApplyFoodYear_SoilFatiguePenalty(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: 1000},
		SoilFatigue: 0.9, // above 0.8 cutoff
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyFoodYear(&c, stream)

	// Without penalty: baseYield 100 + D6 variance [-2, +3]
	// → range [98, 103].
	// With penalty (×0.6): range [58, 61].
	if c.FoodBalance > 61 {
		t.Errorf("FoodBalance = %d, expected ≤ 61 after 40%% soil-fatigue cut",
			c.FoodBalance)
	}
}

// TestApplyFoodYear_NoPenaltyBelowCutoff verifies SoilFatigue below
// the cutoff (0.8) leaves the yield alone.
func TestApplyFoodYear_NoPenaltyBelowCutoff(t *testing.T) {
	c := polity.City{
		Settlement:  polity.Settlement{Population: 1000},
		SoilFatigue: 0.5, // below cutoff
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	ApplyFoodYear(&c, stream)

	// Full yield range: [98, 103].
	if c.FoodBalance < 98 || c.FoodBalance > 103 {
		t.Errorf("FoodBalance = %d, expected full range [98, 103]", c.FoodBalance)
	}
}

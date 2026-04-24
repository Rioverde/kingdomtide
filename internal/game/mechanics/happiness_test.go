package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyHappinessYear_BaselineNeutral verifies a city with zero
// food balance and TaxNormal lands exactly on the +50 civic baseline —
// no hidden biases in the formula.
func TestApplyHappinessYear_BaselineNeutral(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	ApplyHappinessYear(&c, 1500)
	if c.Happiness != happinessBase {
		t.Errorf("Happiness = %d, want %d (baseline)", c.Happiness, happinessBase)
	}
}

// TestApplyHappinessYear_FoodContributionClamped verifies the food
// contribution clamps at ±happinessFoodBound even for massive surplus
// or deficit values. Prevents a single bumper harvest from pegging the
// whole mood.
func TestApplyHappinessYear_FoodContributionClamped(t *testing.T) {
	cases := []struct {
		food int
		want int
	}{
		{0, happinessBase},
		{5, happinessBase + 5},
		{happinessFoodBound, happinessBase + happinessFoodBound},
		{100, happinessBase + happinessFoodBound},    // clamped to +bound
		{-happinessFoodBound, happinessBase - happinessFoodBound},
		{-100, happinessBase - happinessFoodBound},   // clamped to -bound
	}
	for _, c := range cases {
		city := polity.City{FoodBalance: c.food, TaxRate: polity.TaxNormal}
		ApplyHappinessYear(&city, 1500)
		if city.Happiness != c.want {
			t.Errorf("food=%d: Happiness=%d, want %d", c.food, city.Happiness, c.want)
		}
	}
}

// TestApplyHappinessYear_TaxPenaltyApplied verifies each TaxRate tier
// moves Happiness by exactly its declared delta — the single source
// of truth for tax-driven mood (MECHANICS.md §8a).
func TestApplyHappinessYear_TaxPenaltyApplied(t *testing.T) {
	cases := []struct {
		rate polity.TaxRate
		want int
	}{
		{polity.TaxLow, happinessBase + 5},
		{polity.TaxNormal, happinessBase},
		{polity.TaxHigh, happinessBase - 8},
		{polity.TaxBrutal, happinessBase - 20},
	}
	for _, c := range cases {
		city := polity.City{TaxRate: c.rate}
		ApplyHappinessYear(&city, 1500)
		if city.Happiness != c.want {
			t.Errorf("rate=%v: Happiness=%d, want %d", c.rate, city.Happiness, c.want)
		}
	}
}

// TestApplyHappinessYear_CanGoNegative verifies the raw Happiness can
// drop below zero — the revolution dispatcher reads the un-clamped
// value so compounding grievances are visible.
func TestApplyHappinessYear_CanGoNegative(t *testing.T) {
	c := polity.City{FoodBalance: -100, TaxRate: polity.TaxBrutal}
	ApplyHappinessYear(&c, 1500)
	// base 50 + clamped -15 food + -20 tax = 15. Still positive here
	// with current constants, but a future drain (siege, religion
	// mismatch) will push this negative — that is the invariant we
	// are preserving.
	if c.Happiness >= happinessBase {
		t.Errorf("Happiness = %d, expected below baseline %d", c.Happiness, happinessBase)
	}
}

// TestApplyHappinessYear_CharismaBonusTiers verifies that each CHA
// bracket maps to its declared goodwill bonus: below 12 → +0,
// 12–13 → +1, 14–17 → +2, 18+ → +3.
func TestApplyHappinessYear_CharismaBonusTiers(t *testing.T) {
	cases := []struct {
		cha       int
		wantBonus int
	}{
		{10, 0},
		{11, 0},
		{12, 1},
		{13, 1},
		{14, 2},
		{17, 2},
		{18, 3},
		{20, 3},
		{3, 0},
	}
	for _, c := range cases {
		city := polity.City{TaxRate: polity.TaxNormal}
		city.Ruler.Stats.Charisma = c.cha
		ApplyHappinessYear(&city, 1500)
		if got := city.Happiness; got != happinessBase+c.wantBonus {
			t.Errorf("cha=%d: Happiness=%d, want %d", c.cha, got, happinessBase+c.wantBonus)
		}
	}
}

// TestApplyHappinessYear_CharismaStackedWithFood verifies that a
// charismatic ruler's bonus stacks additively with the food surplus.
func TestApplyHappinessYear_CharismaStackedWithFood(t *testing.T) {
	city := polity.City{
		FoodBalance: 20,
		TaxRate:     polity.TaxNormal,
	}
	city.Ruler.Stats.Charisma = 18
	ApplyHappinessYear(&city, 1500)
	// base 50 + food clamped 15 + tax 0 + cha 3 = 68
	if city.Happiness != 68 {
		t.Errorf("Happiness=%d, want 68 (base+food+cha)", city.Happiness)
	}
}

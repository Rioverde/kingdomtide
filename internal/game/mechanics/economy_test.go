package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyEconomicYear_TaxIncome verifies income scales linearly with
// population at TaxNormal and that upkeep drain subtracts cleanly.
func TestApplyEconomicYear_TaxIncome(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		TaxRate:    polity.TaxNormal,
		Army:       0,
	}
	ApplyEconomicYear(&c, 1500)
	// 1000 × 1 × 0.17 = 170 income, no upkeep.
	if c.Wealth != 170 {
		t.Errorf("Wealth = %d, want 170 (1000 pop × normal rate)", c.Wealth)
	}
}

// TestApplyEconomicYear_TaxRateTiers verifies every tax tier produces
// its canonical income fraction on a fixed population — the direct
// §8a mapping.
func TestApplyEconomicYear_TaxRateTiers(t *testing.T) {
	const pop = 10000
	cases := []struct {
		rate polity.TaxRate
		want int // 10k pop × 1 base × fraction
	}{
		{polity.TaxLow, 1000},
		{polity.TaxNormal, 1700},
		{polity.TaxHigh, 2800},
		{polity.TaxBrutal, 4500},
	}
	for _, c := range cases {
		city := polity.City{
			Settlement: polity.Settlement{Population: pop},
			TaxRate:    c.rate,
		}
		ApplyEconomicYear(&city, 1500)
		if city.Wealth != c.want {
			t.Errorf("rate=%v: Wealth = %d, want %d", c.rate, city.Wealth, c.want)
		}
	}
}

// TestApplyEconomicYear_UpkeepDrain verifies standing-army upkeep
// subtracts correctly, even when it exceeds income (deficit).
func TestApplyEconomicYear_UpkeepDrain(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 100},
		TaxRate:    polity.TaxNormal,
		Army:       50, // upkeep 50, income 17 — clear deficit
	}
	ApplyEconomicYear(&c, 1500)
	// 100 × 0.17 = 17 income, 50 upkeep, net -33.
	if c.Wealth != -33 {
		t.Errorf("Wealth = %d, want -33 (deficit)", c.Wealth)
	}
}

// TestApplyEconomicYear_AccumulatesOverYears verifies repeated calls
// accumulate Wealth rather than overwriting it — the tick is additive.
func TestApplyEconomicYear_AccumulatesOverYears(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		TaxRate:    polity.TaxNormal,
	}
	for i := 0; i < 3; i++ {
		ApplyEconomicYear(&c, 1500)
	}
	if c.Wealth != 510 {
		t.Errorf("Wealth after 3 years = %d, want 510", c.Wealth)
	}
}

// TestApplyEconomicYear_TradeIncome verifies that a city with maximum
// TradeScore collects the full trade bonus on top of tax income.
func TestApplyEconomicYear_TradeIncome(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		TaxRate:    polity.TaxNormal,
		TradeScore: 100, // max
	}
	ApplyEconomicYear(&c, 1500)
	// 1000 × 0.17 = 170 tax + 100 × 0.5 = 50 trade = 220 total, no army.
	if c.Wealth != 220 {
		t.Errorf("Wealth = %d, want 220 (tax + max trade)", c.Wealth)
	}
}

// TestApplyEconomicYear_ZeroTradeNoBonus verifies that a city with no
// trade activity receives no trade contribution.
func TestApplyEconomicYear_ZeroTradeNoBonus(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		TaxRate:    polity.TaxNormal,
		TradeScore: 0,
	}
	ApplyEconomicYear(&c, 1500)
	if c.Wealth != 170 {
		t.Errorf("Wealth = %d, want 170 (tax only, no trade)", c.Wealth)
	}
}

// TestApplyEconomicYear_TradeAccumulates verifies that trade income
// accumulates correctly across multiple ticks.
func TestApplyEconomicYear_TradeAccumulates(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Population: 1000},
		TaxRate:    polity.TaxNormal,
		TradeScore: 60,
	}
	for i := 0; i < 3; i++ {
		ApplyEconomicYear(&c, 1500)
	}
	// 3 × (170 tax + 30 trade) = 600
	if c.Wealth != 600 {
		t.Errorf("Wealth after 3 yr = %d, want 600", c.Wealth)
	}
}

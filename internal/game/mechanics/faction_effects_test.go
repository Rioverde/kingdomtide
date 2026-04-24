package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestFactionEffect_Merchants_LiftsTradeScore verifies high Merchants
// influence combined with a charismatic ruler boosts TradeScore.
func TestFactionEffect_Merchants_LiftsTradeScore(t *testing.T) {
	mk := func(merchantShare float64) *polity.City {
		c := &polity.City{Settlement: polity.Settlement{Population: 5000}}
		c.Ruler.Stats = stats.CoreStats{
			Strength: 10, Dexterity: 10, Constitution: 10,
			Intelligence: 10, Wisdom: 10, Charisma: 18,
		}
		c.Factions.Set(polity.FactionMerchants, merchantShare)
		return c
	}
	baseline := mk(0)
	boosted := mk(1.0)

	ApplyTradeYear(baseline)
	ApplyTradeYear(boosted)

	if boosted.TradeScore <= baseline.TradeScore {
		t.Errorf("Merchants should boost trade: baseline=%d boosted=%d",
			baseline.TradeScore, boosted.TradeScore)
	}
}

// TestFactionEffect_Merchants_NoEffectAtZero verifies zero Merchant
// influence leaves TradeScore at the baseline computation.
func TestFactionEffect_Merchants_NoEffectAtZero(t *testing.T) {
	zero := &polity.City{Settlement: polity.Settlement{Population: 5000}}
	zero.Ruler.Stats = stats.CoreStats{Charisma: 18}
	control := &polity.City{Settlement: polity.Settlement{Population: 5000}}
	ApplyTradeYear(zero)
	ApplyTradeYear(control)
	if zero.TradeScore != control.TradeScore {
		t.Errorf("zero-influence merchants should match control: zero=%d control=%d",
			zero.TradeScore, control.TradeScore)
	}
}

// TestFactionEffect_Criminals_DrainsWealth verifies high Criminals
// influence siphons a fraction of Wealth each year.
func TestFactionEffect_Criminals_DrainsWealth(t *testing.T) {
	const startingWealth = 10000
	mk := func(criminalShare float64) *polity.City {
		c := &polity.City{
			Settlement: polity.Settlement{Population: 100},
			Wealth:     startingWealth,
			TaxRate:    polity.TaxNormal,
		}
		c.Factions.Set(polity.FactionCriminals, criminalShare)
		return c
	}
	clean := mk(0)
	criminal := mk(1.0)

	ApplyEconomicYear(clean, 1300)
	ApplyEconomicYear(criminal, 1300)

	if criminal.Wealth >= clean.Wealth {
		t.Errorf("Criminals should drain wealth: clean=%d criminal=%d",
			clean.Wealth, criminal.Wealth)
	}
}

// TestFactionEffect_Criminals_NoDrainOnNegativeWealth verifies the
// drain does not push a deficit city into deeper bankruptcy — the
// drain is gated on positive treasury.
func TestFactionEffect_Criminals_NoDrainOnNegativeWealth(t *testing.T) {
	c := &polity.City{
		Settlement: polity.Settlement{Population: 0},
		Wealth:     -500,
		Army:       0,
		TaxRate:    polity.TaxNormal,
	}
	c.Factions.Set(polity.FactionCriminals, 1.0)
	before := c.Wealth
	ApplyEconomicYear(c, 1300)
	// Net change must come from income/upkeep, not criminal drain.
	// With pop=0, income=0, upkeep=0, trade=0, mod=0 → wealth stays
	// at before.
	if c.Wealth != before {
		t.Errorf("negative-wealth city should not drain, before=%d after=%d",
			before, c.Wealth)
	}
}

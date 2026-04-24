package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestTechEffect_Irrigation_LiftsFoodYield verifies Irrigation
// multiplies base food yield — same population, same stream, higher
// balance when Irrigation is unlocked.
func TestTechEffect_Irrigation_LiftsFoodYield(t *testing.T) {
	const pop = 10000 // gives baseYield = 1000, lifted to 1150 by Irrigation
	mk := func(tech polity.Tech, unlock bool) *polity.City {
		c := &polity.City{Settlement: polity.Settlement{Population: pop}}
		if unlock {
			c.Techs.Set(tech)
		}
		return c
	}

	without := mk(polity.TechIrrigation, false)
	with := mk(polity.TechIrrigation, true)

	s1 := dice.New(42, dice.SaltKingdomYear)
	s2 := dice.New(42, dice.SaltKingdomYear)
	ApplyFoodYear(without, s1)
	ApplyFoodYear(with, s2)

	if with.FoodBalance <= without.FoodBalance {
		t.Errorf("Irrigation should lift food: without=%d with=%d",
			without.FoodBalance, with.FoodBalance)
	}
}

// TestTechEffect_Metallurgy_LiftsArmyBaseline verifies Metallurgy
// raises the per-population army baseline by 15 %.
func TestTechEffect_Metallurgy_LiftsArmyBaseline(t *testing.T) {
	const pop = 10000 // baseline 200; with metallurgy 230
	without := &polity.City{
		Settlement: polity.Settlement{Population: pop},
		Wealth:     100,
	}
	with := &polity.City{
		Settlement: polity.Settlement{Population: pop},
		Wealth:     100,
	}
	with.Techs.Set(polity.TechMetallurgy)

	ApplyArmyYear(without)
	ApplyArmyYear(with)

	if with.Army <= without.Army {
		t.Errorf("Metallurgy should lift baseline: without=%d with=%d",
			without.Army, with.Army)
	}
	// Floating: 10000 × 0.02 × 1.15 = 230.0 in exact math but
	// 229.99… in IEEE-754 — int truncation lands at 229. Accept a
	// tolerance band.
	if with.Army < 228 || with.Army > 231 {
		t.Errorf("Metallurgy baseline: got %d, want ~230", with.Army)
	}
}

// TestTechEffect_Navigation_AddsTradeBonus verifies Navigation lifts
// TradeScore by a flat bonus.
func TestTechEffect_Navigation_AddsTradeBonus(t *testing.T) {
	without := &polity.City{Settlement: polity.Settlement{Population: 5000}}
	with := &polity.City{Settlement: polity.Settlement{Population: 5000}}
	with.Techs.Set(polity.TechNavigation)

	ApplyTradeYear(without)
	ApplyTradeYear(with)

	if with.TradeScore <= without.TradeScore {
		t.Errorf("Navigation should lift trade: without=%d with=%d",
			without.TradeScore, with.TradeScore)
	}
}

// TestTechEffect_Banking_LiftsTradeScore verifies Banking multiplies
// the trade score (1.3×), producing a bigger TradeScore than baseline.
func TestTechEffect_Banking_LiftsTradeScore(t *testing.T) {
	without := &polity.City{Settlement: polity.Settlement{Population: 1000}}
	with := &polity.City{Settlement: polity.Settlement{Population: 1000}}
	with.Techs.Set(polity.TechBanking)

	ApplyTradeYear(without)
	ApplyTradeYear(with)

	if with.TradeScore <= without.TradeScore {
		t.Errorf("Banking should multiply trade: without=%d with=%d",
			without.TradeScore, with.TradeScore)
	}
}

// TestTechEffect_Banking_HalvesTributeRate verifies Banking-unlocked
// vassals pay half the tribute rate.
func TestTechEffect_Banking_HalvesTributeRate(t *testing.T) {
	baseline := techTributeRate(&polity.City{}, 0.10)
	banked := &polity.City{}
	banked.Techs.Set(polity.TechBanking)
	withBank := techTributeRate(banked, 0.10)
	if withBank >= baseline {
		t.Errorf("Banking should halve tribute rate: without=%v with=%v",
			baseline, withBank)
	}
	if withBank != 0.05 {
		t.Errorf("Banking tribute rate: got %v, want 0.05", withBank)
	}
}

// TestTechEffect_Calendar_ShrinksFoodVariance verifies Calendar
// narrows the food-balance variance band across many rolls.
func TestTechEffect_Calendar_ShrinksFoodVariance(t *testing.T) {
	const pop = 1000
	const rolls = 2000
	baseYield := pop / harvestLaborDivisor

	withoutDev := 0
	withDev := 0
	s1 := dice.New(42, dice.SaltKingdomYear)
	s2 := dice.New(42, dice.SaltKingdomYear)
	for i := 0; i < rolls; i++ {
		without := &polity.City{Settlement: polity.Settlement{Population: pop}}
		with := &polity.City{Settlement: polity.Settlement{Population: pop}}
		with.Techs.Set(polity.TechCalendar)
		ApplyFoodYear(without, s1)
		ApplyFoodYear(with, s2)
		dWithout := without.FoodBalance - baseYield
		dWith := with.FoodBalance - baseYield
		if dWithout < 0 {
			dWithout = -dWithout
		}
		if dWith < 0 {
			dWith = -dWith
		}
		withoutDev += dWithout
		withDev += dWith
	}
	if withDev >= withoutDev {
		t.Errorf("Calendar should shrink variance: baseline=%d calendar=%d",
			withoutDev, withDev)
	}
}

// TestTechEffect_Writing_LowersDecreeDC verifies Writing makes decrees
// easier to execute. The D20 execution roll drops its DC from 15 to
// 14 — over many attempts Army / TaxRate / TradeScore move MORE
// often than without the tech because a roll of 14 + CHA=0 mod now
// succeeds where before it backlashed.
func TestTechEffect_Writing_LowersDecreeDC(t *testing.T) {
	mk := func(unlock bool) *polity.City {
		c := &polity.City{
			Settlement: polity.Settlement{Population: 1000},
			TaxRate:    polity.TaxNormal,
			Happiness:  60,
		}
		c.Ruler.Stats = stats.CoreStats{
			Strength: 10, Dexterity: 10, Constitution: 10,
			Intelligence: 10, Wisdom: 10, Charisma: 10,
		}
		if unlock {
			c.Techs.Set(polity.TechWriting)
		}
		return c
	}
	without := mk(false)
	with := mk(true)
	s1 := dice.New(42, dice.SaltKingdomYear)
	s2 := dice.New(42, dice.SaltKingdomYear)

	successWithout := 0
	successWith := 0
	for i := 0; i < 2000; i++ {
		before := *without
		ApplyDecreeYear(without, s1, 1500+i)
		// Success-only indicators: Army changed (RaiseArmy /
		// Fortification), TaxRate stepped (Raise/LowerTax), or
		// TradeScore lifted (TradePost). Backlash only queues
		// happiness mods — those do not count here.
		if without.Army != before.Army ||
			without.TaxRate != before.TaxRate ||
			without.TradeScore != before.TradeScore {
			successWithout++
		}

		b2 := *with
		ApplyDecreeYear(with, s2, 1500+i)
		if with.Army != b2.Army ||
			with.TaxRate != b2.TaxRate ||
			with.TradeScore != b2.TradeScore {
			successWith++
		}
	}
	if successWith <= successWithout {
		t.Errorf("Writing should raise decree success rate: "+
			"without=%d with=%d", successWithout, successWith)
	}
}

// TestTechEffect_Printing_LowersRevolutionDC verifies Printing lowers
// the revolution DC — helper reports the correct reduction.
func TestTechEffect_Printing_LowersRevolutionDC(t *testing.T) {
	without := &polity.City{}
	with := &polity.City{}
	with.Techs.Set(polity.TechPrinting)

	if techRevolutionDCReduction(without) != 0 {
		t.Errorf("no-tech reduction should be 0, got %d",
			techRevolutionDCReduction(without))
	}
	if techRevolutionDCReduction(with) != techPrintingRevolutionDCReduction {
		t.Errorf("Printing reduction: got %d want %d",
			techRevolutionDCReduction(with), techPrintingRevolutionDCReduction)
	}
}

// TestTechEffect_Printing_LowersSchismThreshold verifies Printing
// lowers the schism innovation gate.
func TestTechEffect_Printing_LowersSchismThreshold(t *testing.T) {
	without := &polity.City{}
	with := &polity.City{}
	with.Techs.Set(polity.TechPrinting)

	if techSchismThresholdReduction(without) != 0 {
		t.Errorf("no-tech reduction should be 0, got %d",
			techSchismThresholdReduction(without))
	}
	if techSchismThresholdReduction(with) != techPrintingSchismThresholdReduction {
		t.Errorf("Printing schism reduction: got %d want %d",
			techSchismThresholdReduction(with),
			techPrintingSchismThresholdReduction)
	}
}

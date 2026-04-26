package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestHappinessReligion_MatchGivesBonus verifies that a ruler whose
// faith matches the city majority earns the religion-alignment bonus.
func TestHappinessReligion_MatchGivesBonus(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()},
		TaxRate:    polity.TaxNormal,
	}
	c.Ruler.Faith = polity.FaithOldGods // majority in default distribution
	ApplyHappinessYear(&c, 1500)
	// base 50 + 0 food + 0 tax + 0 cha + 8 religion = 58
	if c.Happiness != 58 {
		t.Errorf("Happiness = %d, want 58", c.Happiness)
	}
}

// TestHappinessReligion_MismatchGivesPenalty verifies that a ruler
// whose faith is a minority earns the mismatch penalty.
func TestHappinessReligion_MismatchGivesPenalty(t *testing.T) {
	c := polity.City{
		Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()},
		TaxRate:    polity.TaxNormal,
	}
	c.Ruler.Faith = polity.FaithSunCovenant // minority in default distribution
	ApplyHappinessYear(&c, 1500)
	// base 50 - 8 religion = 42
	if c.Happiness != 42 {
		t.Errorf("Happiness = %d, want 42", c.Happiness)
	}
}

// TestHappinessFaction_MerchantsBoost verifies full Merchants influence
// adds the maximum mercantile civic bonus.
func TestHappinessFaction_MerchantsBoost(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	c.Factions.Set(polity.FactionMerchants, 1.0)
	ApplyHappinessYear(&c, 1500)
	// base 50 + 6 merchants = 56 (no religion since Faiths is nil)
	if c.Happiness != 56 {
		t.Errorf("Happiness = %d, want 56", c.Happiness)
	}
}

// TestHappinessFaction_MilitaryPenalty verifies full Military influence
// applies the maximum fatigue penalty.
func TestHappinessFaction_MilitaryPenalty(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	c.Factions.Set(polity.FactionMilitary, 1.0)
	ApplyHappinessYear(&c, 1500)
	// base 50 - 6 military = 44
	if c.Happiness != 44 {
		t.Errorf("Happiness = %d, want 44", c.Happiness)
	}
}

// TestHappinessGreatPerson_PresentBonus verifies that a living great
// person contributes the presence bonus.
func TestHappinessGreatPerson_PresentBonus(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	c.GreatPerson = &polity.GreatPerson{
		Kind:      polity.GreatPersonScholar,
		DeathYear: 0, // alive
	}
	ApplyHappinessYear(&c, 1500)
	// base 50 + 3 great person = 53
	if c.Happiness != 53 {
		t.Errorf("Happiness = %d, want 53", c.Happiness)
	}
}

// TestHappinessAllFactors_Stacking verifies all §2g factors stack
// correctly when every contributor is active simultaneously. The raw
// positive-extras sum (CHA 3 + religion 8 + Merchants 6 + GP 3 = 20)
// is clamped by happinessPositiveContribCap to 15 — without that cap
// a fully buffed city would shrug off Brutal tax indefinitely (see
// the cap's doc on happiness.go for the full rationale).
func TestHappinessAllFactors_Stacking(t *testing.T) {
	c := polity.City{
		FoodBalance: 20,
		TaxRate:     polity.TaxLow,
		Settlement:  polity.Settlement{Population: 1000, Faiths: polity.NewFaithDistribution()},
	}
	c.Ruler.Stats.Charisma = 18
	c.Ruler.Faith = polity.FaithOldGods // matches majority
	c.Factions.Set(polity.FactionMerchants, 1.0)
	c.GreatPerson = &polity.GreatPerson{Kind: polity.GreatPersonScholar, DeathYear: 0}
	ApplyHappinessYear(&c, 1500)
	// 50 (base) + 15 (food clamped) + 5 (low tax) + 15 (positive-extras
	// cap: 3 cha + 8 religion + 6 merchants + 3 GP = 20, capped) = 85.
	want := happinessBase + happinessFoodBound +
		polity.TaxLow.HappinessDelta() + happinessPositiveContribCap
	if c.Happiness != want {
		t.Errorf("Happiness = %d, want %d", c.Happiness, want)
	}
}

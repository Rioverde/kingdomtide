package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestGreatPersonEffect_Scholar_BoostsInnovation verifies a living
// Scholar adds +3 Innovation per year on top of the baseline.
func TestGreatPersonEffect_Scholar_BoostsInnovation(t *testing.T) {
	mk := func(withScholar bool) *polity.City {
		c := &polity.City{
			Settlement: polity.Settlement{Population: 1000},
		}
		c.Ruler.Stats = stats.CoreStats{
			Strength: 10, Dexterity: 10, Constitution: 10,
			Intelligence: 10, Wisdom: 10, Charisma: 10,
		}
		c.Faiths = polity.NewFaithDistribution()
		if withScholar {
			c.GreatPerson = &polity.GreatPerson{
				Kind:      polity.GreatPersonScholar,
				BirthYear: 1300,
				DeathYear: 1340,
			}
		}
		return c
	}
	without := mk(false)
	with := mk(true)
	s1 := dice.New(42, dice.SaltKingdomYear)
	s2 := dice.New(42, dice.SaltKingdomYear)

	ApplyTechnologyYear(without, s1)
	ApplyTechnologyYear(with, s2)

	diff := with.Innovation - without.Innovation
	if diff < float64(scholarInnovationBonus-1) ||
		diff > float64(scholarInnovationBonus+1) {
		t.Errorf("Scholar innovation diff = %v, want ~%d",
			diff, scholarInnovationBonus)
	}
}

// TestGreatPersonEffect_General_BoostsArmyBaseline verifies a living
// General lifts the army baseline by ~25 %.
func TestGreatPersonEffect_General_BoostsArmyBaseline(t *testing.T) {
	const pop = 10000 // baseline 200; with General 250
	without := &polity.City{
		Settlement: polity.Settlement{Population: pop},
		Wealth:     100,
	}
	with := &polity.City{
		Settlement: polity.Settlement{Population: pop},
		Wealth:     100,
	}
	with.GreatPerson = &polity.GreatPerson{
		Kind:      polity.GreatPersonGeneral,
		BirthYear: 1300,
		DeathYear: 1340,
	}

	ApplyArmyYear(without)
	ApplyArmyYear(with)

	if with.Army <= without.Army {
		t.Errorf("General should lift army baseline: without=%d with=%d",
			without.Army, with.Army)
	}
	if with.Army != 250 {
		t.Errorf("General army baseline: got %d, want 250", with.Army)
	}
}

// TestGreatPersonEffect_Priest_BoostsReligionPulse verifies a living
// Priest doubles the diffusion pulse — majority faith grows faster.
func TestGreatPersonEffect_Priest_BoostsReligionPulse(t *testing.T) {
	mk := func(withPriest bool) *polity.City {
		c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
		c.Faiths[polity.FaithOldGods] = 0.55
		c.Faiths[polity.FaithSunCovenant] = 0.45
		c.Faiths[polity.FaithGreenSage] = 0
		c.Faiths[polity.FaithOneOath] = 0
		c.Faiths[polity.FaithStormPact] = 0
		if withPriest {
			c.GreatPerson = &polity.GreatPerson{
				Kind:      polity.GreatPersonPriest,
				BirthYear: 1300,
				DeathYear: 1340,
			}
		}
		return c
	}
	without := mk(false)
	with := mk(true)
	s1 := dice.New(42, dice.SaltReligion)
	s2 := dice.New(42, dice.SaltReligion)

	beforeWithout := without.Faiths[polity.FaithOldGods]
	beforeWith := with.Faiths[polity.FaithOldGods]

	ApplyReligionDiffusionYear(without, s1, 1500)
	ApplyReligionDiffusionYear(with, s2, 1500)

	growthWithout := without.Faiths[polity.FaithOldGods] - beforeWithout
	growthWith := with.Faiths[polity.FaithOldGods] - beforeWith

	// With Priest the growth should be ~2× without. Allow floating
	// slack for Normalize's rounding.
	if growthWith <= growthWithout {
		t.Errorf("Priest should accelerate diffusion: without=%v with=%v",
			growthWithout, growthWith)
	}
}

// TestGreatPersonEffect_Priest_SchismHarder verifies a living Priest
// raises the schism innovation gate, blocking a schism that would
// otherwise fire.
func TestGreatPersonEffect_Priest_SchismHarder(t *testing.T) {
	mk := func(withPriest bool) *polity.City {
		c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
		c.Faiths[polity.FaithOldGods] = 0.55
		c.Faiths[polity.FaithSunCovenant] = 0.45
		c.Faiths[polity.FaithGreenSage] = 0
		c.Faiths[polity.FaithOneOath] = 0
		c.Faiths[polity.FaithStormPact] = 0
		c.Innovation = 47 // just above the base 45 gate
		if withPriest {
			c.GreatPerson = &polity.GreatPerson{
				Kind:      polity.GreatPersonPriest,
				BirthYear: 1300,
				DeathYear: 1340,
			}
		}
		return c
	}
	without := mk(false)
	with := mk(true)
	s1 := dice.New(42, dice.SaltReligion)
	s2 := dice.New(42, dice.SaltReligion)

	ApplyReligionDiffusionYear(without, s1, 1500)
	ApplyReligionDiffusionYear(with, s2, 1500)

	// Without Priest at innovation 47 the 4-gate schism opens (45
	// threshold passed) and snaps the split to 0.6 / 0.4.
	if without.Faiths[polity.FaithOldGods] < 0.58 ||
		without.Faiths[polity.FaithOldGods] > 0.62 {
		t.Errorf("baseline schism should snap to ~0.6: got %v",
			without.Faiths[polity.FaithOldGods])
	}
	// With Priest the gate lifts to 50 — innovation 47 is not enough
	// so the split should NOT snap to 60/40.
	if with.Faiths[polity.FaithOldGods] > 0.58 &&
		with.Faiths[polity.FaithOldGods] < 0.62 {
		t.Errorf("Priest should block schism at innovation 47, got %v",
			with.Faiths[polity.FaithOldGods])
	}
}

// TestGreatPersonEffect_NilDoesNotApply verifies a city without a
// great person (nil pointer — i.e. none or already expired) does not
// grant bonuses.
func TestGreatPersonEffect_NilDoesNotApply(t *testing.T) {
	c := &polity.City{
		Settlement: polity.Settlement{Population: 10000},
		Wealth:     100,
	}
	// Leave c.GreatPerson nil — the post-expiry / no-great-person
	// state.
	if greatPersonOf(c, polity.GreatPersonGeneral) {
		t.Errorf("nil great person should not count as present")
	}
	ApplyArmyYear(c)
	// No General → baseline stays at 200 (2 % of 10 000).
	if c.Army != 200 {
		t.Errorf("no General army: got %d, want 200", c.Army)
	}
}

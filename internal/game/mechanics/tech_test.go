package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// newTechCity returns a baseline city with neutral ruler stats and
// the default faith distribution — the zero value of FaithOldGods
// majority so GreenSage bonus does not fire by accident.
func newTechCity() *polity.City {
	return &polity.City{
		Settlement: polity.Settlement{
			Faiths: polity.NewFaithDistribution(),
			Ruler:  polity.Ruler{Stats: stats.DefaultCoreStats()},
		},
	}
}

// TestApplyTechnologyYear_InnovationGrows verifies a neutral city
// sees Innovation increase after one tick — the base D3 term alone
// guarantees strictly positive growth.
func TestApplyTechnologyYear_InnovationGrows(t *testing.T) {
	c := newTechCity()
	before := c.Innovation
	ApplyTechnologyYear(c, dice.New(42, dice.SaltTech))
	if c.Innovation <= before {
		t.Errorf("Innovation did not grow: before=%v after=%v", before, c.Innovation)
	}
}

// TestApplyTechnologyYear_MagesBonus verifies high Mages faction
// influence produces strictly more innovation than zero influence.
func TestApplyTechnologyYear_MagesBonus(t *testing.T) {
	c := newTechCity()
	c.Factions.Set(polity.FactionMages, 1.0)

	ref := newTechCity()
	ref.Factions.Set(polity.FactionMages, 0.0)

	ApplyTechnologyYear(c, dice.New(42, dice.SaltTech))
	ApplyTechnologyYear(ref, dice.New(42, dice.SaltTech))

	if c.Innovation <= ref.Innovation {
		t.Errorf("Mages bonus did not boost: withBonus=%v withoutBonus=%v",
			c.Innovation, ref.Innovation)
	}
}

// TestApplyTechnologyYear_GreenSageBonus verifies a GreenSage
// majority faith beats the OldGods default city on innovation gain.
func TestApplyTechnologyYear_GreenSageBonus(t *testing.T) {
	c := newTechCity()
	c.Faiths[polity.FaithOldGods] = 0
	c.Faiths[polity.FaithGreenSage] = 1.0

	ref := newTechCity()

	ApplyTechnologyYear(c, dice.New(42, dice.SaltTech))
	ApplyTechnologyYear(ref, dice.New(42, dice.SaltTech))

	if c.Innovation <= ref.Innovation {
		t.Errorf("GreenSage bonus did not boost: greenSage=%v oldGods=%v",
			c.Innovation, ref.Innovation)
	}
}

// TestApplyTechnologyYear_UnlocksAtThreshold verifies a city with
// Innovation already at the threshold unlocks the tech on this tick.
func TestApplyTechnologyYear_UnlocksAtThreshold(t *testing.T) {
	c := newTechCity()
	c.Innovation = float64(polity.TechIrrigation.InnovationThreshold())
	ApplyTechnologyYear(c, dice.New(42, dice.SaltTech))
	if !c.Techs.Has(polity.TechIrrigation) {
		t.Errorf("TechIrrigation did not unlock at threshold, techs=%v",
			c.Techs.Unlocked())
	}
}

// TestApplyTechnologyYear_MonotonicTechs verifies already-unlocked
// techs remain set after another tick — the mask grows monotonically.
func TestApplyTechnologyYear_MonotonicTechs(t *testing.T) {
	c := newTechCity()
	c.Techs.Set(polity.TechIrrigation)
	c.Techs.Set(polity.TechMasonry)
	ApplyTechnologyYear(c, dice.New(42, dice.SaltTech))
	if !c.Techs.Has(polity.TechIrrigation) || !c.Techs.Has(polity.TechMasonry) {
		t.Errorf("previously-unlocked techs were lost: %v", c.Techs.Unlocked())
	}
}

// TestApplyTechnologyYear_Determinism verifies two identical cities
// on the same stream produce identical Innovation and Techs.
func TestApplyTechnologyYear_Determinism(t *testing.T) {
	a := newTechCity()
	b := newTechCity()
	ApplyTechnologyYear(a, dice.New(42, dice.SaltTech))
	ApplyTechnologyYear(b, dice.New(42, dice.SaltTech))
	if a.Innovation != b.Innovation || a.Techs != b.Techs {
		t.Errorf("diverged: a=(%v, %v) b=(%v, %v)",
			a.Innovation, a.Techs, b.Innovation, b.Techs)
	}
}

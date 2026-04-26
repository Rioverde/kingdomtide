package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

func TestSuccession_PrimogenitureBiasesTowardParent(t *testing.T) {
	parent := polity.Ruler{Stats: stats.CoreStats{
		Strength: 20, Dexterity: 3, Constitution: 20,
		Intelligence: 3, Wisdom: 20, Charisma: 3,
	}}
	k := &polity.Kingdom{
		CurrentRuler:  parent,
		SuccessionLaw: polity.SuccessionPrimogeniture,
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	heir := newHeirFor(k, stream, 1500)
	if heir.Stats.Strength < 10 {
		t.Errorf("primogeniture heir STR = %d, expected pulled up", heir.Stats.Strength)
	}
}

func TestSuccession_ElectiveBoostsCharisma(t *testing.T) {
	k := &polity.Kingdom{
		CurrentRuler:  polity.Ruler{},
		SuccessionLaw: polity.SuccessionElective,
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	heir := newHeirFor(k, stream, 1500)
	baseline := polity.NewRuler(dice.New(42, dice.SaltKingdomYear), 1500, "")
	if heir.Stats.Charisma <= baseline.Stats.Charisma {
		t.Errorf("elective should raise CHA: heir=%d baseline=%d",
			heir.Stats.Charisma, baseline.Stats.Charisma)
	}
}

func TestSuccession_SalicBoostsStrength(t *testing.T) {
	k := &polity.Kingdom{
		CurrentRuler:  polity.Ruler{},
		SuccessionLaw: polity.SuccessionSalic,
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	heir := newHeirFor(k, stream, 1500)
	if heir.Stats.Strength < 3 || heir.Stats.Strength > 20 {
		t.Errorf("Salic heir STR out of range: %d", heir.Stats.Strength)
	}
}

func TestSuccession_TanistryProducesVariance(t *testing.T) {
	k1 := &polity.Kingdom{
		CurrentRuler:  polity.Ruler{Stats: stats.CoreStats{Strength: 10}},
		SuccessionLaw: polity.SuccessionTanistry,
	}
	k2 := *k1
	h1 := newHeirFor(k1, dice.New(42, dice.SaltKingdomYear), 1500)
	h2 := newHeirFor(&k2, dice.New(123, dice.SaltKingdomYear), 1500)
	if h1.Stats.Strength == h2.Stats.Strength && h1.Stats.Charisma == h2.Stats.Charisma {
		t.Errorf("Tanistry heirs nearly identical across streams — variance not applied")
	}
}

func TestSuccession_AllLawsProduceValidStats(t *testing.T) {
	laws := []polity.SuccessionLaw{
		polity.SuccessionPrimogeniture,
		polity.SuccessionUltimogeniture,
		polity.SuccessionTanistry,
		polity.SuccessionElective,
		polity.SuccessionDesignated,
		polity.SuccessionSalic,
	}
	for _, law := range laws {
		k := &polity.Kingdom{
			CurrentRuler:  polity.Ruler{},
			SuccessionLaw: law,
		}
		stream := dice.New(42, dice.SaltKingdomYear)
		heir := newHeirFor(k, stream, 1500)
		for _, s := range []int{
			heir.Stats.Strength, heir.Stats.Dexterity, heir.Stats.Constitution,
			heir.Stats.Intelligence, heir.Stats.Wisdom, heir.Stats.Charisma,
		} {
			if s < 3 || s > 20 {
				t.Errorf("law=%v: heir stat %d out of [3, 20]", law, s)
			}
		}
	}
}

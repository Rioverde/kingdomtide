package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestPromote_CampToHamlet_AfterSustainedHigh(t *testing.T) {
	st := newState(42, 5)
	c := newCamp(1, 0, 0, polity.RegionNormal, 100, polity.FaithOldGods)
	st.settlements[1] = c
	for y := 0; y < simHamletPromoteSustain; y++ {
		st.tickPromotions(y)
	}
	if _, isHamlet := st.settlements[1].(*polity.Hamlet); !isHamlet {
		t.Errorf("expected Hamlet after sustained high pop, got %T", st.settlements[1])
	}
}

func TestPromote_CampStaysCamp_WhenPopFlaps(t *testing.T) {
	st := newState(42, 5)
	c := newCamp(1, 0, 0, polity.RegionNormal, 100, polity.FaithOldGods)
	st.settlements[1] = c
	st.tickPromotions(0) // streak = 1
	c.Population = 20    // drop below threshold
	st.tickPromotions(1) // streak resets to 0
	c.Population = 100
	st.tickPromotions(2) // streak = 1 again
	if _, isHamlet := st.settlements[1].(*polity.Hamlet); isHamlet {
		t.Error("flapping pop should not promote")
	}
}

func TestPromote_HamletToVillage_AfterSustainedHigh(t *testing.T) {
	st := newState(42, 5)
	h := &polity.Hamlet{Settlement: polity.Settlement{
		ID:         1,
		Tier:       polity.TierHamlet,
		Population: 200,
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = h
	for y := 0; y < simVillagePromoteSustain; y++ {
		st.tickPromotions(y)
	}
	if _, isVillage := st.settlements[1].(*polity.Village); !isVillage {
		t.Errorf("expected Village after sustained high pop, got %T", st.settlements[1])
	}
}

func TestPromote_HamletStaysHamlet_NearVillage(t *testing.T) {
	st := newState(42, 5)
	v := &polity.Village{Settlement: polity.Settlement{
		ID:         1,
		Tier:       polity.TierVillage,
		Population: 300,
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = v

	h := &polity.Hamlet{Settlement: polity.Settlement{
		ID:         2,
		Tier:       polity.TierHamlet,
		Population: 200,
		Region:     polity.RegionNormal,
		// Position zero — within exclusivity radius of village 1 at position zero.
	}}
	st.settlements[2] = h

	for y := 0; y < simVillagePromoteSustain*5; y++ {
		st.tickPromotions(y)
	}
	if _, isVillage := st.settlements[2].(*polity.Village); isVillage {
		t.Error("hamlet near existing village should not promote")
	}
}

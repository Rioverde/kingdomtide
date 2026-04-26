package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestPopulationDynamics_NetGrowth — net rate ~+0.2%/yr in good years.
func TestPopulationDynamics_NetGrowth(t *testing.T) {
	st := newState(42, 1)
	c := &polity.Camp{Settlement: polity.Settlement{
		ID:         1,
		Population: 1000,
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = c
	// No famine.
	st.tickPopulation(0)
	delta := c.Population - 1000
	if delta < 0 || delta > 12 {
		t.Errorf("expected modest positive growth (~+9 over 1 year on pop 1000), got %d", delta)
	}
}

// TestPopulationDynamics_FamineKills — famine multiplier produces a
// large negative delta.
func TestPopulationDynamics_FamineKills(t *testing.T) {
	st := newState(42, 1)
	st.regionFamine[polity.RegionNormal][0] = true
	c := &polity.Camp{Settlement: polity.Settlement{
		ID:         1,
		Population: 1000,
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = c
	st.tickPopulation(0)
	delta := c.Population - 1000
	if delta >= 0 {
		t.Errorf("expected negative delta in famine year, got %d", delta)
	}
}

// TestDeaths_AbandonsCampAfter3LowYears — camps below floor for
// simAbandonStreakYears consecutive years are removed.
func TestDeaths_AbandonsCampAfter3LowYears(t *testing.T) {
	st := newState(42, 5)
	c := &polity.Camp{Settlement: polity.Settlement{
		ID:         1,
		Population: 1, // below simCampPopAbandonFloor=2
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = c
	for y := 0; y < simAbandonStreakYears; y++ {
		st.tickDeaths(y)
	}
	if _, alive := st.settlements[1]; alive {
		t.Error("expected camp to be abandoned after streak years")
	}
}

// TestDeaths_PreservesHealthyCamp — camp with pop above floor stays.
func TestDeaths_PreservesHealthyCamp(t *testing.T) {
	st := newState(42, 5)
	c := &polity.Camp{Settlement: polity.Settlement{
		ID:         1,
		Population: 50,
		Region:     polity.RegionNormal,
	}}
	st.settlements[1] = c
	for y := 0; y < 10; y++ {
		st.tickDeaths(y)
	}
	if _, alive := st.settlements[1]; !alive {
		t.Error("expected healthy camp to remain")
	}
}

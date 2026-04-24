package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestDeriveBaseRank_PopulationThresholds verifies each of the four
// population tiers maps to the correct rank when age is below the
// bonus threshold.
func TestDeriveBaseRank_PopulationThresholds(t *testing.T) {
	cases := []struct {
		pop  int
		want polity.BaseRank
	}{
		{0, polity.RankHamlet},
		{199, polity.RankHamlet},
		{200, polity.RankTown},
		{1999, polity.RankTown},
		{2000, polity.RankCity},
		{19999, polity.RankCity},
		{20000, polity.RankMetropolis},
		{40000, polity.RankMetropolis},
	}
	for _, c := range cases {
		if got := DeriveBaseRank(c.pop, 0); got != c.want {
			t.Errorf("pop=%d age=0: got %v, want %v", c.pop, got, c.want)
		}
	}
}

// TestDeriveBaseRank_AgeBumpsRank verifies an ancient settlement is
// bumped one tier up from what its population alone would yield — a
// thousand-year hamlet becomes a town.
func TestDeriveBaseRank_AgeBumpsRank(t *testing.T) {
	// Young hamlet stays hamlet.
	if got := DeriveBaseRank(100, 100); got != polity.RankHamlet {
		t.Errorf("young hamlet: got %v, want RankHamlet", got)
	}
	// Ancient hamlet gets bumped to Town.
	if got := DeriveBaseRank(100, rankAgeBoostYears); got != polity.RankTown {
		t.Errorf("ancient hamlet: got %v, want RankTown", got)
	}
}

// TestDeriveBaseRank_AgeBumpCapsAtMetropolis verifies the age bonus
// does not push an already-Metropolis city above itself — clamps at
// the top rank.
func TestDeriveBaseRank_AgeBumpCapsAtMetropolis(t *testing.T) {
	got := DeriveBaseRank(40000, rankAgeBoostYears*2)
	if got != polity.RankMetropolis {
		t.Errorf("ancient metropolis: got %v, want RankMetropolis (no overflow)", got)
	}
}

// TestApplyRankYear_UpdatesBaseRank verifies the tick variant writes
// the derived rank back to the city. Round-trip check: read
// population, compute rank, find it on the city.
func TestApplyRankYear_UpdatesBaseRank(t *testing.T) {
	c := polity.NewCity("Anglaria", geom.Position{}, 1400, polity.Ruler{})
	c.Population = 5000
	ApplyRankYear(c, 1500)
	if c.BaseRank != polity.RankCity {
		t.Errorf("BaseRank = %v, want RankCity (pop 5000)", c.BaseRank)
	}
}

// TestApplyRankYear_DoesNotTouchEffectiveRank verifies the step only
// writes BaseRank — EffectiveRank is the kingdom sim's responsibility
// and must survive untouched.
func TestApplyRankYear_DoesNotTouchEffectiveRank(t *testing.T) {
	c := polity.NewCity("Anglaria", geom.Position{}, 1400, polity.Ruler{})
	c.Population = 10
	c.EffectiveRank = polity.RankCapital
	ApplyRankYear(c, 1500)
	if c.EffectiveRank != polity.RankCapital {
		t.Errorf("EffectiveRank = %v, want RankCapital (untouched)", c.EffectiveRank)
	}
}

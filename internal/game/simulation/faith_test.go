package simulation

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestBlendFaiths_PopulationWeighted(t *testing.T) {
	a := polity.FaithDistribution{}
	a[polity.FaithOldGods] = 1.0
	b := polity.FaithDistribution{}
	b[polity.FaithSunCovenant] = 1.0
	got := blendFaiths(a, b, 60, 40)
	if math.Abs(got[polity.FaithOldGods]-0.6) > 1e-6 {
		t.Errorf("expected OldGods 0.6, got %.4f", got[polity.FaithOldGods])
	}
	if math.Abs(got[polity.FaithSunCovenant]-0.4) > 1e-6 {
		t.Errorf("expected SunCovenant 0.4, got %.4f", got[polity.FaithSunCovenant])
	}
}

func TestApplyFaithConversion_PreservesNormalization(t *testing.T) {
	fd := polity.FaithDistribution{}
	fd[polity.FaithOldGods] = 1.0
	for i := 0; i < 50; i++ {
		applyFaithConversion(&fd, polity.RegionNormal)
	}
	var sum float64
	for _, v := range fd {
		sum += v
	}
	if math.Abs(sum-1.0) > 1e-6 {
		t.Errorf("expected sum 1.0, got %.6f", sum)
	}
}

// TestApplyFaithConversion_ExtinctStaysExtinct verifies the fix-2
// symmetry contract: faiths starting below simFaithEpsilon receive no
// dissidence inflow and stay extinct. A starting 100% OldGods
// distribution in a Normal region (which prefers OldGods) should keep
// every minority faith pinned at 0 — there is no source to seed them.
func TestApplyFaithConversion_ExtinctStaysExtinct(t *testing.T) {
	fd := polity.FaithDistribution{}
	fd[polity.FaithOldGods] = 1.0
	for i := 0; i < 200; i++ {
		applyFaithConversion(&fd, polity.RegionNormal)
	}
	for i, v := range fd {
		if i == int(polity.FaithOldGods) {
			continue
		}
		if v != 0 {
			t.Errorf("faith %d should stay extinct under symmetric bookkeeping, got %.6f",
				i, v)
		}
	}
	// Majority should remain ~1.0 (it self-decays via dissidence into
	// nothing, but renormalization restores it to 1.0 each tick).
	if fd[polity.FaithOldGods] < 0.999 {
		t.Errorf("majority should remain ~1.0 in single-faith state, got %.6f",
			fd[polity.FaithOldGods])
	}
}

// TestApplyFaithConversion_ActiveMinoritiesPersist verifies that minorities
// already above ε continue to exchange via conformity/dissidence and stay
// alive over time.
func TestApplyFaithConversion_ActiveMinoritiesPersist(t *testing.T) {
	fd := polity.FaithDistribution{}
	fd[polity.FaithOldGods] = 0.7
	fd[polity.FaithSunCovenant] = 0.2
	fd[polity.FaithGreenSage] = 0.1
	for i := 0; i < 200; i++ {
		applyFaithConversion(&fd, polity.RegionNormal)
	}
	if fd[polity.FaithSunCovenant] < simFaithEpsilon {
		t.Errorf("active SunCovenant minority went extinct: %.6f", fd[polity.FaithSunCovenant])
	}
	if fd[polity.FaithGreenSage] < simFaithEpsilon {
		t.Errorf("active GreenSage minority went extinct: %.6f", fd[polity.FaithGreenSage])
	}
}

func TestApplyFaithConversion_RegionPushesToward_PreferredFaith(t *testing.T) {
	// In a Holy region starting at 100% OldGods, after enough years
	// SunCovenant should be higher than GreenSage/OneOath/StormPact.
	fd := polity.FaithDistribution{}
	fd[polity.FaithOldGods] = 1.0
	for i := 0; i < 500; i++ {
		applyFaithConversion(&fd, polity.RegionHoly)
	}
	if fd[polity.FaithSunCovenant] <= fd[polity.FaithGreenSage] {
		t.Errorf("expected SunCovenant > GreenSage in Holy region, got SunCovenant=%.4f, GreenSage=%.4f",
			fd[polity.FaithSunCovenant], fd[polity.FaithGreenSage])
	}
}

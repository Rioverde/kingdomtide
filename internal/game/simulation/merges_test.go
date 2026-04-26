package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

func newCamp(id polity.SettlementID, x, y int, region polity.RegionCharacter, pop int, dominantFaith polity.Faith) *polity.Camp {
	fd := polity.FaithDistribution{}
	fd[dominantFaith] = 1.0
	return &polity.Camp{Settlement: polity.Settlement{
		ID:         id,
		Tier:       polity.TierCamp,
		Position:   geom.Position{X: x, Y: y},
		Region:     region,
		Faiths:     fd,
		Population: pop,
	}}
}

func TestShouldMerge_SameRegionFaithClose(t *testing.T) {
	a := newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods).Base()
	b := newCamp(2, 3, 0, polity.RegionNormal, 30, polity.FaithOldGods).Base()
	if !shouldMerge(a, b) {
		t.Error("expected merge: same region, same faith, distance ≤ 4")
	}
}

func TestShouldMerge_RejectsDifferentRegions(t *testing.T) {
	a := newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods).Base()
	b := newCamp(2, 3, 0, polity.RegionHoly, 30, polity.FaithOldGods).Base()
	if shouldMerge(a, b) {
		t.Error("expected reject: different regions")
	}
}

func TestShouldMerge_RejectsTooFar(t *testing.T) {
	a := newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods).Base()
	b := newCamp(2, 100, 0, polity.RegionNormal, 30, polity.FaithOldGods).Base()
	if shouldMerge(a, b) {
		t.Error("expected reject: distance > simMergeDistTiles")
	}
}

func TestShouldMerge_RejectsAsymmetricPopulation(t *testing.T) {
	a := newCamp(1, 0, 0, polity.RegionNormal, 100, polity.FaithOldGods).Base()
	b := newCamp(2, 3, 0, polity.RegionNormal, 10, polity.FaithOldGods).Base()
	// 100:10 = 10:1 ratio > simMergeRatioMax=4 → reject
	if shouldMerge(a, b) {
		t.Error("expected reject: pop ratio > 4")
	}
}

func TestMergeAbsorbs_LargerSurvives(t *testing.T) {
	// The merge gate (simMergeProb=0.10) means a single year=0 roll may not
	// fire. Run up to 200 years until the pair actually merges. With p=0.10
	// the probability of not merging in 200 tries is (0.9)^200 ≈ 7×10⁻¹⁰.
	const maxYears = 200
	merged := false
	for year := 0; year < maxYears; year++ {
		st := newState(42, 5)
		st.settlements[1] = newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods)
		st.settlements[2] = newCamp(2, 3, 0, polity.RegionNormal, 50, polity.FaithOldGods)
		st.tickMerges(year)
		if _, ok := st.settlements[1]; !ok {
			// Smaller was absorbed — verify larger survived with combined pop.
			if _, ok2 := st.settlements[2]; !ok2 {
				t.Fatalf("year %d: both settlements gone after merge", year)
			}
			if got := st.settlements[2].Base().Population; got != 80 {
				t.Errorf("year %d: expected pop 80 after merge, got %d", year, got)
			}
			merged = true
			break
		}
	}
	if !merged {
		t.Errorf("merge never fired in %d years; check simMergeProb or eligibility", maxYears)
	}
}

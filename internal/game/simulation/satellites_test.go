package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestSatellites_NoSpawnUnderPopCap(t *testing.T) {
	st := newState(42, 5)
	c := newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods)
	st.settlements[1] = c
	st.tickSatellites(0)
	if len(st.settlements) != 1 {
		t.Errorf("expected 1 settlement, got %d", len(st.settlements))
	}
}

// TestSatellites_SpawnsOverPopCap verifies that a camp permanently over
// cap eventually splinters within ~30 years. With simSatelliteSpawnProb=0.10
// the probability of no spawn in 30 tries is 0.9^30 ≈ 4.2%, so the test
// passes with overwhelming probability.
func TestSatellites_SpawnsOverPopCap(t *testing.T) {
	const maxYears = 30
	st := newState(42, 5)
	c := newCamp(1, 100, 100, polity.RegionNormal, 100, polity.FaithOldGods)
	st.settlements[1] = c

	for year := 0; year < maxYears; year++ {
		// Keep population above cap so the roll is attempted every year.
		if c.Population < simCampPopCap+simCampPopAbandonFloor {
			c.Population = simCampPopCap + simCampPopAbandonFloor
		}
		st.tickSatellites(year)
		if len(st.settlements) >= 2 {
			// Satellite spawned — verify parent was capped at spawn time.
			if c.Population != simCampPopCap {
				t.Errorf("parent population after spawn: want %d, got %d",
					simCampPopCap, c.Population)
			}
			return
		}
	}
	t.Errorf("expected a satellite within %d years, none spawned", maxYears)
}

// TestSatellites_InheritsRegionAndFaith verifies that a spawned satellite
// inherits the parent's region and faith distribution.
func TestSatellites_InheritsRegionAndFaith(t *testing.T) {
	const maxYears = 30
	st := newState(42, 5)
	c := newCamp(1, 100, 100, polity.RegionHoly, 100, polity.FaithSunCovenant)
	st.settlements[1] = c

	for year := 0; year < maxYears; year++ {
		if c.Population < simCampPopCap+simCampPopAbandonFloor {
			c.Population = simCampPopCap + simCampPopAbandonFloor
		}
		st.tickSatellites(year)
		if len(st.settlements) >= 2 {
			for id, place := range st.settlements {
				if id == 1 {
					continue
				}
				base := place.Base()
				if base.Region != polity.RegionHoly {
					t.Errorf("satellite region: expected Holy, got %v", base.Region)
				}
				if base.Faiths[polity.FaithSunCovenant] != 1.0 {
					t.Errorf("satellite faith: expected SunCovenant=1.0, got %.4f",
						base.Faiths[polity.FaithSunCovenant])
				}
			}
			return
		}
	}
	t.Errorf("expected a satellite within %d years, none spawned", maxYears)
}

// TestSatellites_RespectsRadius verifies that spawned satellite anchors
// fall within [simSatelliteRadiusMin, simSatelliteRadiusMax] of the parent.
func TestSatellites_RespectsRadius(t *testing.T) {
	const maxYears = 30
	st := newState(42, 5)
	c := newCamp(1, 100, 100, polity.RegionNormal, 100, polity.FaithOldGods)
	st.settlements[1] = c

	for year := 0; year < maxYears; year++ {
		if c.Population < simCampPopCap+simCampPopAbandonFloor {
			c.Population = simCampPopCap + simCampPopAbandonFloor
		}
		st.tickSatellites(year)
		if len(st.settlements) >= 2 {
			for id, place := range st.settlements {
				if id == 1 {
					continue
				}
				d := geom.ChebyshevDist(place.Base().Position, c.Position)
				if d < simSatelliteRadiusMin || d > simSatelliteRadiusMax {
					t.Errorf("satellite distance %d outside [%d, %d]",
						d, simSatelliteRadiusMin, simSatelliteRadiusMax)
				}
			}
			return
		}
	}
	t.Errorf("expected a satellite within %d years, none spawned", maxYears)
}

func TestSatellites_DeterministicSameSeed(t *testing.T) {
	const maxYears = 30
	mk := func() map[polity.SettlementID]*polity.Settlement {
		st := newState(42, 5)
		c := newCamp(1, 100, 100, polity.RegionNormal, 100, polity.FaithOldGods)
		st.settlements[1] = c
		for year := 0; year < maxYears; year++ {
			if c.Population < simCampPopCap+simCampPopAbandonFloor {
				c.Population = simCampPopCap + simCampPopAbandonFloor
			}
			st.tickSatellites(year)
			if len(st.settlements) >= 2 {
				break
			}
		}
		out := make(map[polity.SettlementID]*polity.Settlement)
		for id, p := range st.settlements {
			base := p.Base()
			copied := *base
			out[id] = &copied
		}
		return out
	}
	a, b := mk(), mk()
	if len(a) != len(b) {
		t.Fatal("settlement count diverged across runs")
	}
	for id, va := range a {
		vb, ok := b[id]
		if !ok {
			t.Fatalf("id %d in run A not in run B", id)
		}
		if va.Position != vb.Position {
			t.Errorf("position diverged for id %d: %v vs %v", id, va.Position, vb.Position)
		}
	}
}

// TestSatellites_PopKeepsGrowingOnFailedRoll verifies that when the
// Bernoulli gate rejects a spawn, population is not reset to cap —
// it keeps growing so future rolls remain eligible.
func TestSatellites_PopKeepsGrowingOnFailedRoll(t *testing.T) {
	// Use a state where we know the first year's roll will fail (roll>0.10).
	// year=0, id=1, seed=42 has roll≈0.1209 which fails the gate.
	st := newState(42, 5)
	const startPop = 80 // well over simCampPopCap=60
	c := newCamp(1, 100, 100, polity.RegionNormal, startPop, polity.FaithOldGods)
	st.settlements[1] = c

	st.tickSatellites(0) // year=0: roll fails for id=1, seed=42

	if len(st.settlements) != 1 {
		// Roll happened to pass — that is fine, skip the pop check.
		return
	}
	if c.Population < startPop {
		t.Errorf("population was reset on failed roll: want >=%d, got %d",
			startPop, c.Population)
	}
}

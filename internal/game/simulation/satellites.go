package simulation

import (
	"fmt"
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// tickSatellites spawns a satellite Camp when a settlement exceeds its
// tier's PopCap. Excess population drains from the parent into the new
// Camp at a radius of [simSatelliteRadiusMin, simSatelliteRadiusMax].
// Parent is capped at PopCap whether or not a free anchor is found.
func (s *state) tickSatellites(year int) {
	occupied := s.buildOccupancyMap()
	for _, id := range s.sortedSettlementIDs() {
		set := s.settlements[id].Base()
		cap := tierPopCap(set.Tier)
		if set.Population < cap {
			continue
		}
		excess := set.Population - cap
		if excess < simCampPopAbandonFloor {
			continue // satellite would die immediately
		}
		// Roll the splinter dice — most years people stay home and
		// pop keeps growing past the cap. The 10% pass-rate means an
		// always-over-cap settlement splinters about once per decade.
		// Separate rng stream (XOR with 1) keeps the anchor stream
		// unaffected when the spawn decision branches.
		spawnRng := newSimRng(s.seed, seedSaltSimSatellite,
			uint64(year)*0x9E3779B97F4A7C15^uint64(id)^1)
		if spawnRng.Float64() >= simSatelliteSpawnProb {
			// No splinter this year. Do NOT reset pop — population
			// continues to grow naturally and may trigger a future roll.
			continue
		}
		rng := newSimRng(s.seed, seedSaltSimSatellite,
			uint64(year)*0x9E3779B97F4A7C15^uint64(id))
		anchor, ok := s.findSatelliteAnchor(set, rng, occupied)
		if !ok {
			// Anchor search failed — pop also keeps growing; no reset.
			continue
		}
		set.Population = cap
		occupied[anchor] = true
		s.foundSatelliteCamp(*set, anchor, excess, year)
	}
}

// buildOccupancyMap returns a set of all currently occupied positions,
// used once per tick to avoid rebuilding on every satellite anchor search.
func (s *state) buildOccupancyMap() map[geom.Position]bool {
	m := make(map[geom.Position]bool, len(s.settlements))
	for _, id := range s.sortedSettlementIDs() {
		m[s.settlements[id].Base().Position] = true
	}
	return m
}

// tierPopCap returns the satellite-spawn threshold for the given tier.
func tierPopCap(t polity.SettlementTier) int {
	switch t {
	case polity.TierCamp:
		return simCampPopCap
	case polity.TierHamlet:
		return simHamletPopCap
	case polity.TierVillage:
		return simVillagePopCap
	}
	return simCampPopCap
}

// findSatelliteAnchor picks an unoccupied tile within the satellite
// radius annulus [simSatelliteRadiusMin, simSatelliteRadiusMax] from
// parent.Position. Returns (anchor, true) on success, or the zero
// Position and false after simSatelliteAttempts exhausted attempts.
//
// occupied is the caller-owned set of taken positions; the caller is
// responsible for updating it after a successful spawn so that
// subsequent anchors within the same tick don't collide.
//
// Terrain validation (water/volcano/landmark) is deferred to the
// Phase 7 orchestrator. The only collision check here is against
// existing settlement positions.
func (s *state) findSatelliteAnchor(parent *polity.Settlement, rng *rand.Rand, occupied map[geom.Position]bool) (geom.Position, bool) {
	// 8-direction integer offsets used instead of float trig to keep
	// the stream deterministic and avoid rounding divergence.
	angles := [8]struct{ dx, dy int }{
		{1, 0}, {1, 1}, {0, 1}, {-1, 1},
		{-1, 0}, {-1, -1}, {0, -1}, {1, -1},
	}

	for attempt := 0; attempt < simSatelliteAttempts; attempt++ {
		r := simSatelliteRadiusMin +
			rng.IntN(simSatelliteRadiusMax-simSatelliteRadiusMin+1)
		a := angles[rng.IntN(8)]
		p := geom.Position{
			X: parent.Position.X + a.dx*r,
			Y: parent.Position.Y + a.dy*r,
		}
		if occupied[p] {
			continue
		}
		return p, true
	}
	return geom.Position{}, false
}

// foundSatelliteCamp inserts a new Camp at anchor inheriting the
// parent's region and faith distribution. Population is set to the
// surplus that triggered the spawn. The ID is derived from
// (seed, parent.ID, year) via Splitmix64 to keep it stable across
// identical runs and to prevent XOR cancellations with nearby IDs.
func (s *state) foundSatelliteCamp(parent polity.Settlement, anchor geom.Position, pop int, year int) {
	id := polity.SettlementID(int64(geom.Splitmix64(
		uint64(s.seed) ^
			uint64(parent.ID) ^
			uint64(year)*0x9E3779B97F4A7C15 ^
			uint64(seedSaltSimSatellite))))
	rulerName := generateRulerName(s.seed, anchor, parent.Region)
	campName := generateSettlementName(s.seed, anchor, parent.Region)
	rulerSalt := geom.Splitmix64(uint64(seedSaltSimRuler) ^ uint64(year)*0x9E3779B97F4A7C15 ^ uint64(parent.ID))
	rulerStream := dice.New(
		s.seed^int64(geom.PackPos(anchor)),
		dice.Salt(rulerSalt),
	)
	camp := &polity.Camp{Settlement: polity.Settlement{
		ID:         id,
		Name:       campName,
		Tier:       polity.TierCamp,
		Position:   anchor,
		Footprint:  []geom.Position{anchor},
		Region:     parent.Region,
		Faiths:     parent.Faiths, // inherit copy
		Population: pop,
		Founded:    year,
		Ruler:      polity.NewRuler(rulerStream, year, rulerName),
	}}
	s.settlements[id] = camp
	s.dirty = true
	s.log.emit(year, "camp-spawned",
		fmt.Sprintf("'%s' %s founded in '%s' — satellite of '%s' (%s)",
			camp.Name, describeRuler(&camp.Settlement),
			camp.Region, parent.Name, describeRuler(&parent)))
}

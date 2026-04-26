package simulation

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// rollAnnualSchedules pre-rolls every per-region per-year random schedule
// the simulation needs (famine + plague). Region-tier rolls (one bad year
// hits every settlement in a region together) are more realistic than
// per-settlement and reduce RNG noise visible to the player. Both streams
// are decorrelated: distinct salts (seedSaltSimFamine vs seedSaltSimPlague)
// guarantee that famine rolls do not predict plague rolls.
func (s *state) rollAnnualSchedules(years int) {
	for r := 0; r < int(polity.RegionCharacterCount); r++ {
		fRng := newSimRng(s.seed, seedSaltSimFamine, uint64(r))
		pRng := newSimRng(s.seed, seedSaltSimPlague, uint64(r))
		for y := 0; y < years; y++ {
			s.regionFamine[r][y] = fRng.Float64() < simD6FamineProb
			s.regionPlague[r][y] = pRng.Float64() < simPlagueProb
		}
	}
}

// tickPopulation advances every settlement's population by one
// simulated year per .omc/plans/simulation.md §4.
//
//	delta = pop * (birthRate * (1 + regionGrowthMod) - deathRate * famineMult? * plagueMult?)
//
// Stochastic rounding ensures small populations have non-zero growth
// probability instead of always rounding to 0. Each settlement gets its
// own per-(year, id) PCG stream so iteration order does not bias rolls
// and future parallelization is possible.
func (s *state) tickPopulation(year int) {
	for _, id := range s.sortedSettlementIDs() {
		set := s.settlements[id].Base()
		rng := newSimRng(s.seed, seedSaltSimPop,
			uint64(year)*0x9E3779B97F4A7C15^uint64(id))
		s.tickSettlementPop(set, year, rng)
	}
	s.flushPlagueLog(year)
}

// tickSettlementPop applies the per-year delta to one settlement.
// Pure of side-effects beyond Population mutation and a one-shot
// per-region plague log entry routed through s.plagueLogged.
func (s *state) tickSettlementPop(set *polity.Settlement, year int, rng *rand.Rand) {
	famine := s.regionFamine[set.Region][year]
	plague := s.regionPlague[set.Region][year]

	deathRate := simBaseDeathRate
	if famine {
		deathRate *= simFamineMult
	}
	if plague {
		deathRate *= simPlagueMult
		s.markPlague(set.Region, year)
	}

	birthRate := simBaseBirthRate * (1.0 + regionGrowthMod[set.Region])
	netRate := birthRate - deathRate
	delta := float64(set.Population) * netRate

	intDelta := int(delta)
	frac := delta - float64(intDelta)
	if rng.Float64() < math.Abs(frac) {
		if frac > 0 {
			intDelta++
		} else {
			intDelta--
		}
	}
	set.Population += intDelta
	if set.Population < 0 {
		set.Population = 0
	}
}

// markPlague records that the (region, year) pair triggered plague death
// rates for at least one settlement so flushPlagueLog can emit a single
// log line per region-year.
func (s *state) markPlague(region polity.RegionCharacter, year int) {
	key := uint64(region)<<32 | uint64(uint32(year))
	if _, seen := s.plagueLogged[key]; seen {
		return
	}
	s.plagueLogged[key] = struct{}{}
}

// flushPlagueLog emits a "plague" event line for each region that
// experienced plague this year. Called once at the end of tickPopulation
// so the line appears next to the deaths it caused. Region order is
// numeric for determinism.
func (s *state) flushPlagueLog(year int) {
	for r := 0; r < int(polity.RegionCharacterCount); r++ {
		if !s.regionPlague[r][year] {
			continue
		}
		key := uint64(r)<<32 | uint64(uint32(year))
		if _, seen := s.plagueLogged[key]; !seen {
			continue
		}
		region := polity.RegionCharacter(r)
		s.log.emit(year, "plague",
			fmt.Sprintf("'%s' region — death rate ×%.1f", region, simPlagueMult))
		// Clear so we never re-emit if the loop runs twice for the same year.
		delete(s.plagueLogged, key)
	}
}

// sortedSettlementIDs returns every live settlement ID in ascending
// order — used by every per-tick step that iterates over settlements
// so iteration order is deterministic across runs. The slice is cached
// on s.cachedSortedIDs and rebuilt only when the dirty flag is set
// (after any settlement insert/delete).
func (s *state) sortedSettlementIDs() []polity.SettlementID {
	if !s.dirty && s.cachedSortedIDs != nil {
		return s.cachedSortedIDs
	}
	ids := make([]polity.SettlementID, 0, len(s.settlements))
	for id := range s.settlements {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	s.cachedSortedIDs = ids
	s.dirty = false
	return ids
}

// refreshSortedIDs forces a rebuild of the cached sorted ID slice. Run
// calls this once at the top of each year so subsequent tick steps
// share a stable snapshot of settlement IDs even before any individual
// step has triggered the dirty flag (e.g. on a year where no births,
// deaths, merges, or satellite spawns occur).
func (s *state) refreshSortedIDs() {
	s.dirty = true
	s.sortedSettlementIDs()
}

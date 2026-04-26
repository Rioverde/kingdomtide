package simulation

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// tickPromotions advances every settlement's promotion eligibility
// by one year and promotes those that meet all gates.
//
// Camp → Hamlet:  pop > simHamletPromotePop sustained
//
//	simHamletPromoteSustain years.
//
// Hamlet → Village: pop > simVillagePromotePop sustained
//
//	simVillagePromoteSustain years AND no existing Village within
//	simVillageExclusivityRadius tiles (Chebyshev).
func (s *state) tickPromotions(year int) {
	villageGrid := s.buildVillageGrid()
	for _, id := range s.sortedSettlementIDs() {
		s.tickSettlementPromotion(id, year, villageGrid)
	}
}

func (s *state) tickSettlementPromotion(id polity.SettlementID, year int, villageGrid map[geom.Position][]polity.SettlementID) {
	place := s.settlements[id]
	set := place.Base()

	switch set.Tier {
	case polity.TierCamp:
		if set.Population <= simHamletPromotePop {
			s.promoteSustain[id] = 0
			return
		}
		s.promoteSustain[id]++
		if s.promoteSustain[id] < simHamletPromoteSustain {
			return
		}
		s.promoteCampToHamlet(id, year)

	case polity.TierHamlet:
		if set.Population <= simVillagePromotePop {
			s.promoteSustain[id] = 0
			return
		}
		if !s.spatialExclusivityClear(set, simVillageExclusivityRadius, villageGrid) {
			s.promoteSustain[id] = 0
			return
		}
		s.promoteSustain[id]++
		if s.promoteSustain[id] < simVillagePromoteSustain {
			return
		}
		s.promoteHamletToVillage(id, year)
	}
}

// buildVillageGrid indexes all Village-tier settlements by grid cell so
// spatialExclusivityClear can probe only the 3×3 neighbourhood instead
// of scanning the full settlement map.
func (s *state) buildVillageGrid() map[geom.Position][]polity.SettlementID {
	cell := simVillageExclusivityRadius
	grid := make(map[geom.Position][]polity.SettlementID)
	for _, id := range s.sortedSettlementIDs() {
		place := s.settlements[id]
		if place.Base().Tier != polity.TierVillage {
			continue
		}
		pos := place.Base().Position
		key := geom.Position{X: pos.X / cell, Y: pos.Y / cell}
		grid[key] = append(grid[key], id)
	}
	return grid
}

// spatialExclusivityClear returns true if no Village-tier settlement is
// within radius (Chebyshev) of set.Position. It probes only the 3×3
// grid-cell neighbourhood, keeping the check sub-linear in settlement count.
func (s *state) spatialExclusivityClear(set *polity.Settlement, radius int, villageGrid map[geom.Position][]polity.SettlementID) bool {
	cell := radius
	homeKey := geom.Position{X: set.Position.X / cell, Y: set.Position.Y / cell}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			cellKey := geom.Position{X: homeKey.X + dx, Y: homeKey.Y + dy}
			for _, otherID := range villageGrid[cellKey] {
				if otherID == set.ID {
					continue
				}
				other := s.settlements[otherID].Base()
				if geom.ChebyshevDist(other.Position, set.Position) <= radius {
					return false
				}
			}
		}
	}
	return true
}

// promoteCampToHamlet replaces the Camp at id with a Hamlet carrying
// the same Settlement state. ID is preserved. Footprint is regrown to
// the hamlet budget inline — regrowth happens at the moment of tier
// change rather than in a separate tickFootprints pass, keeping the
// tick loop simple while preserving §3 ordering.
func (s *state) promoteCampToHamlet(id polity.SettlementID, year int) {
	camp, ok := s.settlements[id].(*polity.Camp)
	if !ok {
		return
	}
	hamlet := &polity.Hamlet{Settlement: camp.Settlement}
	hamlet.Tier = polity.TierHamlet
	s.settlements[id] = hamlet
	s.promoteSustain[id] = 0

	rng := newSimRng(s.seed, seedSaltSimFootprint, uint64(year)^uint64(id))
	budget := settlementFootprintBudget(polity.TierHamlet, hamlet.Population)
	regrowFootprint(&hamlet.Settlement, budget, rng, nil)

	s.log.emit(year, "hamlet-formed",
		fmt.Sprintf("'%s' (id %04d) promoted to Hamlet %s (pop %d) in '%s'",
			hamlet.Name, hamlet.ID, describeRuler(&hamlet.Settlement),
			hamlet.Population, hamlet.Region))
}

// promoteHamletToVillage replaces the Hamlet at id with a Village
// carrying the same Settlement state. ID is preserved. Footprint is
// regrown to the village budget inline at the moment of promotion.
func (s *state) promoteHamletToVillage(id polity.SettlementID, year int) {
	hamlet, ok := s.settlements[id].(*polity.Hamlet)
	if !ok {
		return
	}
	village := &polity.Village{Settlement: hamlet.Settlement}
	village.Tier = polity.TierVillage
	s.settlements[id] = village
	s.promoteSustain[id] = 0

	rng := newSimRng(s.seed, seedSaltSimFootprint, uint64(year)^uint64(id))
	budget := settlementFootprintBudget(polity.TierVillage, village.Population)
	regrowFootprint(&village.Settlement, budget, rng, nil)

	s.log.emit(year, "village-formed",
		fmt.Sprintf("'%s' promoted to Village %s (pop %d) in '%s'",
			village.Name, describeRuler(&village.Settlement),
			village.Population, village.Region))
}

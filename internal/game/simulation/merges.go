package simulation

import (
	"fmt"
	"math"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// shouldMerge runs the symmetric merge predicate from the plan §7.1.
// Cheapest gates first to minimize cost on the common (reject) path.
func shouldMerge(a, b *polity.Settlement) bool {
	if geom.ChebyshevDist(a.Position, b.Position) > simMergeDistTiles {
		return false
	}
	if a.Region != b.Region {
		return false
	}
	smaller, larger := a.Population, b.Population
	if smaller > larger {
		smaller, larger = larger, smaller
	}
	if larger > 0 && float64(larger)/float64(max(smaller, 1)) > simMergeRatioMax {
		return false
	}
	return faithsCompatible(a.Faiths, b.Faiths)
}

// faithsCompatible reports whether two faith distributions are compatible
// enough to permit a merge. OldGods acts as a universal lubricant — any
// settlement with an OldGods majority merges freely. Otherwise the
// distributions must overlap by at least simFaithCohesionMin.
func faithsCompatible(fa, fb polity.FaithDistribution) bool {
	if fa[polity.FaithOldGods] > 0.5 || fb[polity.FaithOldGods] > 0.5 {
		return true
	}
	var overlap float64
	for i := range fa {
		overlap += math.Min(fa[i], fb[i])
	}
	return overlap >= simFaithCohesionMin
}

// cohesion returns a [0, 1] tiebreak score for choosing among multiple
// eligible merge partners. Combines faith overlap with a distance penalty
// so nearby settlements with matching faiths are strongly preferred.
// distPenalty is clamped to 0.99 so pairs at exactly simMergeDistTiles
// boundary remain distinguishable from unrelated pairs (score > 0).
func cohesion(a, b *polity.Settlement) float64 {
	var faithOverlap float64
	for i := range a.Faiths {
		faithOverlap += math.Min(a.Faiths[i], b.Faiths[i])
	}
	distPenalty := float64(geom.ChebyshevDist(a.Position, b.Position)) /
		float64(simMergeDistTiles)
	if distPenalty > 0.99 {
		distPenalty = 0.99
	}
	return faithOverlap * (1.0 - distPenalty)
}

// tickMerges processes every eligible merge for one simulated year.
// Each settlement participates in at most one merge per tick. Pairs
// are collected in (min, max) lex order and executed after the full
// eligibility pass to prevent a settlement absorbed in pair (1,2) from
// also appearing in pair (2,3).
func (s *state) tickMerges(year int) {
	grid := s.buildMergeGrid()
	taken := make(map[polity.SettlementID]bool)

	type pair struct{ a, b polity.SettlementID }
	var pairs []pair

	for _, id := range s.sortedSettlementIDs() {
		if taken[id] {
			continue
		}
		set := s.settlements[id].Base()
		partner, ok := s.bestMergePartner(set, grid, taken)
		if !ok {
			continue
		}
		// Mutual-eligibility check (plan §7.2): B's best partner must also be A.
		// Asymmetric preference means the pair would not naturally cohere — skip.
		partnerSet := s.settlements[partner].Base()
		reverse, ok := s.bestMergePartner(partnerSet, grid, taken)
		if !ok || reverse != id {
			continue
		}
		// Bernoulli gate: eligible pairs don't necessarily unite every year.
		// (year, min(idA,idB)) mixing ensures the same pair on the same year
		// always rolls the same value — deterministic across runs.
		idA, idB := id, partner
		mergeRng := newSimRng(s.seed, seedSaltSimMerge,
			uint64(year)*0x9E3779B97F4A7C15^uint64(min(idA, idB)))
		if mergeRng.Float64() >= simMergeProb {
			// Eligible this year but they don't unite. Leave both available
			// for other potential partners later in the same tick.
			continue
		}
		a, b := idA, idB
		if b < a {
			a, b = b, a
		}
		pairs = append(pairs, pair{a, b})
		taken[id] = true
		taken[partner] = true
	}

	for _, p := range pairs {
		s.mergeSettlements(p.a, p.b, year)
	}
}

// buildMergeGrid bins every live settlement into a spatial hash keyed by
// (Position / simMergeDistTiles) so the merge predicate can find candidates
// in O(1) amortised rather than O(n²).
func (s *state) buildMergeGrid() map[geom.Position][]polity.SettlementID {
	grid := make(map[geom.Position][]polity.SettlementID, len(s.settlements))
	for _, id := range s.sortedSettlementIDs() {
		set := s.settlements[id].Base()
		key := geom.Position{
			X: set.Position.X / simMergeDistTiles,
			Y: set.Position.Y / simMergeDistTiles,
		}
		grid[key] = append(grid[key], id)
	}
	return grid
}

// bestMergePartner scans the 3×3 neighbourhood of grid cells around set and
// returns the eligible candidate with the highest cohesion score, or false if
// none qualifies.
func (s *state) bestMergePartner(
	set *polity.Settlement,
	grid map[geom.Position][]polity.SettlementID,
	taken map[polity.SettlementID]bool,
) (polity.SettlementID, bool) {
	homeKey := geom.Position{
		X: set.Position.X / simMergeDistTiles,
		Y: set.Position.Y / simMergeDistTiles,
	}
	bestID := polity.SettlementID(0)
	bestScore := -1.0
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			cellKey := geom.Position{X: homeKey.X + dx, Y: homeKey.Y + dy}
			for _, candID := range grid[cellKey] {
				if candID == set.ID || taken[candID] {
					continue
				}
				cand := s.settlements[candID].Base()
				if !shouldMerge(set, cand) {
					continue
				}
				score := cohesion(set, cand)
				if score > bestScore {
					bestScore = score
					bestID = candID
				}
			}
		}
	}
	if bestScore < 0 {
		return 0, false
	}
	return bestID, true
}

// mergeSettlements absorbs the smaller settlement into the larger. Population
// sums; faiths blend via population-weighted formula (blendFaiths). The
// survivor keeps its ID, anchor position, and ruler. The absorbed settlement
// is removed from state.
func (s *state) mergeSettlements(aID, bID polity.SettlementID, year int) {
	a := s.settlements[aID].Base()
	b := s.settlements[bID].Base()

	// Larger survives. Tie → lower ID survives (deterministic).
	survivorID, absorbedID := aID, bID
	if b.Population > a.Population || (b.Population == a.Population && bID < aID) {
		survivorID, absorbedID = bID, aID
	}

	survivor := s.settlements[survivorID].Base()
	absorbed := s.settlements[absorbedID].Base()

	survivor.Faiths = blendFaiths(survivor.Faiths, absorbed.Faiths,
		survivor.Population, absorbed.Population)
	survivor.Population += absorbed.Population

	// Regrow surviving settlement's footprint to fit its new population.
	// Inline at merge time — same rationale as promotion regrowth.
	// nil validator: production has no terrain check yet; future hook is
	// in place for a worldgen-aware validator (see footprint.go).
	rng := newSimRng(s.seed, seedSaltSimFootprint, uint64(year)^uint64(survivorID))
	budget := settlementFootprintBudget(survivor.Tier, survivor.Population)
	regrowFootprint(survivor, budget, rng, nil)

	// Carry lineage onto tier-specific structs for log provenance.
	switch typed := s.settlements[survivorID].(type) {
	case *polity.Hamlet:
		typed.AbsorbedCampIDs = append(typed.AbsorbedCampIDs, absorbedID)
	case *polity.Village:
		typed.AbsorbedHamletIDs = append(typed.AbsorbedHamletIDs, absorbedID)
	}

	// Emit merge event before removing the absorbed settlement.
	eventKind := "camps-merged"
	if absorbed.Tier == polity.TierHamlet {
		eventKind = "hamlet-merged"
	}
	s.log.emit(year, eventKind,
		fmt.Sprintf("'%s' %s absorbed into '%s' %s — same region/faith",
			absorbed.Name, describeRuler(absorbed),
			survivor.Name, describeRuler(survivor)))

	delete(s.settlements, absorbedID)
	delete(s.abandonStreak, absorbedID)
	s.dirty = true
}

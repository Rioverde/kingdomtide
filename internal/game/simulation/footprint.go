package simulation

import (
	"math"
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// settlementFootprintBudget returns the target footprint tile count for a
// settlement given its tier and population. Camps keep the 2-3 tiles they
// were born with; hamlets grow to 4-6 tiles; villages scale with population
// via a sqrt curve, clamped to [8, 20].
func settlementFootprintBudget(tier polity.SettlementTier, pop int) int {
	switch tier {
	case polity.TierCamp:
		if pop <= 25 {
			return 2
		}
		return 3
	case polity.TierHamlet:
		if pop <= 80 {
			return 4
		}
		if pop <= 120 {
			return 5
		}
		return 6
	case polity.TierVillage:
		// sqrt(pop/100), clamped to [8, 20].
		n := int(math.Round(math.Sqrt(float64(pop) / 100.0)))
		if n < 8 {
			return 8
		}
		if n > 20 {
			return 20
		}
		return n
	}
	return 1
}

// tileValidator reports whether a tile is acceptable for footprint
// expansion. Returning false skips the candidate without aborting the
// regrowth — the random walk simply tries another neighbour.
//
// Contract: callers may pass nil to skip terrain validation entirely
// (current production behaviour). When the worldgen tilemap is wired
// into the simulation tier, a validator that rejects water, peaks,
// volcano cones, and existing landmarks should be supplied so newly
// claimed tiles never sit on impassable terrain.
type tileValidator func(geom.Position) bool

// regrowFootprint expands s.Footprint toward budget via a 4-neighbour
// random walk from the existing tiles. Existing tiles are kept; new
// tiles are appended until the budget is met or the frontier is
// exhausted.
//
// Regrowth is geometry-only by default — when valid is nil it does not
// re-validate against worldgen terrain (water, peaks, volcanoes,
// landmarks). The original camp placement already passed those gates;
// expanding 1-6 tiles outward is an acceptable approximation. Future
// refinement: pass a non-nil validator that consults the worldgen
// tilemap so growth never claims impassable terrain.
//
// Determinism: rng must be seeded deterministically from
// (world seed, seedSaltSimFootprint, year ^ settlementID).
func regrowFootprint(s *polity.Settlement, budget int, rng *rand.Rand, valid tileValidator) {
	if len(s.Footprint) >= budget {
		return
	}

	claimed := make(map[geom.Position]struct{}, budget)
	for _, p := range s.Footprint {
		claimed[p] = struct{}{}
	}

	frontier := make([]geom.Position, len(s.Footprint))
	copy(frontier, s.Footprint)

	deltas := [4]geom.Position{
		{X: 1, Y: 0}, {X: -1, Y: 0}, {X: 0, Y: 1}, {X: 0, Y: -1},
	}

	for len(s.Footprint) < budget && len(frontier) > 0 {
		idx := rng.IntN(len(frontier))
		p := frontier[idx]
		grew := false

		order := [4]int{0, 1, 2, 3}
		rng.Shuffle(4, func(i, j int) { order[i], order[j] = order[j], order[i] })

		for _, oi := range order {
			d := deltas[oi]
			n := geom.Position{X: p.X + d.X, Y: p.Y + d.Y}
			if _, taken := claimed[n]; taken {
				continue
			}
			if valid != nil && !valid(n) {
				// Reject this neighbour but keep trying others — the
				// frontier tile may still grow in another direction.
				continue
			}
			claimed[n] = struct{}{}
			s.Footprint = append(s.Footprint, n)
			frontier = append(frontier, n)
			grew = true
			break
		}

		if !grew {
			// Swap-pop the exhausted frontier tile.
			frontier[idx] = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}
	}
}

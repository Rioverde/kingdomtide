package worldgen

import (
	"math"
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// bridsonSample returns positions inside bounds satisfying:
//   - every pair has Chebyshev distance >= minSpacing
//   - density is approximately Poisson-disk-uniform
//
// Determinism: rng must be deterministic; same rng state + same bounds +
// minSpacing + k yields the same []geom.Position every call, in stable
// insertion order. Empty bounds return nil.
//
// k is the rejection threshold (Bridson's k parameter); 30 is the standard
// choice. Higher k slightly increases packing density at quadratic time cost.
//
// Algorithm: Bridson, R. "Fast Poisson Disk Sampling in Arbitrary Dimensions"
// (SIGGRAPH 2007). The backing spatial-hash grid uses cell size
// minSpacing/√2 so that any two points violating the spacing constraint must
// share a grid cell or be immediate neighbours — the per-candidate neighbour
// scan is therefore O(1) amortised regardless of sample count.
//
// Determinism is preserved by three invariants: (1) candidates are generated
// in continuous space and floored to integer tile coordinates before any
// acceptance test, so the integer rounding is part of the canonical stream;
// (2) the active list shrinks via swap-and-pop, which is order-preserving for
// the already-accepted set; (3) the first seed point is drawn uniformly from
// bounds using a fixed draw-order before the active-list loop begins.
func bridsonSample(rng *rand.Rand, bounds geom.Rect, minSpacing, k int) []geom.Position {
	if minSpacing <= 0 || bounds.Empty() {
		return nil
	}

	// cellSize is minSpacing/√2. With this size, a circle of radius minSpacing
	// centred on any point covers at most a 5×5 neighbourhood of cells, so the
	// conflict check visits a bounded constant number of candidates.
	cellSize := float64(minSpacing) / math.Sqrt2

	w := bounds.MaxX - bounds.MinX
	h := bounds.MaxY - bounds.MinY

	cols := int(math.Ceil(float64(w)/cellSize)) + 1
	rows := int(math.Ceil(float64(h)/cellSize)) + 1

	// grid maps grid-cell index → index+1 into the accepted slice (0 = empty).
	grid := make([]int, cols*rows)

	cellOf := func(p geom.Position) (int, int) {
		cx := int((float64(p.X-bounds.MinX)) / cellSize)
		cy := int((float64(p.Y-bounds.MinY)) / cellSize)
		return cx, cy
	}

	addToGrid := func(p geom.Position, idx int) {
		cx, cy := cellOf(p)
		grid[cy*cols+cx] = idx + 1
	}

	// conflict reports whether pos is too close to any already-accepted point.
	// It checks the 5×5 neighbourhood around pos's cell (radius 2 in each
	// direction), which is sufficient to catch all points within minSpacing
	// given cell size minSpacing/√2.
	conflict := func(accepted []geom.Position, pos geom.Position) bool {
		cx, cy := cellOf(pos)
		for dy := -2; dy <= 2; dy++ {
			ny := cy + dy
			if ny < 0 || ny >= rows {
				continue
			}
			for dx := -2; dx <= 2; dx++ {
				nx := cx + dx
				if nx < 0 || nx >= cols {
					continue
				}
				entry := grid[ny*cols+nx]
				if entry == 0 {
					continue
				}
				if geom.ChebyshevDist(pos, accepted[entry-1]) < minSpacing {
					return true
				}
			}
		}
		return false
	}

	// Seed: draw the first point uniformly from the integer tile grid inside bounds.
	seed := geom.Position{
		X: bounds.MinX + rng.IntN(w),
		Y: bounds.MinY + rng.IntN(h),
	}

	accepted := make([]geom.Position, 0, 64)
	accepted = append(accepted, seed)
	addToGrid(seed, 0)

	// active holds indices into accepted for positions that can still spawn neighbours.
	active := make([]int, 0, 64)
	active = append(active, 0)

	for len(active) > 0 {
		// Pick a random entry from the active list.
		ri := rng.IntN(len(active))
		parent := accepted[active[ri]]

		placed := false
		for attempt := 0; attempt < k; attempt++ {
			// Generate a candidate in the annulus [minSpacing, 2*minSpacing)
			// around parent in continuous space, then floor to integer tiles.
			angle := rng.Float64() * 2 * math.Pi
			radius := float64(minSpacing) * (1.0 + rng.Float64())
			cx := float64(parent.X) + radius*math.Cos(angle)
			cy := float64(parent.Y) + radius*math.Sin(angle)

			candidate := geom.Position{
				X: int(math.Floor(cx)),
				Y: int(math.Floor(cy)),
			}

			if !bounds.Contains(candidate) {
				continue
			}
			if conflict(accepted, candidate) {
				continue
			}

			idx := len(accepted)
			accepted = append(accepted, candidate)
			addToGrid(candidate, idx)
			active = append(active, idx)
			placed = true
			break
		}

		if !placed {
			// Exhausted k attempts — remove from active via swap-and-pop.
			last := len(active) - 1
			active[ri] = active[last]
			active = active[:last]
		}
	}

	return accepted
}

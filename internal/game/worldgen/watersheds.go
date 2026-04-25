package worldgen

// computeWatersheds derives, for each land cell, the corner index it
// ultimately drains to (its coast outlet). Two cells with the same
// value belong to the same drainage basin — the natural unit Patel's
// mapgen2 uses for downstream features (city placement on big basins,
// kingdom borders along watershed divides).
//
// Algorithm (from Patel's mapgen2 article):
//  1. Per-corner pass: every corner's watershed = its downslope's
//     watershed; coast-touching corners are their own watershed.
//     Iterate until the propagation reaches the inland minima.
//  2. Per-cell pass: each cell takes the most-common watershed among
//     its adjacent corners (modal vote — robust against the few
//     corners that get stuck at false inland minima).
//
// Output is a per-cell slice indexed by cellID. -1 means no path to
// the coast — ocean cells, or endorheic basins. int32 not int —
// corner indices fit comfortably inside ±2³¹ and we save half the
// memory on big worlds.
func computeWatersheds(w *Map, corners []corner, isOcean []bool) []int32 {
	cellCount := len(w.Voronoi.Cells)
	out := make([]int32, cellCount)
	for i := range out {
		out[i] = -1
	}
	if len(corners) == 0 {
		return out
	}

	// Per-corner: coast corners drain to themselves; everyone else
	// inherits from their downslope.
	cwatershed := make([]int32, len(corners))
	for i := range corners {
		if corners[i].touchOcean {
			cwatershed[i] = int32(i)
		} else {
			cwatershed[i] = -1
		}
	}

	// Iterate downslope propagation. Convergence is bounded by the
	// longest downslope chain on the map; 100 passes is well above the
	// graph diameter for any world size we generate.
	const maxPasses = 100
	for pass := 0; pass < maxPasses; pass++ {
		changed := false
		for i := range corners {
			if corners[i].touchOcean {
				continue
			}
			ds := corners[i].downslope
			if ds < 0 {
				continue
			}
			if cwatershed[ds] >= 0 && cwatershed[i] != cwatershed[ds] {
				cwatershed[i] = cwatershed[ds]
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Per-cell modal vote — accumulated by walking corners once and
	// fanning into their adjacent cells. O(corners · adjCells) instead
	// of the naïve O(cells · corners) double loop, which would burn
	// ~9M ops on a Standard world.
	//
	// Each cell sees at most ~6 distinct watershed values (Voronoi cells
	// are degree-3, so each cell touches at most 6 corners). A fixed
	// [6]wsTally array per cell avoids ~38K map allocations on Standard.
	type wsTally struct {
		ws    int32
		count uint8
	}
	// cellVotes holds the running tally for each cell as a fixed-size
	// array. n tracks how many distinct watershed values have been seen.
	type cellVote struct {
		buf [6]wsTally
		n   int
	}
	votes := make([]cellVote, cellCount)

	for ci, c := range corners {
		ws := cwatershed[ci]
		if ws < 0 {
			continue
		}
		for _, adj := range c.adjCells {
			if isOcean[adj] {
				continue
			}
			v := &votes[adj]
			found := false
			for i := 0; i < v.n; i++ {
				if v.buf[i].ws == ws {
					v.buf[i].count++
					found = true
					break
				}
			}
			if !found && v.n < 6 {
				v.buf[v.n] = wsTally{ws, 1}
				v.n++
			}
		}
	}
	for cellID := range votes {
		v := &votes[cellID]
		if v.n == 0 {
			continue
		}
		bestI := 0
		for i := 1; i < v.n; i++ {
			if v.buf[i].count > v.buf[bestI].count {
				bestI = i
			}
		}
		out[cellID] = v.buf[bestI].ws
	}
	return out
}

package game

import (
	"reflect"
	"testing"
)

// findRiverSource scans tiles in a region around the origin to locate the first
// tile that qualifies as a river source for g. It returns the coordinates and
// true when found, or 0, 0, false when the search area contains no source.
// A radius of 500 covers a 1001×1001 tile area — enough for any reasonable seed.
func findRiverSource(g *WorldGenerator, radius int) (q, r int, found bool) {
	for sq := -radius; sq <= radius; sq++ {
		for sr := -radius; sr <= radius; sr++ {
			if g.IsRiverSource(sq, sr) {
				return sq, sr, true
			}
		}
	}
	return 0, 0, false
}

// TestRiverPathDeterministic verifies that two calls to RiverPath with the same
// source and the same generator (identical seed) produce bit-for-bit identical
// paths. This is the core correctness property: deterministic generation means
// no seams between separately-computed chunks.
func TestRiverPathDeterministic(t *testing.T) {
	g := NewWorldGenerator(42)

	q, r, found := findRiverSource(g, 500)
	if !found {
		t.Skip("no river source found in search area — adjust radius or seed")
	}

	path1 := g.RiverPath(q, r)
	path2 := g.RiverPath(q, r)

	if !reflect.DeepEqual(path1, path2) {
		t.Fatalf("RiverPath(%d, %d) returned different results on two calls", q, r)
	}
	if len(path1) == 0 {
		t.Fatal("expected non-empty path from a valid river source")
	}
}

// TestRiverPathStopsAtOcean checks that a river path either terminates before
// reaching riverMaxLength or ends at an ocean tile. It finds a source and
// inspects the last element of the returned path.
func TestRiverPathStopsAtOcean(t *testing.T) {
	g := NewWorldGenerator(99)

	q, r, found := findRiverSource(g, 500)
	if !found {
		t.Skip("no river source found in search area — adjust radius or seed")
	}

	path := g.RiverPath(q, r)
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}

	// Either the path hit the cap (acceptable) or the final tile is ocean/below ocean.
	if len(path) < riverMaxLength {
		last := path[len(path)-1]
		lq, lr := last[0], last[1]
		elev := g.elevation.Eval2Normalized(float64(lq), float64(lr))
		// The path stopped early — either it reached ocean or a local minimum.
		// Both are valid. We only require that if elevation is above ocean the
		// tile must be a local minimum (no lower neighbour).
		if elev >= elevationOcean {
			// Verify it is a local minimum by checking all neighbours.
			isMin := true
			for _, off := range hexNeighborOffsets {
				ne := g.elevation.Eval2Normalized(float64(lq+off[0]), float64(lr+off[1]))
				if ne < elev {
					isMin = false
					break
				}
			}
			if !isMin {
				t.Errorf("path ended at (%d,%d) elev=%.3f but it is not a local minimum and not ocean", lq, lr, elev)
			}
		}
	}
}

// TestRiverTilesInChunkDeterministic calls RiverTilesInChunk twice for the same
// chunk and verifies the returned sets are equal. Determinism here ensures that
// separate HTTP requests for the same chunk produce the same river layout.
func TestRiverTilesInChunkDeterministic(t *testing.T) {
	g := NewWorldGenerator(7)
	cc := ChunkCoord{X: 3, Y: -2}

	set1 := g.RiverTilesInChunk(cc)
	set2 := g.RiverTilesInChunk(cc)

	if !reflect.DeepEqual(set1, set2) {
		t.Fatal("RiverTilesInChunk returned different sets on two calls for the same chunk")
	}
}

// chunkWindowHasRiver reports whether any tile within a 21×21 chunk window
// centred on centerCC contains a river tile for the given generator.
func chunkWindowHasRiver(gen *WorldGenerator, centerCC ChunkCoord) bool {
	for dcx := -10; dcx <= 10; dcx++ {
		for dcy := -10; dcy <= 10; dcy++ {
			cc := ChunkCoord{X: centerCC.X + dcx, Y: centerCC.Y + dcy}
			chunk := gen.Chunk(cc)
			for dr := 0; dr < ChunkSize; dr++ {
				for dq := 0; dq < ChunkSize; dq++ {
					if chunk.Tiles[dr][dq].River {
						return true
					}
				}
			}
		}
	}
	return false
}

// TestChunkHasRivers is a sanity check that river generation produces at least
// one river tile somewhere across a broad sweep of seeds and chunks. It tries
// seeds 1–20 and for each seed scans a 21×21 chunk window centred on the first
// discovered river source. The test passes as soon as any chunk in any seed
// contains a river tile, so it is not coupled to the topology of a single seed.
func TestChunkHasRivers(t *testing.T) {
	const seedCount = 20

	for s := int64(1); s <= seedCount; s++ {
		gen := NewWorldGenerator(s)

		sq, sr, found := findRiverSource(gen, 500)
		if !found {
			// No source found in the search radius for this seed — try the next.
			continue
		}

		if chunkWindowHasRiver(gen, WorldToChunk(sq, sr)) {
			return // feature is present; test passes
		}
	}

	t.Fatal("no river tiles found across 20 seeds in 21×21 chunk windows — river generation likely broken")
}

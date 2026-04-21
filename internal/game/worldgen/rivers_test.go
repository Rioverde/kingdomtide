package worldgen

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// findRiverSourceRadius is the search half-width in tiles. 500 covers a
// 1001×1001 area — enough for any reasonable seed. A constant (not a
// parameter) because all callers use the same value; keeping it a parameter
// tripped the unparam linter and invited accidental tuning.
const findRiverSourceRadius = 500

// findRiverSource scans chunks in a region around the origin to locate the
// first tile that qualifies as a river source for g. It returns the
// coordinates and true when found, or 0, 0, false when the search area
// contains no source.
//
// Phase-3: IsRiverSource now requires a drainage field, so the search iterates
// chunk-by-chunk, materialising (or reusing) the drainage field per chunk and
// scanning every tile inside. The radius (≈31 chunks per side) is enough
// coverage for any reasonable seed.
func findRiverSource(g *WorldGenerator) (x, y int, found bool) {
	chunkRadius := findRiverSourceRadius/ChunkSize + 1
	for cx := -chunkRadius; cx <= chunkRadius; cx++ {
		for cy := -chunkRadius; cy <= chunkRadius; cy++ {
			cc := ChunkCoord{X: cx, Y: cy}
			field := g.drainageFor(cc)
			minX, maxX, minY, maxY := cc.Bounds()
			for sy := minY; sy < maxY; sy++ {
				for sx := minX; sx < maxX; sx++ {
					if sx < -findRiverSourceRadius || sx > findRiverSourceRadius ||
						sy < -findRiverSourceRadius || sy > findRiverSourceRadius {
						continue
					}
					if g.IsRiverSource(field, sx, sy) {
						return sx, sy, true
					}
				}
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

	x, y, found := findRiverSource(g)
	if !found {
		t.Skip("no river source found in search area — adjust radius or seed")
	}

	path1 := g.RiverPath(x, y)
	path2 := g.RiverPath(x, y)

	if !reflect.DeepEqual(path1, path2) {
		t.Fatalf("RiverPath(%d, %d) returned different results on two calls", x, y)
	}
	if len(path1) == 0 {
		t.Fatal("expected non-empty path from a valid river source")
	}
}

// TestRiverPathStopsAtOcean checks that a river path terminates in one of three
// legitimate ways after Phase 3: reached ocean on the filled surface, hit the
// riverMaxLength cap, or left the drainage buffer ("scan window"). Priority-
// flood guarantees there is no longer a "stopped at a mid-land local minimum"
// case — that was the Phase-2 behaviour this phase was specifically built to
// eliminate.
func TestRiverPathStopsAtOcean(t *testing.T) {
	g := NewWorldGenerator(99)

	x, y, found := findRiverSource(g)
	if !found {
		t.Skip("no river source found in search area — adjust radius or seed")
	}

	path := g.RiverPath(x, y)
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}

	// Hitting the cap is always acceptable — very long paths fall through.
	if len(path) >= riverMaxLength {
		return
	}

	last := path[len(path)-1]
	lx, ly := last[0], last[1]

	// Build the drainage field for the chunk owning the source so the
	// termination check sees the same filled surface the tracer used.
	sourceField := g.drainageFor(WorldToChunk(x, y))
	if fillElev, inBuf := sourceField.elevationAt(lx, ly); inBuf {
		if fillElev < elevationOcean {
			return // reached ocean — fine
		}
		// Still in the buffer and above ocean on the filled surface — this
		// would be a mid-land dead end, which Phase 3 is supposed to prevent.
		t.Errorf("path ended at (%d,%d) fillElev=%.6f — mid-land stop on filled surface",
			lx, ly, fillElev)
		return
	}
	// Last tile is outside the original buffer — the tracer left the scan
	// window. That is an acceptable termination per riverPathOnField's doc.
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

// TestRiverPathOnFieldNoInfiniteLoop exercises the no-progress termination guard
// in riverPathOnField. A synthetic drainageField is constructed with a flat
// (zero) fillElev so that no neighbour is ever strictly lower than the current
// cell — the loop must stop at step 1 rather than spinning forever. This proves
// that removing the dead bestInBuf flag did not break the termination invariant:
// the bestX == x && bestY == y check is the sole (and sufficient) guard.
func TestRiverPathOnFieldNoInfiniteLoop(t *testing.T) {
	g := NewWorldGenerator(1)

	// Build a real drainage field but then zero all fillElev entries so every
	// cell looks like a flat plateau — no strictly-lower neighbour anywhere.
	cc := ChunkCoord{X: 0, Y: 0}
	field := g.drainageFor(cc)

	// Flatten the fill surface so the no-progress guard is always hit.
	for y := range drainageBufferSide {
		for x := range drainageBufferSide {
			field.fillElev[y][x] = 0.5 // uniform elevation above ocean
		}
	}

	// Pick the centre tile of the buffer as the source.
	originX := field.originX + drainageBufferSide/2
	originY := field.originY + drainageBufferSide/2

	path := g.riverPathOnField(field, originX, originY)

	// The path must contain exactly the source tile and then stop (no lower
	// neighbour is available). If the loop spun without the guard we would hit
	// riverMaxLength; here we want exactly 1 step.
	if len(path) != 1 {
		t.Errorf("expected path length 1 on flat surface, got %d — no-progress guard may be broken", len(path))
	}
	if path[0][0] != originX || path[0][1] != originY {
		t.Errorf("first path step is (%d,%d), want (%d,%d)", path[0][0], path[0][1], originX, originY)
	}
}

// chunkWindowHasRiver reports whether any tile within a 21×21 chunk window
// centred on centerCC contains a river tile for the given generator.
func chunkWindowHasRiver(gen *WorldGenerator, centerCC ChunkCoord) bool {
	for dcx := -10; dcx <= 10; dcx++ {
		for dcy := -10; dcy <= 10; dcy++ {
			cc := ChunkCoord{X: centerCC.X + dcx, Y: centerCC.Y + dcy}
			chunk := gen.Chunk(cc)
			for dy := range ChunkSize {
				for dx := range ChunkSize {
					if chunk.Tiles[dy][dx].Overlays.Has(game.OverlayRiver) {
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

		sx, sy, found := findRiverSource(gen)
		if !found {
			// No source found in the search radius for this seed — try the next.
			continue
		}

		if chunkWindowHasRiver(gen, WorldToChunk(sx, sy)) {
			return // feature is present; test passes
		}
	}

	t.Fatal("no river tiles found across 20 seeds in 21×21 chunk windows — river generation likely broken")
}

package worldgen

// riverMoistureThreshold gates river spawning by moisture. Arid tiles (below
// this value) do not produce rivers regardless of their elevation, preventing
// implausible mountain streams in the middle of deserts. Retained from
// Phase 2 — the Phase-3 flow-accumulation gate replaces elevation/sparsity
// but the moisture climate gate still has teeth.
const riverMoistureThreshold = 0.55

// riverMaxLength caps how many steps a single river trace may take. The limit
// keeps generation bounded and prevents degenerate cycles in flat terrain from
// running forever. At ChunkSize=16 a cap of 128 crosses eight chunks at most.
const riverMaxLength = 128

// hexNeighborOffsets lists six step directions for river flow. Six directions
// (rather than the square grid's four) give the path tracer enough freedom to
// curve naturally; the extra two diagonals keep rivers from looking like a
// Manhattan grid on flat terrain. Kept under this name because the flow
// algorithm is a direct port from the earlier pointy-top hex prototype.
var hexNeighborOffsets = [6][2]int{
	{+1, 0},
	{-1, 0},
	{0, +1},
	{0, -1},
	{+1, -1},
	{-1, +1},
}

// riverSourceHash mixes three integers into a single unsigned value. The
// constants are arbitrary large primes; XOR-ing them produces good bit
// diffusion without a full cryptographic hash. The result is platform-stable
// because all arithmetic is on unsigned 64-bit values.
//
// Retained post-Phase-3 for seedJitter01 (generator.go) which still needs a
// deterministic 64-bit mixer. River-source selection itself no longer uses
// this hash — flow accumulation provides natural sparsity.
func riverSourceHash(x, y int, seed int64) uint64 {
	a := uint64(x)*0x9e3779b97f4a7c15 ^ uint64(y)*0x6c62272e07bb0142 ^ uint64(seed)*0xbf58476d1ce4e5b9
	a ^= a >> 30
	a *= 0xbf58476d1ce4e5b9
	a ^= a >> 27
	a *= 0x94d049bb133111eb
	a ^= a >> 31
	return a
}

// IsRiverSource reports whether tile (x, y) is a river source for this world.
// Phase 3 semantics: a source exists where flow accumulation clears
// flowAccumThreshold AND the tile is above ocean AND the local moisture climate
// is not arid. The caller must already hold the drainage field for a buffer
// containing (x, y) — use drainageFor(cc) on the chunk owning the coord.
//
// The signature takes the drainage field as an explicit parameter instead of
// re-building one from g.drainage. This keeps RiverTilesInChunk O(buffer) and
// lets tests exercise the source gate on a synthetic field.
func (g *WorldGenerator) IsRiverSource(field *drainageField, x, y int) bool {
	accum, inBuf := field.accumAt(x, y)
	if !inBuf {
		return false
	}
	if accum < flowAccumThreshold {
		return false
	}
	elev, _ := field.elevationAt(x, y)
	if elev < elevationOcean {
		return false
	}
	fx, fy := float64(x), float64(y)
	moist := g.moisture.Eval2Normalized(fx, fy)
	return moist >= riverMoistureThreshold
}

// RiverPath traces a river starting at (sourceX, sourceY) and returns the
// ordered slice of grid coordinates the river passes through, including the
// source itself. The trace picks the neighbour with the lowest filled
// elevation at each step, mimicking gravity-driven flow on the
// depression-filled surface produced by priority-flood.
//
// Termination:
//   - Current tile is below elevationOcean (on the filled surface) — reached the sea.
//   - Stepping would leave the drainage buffer — documented as "left the scan window".
//   - Path length reaches riverMaxLength.
//
// Post-Phase-3 the "local minimum on land" termination is gone: priority-flood
// guarantees every interior cell has a strictly-lower neighbour (or hits the
// boundary), so a mid-land dead end can no longer happen on the filled field.
//
// The result is deterministic: identical (field, source) pairs always produce
// identical paths. Two callers sharing one drainage field will see identical
// tributary paths once they converge, which is what flow accumulation needs
// for correct merging.
func (g *WorldGenerator) RiverPath(sourceX, sourceY int) [][2]int {
	// Convenience wrapper: build the drainage field for the chunk owning the
	// source on the fly. Hot-path callers inside Chunk() use
	// riverPathOnField to reuse the field already materialised there.
	cc := WorldToChunk(sourceX, sourceY)
	field := g.drainageFor(cc)
	return g.riverPathOnField(field, sourceX, sourceY)
}

// riverPathOnField is the inner trace that consumes a caller-supplied
// drainage field. Extracted so RiverTilesInChunk can trace every source in
// the 5×5 buffer against one shared field — which is what lets flow
// accumulation's shared-tributary dedup work at the tile-set level.
func (g *WorldGenerator) riverPathOnField(field *drainageField, sourceX, sourceY int) [][2]int {
	path := make([][2]int, 0, riverMaxLength)
	x, y := sourceX, sourceY

	for len(path) < riverMaxLength {
		path = append(path, [2]int{x, y})

		elev, inBuf := field.elevationAt(x, y)
		if !inBuf {
			// Left the scan window — stop. The path up to here is still
			// valid; the continuation is some downstream chunk's problem.
			break
		}
		if elev < elevationOcean {
			// Reached the sea on the filled surface.
			break
		}

		bestX, bestY := x, y
		bestElev := elev
		for _, off := range hexNeighborOffsets {
			nx, ny := x+off[0], y+off[1]
			ne, inNeighbour := field.elevationAt(nx, ny)
			if !inNeighbour {
				// Out-of-buffer neighbours are skipped; only in-buffer tiles
				// are candidates, so bestInBuf is implied by any improvement.
				continue
			}
			if ne < bestElev {
				bestElev = ne
				bestX, bestY = nx, ny
			}
		}

		if bestX == x && bestY == y {
			// No strictly-lower in-buffer neighbour found. After priority-flood
			// this cannot happen on interior cells; on the boundary it means the
			// path has reached the edge of the scan window — stop gracefully.
			break
		}

		x, y = bestX, bestY
	}

	return path
}

// riverBufferChunks is the number of chunks to expand the scan region around cc on each
// side when collecting river sources. A buffer of 2 catches rivers whose source lies up to
// 2 chunks away and whose path enters cc — a 1-chunk buffer missed these. The drainage
// field uses the same buffer radius so the fill and the tracer agree on the scan window.
const riverBufferChunks = 2

// RiverTilesInChunk returns the set of world-space grid coordinates inside
// chunk cc that belong to a river. Phase-3 implementation: scan the 5×5
// buffer for flow-accumulation river sources, trace each on the shared
// drainage field, and collect the intersection with cc.Bounds(). Tributary
// merging is automatic — two upstream sources whose paths converge produce
// overlapping tile sets, and the map-set dedup handles the overlap.
func (g *WorldGenerator) RiverTilesInChunk(cc ChunkCoord) map[[2]int]struct{} {
	field := g.drainageFor(cc)
	return g.riverTilesInChunkOnField(cc, field)
}

// riverTilesInChunkOnField is the field-aware variant used internally by
// Chunk() — which already materialised the drainage field and should not pay
// for a second cache lookup.
func (g *WorldGenerator) riverTilesInChunkOnField(cc ChunkCoord, field *drainageField) map[[2]int]struct{} {
	result := make(map[[2]int]struct{})

	minX, maxX, minY, maxY := cc.Bounds()
	scanMinX := minX - riverBufferChunks*ChunkSize
	scanMaxX := maxX + riverBufferChunks*ChunkSize
	scanMinY := minY - riverBufferChunks*ChunkSize
	scanMaxY := maxY + riverBufferChunks*ChunkSize

	for sx := scanMinX; sx < scanMaxX; sx++ {
		for sy := scanMinY; sy < scanMaxY; sy++ {
			if !g.IsRiverSource(field, sx, sy) {
				continue
			}
			for _, coord := range g.riverPathOnField(field, sx, sy) {
				x, y := coord[0], coord[1]
				if x >= minX && x < maxX && y >= minY && y < maxY {
					result[[2]int{x, y}] = struct{}{}
				}
			}
		}
	}

	return result
}

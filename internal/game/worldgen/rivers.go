package worldgen

// riverSourceThreshold is the minimum normalised elevation for a tile to be
// considered a river source. Tiles at or above this value sit in the high
// hills / mountain band — water accumulates there and flows downhill.
const riverSourceThreshold = 0.72

// riverMoistureThreshold gates river spawning by moisture. Arid tiles (below
// this value) do not produce rivers regardless of their elevation, preventing
// implausible mountain streams in the middle of deserts.
const riverMoistureThreshold = 0.55

// riverMaxLength caps how many steps a single river trace may take. The limit
// keeps generation bounded and prevents degenerate cycles in flat terrain from
// running forever. At ChunkSize=16 a cap of 128 crosses eight chunks at most.
const riverMaxLength = 128

// riverSourceSparsity is the modulus used to thin out sources. Only coordinates
// whose hash lands on zero survive, so roughly 1-in-N eligible mountain tiles
// become sources.
const riverSourceSparsity = 7

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
// Three conditions must all hold: the tile must be high enough (≥ riverSourceThreshold),
// moist enough (≥ riverMoistureThreshold), and its hash must land on zero modulo
// riverSourceSparsity so that sources remain sparse but fully deterministic.
func (g *WorldGenerator) IsRiverSource(x, y int) bool {
	fx, fy := float64(x), float64(y)
	elev := g.elevation.Eval2Normalized(fx, fy)
	if elev < riverSourceThreshold {
		return false
	}
	moist := g.moisture.Eval2Normalized(fx, fy)
	if moist < riverMoistureThreshold {
		return false
	}
	return riverSourceHash(x, y, g.seed)%riverSourceSparsity == 0
}

// RiverPath traces a river starting at (sourceX, sourceY) and returns the
// ordered slice of grid coordinates the river passes through, including the
// source itself. The trace picks the neighbour with the lowest elevation at
// each step, mimicking gravity-driven flow. It terminates when:
//   - the current tile is ocean (deep ocean or ocean) — water has reached the sea,
//   - no strictly-lower neighbour exists (local minimum / plateau), or
//   - the path length exceeds riverMaxLength.
//
// The result is deterministic: identical (source, seed) pairs always produce
// identical paths.
func (g *WorldGenerator) RiverPath(sourceX, sourceY int) [][2]int {
	path := make([][2]int, 0, riverMaxLength)
	x, y := sourceX, sourceY

	for len(path) < riverMaxLength {
		path = append(path, [2]int{x, y})

		// Stop if we have reached ocean — the river has found the sea.
		elev := g.elevation.Eval2Normalized(float64(x), float64(y))
		if elev < elevationOcean {
			break
		}

		// Find the neighbour with the lowest elevation.
		bestX, bestY := x, y
		bestElev := elev
		for _, off := range hexNeighborOffsets {
			nx, ny := x+off[0], y+off[1]
			ne := g.elevation.Eval2Normalized(float64(nx), float64(ny))
			if ne < bestElev {
				bestElev = ne
				bestX, bestY = nx, ny
			}
		}

		// No strictly lower neighbour — we are at a local minimum; stop here.
		if bestX == x && bestY == y {
			break
		}

		x, y = bestX, bestY
	}

	return path
}

// riverBufferChunks is the number of chunks to expand the scan region around cc on each
// side when collecting river sources. A buffer of 2 catches rivers whose source lies up to
// 2 chunks away and whose path enters cc — a 1-chunk buffer missed these. Rivers sourced
// more than 2 chunks away are still unsupported but are rarer given source sparsity.
const riverBufferChunks = 2

// RiverTilesInChunk returns the set of world-space grid coordinates inside
// chunk cc that belong to a river. To achieve cross-chunk coherence without a
// global pre-pass, the function scans a two-chunk-wide buffer ring around cc
// (a 5×5 region of chunks): for every tile in that enlarged region that qualifies
// as a river source it traces the full path and collects any coordinate that falls
// inside cc.Bounds(). Rivers whose source lies more than 2 chunks away and that
// still enter cc via a very long trace are out of scope — riverMaxLength + source
// sparsity make this an acceptable rare edge case.
func (g *WorldGenerator) RiverTilesInChunk(cc ChunkCoord) map[[2]int]struct{} {
	result := make(map[[2]int]struct{})

	minX, maxX, minY, maxY := cc.Bounds()

	// Expand the scan region by riverBufferChunks chunks on each side.
	scanMinX := minX - riverBufferChunks*ChunkSize
	scanMaxX := maxX + riverBufferChunks*ChunkSize
	scanMinY := minY - riverBufferChunks*ChunkSize
	scanMaxY := maxY + riverBufferChunks*ChunkSize

	for sx := scanMinX; sx < scanMaxX; sx++ {
		for sy := scanMinY; sy < scanMaxY; sy++ {
			if !g.IsRiverSource(sx, sy) {
				continue
			}
			for _, coord := range g.RiverPath(sx, sy) {
				x, y := coord[0], coord[1]
				if x >= minX && x < maxX && y >= minY && y < maxY {
					result[[2]int{x, y}] = struct{}{}
				}
			}
		}
	}

	return result
}

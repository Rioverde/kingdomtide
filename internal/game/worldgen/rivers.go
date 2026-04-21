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

// hexNeighborOffsets lists the six axial directions for pointy-top hexagons.
// Order: +q, -q, +r, -r, and the two diagonal axes that keep q+r constant.
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
func riverSourceHash(q, r int, seed int64) uint64 {
	a := uint64(q)*0x9e3779b97f4a7c15 ^ uint64(r)*0x6c62272e07bb0142 ^ uint64(seed)*0xbf58476d1ce4e5b9
	a ^= a >> 30
	a *= 0xbf58476d1ce4e5b9
	a ^= a >> 27
	a *= 0x94d049bb133111eb
	a ^= a >> 31
	return a
}

// IsRiverSource reports whether tile (q, r) is a river source for this world.
// Three conditions must all hold: the tile must be high enough (≥ riverSourceThreshold),
// moist enough (≥ riverMoistureThreshold), and its hash must land on zero modulo
// riverSourceSparsity so that sources remain sparse but fully deterministic.
func (g *WorldGenerator) IsRiverSource(q, r int) bool {
	x, y := float64(q), float64(r)
	elev := g.elevation.Eval2Normalized(x, y)
	if elev < riverSourceThreshold {
		return false
	}
	moist := g.moisture.Eval2Normalized(x, y)
	if moist < riverMoistureThreshold {
		return false
	}
	return riverSourceHash(q, r, g.seed)%riverSourceSparsity == 0
}

// RiverPath traces a river starting at (sourceQ, sourceR) and returns the
// ordered slice of axial coordinates the river passes through, including the
// source itself. The trace picks the axial neighbour with the lowest elevation
// at each step, mimicking gravity-driven flow. It terminates when:
//   - the current tile is ocean (deep ocean or ocean) — water has reached the sea,
//   - no strictly-lower neighbour exists (local minimum / plateau), or
//   - the path length exceeds riverMaxLength.
//
// The result is deterministic: identical (source, seed) pairs always produce
// identical paths.
func (g *WorldGenerator) RiverPath(sourceQ, sourceR int) [][2]int {
	path := make([][2]int, 0, riverMaxLength)
	q, r := sourceQ, sourceR

	for len(path) < riverMaxLength {
		path = append(path, [2]int{q, r})

		// Stop if we have reached ocean — the river has found the sea.
		elev := g.elevation.Eval2Normalized(float64(q), float64(r))
		if elev < elevationOcean {
			break
		}

		// Find the neighbour with the lowest elevation.
		bestQ, bestR := q, r
		bestElev := elev
		for _, off := range hexNeighborOffsets {
			nq, nr := q+off[0], r+off[1]
			ne := g.elevation.Eval2Normalized(float64(nq), float64(nr))
			if ne < bestElev {
				bestElev = ne
				bestQ, bestR = nq, nr
			}
		}

		// No strictly lower neighbour — we are at a local minimum; stop here.
		if bestQ == q && bestR == r {
			break
		}

		q, r = bestQ, bestR
	}

	return path
}

// riverBufferChunks is the number of chunks to expand the scan region around cc on each
// side when collecting river sources. A buffer of 2 catches rivers whose source lies up to
// 2 chunks away and whose path enters cc — a 1-chunk buffer missed these. Rivers sourced
// more than 2 chunks away are still unsupported but are rarer given source sparsity.
const riverBufferChunks = 2

// RiverTilesInChunk returns the set of world-space axial coordinates inside
// chunk cc that belong to a river. To achieve cross-chunk coherence without a
// global pre-pass, the function scans a two-chunk-wide buffer ring around cc
// (a 5×5 region of chunks): for every tile in that enlarged region that qualifies
// as a river source it traces the full path and collects any coordinate that falls
// inside cc.Bounds(). Rivers whose source lies more than 2 chunks away and that
// still enter cc via a very long trace are out of scope — riverMaxLength + source
// sparsity make this an acceptable rare edge case.
func (g *WorldGenerator) RiverTilesInChunk(cc ChunkCoord) map[[2]int]struct{} {
	result := make(map[[2]int]struct{})

	minQ, maxQ, minR, maxR := cc.Bounds()

	// Expand the scan region by riverBufferChunks chunks on each side.
	scanMinQ := minQ - riverBufferChunks*ChunkSize
	scanMaxQ := maxQ + riverBufferChunks*ChunkSize
	scanMinR := minR - riverBufferChunks*ChunkSize
	scanMaxR := maxR + riverBufferChunks*ChunkSize

	for sq := scanMinQ; sq < scanMaxQ; sq++ {
		for sr := scanMinR; sr < scanMaxR; sr++ {
			if !g.IsRiverSource(sq, sr) {
				continue
			}
			for _, coord := range g.RiverPath(sq, sr) {
				q, r := coord[0], coord[1]
				if q >= minQ && q < maxQ && r >= minR && r < maxR {
					result[[2]int{q, r}] = struct{}{}
				}
			}
		}
	}

	return result
}

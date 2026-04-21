package game

import (
	"math/rand/v2"
)

// SuperChunkSize is the edge length of a super-chunk in tiles. 64 tiles =
// 4×4 regular chunks. Each super-chunk contributes one region anchor;
// average region area ≈ 64² tiles but varies substantially because anchors
// are jittered and regions are Voronoi cells of those anchors.
const SuperChunkSize = 64

// SuperChunkCoord identifies the super-chunk that owns a region anchor. It
// is not necessarily the coord of the super-chunk containing a given tile —
// use AnchorAt for the tile-to-region lookup. Two regions are equal iff
// their SuperChunkCoords are equal.
type SuperChunkCoord struct {
	X, Y int
}

// anchorJitterMin and anchorJitterMax bound where an anchor may land inside
// its super-chunk cell. Restricting jitter to [8, 56] keeps a minimum 16-
// tile gap between any two neighbouring anchors and guarantees every Voronoi
// region is at least ~8 tiles wide, avoiding degenerate zero-width cells.
const (
	anchorJitterMin = 8
	anchorJitterMax = SuperChunkSize - 8
)

// Salts that mix the seed with the super-chunk coordinate when deriving an
// anchor. The two constants are taken from the golden-ratio / splitmix64
// literature and are chosen specifically to decorrelate the X and Y hash
// streams. The literal values 0x9E3779B97F4A7C15 and 0xBF58476D1CE4E5B9 do
// not fit in a signed int64 as positive integers, so we express them via
// their equivalent two's-complement form using int64(uint64(x)).
// seedSaltAnchorX and seedSaltAnchorY are the signed-int64 views of the
// splitmix64/golden-ratio constants. Both literal values exceed
// math.MaxInt64 in their unsigned form, so we cannot spell them as untyped
// int64 constants. Routing through a uint64 variable lets Go perform the
// conversion at runtime with the usual two's-complement wraparound, which
// preserves the full 64-bit pattern.
var (
	seedSaltAnchorX = toInt64(0x9e3779b97f4a7c15)
	seedSaltAnchorY = toInt64(0xbf58476d1ce4e5b9)
)

// toInt64 reinterprets a uint64 bit pattern as int64. Passing the literal
// as an argument turns constant checking off for the conversion — the
// function call adds a runtime step that Go does not optimize into a
// constant overflow error.
func toInt64(u uint64) int64 { return int64(u) }

// WorldToSuperChunk returns the super-chunk whose cell contains the tile at
// world coord (x, y). Uses floor division so negative coordinates map to
// negative super-chunk indices: (-1, -1) → (-1, -1), not (0, 0). Go's / and
// % truncate toward zero, which would break the adjacency invariant for
// negative inputs, so we compute the floor explicitly.
func WorldToSuperChunk(x, y int) SuperChunkCoord {
	return SuperChunkCoord{X: floorDiv(x, SuperChunkSize), Y: floorDiv(y, SuperChunkSize)}
}

// floorDiv returns the mathematical floor of a/b for positive b. Go's /
// truncates toward zero; this adjusts the result when a is negative and
// does not divide evenly.
func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// splitmix64 is the classic Steele-Vigna finalizer. Used to derive a
// deterministic 64-bit mix from the seed and a per-super-chunk input; the
// resulting bits are statistically strong enough to split into two jitter
// offsets and a PRNG state without visible lattice artifacts.
func splitmix64(x uint64) uint64 {
	x += 0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return x ^ (x >> 31)
}

// anchorMix combines the world seed with a SuperChunkCoord into a single
// 64-bit value. The XOR structure matches the "seed ^ (X*saltX) ^ (Y*saltY)"
// recipe in the Phase 1 plan; splitmix64 then diffuses the bits so close
// coords produce visibly different outputs.
func anchorMix(seed int64, sc SuperChunkCoord) uint64 {
	h := uint64(seed) ^
		uint64(int64(sc.X)*seedSaltAnchorX) ^
		uint64(int64(sc.Y)*seedSaltAnchorY)
	return splitmix64(h)
}

// AnchorOf returns the jittered anchor position for a given super-chunk.
// The anchor is a deterministic (seed, sc)-derived point inside the super-
// chunk's cell, clamped to [anchorJitterMin, anchorJitterMax] on each axis.
// Same (seed, sc) always returns the same Position.
func AnchorOf(seed int64, sc SuperChunkCoord) Position {
	h := anchorMix(seed, sc)
	// The high 32 bits drive the X offset, the low 32 bits drive Y. This
	// yields two independent streams from one mixed value without a second
	// hash call.
	hi := uint32(h >> 32)
	lo := uint32(h)
	const span = anchorJitterMax - anchorJitterMin + 1
	dx := int(hi%uint32(span)) + anchorJitterMin
	dy := int(lo%uint32(span)) + anchorJitterMin
	return Position{
		X: sc.X*SuperChunkSize + dx,
		Y: sc.Y*SuperChunkSize + dy,
	}
}

// AnchorAt returns the nearest anchor (and its SuperChunkCoord) to the given
// world tile. It checks the containing super-chunk plus its 8 neighbours —
// 9 candidates total. Ties are broken by SuperChunkCoord lex order (X then
// Y) for determinism.
//
// This is the region lookup primitive. Two tiles belong to the same region
// iff AnchorAt returns the same SuperChunkCoord for both.
func AnchorAt(seed int64, worldX, worldY int) (Position, SuperChunkCoord) {
	home := WorldToSuperChunk(worldX, worldY)
	bestSC := home
	bestAnchor := AnchorOf(seed, home)
	bestDist := sqDist(bestAnchor.X, bestAnchor.Y, worldX, worldY)

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			cand := SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			a := AnchorOf(seed, cand)
			d := sqDist(a.X, a.Y, worldX, worldY)
			if d < bestDist || (d == bestDist && lessSC(cand, bestSC)) {
				bestDist = d
				bestSC = cand
				bestAnchor = a
			}
		}
	}
	return bestAnchor, bestSC
}

// sqDist returns the squared Euclidean distance between (ax, ay) and
// (bx, by). Keeping the result in int avoids float rounding noise in the
// tie-break comparison and saves a sqrt call.
func sqDist(ax, ay, bx, by int) int {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

// lessSC reports whether a sorts before b in SuperChunkCoord lex order: X
// first, then Y. Used as the deterministic tie-break when two anchors are
// equidistant to a tile.
func lessSC(a, b SuperChunkCoord) bool {
	if a.X != b.X {
		return a.X < b.X
	}
	return a.Y < b.Y
}

// normalizeNeighbourOffsets lists the four orthogonal neighbours used by
// NormalizeAt. Kept as a package-level array so the peninsula cleanup loop
// does not rebuild it on every call.
var normalizeNeighbourOffsets = [4][2]int{
	{0, -1}, // north
	{0, 1},  // south
	{-1, 0}, // west
	{1, 0},  // east
}

// NormalizeAt returns the canonical region SuperChunkCoord for a tile,
// applying a one-step four-neighbour vote to smooth peninsula artifacts
// from the raw AnchorAt lookup. A tile is reassigned when strictly fewer
// than two of its four orthogonal neighbours share its raw region AND
// strictly more than two of its neighbours share one other region's coord;
// otherwise the raw AnchorAt result is returned unchanged.
//
// Deterministic: pure function of (seed, worldX, worldY). Cost: 5× AnchorAt
// (= 45 anchor checks per tile worst case), so callers that invoke it on
// hot paths should cache the result by tile coord.
func NormalizeAt(seed int64, worldX, worldY int) SuperChunkCoord {
	_, raw := AnchorAt(seed, worldX, worldY)

	sameCount := 0
	// Track the most frequent "other" region among the neighbours. counts is
	// tiny (≤4 entries) so a slice beats a map here.
	type tally struct {
		sc    SuperChunkCoord
		count int
	}
	var others [4]tally
	othersLen := 0

	for _, off := range normalizeNeighbourOffsets {
		_, nsc := AnchorAt(seed, worldX+off[0], worldY+off[1])
		if nsc == raw {
			sameCount++
			continue
		}
		found := false
		for i := range othersLen {
			if others[i].sc == nsc {
				others[i].count++
				found = true
				break
			}
		}
		if !found {
			others[othersLen] = tally{sc: nsc, count: 1}
			othersLen++
		}
	}

	if sameCount >= 2 {
		return raw
	}

	var bestSC SuperChunkCoord
	bestCount := 0
	for i := range othersLen {
		o := others[i]
		if o.count > bestCount || (o.count == bestCount && lessSC(o.sc, bestSC)) {
			bestCount = o.count
			bestSC = o.sc
		}
	}
	if bestCount > 2 {
		return bestSC
	}
	return raw
}

// IsInRegion reports whether (worldX, worldY) belongs to the region anchored
// at sc. Equivalent to comparing AnchorAt's result against sc; provided as
// a named helper so call sites read clearly.
func IsInRegion(seed int64, sc SuperChunkCoord, worldX, worldY int) bool {
	_, tileSC := AnchorAt(seed, worldX, worldY)
	return tileSC == sc
}

// RegionTilesNear returns up to n candidate tile positions inside the region
// anchored at sc, biased toward the anchor via a radius-bounded scatter.
// Positions are sampled deterministically from (seed, sc) and filtered
// through IsInRegion so every returned tile genuinely belongs to the cell.
// If the sampler cannot find n valid positions within 3*n attempts, it
// returns the accumulated subset (possibly empty) rather than spinning.
func RegionTilesNear(seed int64, sc SuperChunkCoord, n, radius int) []Position {
	if n <= 0 {
		return nil
	}
	if radius < 1 {
		radius = 1
	}

	anchor := AnchorOf(seed, sc)

	// PCG seeded from (seed, sc) so the scatter is deterministic and
	// independent from any other PRNG stream in the program.
	pcg := rand.NewPCG(
		uint64(seed)^uint64(int64(sc.X)*seedSaltAnchorX),
		uint64(seed)^uint64(int64(sc.Y)*seedSaltAnchorY),
	)
	r := rand.New(pcg)

	out := make([]Position, 0, n)
	maxAttempts := 3 * n
	for i := 0; i < maxAttempts && len(out) < n; i++ {
		dx := r.IntN(2*radius+1) - radius
		dy := r.IntN(2*radius+1) - radius
		x := anchor.X + dx
		y := anchor.Y + dy
		if !IsInRegion(seed, sc, x, y) {
			continue
		}
		out = append(out, Position{X: x, Y: y})
	}
	return out
}

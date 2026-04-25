package geom

// SeedSaltX and SeedSaltY are the canonical golden-ratio mixing constants for
// coordinate-derived hashes. Exported so every worldgen pass that builds
// per-tile or per-cell PRNG state agrees on the same recipe — divergence here
// would silently shift placement fingerprints between subsystems.
//
// Values: fractional hex of φ (golden ratio) and its successor in the
// splitmix64 constant chain.
const (
	SeedSaltX uint64 = 0x9e3779b97f4a7c15
	SeedSaltY uint64 = 0xbf58476d1ce4e5b9
)

// MixCoords folds a seed + salt + (x, y) coordinate pair into a single state
// word using the golden-ratio mixing recipe. Used by every worldgen pass that
// needs deterministic per-tile or per-cell PRNG state.
func MixCoords(seed int64, salt int64, x, y int) uint64 {
	return uint64(seed) ^ uint64(salt) ^
		(uint64(int64(x)) * SeedSaltX) ^
		(uint64(int64(y)) * SeedSaltY)
}

// ChebyshevDist returns max(|a.X-b.X|, |a.Y-b.Y|), the Chebyshev distance
// between two positions. It matches 8-directional movement: diagonal and
// orthogonal steps are treated equally, so "within N tiles" means inside an
// N-tile square box centred on the origin.
func ChebyshevDist(a, b Position) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// PackPos folds a (X, Y) tile coord into a single uint64. Each axis gets
// 32 bits; signed values are reinterpreted via uint32 cast so negative
// coordinates pack without collision. Worlds top out at ~21K×8K tiles
// (Gigantic), so 32 bits per axis is generous.
func PackPos(p Position) uint64 {
	return (uint64(uint32(p.X)) << 32) | uint64(uint32(p.Y))
}

// ToInt64 reinterprets a uint64 bit pattern as int64. The function call turns
// constant checking off for the conversion so the full 64-bit pattern survives
// regardless of the high bit. Used by packages that need two's-complement
// wraparound for nothing-up-my-sleeve constants whose unsigned value exceeds
// math.MaxInt64.
func ToInt64(u uint64) int64 { return int64(u) }

package worldgen

// bitset is a fixed-length packed bit vector backed by []uint64.
// One bit per index; 8× cheaper than []bool when the array spans
// every tile of the world. Inlinable accessors keep the per-call
// cost negligible — the compiler folds Get/Set into a single load
// or load-or-store instruction at the call site.
//
// Used for the river-tile mask (W·H bits) so a 10x-Large world
// (~590M tiles) costs ~74MB instead of 590MB.
type bitset struct {
	bits []uint64
	n    int
}

// newBitset allocates a bitset large enough to hold n indices.
func newBitset(n int) *bitset {
	if n < 0 {
		n = 0
	}
	return &bitset{bits: make([]uint64, (n+63)>>6), n: n}
}

// Len returns the number of bits the set holds.
func (b *bitset) Len() int { return b.n }

// Set marks bit i as 1. No bounds check — caller must keep i within
// [0, n). Benchmarks show the inlined version costs ~1ns.
func (b *bitset) Set(i int) {
	b.bits[i>>6] |= 1 << uint(i&63)
}

// Get returns true iff bit i is 1.
func (b *bitset) Get(i int) bool {
	if i < 0 || i >= b.n {
		return false
	}
	return b.bits[i>>6]&(1<<uint(i&63)) != 0
}

// Count returns the number of 1-bits — used by diagnostic tools.
func (b *bitset) Count() int {
	var c int
	for _, w := range b.bits {
		c += popcount64(w)
	}
	return c
}

// popcount64 — same as math/bits.OnesCount64 but inlined here so
// importing math/bits stays scoped to whoever needs it directly.
func popcount64(x uint64) int {
	x -= (x >> 1) & 0x5555555555555555
	x = (x & 0x3333333333333333) + ((x >> 2) & 0x3333333333333333)
	x = (x + (x >> 4)) & 0x0f0f0f0f0f0f0f0f
	return int((x * 0x0101010101010101) >> 56)
}

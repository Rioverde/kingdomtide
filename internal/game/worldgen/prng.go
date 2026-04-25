package worldgen

import (
	"math/rand/v2"
	"sync"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// newPCG constructs a deterministic seeded PCG from a pre-mixed state word.
// The stream is derived via Splitmix64 so the two PCG halves are statistically
// independent even when state values are adjacent — important for tile-grid
// coordinate seeding where (x, y) and (x+1, y) only differ by one bit in the
// input.
func newPCG(state uint64) *rand.Rand {
	stream := geom.Splitmix64(state ^ 0x94d049bb133111eb)
	return rand.New(rand.NewPCG(state, stream))
}

// lazyLoad returns the cached value for key, or calls compute and stores the
// result atomically. Duplicate work on cold misses is bounded to the miss
// window — first-stored wins via LoadOrStore.
func lazyLoad[K comparable, V any](m *sync.Map, key K, compute func() V) V {
	if cached, ok := m.Load(key); ok {
		return cached.(V)
	}
	v := compute()
	actual, _ := m.LoadOrStore(key, v)
	return actual.(V)
}

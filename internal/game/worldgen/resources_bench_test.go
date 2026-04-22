package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// BenchmarkPointDepositsInRegion measures one full Poisson-disk pass
// across every point-like kind over a single super-region, including
// biome-gate and landmark / volcano collision. Plan target: < 50 ms
// per super-region.
func BenchmarkPointDepositsInRegion(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Walk a diagonal so every iteration hits a fresh super-region
		// and the bench reflects generation cost rather than cache reads.
		sr := superRegion{X: i, Y: i}
		_ = pointDepositsInRegion(seed, sr, wg, lm, vs)
	}
}

// BenchmarkDepositAt_Cached measures the DepositAt hot path when the
// super-region is already generated. Plan target: < 500 ns.
func BenchmarkDepositAt_Cached(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	src := NewNoiseDepositSource(seed, wg, lm, vs)

	// Warm the origin super-region and pick a tile inside it.
	_ = src.ensureRegion(superRegion{X: 0, Y: 0})
	p := game.Position{X: 42, Y: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = src.DepositAt(p)
	}
}

// BenchmarkDepositsIn_100x100 measures a 100x100-tile rect query on a
// fully warmed source. Plan target: < 5 ms.
func BenchmarkDepositsIn_100x100(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	src := NewNoiseDepositSource(seed, wg, lm, vs)

	rect := game.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	// Warm every super-region the rect touches so the bench reflects
	// iteration cost rather than first-generation cost.
	_ = src.DepositsIn(rect)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.DepositsIn(rect)
	}
}

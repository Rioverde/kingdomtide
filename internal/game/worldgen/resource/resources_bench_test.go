package resource_test

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/landmark"
	"github.com/Rioverde/gongeons/internal/game/worldgen/region"
	"github.com/Rioverde/gongeons/internal/game/worldgen/resource"
	"github.com/Rioverde/gongeons/internal/game/worldgen/volcano"
)

// BenchmarkPointDepositsInRegion measures one full Poisson-disk pass
// across every point-like kind over a single super-region, including
// biome-gate and landmark / volcano collision. Plan target: < 50 ms
// per super-region.
func BenchmarkPointDepositsInRegion(b *testing.B) {
	const seed int64 = 42
	wg := worldgen.NewWorldGenerator(seed)
	regions := region.NewNoiseRegionSource(seed, wg)
	lm := landmark.NewNoiseLandmarkSource(seed, regions, wg)
	vs := volcano.NewNoiseVolcanoSource(seed, wg, lm)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Walk a diagonal so every iteration hits a fresh super-region
		// and the bench reflects generation cost rather than cache reads.
		sr := genprim.SuperRegion{X: i, Y: i}
		_ = resource.PointDepositsInRegionForTest(seed, sr, wg, lm, vs)
	}
}

// BenchmarkDepositAt_Cached measures the DepositAt hot path when the
// super-region is already generated. Plan target: < 500 ns.
func BenchmarkDepositAt_Cached(b *testing.B) {
	const seed int64 = 42
	wg := worldgen.NewWorldGenerator(seed)
	regions := region.NewNoiseRegionSource(seed, wg)
	lm := landmark.NewNoiseLandmarkSource(seed, regions, wg)
	vs := volcano.NewNoiseVolcanoSource(seed, wg, lm)
	src := resource.NewNoiseDepositSource(seed, wg, lm, vs)

	// Warm the origin super-region and pick a tile inside it.
	src.EnsureRegionForTest(genprim.SuperRegion{X: 0, Y: 0})
	p := geom.Position{X: 42, Y: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = src.DepositAt(p)
	}
}

// BenchmarkDepositsIn_100x100 measures a 100x100-tile rect query on a
// fully warmed source. Plan target: < 5 ms.
func BenchmarkDepositsIn_100x100(b *testing.B) {
	const seed int64 = 42
	wg := worldgen.NewWorldGenerator(seed)
	regions := region.NewNoiseRegionSource(seed, wg)
	lm := landmark.NewNoiseLandmarkSource(seed, regions, wg)
	vs := volcano.NewNoiseVolcanoSource(seed, wg, lm)
	src := resource.NewNoiseDepositSource(seed, wg, lm, vs)

	rect := geom.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	// Warm every super-region the rect touches so the bench reflects
	// iteration cost rather than first-generation cost.
	_ = src.DepositsIn(rect)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.DepositsIn(rect)
	}
}

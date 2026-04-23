package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// BenchmarkPickVolcanoAnchors measures one full Poisson-disk + acceptance
// pass over a single super-region. Plan target: < 5 ms per super-region.
func BenchmarkPickVolcanoAnchors(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	sr := superRegion{X: 0, Y: 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pickVolcanoAnchors(seed, superRegion{X: sr.X + i, Y: sr.Y}, lm, wg)
	}
}

// BenchmarkGrowFootprint measures one full footprint build for a volcano
// of each state. Plan target: < 1 ms per volcano.
func BenchmarkGrowFootprint(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)

	states := []world.VolcanoState{world.VolcanoActive, world.VolcanoDormant, world.VolcanoExtinct}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		st := states[i%len(states)]
		anchor := geom.Position{X: i * 97, Y: i * 131}
		_, _, _ = growFootprint(anchor, st, seed, wg, nil)
	}
}

// BenchmarkTerrainOverrideAt_Cold measures a cold-cache resolve. The
// first call on any super-region triggers generation; this benchmark
// keeps hitting fresh super-regions so it reflects the worst case.
func BenchmarkTerrainOverrideAt_Cold(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Fresh source each iteration so every call hits cold super-regions.
		src := NewNoiseVolcanoSource(seed, wg, lm)
		_, _ = src.TerrainOverrideAt(geom.Position{X: i * 10000, Y: i * 10000})
	}
}

// BenchmarkTerrainOverrideAt_Warm measures the hot path once every
// neighbouring super-region is cached. Plan target: < 2 µs warm.
func BenchmarkTerrainOverrideAt_Warm(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	src := NewNoiseVolcanoSource(seed, wg, lm)

	// Warm the 3x3 super-region neighbourhood around the origin.
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			_ = src.ensureSuperRegion(superRegion{X: x, Y: y})
		}
	}
	p := geom.Position{X: 42, Y: 42}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = src.TerrainOverrideAt(p)
	}
}

// BenchmarkVolcanoAt measures a VolcanoAt call with warm cache.
func BenchmarkVolcanoAt(b *testing.B) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	src := NewNoiseVolcanoSource(seed, wg, lm)
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			_ = src.ensureSuperRegion(superRegion{X: x, Y: y})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = src.VolcanoAt(geom.SuperChunkCoord{X: 0, Y: 0})
	}
}

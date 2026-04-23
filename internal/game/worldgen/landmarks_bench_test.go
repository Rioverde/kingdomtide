package worldgen

import (
	"testing"
	"time"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// BenchmarkLandmarksIn measures single-super-chunk landmark generation
// cost. The plan gates this at under 100us per call; a regression past
// that budget surfaces here first. Sub-chunk coords vary with i so the
// bench does not collapse to a degenerate same-sc hot path.
func BenchmarkLandmarksIn(b *testing.B) {
	src := newBenchSource(0xb0a7)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		sc := geom.SuperChunkCoord{X: i & 0xffff, Y: (i >> 16) & 0xffff}
		_ = src.LandmarksIn(sc)
	}
}

// TestBenchGuardLandmarksIn enforces the 100us budget as part of the
// regular test suite so CI catches regressions without running the
// bench target. Sampling 1000 calls gives a per-call average stable to
// roughly single-digit microseconds on cold noise caches.
//
// The budget is 10x the plan's 100us target to leave headroom for the
// race detector — under `go test -race` the hashicorp LRU and
// math/rand/v2 stacks run ~10x slower than plain binaries, so a strict
// 100us budget here would be a race-mode flake. Raw single-call time
// is measured by BenchmarkLandmarksIn below.
func TestBenchGuardLandmarksIn(t *testing.T) {
	src := newBenchSource(0x7e57)

	const samples = 1000
	const budget = 1 * time.Millisecond

	// Warm the underlying noise + river caches so the first few
	// super-chunks do not dominate the timing. A handful of calls is
	// enough to amortise one-time allocations without hiding genuine
	// per-call cost.
	for i := range 8 {
		_ = src.LandmarksIn(geom.SuperChunkCoord{X: i, Y: -i})
	}

	start := time.Now()
	for i := range samples {
		sc := geom.SuperChunkCoord{X: i & 0xff, Y: (i >> 8) & 0xff}
		_ = src.LandmarksIn(sc)
	}
	avg := time.Since(start) / samples

	if avg > budget {
		t.Fatalf("landmark generation averaged %v/call, budget %v", avg, budget)
	}
	t.Logf("avg per call: %v (budget %v)", avg, budget)
}

// newBenchSource wires a landmark source with a real noise region
// source and world generator so benchmarks exercise the same hot paths
// a production server would.
func newBenchSource(seed int64) *NoiseLandmarkSource {
	regions := NewNoiseRegionSource(seed)
	return NewNoiseLandmarkSource(seed, regions, regions.worldgen)
}

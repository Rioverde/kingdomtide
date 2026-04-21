package worldgen

import (
	"testing"
	"time"
)

// raceEnabled is set to true in race_on_test.go when the `race` build tag is
// active and false otherwise. The file-split lets the performance test
// self-skip under `go test -race` without calling into runtime internals —
// race-detector overhead would blow the 5 ms budget by 10× regardless of
// generator quality.
var raceEnabled = false

// TestChunkPerformanceBudget is a soft, self-timing assertion on the cold
// Chunk() budget. The plan sets < 5 ms per call on an 80×80 buffer; this test
// measures an average over a small sample and fails only if the average is
// well outside the budget. CI latency noise (cold CPU, shared runners) makes
// a tight per-iteration assertion unreliable — an average over 10 fresh
// generators is stable enough to flag genuine regressions without flaking.
//
// Skipped under -race: the race detector's 5–10× slowdown on tight numeric
// loops makes the budget unattainable and the assertion meaningless there.
// The budget is enforced in normal test runs only.
func TestChunkPerformanceBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping performance budget under -race; race detector overhead blows the 5 ms target")
	}
	if testing.Short() {
		t.Skip("skipping performance budget under -short")
	}

	const samples = 10
	const budget = 5 * time.Millisecond

	start := time.Now()
	for i := range samples {
		g := NewWorldGenerator(int64(i + 1))
		_ = g.Chunk(ChunkCoord{X: i, Y: -i})
	}
	avg := time.Since(start) / samples
	t.Logf("cold Chunk() avg = %s (budget %s)", avg, budget)
	if avg > budget {
		t.Errorf("cold Chunk() avg %s exceeds budget %s", avg, budget)
	}
}

// BenchmarkChunkWithHydrology measures the end-to-end cost of generating one
// chunk with the full hydrology pipeline (Priority-Flood+ε + D8 flow
// accumulation + river mask + lake overlay). Each iteration runs against a
// fresh generator so the hydrology cache cannot hide the real cost of a cold
// buffer — the performance budget is < 5 ms per cold Chunk() call on an 80×80
// buffer.
func BenchmarkChunkWithHydrology(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		g := NewWorldGenerator(int64(i + 1))
		_ = g.Chunk(ChunkCoord{X: i, Y: -i})
	}
}

// BenchmarkChunkWithHydrologyWarm measures the warm-cache cost: same chunk
// generated repeatedly from the same generator. This is what the viewport
// hot path sees — the first access pays the full fill, every subsequent
// access is a straight hydrology-cache hit.
func BenchmarkChunkWithHydrologyWarm(b *testing.B) {
	g := NewWorldGenerator(42)
	cc := ChunkCoord{X: 3, Y: -1}
	// Prime: first call fills, rest hit the hydrology cache.
	_ = g.Chunk(cc)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = g.Chunk(cc)
	}
}

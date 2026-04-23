package worldgen

import (
	"testing"
	"time"

	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
)

// raceEnabled is set to true in race_on_test.go when the `race` build tag is
// active and false otherwise. The file-split lets the performance test
// self-skip under `go test -race` without calling into runtime internals —
// race-detector overhead would blow the perf budget by 10× regardless of
// generator quality.
var raceEnabled = false

// TestChunkPerformanceBudget is a soft assertion on the cost of generating a
// cold chunk. With the deterministic trace algorithm (rivers.go) the chunk
// enumerates head candidates in a radius around itself, traces each valid
// head via D8 steepest-descent, and collects path / lake cells that land
// inside the chunk. There is no priority-flood buffer to amortise; the cost
// is intrinsic to each chunk.
//
// The budget is 10 ms averaged over 10 fresh generators. This is the
// worst-case "first-chunk-on-a-new-generator" cost. Real gameplay hits the
// river cache on repeat queries, dropping amortised cost to sub-millisecond
// — verified by the warm benchmark below. 10 ms leaves comfortable headroom
// for CI latency jitter.
//
// Skipped under -race: the race detector's 5–10× slowdown on tight numeric
// loops makes the budget unattainable and the assertion meaningless there.
func TestChunkPerformanceBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("skipping performance budget under -race; race detector overhead blows the target")
	}
	if testing.Short() {
		t.Skip("skipping performance budget under -short")
	}

	const samples = 10
	const budget = 10 * time.Millisecond

	start := time.Now()
	for i := range samples {
		g := NewWorldGenerator(int64(i + 1))
		_ = g.Chunk(chunk.ChunkCoord{X: i, Y: -i})
	}
	avg := time.Since(start) / samples

	t.Logf("cold Chunk() avg = %s (budget %s)", avg, budget)
	if avg > budget {
		t.Errorf("cold Chunk() avg %s exceeds budget %s", avg, budget)
	}
}

// BenchmarkChunkCold measures the cost of generating one chunk on a fresh
// generator — no caches hit. Dominated by head enumeration + trace per
// candidate head within riverMaxTraceLen of the chunk.
func BenchmarkChunkCold(b *testing.B) {
	b.ReportAllocs()
	for i := range b.N {
		g := NewWorldGenerator(int64(i + 1))
		_ = g.Chunk(chunk.ChunkCoord{X: i, Y: -i})
	}
}

// BenchmarkChunkWarm measures the warm-cache path: repeated requests for the
// same chunk on the same generator. Reflects the viewport hot path where a
// single frame may query the same chunk many times.
func BenchmarkChunkWarm(b *testing.B) {
	g := NewWorldGenerator(42)
	cc := chunk.ChunkCoord{X: 3, Y: -1}
	_ = g.Chunk(cc)
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		_ = g.Chunk(cc)
	}
}

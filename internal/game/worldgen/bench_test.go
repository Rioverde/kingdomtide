package worldgen

import (
	"testing"
	"time"
)

// BenchmarkGenerate measures end-to-end world generation per size.
// Run with: go test -bench=BenchmarkGenerate -run=^$ -benchmem ./internal/game/worldgen/
func BenchmarkGenerate(b *testing.B) {
	cases := []struct {
		name string
		size WorldSize
	}{
		{"Tiny", WorldSizeTiny},
		{"Small", WorldSizeSmall},
		{"Standard", WorldSizeStandard},
		{"Large", WorldSizeLarge},
		{"Huge", WorldSizeHuge},
		{"Colossal", WorldSizeColossal},
		{"Gigantic", WorldSizeGigantic},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = Generate(int64(i), c.size)
			}
		})
	}
}

// BenchmarkGenerateStages breaks the Standard pipeline into per-stage
// measurements so a regression in one pass shows up directly. Uses
// the same GenStageHook the visual test consumes.
func BenchmarkGenerateStages(b *testing.B) {
	b.ReportAllocs()
	totals := make(map[string]int64)
	GenStageHook = func(stage string, dur time.Duration) {
		totals[stage] += int64(dur)
	}
	defer func() { GenStageHook = nil }()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Generate(int64(i), WorldSizeStandard)
	}
	b.StopTimer()
	for k, v := range totals {
		b.ReportMetric(float64(v)/float64(b.N)/1e6, k+"_ms/op")
	}
}

package stats

import "testing"

// BenchmarkModifier_LookupTable measures the lookup-table hot path.
// Compare against the pre-optimization baseline noted in
// MECHANICS_STATUS.md if a baseline benchmark file ever lands.
func BenchmarkModifier_LookupTable(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = Modifier(i % 21)
	}
}

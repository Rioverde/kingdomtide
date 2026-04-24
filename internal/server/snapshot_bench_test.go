package server

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// benchCenter is the viewport centre used by the composite benches. The
// origin is a stable choice — no spawn lottery, and the surrounding
// super-chunks carry a representative mix of biomes, landmarks, and
// volcanoes under snapshotTestSeed (see testhelpers_test.go).
var benchCenter = geom.Position{X: 0, Y: 0}

// buildBenchService mirrors cmd/server/main.go's buildWorld wiring
// (region + landmark + volcano + deposit sources) so the bench measures
// the real per-frame snapshot cost the server pays, not a stripped
// fixture. Returns the service so the caller can reach its caches
// directly when assembling snapshots.
func buildBenchService(tb testing.TB) *Service {
	tb.Helper()
	wg := worldgen.NewChunkedSource(snapshotTestSeed)
	regionSrc := worldgen.NewNoiseRegionSource(snapshotTestSeed, wg.Generator())
	landmarkSrc := worldgen.NewNoiseLandmarkSource(snapshotTestSeed, regionSrc, wg.Generator())
	volcanoSrc := worldgen.NewNoiseVolcanoSource(snapshotTestSeed, wg.Generator(), landmarkSrc)
	depositSrc := worldgen.NewNoiseDepositSource(snapshotTestSeed, wg.Generator(), landmarkSrc, volcanoSrc)
	w := world.NewWorld(
		wg,
		world.WithSeed(snapshotTestSeed),
		world.WithRegionSource(regionSrc),
		world.WithLandmarkSource(landmarkSrc),
		world.WithVolcanoSource(volcanoSrc),
		world.WithDepositSource(depositSrc),
	)
	return NewService(w, silentLog())
}

// snapshotOnce drives one end-to-end snapshot through the real service
// path: region lookup via the LRU, tile assembly (terrain + volcano
// override + landmark), and wire-struct allocation. Matches what a
// single viewport tick pays on the server.
func snapshotOnce(svc *Service, w, h int) {
	_ = snapshotOf(
		svc.world,
		benchCenter,
		w,
		h,
		svc.regionAt(benchCenter),
		svc.landmarks,
		svc.volcanoes,
	)
}

// BenchmarkSnapshot_Cold_64x64 measures one 64x64 snapshot against a
// freshly-built worldgen stack on every iteration — region, landmark,
// and volcano caches all start empty so each op absorbs the first-touch
// noise sampling and placement cost.
func BenchmarkSnapshot_Cold_64x64(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		svc := buildBenchService(b)
		b.StartTimer()
		snapshotOnce(svc, 64, 64)
	}
}

// BenchmarkSnapshot_Warm_64x64 measures one 64x64 snapshot on a
// pre-warmed service — a single priming call populates every cache
// touching the viewport so subsequent ops exercise the steady-state
// hot path.
func BenchmarkSnapshot_Warm_64x64(b *testing.B) {
	svc := buildBenchService(b)
	snapshotOnce(svc, 64, 64)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snapshotOnce(svc, 64, 64)
	}
}

// BenchmarkSnapshot_Warm_128x128 is the same shape as the warm 64x64
// bench but over a 4x-larger viewport so tile assembly dominates and
// cache-hit savings are amortised across more tiles — the delta
// against Warm_64x64 shows how snapshot cost scales with viewport
// area.
func BenchmarkSnapshot_Warm_128x128(b *testing.B) {
	svc := buildBenchService(b)
	snapshotOnce(svc, 128, 128)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		snapshotOnce(svc, 128, 128)
	}
}

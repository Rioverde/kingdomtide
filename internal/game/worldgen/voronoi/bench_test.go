package voronoi

import (
	"math/rand/v2"
	"testing"
)

// BenchmarkGenerate measures end-to-end Voronoi build at scales
// matching the worldgen size presets.
func BenchmarkGenerate(b *testing.B) {
	cases := []struct {
		name        string
		w, h, count int
	}{
		{"Tiny", 640, 256, 4047},          // matches WorldSizeTiny @ K=10
		{"Small", 1280, 512, 8094},        // matches WorldSizeSmall @ K=10
		{"Standard", 2560, 1024, 16188},   // matches WorldSizeStandard @ K=10
		{"Large", 3840, 1536, 24295},      // matches WorldSizeLarge @ K=10
		{"Huge", 5120, 2048, 32393},       // matches WorldSizeHuge @ K=10
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = Generate(int64(i), c.w, c.h, c.count, 2, 0)
			}
		})
	}
}

// BenchmarkRasterizeNearest isolates the per-tile nearest-site lookup
// — the hot path that every Lloyd iteration plus the final rasterise
// hit. Sites are placed once outside the timed region.
func BenchmarkRasterizeNearest(b *testing.B) {
	cases := []struct {
		name        string
		w, h, count int
	}{
		{"Standard_16K", 2560, 1024, 16190},
		{"Huge_32K", 5120, 2048, 32393},
	}
	for _, c := range cases {
		rng := rand.New(rand.NewPCG(1, 1))
		sites := placeSeeds(rng, c.w, c.h, c.count)
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = rasterizeNearest(c.w, c.h, sites)
			}
		})
	}
}

// BenchmarkPlaceSeeds isolates the bucket-accelerated rejection
// sampler — the part that scales worst with cell count.
func BenchmarkPlaceSeeds(b *testing.B) {
	cases := []struct {
		name        string
		w, h, count int
	}{
		{"Standard_16K", 2560, 1024, 16190},
		{"Huge_32K", 5120, 2048, 32393},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rng := rand.New(rand.NewPCG(uint64(i), uint64(i)))
				_ = placeSeeds(rng, c.w, c.h, c.count)
			}
		})
	}
}

// BenchmarkComputeRasterCentroids isolates the Lloyd's-relaxation
// centroid pass — runs once per Lloyd iteration so its cost
// multiplies by lloydIterations in the full pipeline.
func BenchmarkComputeRasterCentroids(b *testing.B) {
	const w, h, count = 2560, 1024, 16190
	rng := rand.New(rand.NewPCG(1, 1))
	sites := placeSeeds(rng, w, h, count)
	cellID := rasterizeNearest(w, h, sites)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = computeRasterCentroids(cellID, sites, w, h)
	}
}

// BenchmarkFindCornersEdges isolates the corner-detection +
// edge-construction pass.
func BenchmarkFindCornersEdges(b *testing.B) {
	const w, h, count = 2560, 1024, 16190
	rng := rand.New(rand.NewPCG(1, 1))
	sites := placeSeeds(rng, w, h, count)
	cellID := rasterizeNearest(w, h, sites)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, vertCells := findCorners(cellID, w, h)
		_ = buildEdges(vertCells)
	}
}

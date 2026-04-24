package worldgen

// heap_diag_bench_test.go — per-subsystem SR-generation benchmarks plus
// cold/warm Chunk comparison and a memprofile workload.
//
// Each bench that exercises a cold SR-generation path either re-constructs
// the source inside b.StopTimer/b.StartTimer, or walks a diagonal so each
// iteration hits a fresh super-region. Setup cost is excluded from the
// timed window; only the generation step itself is measured.

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/landmark"
	"github.com/Rioverde/gongeons/internal/game/worldgen/region"
	"github.com/Rioverde/gongeons/internal/game/worldgen/resource"
)

const diagSeed int64 = 0x5ca1ab1e

// --- Part A: per-subsystem SR-generation benchmarks -------------------------

// BenchmarkWG_VolcanoSR_Generate measures one cold VolcanoAt call that forces
// ensureSuperRegion → generate for a fresh SR. A new NoiseVolcanoSource is
// constructed per iteration so no SR is ever reused.
func BenchmarkWG_VolcanoSR_Generate(b *testing.B) {
	wg := NewWorldGenerator(diagSeed)
	regions := region.NewNoiseRegionSource(diagSeed, wg)
	lm := landmark.NewNoiseLandmarkSource(diagSeed, regions, wg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		src := NewNoiseVolcanoSource(diagSeed, wg, lm)
		// Pick an SC inside SR (i, 0) so every iteration is a cold hit.
		sc := geom.SuperChunkCoord{X: i * genprim.SuperRegionSideSC, Y: 0}
		b.StartTimer()

		_ = src.VolcanoAt(sc)
	}
}

// BenchmarkWG_DepositSR_Generate measures one cold SR generation through
// NoiseDepositSource. Triggers the full pipeline: zonal + fish per-tile scan,
// Poisson-disk point kinds, volcanic structural pass.
func BenchmarkWG_DepositSR_Generate(b *testing.B) {
	wg := NewWorldGenerator(diagSeed)
	regions := region.NewNoiseRegionSource(diagSeed, wg)
	lm := landmark.NewNoiseLandmarkSource(diagSeed, regions, wg)
	vs := NewNoiseVolcanoSource(diagSeed, wg, lm)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		src := NewNoiseDepositSource(diagSeed, wg, lm, vs)
		// One tile inside SR (i, 0).
		p := geom.Position{X: i * genprim.SuperRegionSideTiles, Y: 0}
		b.StartTimer()

		_, _ = src.DepositAt(p)
	}
}

// BenchmarkWG_RegionSR_Generate measures one RegionAt call. NoiseRegionSource
// has no per-SR cache — it samples noise fields on every call — so this bench
// measures one deterministic noise + name composition pass for a fresh SC.
// A new source is not strictly needed here because every call is independent,
// but we walk a diagonal to keep the bench shape consistent with the others.
func BenchmarkWG_RegionSR_Generate(b *testing.B) {
	wg := NewWorldGenerator(diagSeed)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		src := NewNoiseRegionSource(diagSeed, wg)
		sc := geom.SuperChunkCoord{X: i, Y: i}
		b.StartTimer()

		_ = src.RegionAt(sc)
	}
}

// BenchmarkWG_RiversChunk_Cold measures the rivers pipeline cold path inside
// WorldGenerator.Chunk. Rivers are computed implicitly during Chunk generation
// (RiverTilesInChunk + LakeTilesInChunk) and have no standalone public
// entry-point; this bench captures their cost as part of Chunk. A fresh
// WorldGenerator is constructed per iteration so the river LRU always misses.
func BenchmarkWG_RiversChunk_Cold(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		g := NewWorldGenerator(diagSeed ^ int64(i))
		cc := chunk.ChunkCoord{X: i, Y: -i}
		b.StartTimer()

		_ = g.Chunk(cc)
	}
}

// BenchmarkWG_LandmarkSR_Generate measures one cold LandmarksIn call. The
// NoiseLandmarkSource has no SR-level cache — it computes the four sub-cells
// from scratch on every call — so every iteration is equally cold.
func BenchmarkWG_LandmarkSR_Generate(b *testing.B) {
	wg := NewWorldGenerator(diagSeed)
	regions := NewNoiseRegionSource(diagSeed, wg)

	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		b.StopTimer()
		lm := NewNoiseLandmarkSource(diagSeed, regions, wg)
		sc := geom.SuperChunkCoord{X: i, Y: i}
		b.StartTimer()

		_ = lm.LandmarksIn(sc)
	}
}

// BenchmarkWG_CitiesSR_Generate — cities/settlement_names.go contains only
// name-generation helpers (SettlementName), not a per-SR placement source.
// There is no SR-level generator to bench; city placement is driven by the
// history/simulation pipeline, not by worldgen on-demand. Skipped with a note.
func BenchmarkWG_CitiesSR_Generate(b *testing.B) {
	b.Skip("cities package has no per-SR generator; settlement placement is history-pipeline driven, not worldgen on-demand")
}

// --- Part B: cold vs warm Chunk() comparison --------------------------------

// BenchmarkWG_Chunk_Cold exercises the full SR-generation pipeline including
// river tracing by requesting a fresh WorldGenerator per iteration. Every
// subsystem cache (river LRU, volcano SR map, deposit SR map) misses.
func BenchmarkWG_Chunk_Cold(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		g := NewWorldGenerator(int64(i + 1))
		_ = g.Chunk(chunk.ChunkCoord{X: i, Y: -i})
	}
}

// BenchmarkWG_Chunk_Warm exercises the hot path where the river LRU already
// contains the chunk's SR data. All noise sampling still runs but the
// river-trace work is skipped.
func BenchmarkWG_Chunk_Warm(b *testing.B) {
	g := NewWorldGenerator(diagSeed)
	cc := chunk.ChunkCoord{X: 3, Y: -1}
	_ = g.Chunk(cc) // prime caches

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = g.Chunk(cc)
	}
}

// --- Part C: memprofile workload --------------------------------------------

// BenchmarkWG_ColdChunkWorkload_MemProfile simulates a player walking through
// 50 cold chunks along a linear path. Every chunk coord is distinct so every
// iteration is a full cold hit across all subsystem caches. Intended to be run
// with -memprofile to capture the allocation profile of the full on-demand
// worldgen pipeline.
func BenchmarkWG_ColdChunkWorkload_MemProfile(b *testing.B) {
	const coldChunks = 50

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		g := NewWorldGenerator(diagSeed)
		for i := range coldChunks {
			_ = g.Chunk(chunk.ChunkCoord{X: i, Y: 0})
		}
	}
}

// --- helpers ----------------------------------------------------------------

// Ensure the resource package is referenced (prevents import elision when the
// compiler sees only the alias NewNoiseDepositSource in sources.go).
var _ *resource.NoiseDepositSource

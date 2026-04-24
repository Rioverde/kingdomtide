package server

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"sync"
	"testing"
	"time"
)

// testLandmarkSeed is the fixed seed used by landmark tests. Decoupled from
// testRegionSeed so the two test suites are independent and failures from one
// do not bleed into the other.
const testLandmarkSeed int64 = 0x1a2b3c4d5e6f7a8b

// testLandmarkWorld builds a fully-wired world (tile source, region source,
// landmark source) at testLandmarkSeed, matching the production buildWorld
// path in cmd/server/main.go.
func testLandmarkWorld() *world.World {
	wg := worldgen.NewChunkedSource(testLandmarkSeed)
	regionSrc := worldgen.NewNoiseRegionSource(testLandmarkSeed, wg.Generator())
	landmarkSrc := worldgen.NewNoiseLandmarkSource(testLandmarkSeed, regionSrc, wg.Generator())
	return world.NewWorld(
		wg,
		world.WithSeed(testLandmarkSeed),
		world.WithRegionSource(regionSrc),
		world.WithLandmarkSource(landmarkSrc),
	)
}

// TestSnapshotTileLandmarksPopulated verifies that a snapshot centred on a
// known landmark coordinate carries at least one tile with a non-NONE landmark.
// The test builds a fully-wired world, locates the first landmark in the
// super-chunk grid nearest to origin, then calls snapshotOf centred exactly on
// that coordinate — guaranteeing the landmark tile appears in the returned grid.
// This also confirms the full wiring path: world → service → landmark cache →
// snapshotOf → fillTile → pb.Tile.Landmark.
func TestSnapshotTileLandmarksPopulated(t *testing.T) {
	w := testLandmarkWorld()
	svc := NewService(w, silentLog())

	if svc.landmarks == nil {
		t.Fatal("service.landmarks: want non-nil, got nil — landmark source not wired into service")
	}

	// Locate the first landmark in the 5×5 super-chunk grid centred on origin.
	var landmarkPos geom.Position
	var found bool
outer:
	for scx := -2; scx <= 2 && !found; scx++ {
		for scy := -2; scy <= 2 && !found; scy++ {
			sc := geom.SuperChunkCoord{X: scx, Y: scy}
			for _, lm := range svc.landmarks.LandmarksIn(sc) {
				landmarkPos = lm.Coord
				found = true
				break outer
			}
		}
	}
	if !found {
		t.Fatal("could not find any landmark in the 5×5 super-chunk grid around origin")
	}

	// Build a snapshot centred on the landmark tile; the landmark must appear
	// in the returned tile grid as a non-NONE LandmarkKind.
	snap := snapshotOf(w, landmarkPos, DefaultViewportWidth, DefaultViewportHeight, nil, svc.landmarks, svc.volcanoes)

	var hasLandmark bool
	for _, tile := range snap.GetTiles() {
		if lm := tile.GetLandmark(); lm != nil && lm.GetKind() != pb.LandmarkKind_LANDMARK_KIND_NONE {
			hasLandmark = true
			break
		}
	}
	if !hasLandmark {
		t.Fatalf("snapshot centred on landmark coord %v: want at least one non-NONE landmark tile, got all NONE across %d tiles",
			landmarkPos, len(snap.GetTiles()))
	}
}

// countingLandmarkSource wraps an inner LandmarkSource with an atomic call
// counter. Constructed once and used read-only — safe for concurrent use.
type countingLandmarkSource struct {
	callCounter
	inner world.LandmarkSource
}

func (c *countingLandmarkSource) LandmarksIn(sc geom.SuperChunkCoord) []world.Landmark {
	c.hit()
	return c.inner.LandmarksIn(sc)
}

// TestLandmarkCacheHitRate verifies that 10 sequential lookups of the same
// SuperChunkCoord result in exactly 1 upstream source call — all subsequent
// lookups must be served from the LRU.
func TestLandmarkCacheHitRate(t *testing.T) {
	wg := worldgen.NewChunkedSource(testLandmarkSeed)
	regionSrc := worldgen.NewNoiseRegionSource(testLandmarkSeed, wg.Generator())
	inner := worldgen.NewNoiseLandmarkSource(testLandmarkSeed, regionSrc, wg.Generator())

	counter := &countingLandmarkSource{inner: inner}
	cache := newLandmarkCache(counter, DefaultLandmarkCacheCapacity)

	sc := geom.SuperChunkCoord{X: 5, Y: -3}
	const repeats = 10
	for range repeats {
		_ = cache.LandmarksIn(sc)
	}

	if got := counter.count(); got != 1 {
		t.Fatalf("source call count after %d lookups on one coord: want 1, got %d",
			repeats, got)
	}
	if got := cache.Len(); got != 1 {
		t.Fatalf("cache.Len: want 1, got %d", got)
	}
}

// TestLandmarkCacheRace smokes 100 goroutines × 100 lookups across varying
// SuperChunkCoords through a shared cache. The assertion is "no data race" —
// the -race detector flags any accidental shared-state mutation introduced by
// future refactors. Hit-rate correctness is covered by TestLandmarkCacheHitRate.
func TestLandmarkCacheRace(t *testing.T) {
	wg := worldgen.NewChunkedSource(testLandmarkSeed)
	regionSrc := worldgen.NewNoiseRegionSource(testLandmarkSeed, wg.Generator())
	inner := worldgen.NewNoiseLandmarkSource(testLandmarkSeed, regionSrc, wg.Generator())

	counter := &countingLandmarkSource{inner: inner}
	cache := newLandmarkCache(counter, DefaultLandmarkCacheCapacity)

	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1},
		{X: -1, Y: 0}, {X: 0, Y: -1}, {X: 2, Y: 3}, {X: -3, Y: 2},
	}

	const goroutines = 100
	const iters = 100
	var wgg sync.WaitGroup
	wgg.Add(goroutines)
	for r := range goroutines {
		go func(r int) {
			defer wgg.Done()
			for i := range iters {
				_ = cache.LandmarksIn(coords[(r+i)%len(coords)])
			}
		}(r)
	}
	wgg.Wait()
}

// BenchmarkSnapshotWithLandmarks measures the time to assemble one full
// snapshot with landmark lookups on a fully-wired service. The soft target is
// <500µs per call; we report ns/op via the standard benchmark output and
// additionally record µs/op as a custom metric for readability. No hard
// failure gate is applied inside the benchmark — regression detection is left
// to the benchstat workflow so CI noise does not cause flaky failures.
func BenchmarkSnapshotWithLandmarks(b *testing.B) {
	w := testLandmarkWorld()
	svc := NewService(w, silentLog())

	const playerID = "bench-player"
	svc.mu.Lock()
	_, err := w.ApplyCommand(world.JoinCmd{PlayerID: playerID, Name: "bench"})
	svc.viewports[playerID] = viewportDims{width: DefaultViewportWidth, height: DefaultViewportHeight}
	svc.mu.Unlock()
	if err != nil {
		b.Fatalf("join: %v", err)
	}

	pos, ok := w.PositionOf(playerID)
	if !ok {
		b.Fatal("player position not found after join")
	}

	b.ResetTimer()
	for range b.N {
		svc.mu.Lock()
		snap := svc.snapshotFor(playerID, pos)
		svc.mu.Unlock()
		if snap == nil {
			b.Fatal("nil snapshot")
		}
	}
	b.StopTimer()

	// Report µs/op as a custom metric alongside the default ns/op.
	// Skip the gate check on the probe run (b.N == 1) because that single
	// iteration absorbs world-generator cold-start costs that are not
	// representative of steady-state snapshot throughput.
	nsPerOp := b.Elapsed().Nanoseconds() / int64(b.N)
	b.ReportMetric(float64(nsPerOp)/float64(time.Microsecond), "µs/op")
	if b.N > 1 && time.Duration(nsPerOp) > time.Millisecond {
		b.Logf("WARN: BenchmarkSnapshotWithLandmarks %v/op exceeds 1ms target — possible regression", time.Duration(nsPerOp))
	}
}

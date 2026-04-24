package server

import (
	"sync"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// fakeVolcanoSource is an in-memory world.VolcanoSource used by the
// volcano-cache unit tests. volcanoCalls and overrideCalls record the
// number of delegated lookups so tests can assert cache behaviour
// without coupling to the production noise-based source. Two named
// callCounter fields (rather than embedding) because both counters
// would otherwise promote hit/count ambiguously.
type fakeVolcanoSource struct {
	volcanoCalls  callCounter
	overrideCalls callCounter
	volcanoes     map[geom.SuperChunkCoord][]world.Volcano
	overrides     map[geom.Position]world.Terrain
}

// VolcanoAt returns the canned slice for sc and increments the call
// counter. A nil inner map (never populated) still yields nil, matching
// the production VolcanoSource contract.
func (f *fakeVolcanoSource) VolcanoAt(sc geom.SuperChunkCoord) []world.Volcano {
	f.volcanoCalls.hit()
	return f.volcanoes[sc]
}

// TerrainOverrideAt returns the canned override for p (or "", false
// when absent) and increments the call counter.
func (f *fakeVolcanoSource) TerrainOverrideAt(p geom.Position) (world.Terrain, bool) {
	f.overrideCalls.hit()
	t, ok := f.overrides[p]
	return t, ok
}

// TestVolcanoCache_VolcanoAt_CachesHit verifies that repeated lookups
// on the same SuperChunkCoord incur exactly one upstream call — all
// subsequent hits must be served from the LRU.
func TestVolcanoCache_VolcanoAt_CachesHit(t *testing.T) {
	src := &fakeVolcanoSource{
		volcanoes: map[geom.SuperChunkCoord][]world.Volcano{
			{X: 1, Y: 2}: {{Anchor: geom.Position{X: 10, Y: 20}, State: world.VolcanoActive}},
		},
	}
	cache := newVolcanoCache(src, DefaultVolcanoCacheCapacity)

	sc := geom.SuperChunkCoord{X: 1, Y: 2}
	const repeats = 10
	for range repeats {
		_ = cache.VolcanoAt(sc)
	}

	if got := src.volcanoCalls.count(); got != 1 {
		t.Fatalf("source call count after %d lookups on one coord: want 1, got %d",
			repeats, got)
	}
	if got := cache.Len(); got != 1 {
		t.Fatalf("cache.Len: want 1, got %d", got)
	}
}

// TestVolcanoCache_VolcanoAt_LRUEviction verifies that filling the
// cache beyond its capacity drops the oldest entry. With capacity=2,
// inserting three distinct coords leaves the first one gone; re-asking
// for it triggers a second upstream call.
func TestVolcanoCache_VolcanoAt_LRUEviction(t *testing.T) {
	src := &fakeVolcanoSource{
		volcanoes: map[geom.SuperChunkCoord][]world.Volcano{
			{X: 0, Y: 0}: nil,
			{X: 1, Y: 0}: nil,
			{X: 2, Y: 0}: nil,
		},
	}
	cache := newVolcanoCache(src, 2)

	coords := []geom.SuperChunkCoord{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}}
	for _, sc := range coords {
		_ = cache.VolcanoAt(sc)
	}
	if got := cache.Len(); got != 2 {
		t.Fatalf("cache.Len after 3 inserts at cap=2: want 2, got %d", got)
	}
	if got := src.volcanoCalls.count(); got != 3 {
		t.Fatalf("source call count after 3 distinct inserts: want 3, got %d", got)
	}

	// The least-recently-used coord ({0,0}) must have been evicted; a
	// fresh lookup for it triggers another upstream call.
	_ = cache.VolcanoAt(coords[0])
	if got := src.volcanoCalls.count(); got != 4 {
		t.Fatalf("source call count after evicted re-lookup: want 4, got %d", got)
	}
}

// TestVolcanoCache_TerrainOverrideAt_PassesThrough verifies that the
// cache does not add a tile-level LRU on top of TerrainOverrideAt —
// every call must reach the underlying source so the source's own
// per-super-region caching remains the single layer of truth.
func TestVolcanoCache_TerrainOverrideAt_PassesThrough(t *testing.T) {
	src := &fakeVolcanoSource{
		overrides: map[geom.Position]world.Terrain{
			{X: 5, Y: 5}: world.TerrainVolcanoCore,
		},
	}
	cache := newVolcanoCache(src, DefaultVolcanoCacheCapacity)

	// Same position queried three times — each call must forward.
	p := geom.Position{X: 5, Y: 5}
	for i := range 3 {
		terrain, ok := cache.TerrainOverrideAt(p)
		if !ok {
			t.Fatalf("TerrainOverrideAt(%v) iter %d: want ok, got miss", p, i)
		}
		if terrain != world.TerrainVolcanoCore {
			t.Fatalf("TerrainOverrideAt(%v) iter %d: want %q, got %q",
				p, i, world.TerrainVolcanoCore, terrain)
		}
	}
	if got := src.overrideCalls.count(); got != 3 {
		t.Fatalf("source override call count: want 3 (pass-through), got %d", got)
	}

	// A position with no override forwards and returns ("", false).
	terrain, ok := cache.TerrainOverrideAt(geom.Position{X: 99, Y: 99})
	if ok {
		t.Fatalf("miss override: want (\"\", false), got (%q, true)", terrain)
	}
	if got := src.overrideCalls.count(); got != 4 {
		t.Fatalf("source override call count after miss: want 4, got %d", got)
	}
}

// TestVolcanoCache_NonPositiveCapacity_DefaultsApplied verifies that
// capacity <= 0 falls back to DefaultVolcanoCacheCapacity — mirrors the
// landmarkCache guard so newVolcanoCache never constructs a zero-size
// LRU that silently forwards every call.
func TestVolcanoCache_NonPositiveCapacity_DefaultsApplied(t *testing.T) {
	src := &fakeVolcanoSource{}
	for _, cap := range []int{0, -1, -128} {
		cache := newVolcanoCache(src, cap)
		if cache == nil {
			t.Fatalf("newVolcanoCache(%d): want non-nil, got nil", cap)
		}
		// Fill DefaultVolcanoCacheCapacity+1 entries and assert Len
		// caps at the default — proves the guard was applied.
		for i := 0; i <= DefaultVolcanoCacheCapacity; i++ {
			_ = cache.VolcanoAt(geom.SuperChunkCoord{X: i, Y: 0})
		}
		if got := cache.Len(); got != DefaultVolcanoCacheCapacity {
			t.Fatalf("cache.Len with cap=%d: want %d (default), got %d",
				cap, DefaultVolcanoCacheCapacity, got)
		}
	}
}

// TestVolcanoCache_Race smokes concurrent readers through a shared
// cache. The assertion is "no data race" — the -race detector flags
// any accidental shared-state mutation introduced by future refactors.
// Hit-rate correctness is covered by TestVolcanoCache_VolcanoAt_CachesHit.
func TestVolcanoCache_Race(t *testing.T) {
	src := &fakeVolcanoSource{
		volcanoes: map[geom.SuperChunkCoord][]world.Volcano{
			{X: 0, Y: 0}: nil, {X: 1, Y: 0}: nil, {X: 0, Y: 1}: nil, {X: 1, Y: 1}: nil,
			{X: -1, Y: 0}: nil, {X: 0, Y: -1}: nil, {X: 2, Y: 3}: nil, {X: -3, Y: 2}: nil,
		},
		overrides: map[geom.Position]world.Terrain{
			{X: 10, Y: 10}: world.TerrainVolcanoCore,
			{X: 20, Y: 20}: world.TerrainAshland,
		},
	}
	cache := newVolcanoCache(src, DefaultVolcanoCacheCapacity)

	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1},
		{X: -1, Y: 0}, {X: 0, Y: -1}, {X: 2, Y: 3}, {X: -3, Y: 2},
	}
	tilePositions := []geom.Position{
		{X: 10, Y: 10}, {X: 20, Y: 20}, {X: 30, Y: 30}, {X: 40, Y: 40},
	}

	const goroutines = 8
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for r := range goroutines {
		go func(r int) {
			defer wg.Done()
			for i := range iters {
				_ = cache.VolcanoAt(coords[(r+i)%len(coords)])
				_, _ = cache.TerrainOverrideAt(tilePositions[(r+i)%len(tilePositions)])
			}
		}(r)
	}
	wg.Wait()
}

package game

import (
	"math/rand"
	"sync"
	"testing"
)

// TestChunkCacheEvictsLRU puts three chunks into a capacity-2 cache and verifies the oldest
// entry — which was neither Put nor Get most recently — was evicted.
func TestChunkCacheEvictsLRU(t *testing.T) {
	c := NewChunkCache(2)
	a := &Chunk{Coord: ChunkCoord{X: 1}}
	b := &Chunk{Coord: ChunkCoord{X: 2}}
	d := &Chunk{Coord: ChunkCoord{X: 3}}

	c.Put(a.Coord, a)
	c.Put(b.Coord, b)
	c.Put(d.Coord, d) // should evict a

	if _, ok := c.Get(a.Coord); ok {
		t.Fatal("oldest entry a should have been evicted")
	}
	if _, ok := c.Get(b.Coord); !ok {
		t.Fatal("b should still be present")
	}
	if _, ok := c.Get(d.Coord); !ok {
		t.Fatal("newly inserted d should be present")
	}
	if got := c.Len(); got != 2 {
		t.Fatalf("cache len = %d, want 2", got)
	}
}

// TestChunkCacheGetPromotesToMRU verifies Get keeps the touched entry alive across later
// eviction rounds.
func TestChunkCacheGetPromotesToMRU(t *testing.T) {
	c := NewChunkCache(2)
	a := &Chunk{Coord: ChunkCoord{X: 1}}
	b := &Chunk{Coord: ChunkCoord{X: 2}}
	d := &Chunk{Coord: ChunkCoord{X: 3}}

	c.Put(a.Coord, a)
	c.Put(b.Coord, b)
	// Touch a — this should promote it to MRU so the next insert evicts b instead.
	if _, ok := c.Get(a.Coord); !ok {
		t.Fatal("a should be in cache before promotion test")
	}
	c.Put(d.Coord, d)

	if _, ok := c.Get(b.Coord); ok {
		t.Fatal("b should have been evicted after a was promoted")
	}
	if _, ok := c.Get(a.Coord); !ok {
		t.Fatal("a should still be present after promotion")
	}
}

// TestChunkCacheUpdateDoesNotEvict verifies that overwriting an existing key does not push
// any other entry out.
func TestChunkCacheUpdateDoesNotEvict(t *testing.T) {
	c := NewChunkCache(2)
	a := &Chunk{Coord: ChunkCoord{X: 1}}
	b := &Chunk{Coord: ChunkCoord{X: 2}}
	aReplace := &Chunk{Coord: ChunkCoord{X: 1}}

	c.Put(a.Coord, a)
	c.Put(b.Coord, b)
	c.Put(aReplace.Coord, aReplace)

	if got := c.Len(); got != 2 {
		t.Fatalf("cache len after in-place update = %d, want 2", got)
	}
	if got, _ := c.Get(a.Coord); got != aReplace {
		t.Fatalf("Get(a) = %p, want %p (the replacement)", got, aReplace)
	}
}

// TestChunkCacheDefaultsToFallback verifies a non-positive capacity falls back to the default.
func TestChunkCacheDefaultsToFallback(t *testing.T) {
	c := NewChunkCache(0)
	if got := c.Capacity(); got != DefaultChunkCacheCapacity {
		t.Fatalf("Capacity() = %d, want %d", got, DefaultChunkCacheCapacity)
	}
}

// TestChunkCacheConcurrent hammers the cache from many goroutines simultaneously
// to verify there are no data races. Run with -race to exercise the detector.
// The test is skipped under -short because it spawns 32 goroutines doing 2 000
// operations each — acceptable in a normal CI run but noisy in quick checks.
func TestChunkCacheConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("race test skipped under -short")
	}

	const goroutines = 32
	const ops = 2000

	cache := NewChunkCache(128)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(g)))
			for i := 0; i < ops; i++ {
				cc := ChunkCoord{X: rng.Intn(64) - 32, Y: rng.Intn(64) - 32}
				if rng.Intn(2) == 0 {
					_, _ = cache.Get(cc)
				} else {
					cache.Put(cc, &Chunk{Coord: cc})
				}
			}
		}(g)
	}
	wg.Wait()
}

package game

import "testing"

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

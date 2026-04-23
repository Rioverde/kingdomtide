package worldgen

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// DefaultChunkCacheCapacity is the standard LRU size for ChunkCache. 1024 chunks is roughly
// 256k tiles and a handful of MB — enough to keep the camera and a generous buffer hot
// without letting memory creep unboundedly on long-lived servers.
const DefaultChunkCacheCapacity = 1024

// ChunkCache is a bounded LRU keyed by ChunkCoord, backed by hashicorp/golang-lru/v2.
// The underlying cache is goroutine-safe, so no external synchronization is required.
//
// Eviction happens on Add when the cache is at capacity; Get promotes the touched entry
// to most-recently-used.
type ChunkCache struct {
	capacity int
	c        *lru.Cache[ChunkCoord, *Chunk]
}

// NewChunkCache builds a ChunkCache with the requested capacity. A capacity of zero or less
// falls back to DefaultChunkCacheCapacity so "give me a reasonable default" callers do not
// accidentally disable caching entirely.
func NewChunkCache(capacity int) *ChunkCache {
	if capacity <= 0 {
		capacity = DefaultChunkCacheCapacity
	}
	// lru.New only returns an error when size <= 0, which we have already guarded against.
	c, err := lru.New[ChunkCoord, *Chunk](capacity)
	if err != nil {
		panic("worldgen: chunk cache init: " + err.Error())
	}
	return &ChunkCache{capacity: capacity, c: c}
}

// Capacity returns the configured upper bound. Useful for tests and metrics.
func (c *ChunkCache) Capacity() int {
	return c.capacity
}

// Len returns the current number of entries. Primarily useful for tests.
func (c *ChunkCache) Len() int {
	return c.c.Len()
}

// Get returns the cached chunk for cc if present and promotes it to MRU. The returned
// pointer aliases cache storage; callers that mutate chunks should clone first (current
// usage is read-only).
func (c *ChunkCache) Get(cc ChunkCoord) (*Chunk, bool) {
	return c.c.Get(cc)
}

// Put inserts or replaces the chunk for cc and evicts the LRU entry if the cache overflows.
// Updating an existing key promotes it to MRU and does not count as a new insertion.
func (c *ChunkCache) Put(cc ChunkCoord, chunk *Chunk) {
	c.c.Add(cc, chunk)
}

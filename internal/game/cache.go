package game

import (
	"container/list"
	"sync"
)

// DefaultChunkCacheCapacity is the standard LRU size for ChunkCache. 1024 chunks is roughly
// 256k tiles and a handful of MB — enough to keep the camera and a generous buffer hot
// without letting memory creep unboundedly on long-lived servers.
const DefaultChunkCacheCapacity = 1024

// ChunkCache is a bounded LRU keyed by ChunkCoord. Eviction happens on Put when the cache
// is at capacity; Get promotes the touched entry to most-recently-used.
//
// Safe for concurrent use via an internal mutex. The mutex is held for the entire list
// manipulation because container/list is not goroutine-safe and the operations are cheap.
type ChunkCache struct {
	capacity int

	mu      sync.Mutex
	order   *list.List
	entries map[ChunkCoord]*list.Element
}

// cacheEntry is the value stored in each list element. Keeping the key alongside the value
// lets us find the ChunkCoord when evicting the tail without walking the map.
type cacheEntry struct {
	key   ChunkCoord
	value *Chunk
}

// NewChunkCache builds a ChunkCache with the requested capacity. A capacity of zero or less
// falls back to DefaultChunkCacheCapacity so "give me a reasonable default" callers do not
// accidentally disable caching entirely.
func NewChunkCache(capacity int) *ChunkCache {
	if capacity <= 0 {
		capacity = DefaultChunkCacheCapacity
	}
	return &ChunkCache{
		capacity: capacity,
		order:    list.New(),
		entries:  make(map[ChunkCoord]*list.Element, capacity),
	}
}

// Capacity returns the configured upper bound. Useful for tests and metrics.
func (c *ChunkCache) Capacity() int {
	return c.capacity
}

// Len returns the current number of entries. Primarily useful for tests.
func (c *ChunkCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Get returns the cached chunk for cc if present and promotes it to MRU. The returned
// pointer aliases cache storage; callers that mutate chunks should clone first (current
// usage is read-only).
func (c *ChunkCache) Get(cc ChunkCoord) (*Chunk, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.entries[cc]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*cacheEntry).value, true
}

// Put inserts or replaces the chunk for cc and evicts the LRU entry if the cache overflows.
// Updating an existing key does not count as a new insertion and never evicts.
func (c *ChunkCache) Put(cc ChunkCoord, chunk *Chunk) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.entries[cc]; ok {
		elem.Value.(*cacheEntry).value = chunk
		c.order.MoveToFront(elem)
		return
	}
	elem := c.order.PushFront(&cacheEntry{key: cc, value: chunk})
	c.entries[cc] = elem
	if c.order.Len() > c.capacity {
		c.evictOldest()
	}
}

// evictOldest removes the tail of the list and its map entry. Callers must hold c.mu.
func (c *ChunkCache) evictOldest() {
	tail := c.order.Back()
	if tail == nil {
		return
	}
	entry := tail.Value.(*cacheEntry)
	c.order.Remove(tail)
	delete(c.entries, entry.key)
}

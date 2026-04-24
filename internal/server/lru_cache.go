package server

import (
	lru "github.com/hashicorp/golang-lru/v2"
)

// lruCache is a thin generic wrapper around hashicorp/golang-lru/v2 that the
// server's per-source caches (regionCache, landmarkCache, volcanoCache) share.
// The underlying lru.Cache is safe for concurrent use, so this wrapper adds no
// synchronisation of its own. A zero value is not usable; build one with
// newLRUCache.
type lruCache[K comparable, V any] struct {
	inner *lru.Cache[K, V]
}

// newLRUCache builds an lruCache of capacity entries. The name prefix tags the
// panic message so a construction failure identifies the offending cache. Panics
// on lru.New failure because the only documented error is a non-positive size,
// which callers must guard against before calling.
func newLRUCache[K comparable, V any](name string, capacity int) *lruCache[K, V] {
	inner, err := lru.New[K, V](capacity)
	if err != nil {
		panic(name + " cache: " + err.Error())
	}
	return &lruCache[K, V]{inner: inner}
}

// getOrLoad returns the value for key, consulting the LRU first and calling
// load on a miss. The loaded value is stored and returned as-is; callers must
// not mutate shared slices or maps returned from load.
//
// Concurrent misses on the same key invoke load independently; callers whose
// load is expensive should wrap with singleflight.Group.
func (c *lruCache[K, V]) getOrLoad(key K, load func(K) V) V {
	if v, ok := c.inner.Get(key); ok {
		return v
	}
	v := load(key)
	c.inner.Add(key, v)
	return v
}

// Len returns the number of entries currently held. Test-only.
func (c *lruCache[K, V]) Len() int { return c.inner.Len() }

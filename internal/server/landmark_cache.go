package server

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// DefaultLandmarkCacheCapacity is the default LRU capacity for landmarkCache.
// At 64 entries the cache spans a ~512×512-tile footprint around a player's
// viewport — each super-chunk covers 64×64 tiles, so 64 entries holds all
// super-chunks reachable in a typical play session without eviction.
const DefaultLandmarkCacheCapacity = 64

// landmarkCache wraps a world.LandmarkSource with a fixed-size LRU keyed by
// SuperChunkCoord. Landmark lookups are deterministic and cheap once cached;
// even a small cache is highly effective under typical viewport drift because
// players stay in the same cluster of super-chunks across many moves.
// The shared lruCache helper is safe for concurrent use, so landmarkCache
// has no additional synchronisation of its own.
type landmarkCache struct {
	source world.LandmarkSource
	lru    *lruCache[geom.SuperChunkCoord, []world.Landmark]
}

// newLandmarkCache builds a cache of the requested capacity around source.
// A non-positive capacity is treated as DefaultLandmarkCacheCapacity so
// callers cannot accidentally construct a zero-size cache that silently
// forwards every call.
func newLandmarkCache(source world.LandmarkSource, capacity int) *landmarkCache {
	if capacity <= 0 {
		capacity = DefaultLandmarkCacheCapacity
	}
	return &landmarkCache{
		source: source,
		lru:    newLRUCache[geom.SuperChunkCoord, []world.Landmark]("landmark", capacity),
	}
}

// LandmarksIn returns the landmark slice for the given super-chunk,
// consulting the LRU first and delegating to the underlying source on
// a miss. The returned slice is the same value stored in the cache;
// callers must not mutate it. Landmark names are language-agnostic
// Parts records so the cache key is just sc — no per-language
// sharding required.
func (c *landmarkCache) LandmarksIn(sc geom.SuperChunkCoord) []world.Landmark {
	return c.lru.getOrLoad(sc, c.source.LandmarksIn)
}

// Len returns the number of entries currently held by the LRU. Test-only.
func (c *landmarkCache) Len() int { return c.lru.Len() }

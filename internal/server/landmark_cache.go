package server

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Rioverde/gongeons/internal/game"
)

// DefaultLandmarkCacheCapacity is the default LRU capacity for landmarkCache.
// At 64 entries the cache spans a ~512×512-tile footprint around a player's
// viewport — each super-chunk covers 64×64 tiles, so 64 entries holds all
// super-chunks reachable in a typical play session without eviction.
const DefaultLandmarkCacheCapacity = 64

// landmarkCache wraps a game.LandmarkSource with a fixed-size LRU keyed by
// SuperChunkCoord. Landmark lookups are deterministic and cheap once cached;
// even a small cache is highly effective under typical viewport drift because
// players stay in the same cluster of super-chunks across many moves.
// hashicorp/golang-lru/v2 is safe for concurrent use, so landmarkCache has
// no additional synchronisation of its own.
type landmarkCache struct {
	source game.LandmarkSource
	lru    *lru.Cache[game.SuperChunkCoord, []game.Landmark]
}

// newLandmarkCache builds a cache of the requested capacity around source.
// A non-positive capacity is treated as DefaultLandmarkCacheCapacity so
// callers cannot accidentally construct a zero-size cache that silently
// forwards every call. Panics on lru.New failure because that can only
// happen with a non-positive size, which we guard against above.
func newLandmarkCache(source game.LandmarkSource, capacity int) *landmarkCache {
	if capacity <= 0 {
		capacity = DefaultLandmarkCacheCapacity
	}
	cache, err := lru.New[game.SuperChunkCoord, []game.Landmark](capacity)
	if err != nil {
		panic("landmark cache: " + err.Error())
	}
	return &landmarkCache{source: source, lru: cache}
}

// LandmarksIn returns the landmark slice for the given super-chunk,
// consulting the LRU first and delegating to the underlying source on
// a miss. The returned slice is the same value stored in the cache;
// callers must not mutate it. Landmark names are language-agnostic
// Parts records so the cache key is just sc — no per-language
// sharding required.
func (c *landmarkCache) LandmarksIn(sc game.SuperChunkCoord) []game.Landmark {
	if v, ok := c.lru.Get(sc); ok {
		return v
	}
	v := c.source.LandmarksIn(sc)
	c.lru.Add(sc, v)
	return v
}

// Len returns the number of entries currently held by the LRU. Test-only.
func (c *landmarkCache) Len() int { return c.lru.Len() }

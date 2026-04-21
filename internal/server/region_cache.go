package server

import (
	lru "github.com/hashicorp/golang-lru/v2"

	"github.com/Rioverde/gongeons/internal/game"
)

// DefaultRegionCacheCapacity is the default LRU capacity for regionCache.
// At 64 entries the cache spans a ~512×512-tile footprint around a
// player's viewport — ample headroom for normal drift across Voronoi
// cells without re-sampling six noise fields per snapshot.
const DefaultRegionCacheCapacity = 64

// regionCache wraps a game.RegionSource with a fixed-size LRU keyed by the
// anchor's SuperChunkCoord. Two distant tiles that share an anchor share a
// cache entry. hashicorp/golang-lru/v2 is safe for concurrent use, so
// regionCache has no additional synchronisation of its own.
type regionCache struct {
	source game.RegionSource
	lru    *lru.Cache[game.SuperChunkCoord, game.Region]
}

// newRegionCache builds a cache of the requested capacity around source.
// A non-positive capacity is treated as DefaultRegionCacheCapacity so
// callers cannot accidentally construct a zero-size cache that silently
// forwards every call. Panics on lru.New failure because a failure there
// is a programmer error — the only documented error is a non-positive
// size, which we've already guarded against.
func newRegionCache(source game.RegionSource, capacity int) *regionCache {
	if capacity <= 0 {
		capacity = DefaultRegionCacheCapacity
	}
	cache, err := lru.New[game.SuperChunkCoord, game.Region](capacity)
	if err != nil {
		panic("region cache: " + err.Error())
	}
	return &regionCache{source: source, lru: cache}
}

// At returns the Region for the given anchor's SuperChunkCoord, consulting
// the LRU first and delegating to the underlying source on a miss. The
// returned Region is the same value stored in the cache; callers must not
// mutate its fields.
func (c *regionCache) At(sc game.SuperChunkCoord) game.Region {
	if r, ok := c.lru.Get(sc); ok {
		return r
	}
	r := c.source.RegionAt(sc)
	c.lru.Add(sc, r)
	return r
}

// Len returns the number of entries currently held by the LRU. Test-only.
func (c *regionCache) Len() int { return c.lru.Len() }

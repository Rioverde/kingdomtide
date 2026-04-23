package server

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// DefaultVolcanoCacheCapacity is the default LRU size for volcanoCache.
// Sized to match landmarkCache — a 128-entry window covers the player's
// 3×3 super-region neighborhood (9 super-regions × 8×8 super-chunks = 576
// super-chunks) minus the ones the player never re-visits, so 128 hits
// the hot path without bloating memory. A larger cache offers
// diminishing returns because volcano density is sparse (≈1 per 50
// super-chunks) and most entries would store an empty slice.
const DefaultVolcanoCacheCapacity = 128

// volcanoCache wraps a world.VolcanoSource with a fixed-size LRU keyed by
// SuperChunkCoord. Volcano placement is deterministic and driven by
// per-super-region generation, which the underlying NoiseVolcanoSource
// already memoizes via sync.Map; this cache layers a smaller,
// hot-path-only lookup so repeated snapshots on the same super-chunk
// skip a map read plus the per-super-region fan-out entirely.
// The shared lruCache helper is safe for concurrent use, so volcanoCache
// has no additional synchronisation of its own.
type volcanoCache struct {
	source world.VolcanoSource
	lru    *lruCache[geom.SuperChunkCoord, []world.Volcano]
}

// newVolcanoCache builds a cache of the requested capacity around source.
// A non-positive capacity is treated as DefaultVolcanoCacheCapacity so
// callers cannot accidentally construct a zero-size cache that silently
// forwards every call.
func newVolcanoCache(source world.VolcanoSource, capacity int) *volcanoCache {
	if capacity <= 0 {
		capacity = DefaultVolcanoCacheCapacity
	}
	return &volcanoCache{
		source: source,
		lru:    newLRUCache[geom.SuperChunkCoord, []world.Volcano]("volcano", capacity),
	}
}

// VolcanoAt returns the volcano slice for the given super-chunk,
// consulting the LRU first and delegating to the underlying source on
// a miss. The returned slice is the same value stored in the cache;
// callers must not mutate it.
func (c *volcanoCache) VolcanoAt(sc geom.SuperChunkCoord) []world.Volcano {
	return c.lru.getOrLoad(sc, c.source.VolcanoAt)
}

// TerrainOverrideAt passes straight through to the underlying source.
// The production NoiseVolcanoSource already caches per-super-region
// override data internally via sync.Map, so layering a tile-level LRU
// on top here would duplicate work without meaningful gains and make
// eviction reasoning harder — a moving viewport touches thousands of
// unique tiles per session while the super-region cache only ever
// holds a handful of entries. Document this explicitly so future
// readers do not add a redundant LRU.
func (c *volcanoCache) TerrainOverrideAt(t geom.Position) (world.Terrain, bool) {
	return c.source.TerrainOverrideAt(t)
}

// Len returns the number of entries currently held by the LRU. Test-only.
func (c *volcanoCache) Len() int { return c.lru.Len() }

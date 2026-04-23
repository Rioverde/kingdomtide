package worldgen

import (
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
)

// ChunkedSource is the procedural, infinite world.TileSource: a
// WorldGenerator plus an LRU chunk cache in front of it. Determined
// fully by seed.
type ChunkedSource struct {
	gen   *WorldGenerator
	cache *chunk.ChunkCache
}

// NewChunkedSource wires a fresh generator and cache around the given seed.
func NewChunkedSource(seed int64) *ChunkedSource {
	return &ChunkedSource{
		gen:   NewWorldGenerator(seed),
		cache: chunk.NewChunkCache(chunk.DefaultChunkCacheCapacity),
	}
}

// TileAt returns the procedurally-generated tile at (x, y), memoised at
// the chunk level so repeated reads inside the same chunk are cheap.
func (s *ChunkedSource) TileAt(x, y int) world.Tile {
	cc := chunk.WorldToChunk(x, y)
	if cached, ok := s.cache.Get(cc); ok {
		return cached.At(x, y)
	}
	c := s.gen.Chunk(cc)
	s.cache.Put(cc, &c)
	return c.At(x, y)
}

// Generator returns the underlying WorldGenerator. Callers that need to
// share the same procedural pipeline (e.g. NoiseLandmarkSource for terrain
// sampling) use this to avoid constructing a second generator for the same
// seed, which would double noise-layer allocations at startup.
func (s *ChunkedSource) Generator() *WorldGenerator { return s.gen }

// Compile-time assertion that ChunkedSource satisfies world.TileSource —
// any drift in the interface surfaces here at build time.
var _ world.TileSource = (*ChunkedSource)(nil)

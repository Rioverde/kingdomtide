package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// ChunkedSource is the procedural, infinite game.TileSource: a
// WorldGenerator plus an LRU chunk cache in front of it. Determined
// fully by seed.
type ChunkedSource struct {
	gen   *WorldGenerator
	cache *ChunkCache
}

// NewChunkedSource wires a fresh generator and cache around the given seed.
func NewChunkedSource(seed int64) *ChunkedSource {
	return &ChunkedSource{
		gen:   NewWorldGenerator(seed),
		cache: NewChunkCache(DefaultChunkCacheCapacity),
	}
}

// TileAt returns the procedurally-generated tile at (x, y), memoised at
// the chunk level so repeated reads inside the same chunk are cheap.
func (s *ChunkedSource) TileAt(x, y int) game.Tile {
	cc := WorldToChunk(x, y)
	if cached, ok := s.cache.Get(cc); ok {
		return cached.At(x, y)
	}
	c := s.gen.Chunk(cc)
	s.cache.Put(cc, &c)
	return c.At(x, y)
}

// Compile-time assertion that ChunkedSource satisfies game.TileSource —
// any drift in the interface surfaces here at build time.
var _ game.TileSource = (*ChunkedSource)(nil)

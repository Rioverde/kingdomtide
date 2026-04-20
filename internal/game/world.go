package game

// World is the façade the rest of the program talks to. It owns the infinite procedural
// pipeline — a WorldGenerator that produces tiles on demand and a ChunkCache that memoises
// recently visited chunks so repeated reads are cheap.
type World struct {
	gen   *WorldGenerator
	cache *ChunkCache
}

// NewWorld wires a deterministic infinite world: a generator seeded from seed and a bounded
// LRU cache sized for a reasonable camera buffer.
func NewWorld(seed int64) *World {
	return &World{
		gen:   NewWorldGenerator(seed),
		cache: NewChunkCache(DefaultChunkCacheCapacity),
	}
}

// Generator exposes the underlying WorldGenerator.
func (w *World) Generator() *WorldGenerator {
	return w.gen
}

// Cache exposes the chunk cache.
func (w *World) Cache() *ChunkCache {
	return w.cache
}

// TileAt returns the tile at global axial coord (q, r). It routes through the cache: a hit
// serves instantly, a miss generates the owning chunk, stores it, and reads the tile back.
func (w *World) TileAt(q, r int) Tile {
	cc := WorldToChunk(q, r)
	chunk := w.ChunkAt(cc)
	return chunk.At(q, r)
}

// ChunkAt returns the chunk that owns cc. Cache hit → returned directly. Cache miss → the
// generator fills a fresh chunk, stores it, and returns a pointer to the stored chunk.
// The returned *Chunk aliases cache storage; callers should treat it as read-only.
func (w *World) ChunkAt(cc ChunkCoord) *Chunk {
	if cached, ok := w.cache.Get(cc); ok {
		return cached
	}
	chunk := w.gen.Chunk(cc)
	w.cache.Put(cc, &chunk)
	return &chunk
}

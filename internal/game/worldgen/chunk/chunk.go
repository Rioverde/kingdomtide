package chunk

import "github.com/Rioverde/gongeons/internal/game/world"

// ChunkSize is the edge length of a chunk in tiles (grid units).
// Picked to mirror Minecraft's 16 — small enough to generate in a couple of
// milliseconds, large enough to amortize lookup overhead.
const ChunkSize = 16

// ChunkCoord identifies a chunk in chunk-space. Multiplying a ChunkCoord by
// ChunkSize yields the world-space position of the chunk's (0, 0) corner.
type ChunkCoord struct {
	X, Y int
}

// WorldToChunk returns the chunk that owns the tile at world coord (x, y).
// Floor division is required because Go's / truncates toward zero — a bare x/ChunkSize
// would place x=-1 into chunk 0 instead of chunk -1 and split negative coordinates
// across two chunks.
func WorldToChunk(x, y int) ChunkCoord {
	return ChunkCoord{
		X: floorDiv(x, ChunkSize),
		Y: floorDiv(y, ChunkSize),
	}
}

// Bounds returns the grid coord range [MinX, MaxX) × [MinY, MaxY) covered by c.
func (c ChunkCoord) Bounds() (minX, maxX, minY, maxY int) {
	minX = c.X * ChunkSize
	minY = c.Y * ChunkSize
	return minX, minX + ChunkSize, minY, minY + ChunkSize
}

// floorDiv returns the mathematical floor of a/b. Unlike Go's /, which truncates toward zero,
// this rounds toward negative infinity so negative inputs map into the expected chunk.
func floorDiv(a, b int) int {
	quot := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		quot--
	}
	return quot
}

// Chunk is a fixed-size square of tiles in grid space. A 2D array indexed as Tiles[dy][dx]
// is preferred over a map here: the chunk is always fully populated by the generator, so
// dense storage wins on both memory (contiguous 16x16 array) and iteration (tight loop
// with no hashing). dy is the outer index to match row-major iteration order when rendering.
type Chunk struct {
	Coord ChunkCoord
	Tiles [ChunkSize][ChunkSize]world.Tile
}

// At returns the tile at global grid coord (x, y). It panics if the coord does not belong
// to this chunk — callers should check with WorldToChunk first.
func (c *Chunk) At(x, y int) world.Tile {
	dx, dy := c.localOffset(x, y)
	return c.Tiles[dy][dx]
}

// Set writes a tile at global grid coord (x, y). Same panic rule as At.
func (c *Chunk) Set(x, y int, t world.Tile) {
	dx, dy := c.localOffset(x, y)
	c.Tiles[dy][dx] = t
}

// AtSafe returns the tile at global grid coord (x, y) and true if the coord belongs to
// this chunk. Unlike At, it returns ok=false instead of panicking, so HTTP handlers can
// accept user-supplied coords without crashing the server's recoverer middleware.
func (c *Chunk) AtSafe(x, y int) (world.Tile, bool) {
	minX, maxX, minY, maxY := c.Bounds()
	if x < minX || x >= maxX || y < minY || y >= maxY {
		return world.Tile{}, false
	}
	return c.Tiles[y-minY][x-minX], true
}

// localOffset converts a global (x, y) into the chunk-local (dx, dy) pair and panics on
// out-of-bounds — a panic here means a caller mis-routed a coord to the wrong chunk.
func (c *Chunk) localOffset(x, y int) (int, int) {
	minX, maxX, minY, maxY := c.Bounds()
	if x < minX || x >= maxX || y < minY || y >= maxY {
		panic("chunk: coord out of bounds")
	}
	return x - minX, y - minY
}

// Bounds is the inclusive-exclusive grid range covered by this chunk, matching
// ChunkCoord.Bounds. Provided on Chunk for callers that already hold a *Chunk and would
// otherwise need to dip back into the coord struct.
func (c *Chunk) Bounds() (minX, maxX, minY, maxY int) {
	return c.Coord.Bounds()
}

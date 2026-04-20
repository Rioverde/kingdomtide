package game

// ChunkSize is the edge length of a chunk in hex tiles (axial units).
// Picked to mirror Minecraft's 16 — small enough to generate in a couple of
// milliseconds, large enough to amortize lookup overhead.
const ChunkSize = 16

// ChunkCoord identifies a chunk in chunk-space. Multiplying a ChunkCoord by
// ChunkSize yields the world-space (axial) position of the chunk's (0, 0) corner.
type ChunkCoord struct {
	X, Y int
}

// WorldToChunk returns the chunk that owns the tile at world coord (q, r).
// Floor division is required because Go's / truncates toward zero — a bare q/ChunkSize
// would place q=-1 into chunk 0 instead of chunk -1 and split negative coordinates
// across two chunks.
func WorldToChunk(q, r int) ChunkCoord {
	return ChunkCoord{
		X: floorDiv(q, ChunkSize),
		Y: floorDiv(r, ChunkSize),
	}
}

// Bounds returns the axial coord range [MinQ, MaxQ) × [MinR, MaxR) covered by c.
func (c ChunkCoord) Bounds() (minQ, maxQ, minR, maxR int) {
	minQ = c.X * ChunkSize
	minR = c.Y * ChunkSize
	return minQ, minQ + ChunkSize, minR, minR + ChunkSize
}

// floorDiv returns the mathematical floor of a/b. Unlike Go's /, which truncates toward zero,
// this rounds toward negative infinity so negative inputs map into the expected chunk.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// Chunk is a fixed-size square of tiles in axial space. A 2D array indexed as Tiles[dr][dq]
// is preferred over a map here: the chunk is always fully populated by the generator, so
// dense storage wins on both memory (contiguous 16x16 array) and iteration (tight loop
// with no hashing). dr is the outer index to match row-major iteration order when rendering.
type Chunk struct {
	Coord ChunkCoord
	Tiles [ChunkSize][ChunkSize]Tile
}

// At returns the tile at global axial coord (q, r). It panics if the coord does not belong
// to this chunk — callers should check with WorldToChunk first.
func (c *Chunk) At(q, r int) Tile {
	dq, dr := c.localOffset(q, r)
	return c.Tiles[dr][dq]
}

// Set writes a tile at global axial coord (q, r). Same panic rule as At.
func (c *Chunk) Set(q, r int, t Tile) {
	dq, dr := c.localOffset(q, r)
	c.Tiles[dr][dq] = t
}

// localOffset converts a global (q, r) into the chunk-local (dq, dr) pair and panics on
// out-of-bounds — a panic here means a caller mis-routed a coord to the wrong chunk.
func (c *Chunk) localOffset(q, r int) (int, int) {
	minQ, maxQ, minR, maxR := c.Bounds()
	if q < minQ || q >= maxQ || r < minR || r >= maxR {
		panic("chunk: coord out of bounds")
	}
	return q - minQ, r - minR
}

// Bounds is the inclusive-exclusive axial range covered by this chunk, matching
// ChunkCoord.Bounds. Provided on Chunk for callers that already hold a *Chunk and would
// otherwise need to dip back into the coord struct.
func (c *Chunk) Bounds() (minQ, maxQ, minR, maxR int) {
	return c.Coord.Bounds()
}


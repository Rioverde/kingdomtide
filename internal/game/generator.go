package game

// Seed salts for per-layer noise decorrelation. Independent layers must see different
// underlying noise fields, otherwise elevation and temperature would look visually
// correlated across the map. XOR-ing the user seed with a fixed salt is cheap and keeps
// determinism — two runs with the same base seed produce the same per-layer seeds.
// The constants are the fractional digits of π and e in hex (Knuth-style nothing-up-my-sleeve
// numbers), so small user seeds like 0, 1, 2 cannot accidentally cancel the salt.
const (
	seedSaltTemperature int64 = 0x243f6a8885a308d3
	seedSaltMoisture    int64 = 0x13198a2e03707344
)

// temperatureOpts is a lower-frequency two-octave fBm field. Temperature varies over large
// distances (continents worth of terrain) so a bigger scale and fewer octaves feel right.
var temperatureOpts = OctaveOpts{
	Octaves:     2,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       80.0,
}

// moistureOpts adds a touch more detail than temperature — rain shadows feel local — but
// still coarser than elevation.
var moistureOpts = OctaveOpts{
	Octaves:     3,
	Lacunarity:  2.0,
	Persistence: 0.5,
	Scale:       64.0,
}

// WorldGenerator is the deterministic pure function layer: given a (q, r) coordinate (or a
// whole chunk coord) it returns the tile that would live there for this seed. It owns three
// independent noise fields — elevation, temperature, moisture — sampled on global world
// coordinates so that chunk borders stitch seamlessly.
type WorldGenerator struct {
	seed        int64
	elevation   OctaveNoise
	temperature OctaveNoise
	moisture    OctaveNoise
}

// NewWorldGenerator builds the three noise fields off the supplied base seed. The per-layer
// seeds are derived by XOR-ing with fixed salts so callers that care about determinism only
// need to remember one number.
func NewWorldGenerator(seed int64) *WorldGenerator {
	return &WorldGenerator{
		seed:        seed,
		elevation:   NewOctaveNoise(seed, DefaultOctaveOpts),
		temperature: NewOctaveNoise(seed^seedSaltTemperature, temperatureOpts),
		moisture:    NewOctaveNoise(seed^seedSaltMoisture, moistureOpts),
	}
}

// Seed returns the base seed used to construct the generator. Useful for the JSON meta API
// and for reproducing a world elsewhere.
func (g *WorldGenerator) Seed() int64 {
	return g.seed
}

// TileAt is the canonical per-coord lookup. It samples the three noise fields at the given
// global axial coordinate and hands them to the biome matrix. Same (seed, q, r) always
// yields the same tile, with or without the chunk cache in front.
func (g *WorldGenerator) TileAt(q, r int) Tile {
	x, y := float64(q), float64(r)
	elev := g.elevation.Eval2Normalized(x, y)
	temp := g.temperature.Eval2Normalized(x, y)
	moist := g.moisture.Eval2Normalized(x, y)
	return Tile{Terrain: Biome(elev, temp, moist)}
}

// Chunk fills an entire Chunk worth of tiles by calling TileAt for every coord in the chunk
// bounds. After biome assignment it overlays river data: any tile whose axial coord appears
// in RiverTilesInChunk has its River flag set to true. Biome is intentionally not altered
// here — river-adjacent biome blending is a later tuning pass.
func (g *WorldGenerator) Chunk(cc ChunkCoord) Chunk {
	chunk := Chunk{Coord: cc}
	minQ, _, minR, _ := cc.Bounds()
	for dr := 0; dr < ChunkSize; dr++ {
		for dq := 0; dq < ChunkSize; dq++ {
			chunk.Tiles[dr][dq] = g.TileAt(minQ+dq, minR+dr)
		}
	}

	riverTiles := g.RiverTilesInChunk(cc)
	for dr := 0; dr < ChunkSize; dr++ {
		for dq := 0; dq < ChunkSize; dq++ {
			if _, ok := riverTiles[[2]int{minQ + dq, minR + dr}]; ok {
				chunk.Tiles[dr][dq].River = true
			}
		}
	}

	// Overlay point-of-interest objects on top of the biome + river layer. A POI on a
	// river tile is intentional — the river flag stays true alongside the object.
	objects := g.ObjectsInChunk(cc)
	for key, kind := range objects {
		chunk.Tiles[key[1]][key[0]].Object = kind
	}

	return chunk
}

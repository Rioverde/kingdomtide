package worldgen

import "github.com/Rioverde/gongeons/internal/game"

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

// WorldGenerator is the deterministic pure function layer: given an (x, y) coordinate (or a
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
// global grid coordinate and hands them to the biome matrix. Same (seed, x, y) always
// yields the same tile, with or without the chunk cache in front.
func (g *WorldGenerator) TileAt(x, y int) game.Tile {
	fx, fy := float64(x), float64(y)
	elev := g.elevation.Eval2Normalized(fx, fy)
	temp := g.temperature.Eval2Normalized(fx, fy)
	moist := g.moisture.Eval2Normalized(fx, fy)
	return game.Tile{Terrain: Biome(elev, temp, moist)}
}

// Chunk fills an entire Chunk worth of tiles by calling TileAt for every coord in the chunk
// bounds. After biome assignment it overlays two layers in a single pass: any tile whose
// grid coord appears in RiverTilesInChunk has its River flag set, and any tile matching a
// POI entry from ObjectsInChunk gets its Object field populated. Biome is intentionally not
// altered here — river-adjacent biome blending is a later tuning pass. A POI on a river
// tile is intentional — the river flag stays true alongside the object.
func (g *WorldGenerator) Chunk(cc ChunkCoord) Chunk {
	chunk := Chunk{Coord: cc}
	minX, _, minY, _ := cc.Bounds()
	for dy := range ChunkSize {
		for dx := range ChunkSize {
			chunk.Tiles[dy][dx] = g.TileAt(minX+dx, minY+dy)
		}
	}

	// Overlay rivers and POIs in a single pass. River keys are global grid coords;
	// POI keys are chunk-local (dx, dy) offsets — preserve both contracts.
	riverTiles := g.RiverTilesInChunk(cc)
	objects := g.ObjectsInChunk(cc)
	for dy := range ChunkSize {
		for dx := range ChunkSize {
			if _, wet := riverTiles[[2]int{minX + dx, minY + dy}]; wet {
				chunk.Tiles[dy][dx].River = true
			}
			if obj, ok := objects[[2]int{dx, dy}]; ok {
				chunk.Tiles[dy][dx].Object = obj
			}
		}
	}

	return chunk
}

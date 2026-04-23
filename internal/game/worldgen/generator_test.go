package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/worldgen/chunk"
)

func TestWorldGeneratorDeterministic(t *testing.T) {
	a := NewWorldGenerator(42)
	b := NewWorldGenerator(42)

	coords := []struct{ x, y int }{
		{0, 0},
		{7, -3},
		{128, 256},
		{-1000, 999},
	}
	for _, c := range coords {
		ta := a.TileAt(c.x, c.y)
		tb := b.TileAt(c.x, c.y)
		if ta != tb {
			t.Errorf("TileAt(%d, %d) not deterministic: %+v vs %+v", c.x, c.y, ta, tb)
		}
	}
}

func TestWorldGeneratorDifferentSeedsDiffer(t *testing.T) {
	a := NewWorldGenerator(1)
	b := NewWorldGenerator(987654321)

	// Two different seeds should disagree on at least one of a reasonably-sized sample.
	// Biomes quantise continuous noise so many tiles may coincide by chance; a larger
	// block makes the coincidence vanishingly unlikely for a correctly seeded generator.
	sameEverywhere := true
	for x := -20; x <= 20 && sameEverywhere; x += 4 {
		for y := -20; y <= 20 && sameEverywhere; y += 4 {
			if a.TileAt(x, y) != b.TileAt(x, y) {
				sameEverywhere = false
			}
		}
	}
	if sameEverywhere {
		t.Fatal("distinct seeds produced identical tiles across the sample; noise not seeded")
	}
}

func TestGeneratorChunkMatchesTileAt(t *testing.T) {
	g := NewWorldGenerator(123456)
	cc := chunk.ChunkCoord{X: 4, Y: -7}
	c := g.Chunk(cc)

	if c.Coord != cc {
		t.Fatalf("chunk.Coord = %+v, want %+v", c.Coord, cc)
	}

	minX, _, minY, _ := cc.Bounds()
	for dy := range chunk.ChunkSize {
		for dx := range chunk.ChunkSize {
			x, y := minX+dx, minY+dy
			// TileAt returns the raw biome tile without overlays. Chunk()
			// adds river and lake overlays, so only the Terrain field must agree.
			want := g.TileAt(x, y)
			got := c.Tiles[dy][dx]
			if got.Terrain != want.Terrain {
				t.Fatalf("chunk.Tiles[%d][%d].Terrain = %q, TileAt(%d,%d).Terrain = %q",
					dy, dx, got.Terrain, x, y, want.Terrain)
			}
		}
	}
}

func TestGeneratorChunkAllTilesPopulated(t *testing.T) {
	g := NewWorldGenerator(7)
	c := g.Chunk(chunk.ChunkCoord{X: 0, Y: 0})

	count := 0
	for dy := range chunk.ChunkSize {
		for dx := range chunk.ChunkSize {
			if c.Tiles[dy][dx].Terrain == "" {
				t.Fatalf("chunk tile at [%d][%d] has empty terrain", dy, dx)
			}
			count++
		}
	}
	if want := chunk.ChunkSize * chunk.ChunkSize; count != want {
		t.Fatalf("populated %d tiles, want %d", count, want)
	}
}

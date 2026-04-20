package game

import "testing"

func TestWorldGeneratorDeterministic(t *testing.T) {
	a := NewWorldGenerator(42)
	b := NewWorldGenerator(42)

	coords := []struct{ q, r int }{
		{0, 0},
		{7, -3},
		{128, 256},
		{-1000, 999},
	}
	for _, c := range coords {
		ta := a.TileAt(c.q, c.r)
		tb := b.TileAt(c.q, c.r)
		if ta != tb {
			t.Errorf("TileAt(%d, %d) not deterministic: %+v vs %+v", c.q, c.r, ta, tb)
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
	for q := -20; q <= 20 && sameEverywhere; q += 4 {
		for r := -20; r <= 20 && sameEverywhere; r += 4 {
			if a.TileAt(q, r) != b.TileAt(q, r) {
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
	cc := ChunkCoord{X: 4, Y: -7}
	chunk := g.Chunk(cc)

	if chunk.Coord != cc {
		t.Fatalf("chunk.Coord = %+v, want %+v", chunk.Coord, cc)
	}

	minQ, _, minR, _ := cc.Bounds()
	for dr := 0; dr < ChunkSize; dr++ {
		for dq := 0; dq < ChunkSize; dq++ {
			q, r := minQ+dq, minR+dr
			// TileAt returns the raw biome tile without river or POI overlays. Chunk()
			// enriches tiles with both layers, so only the Terrain field must agree.
			want := g.TileAt(q, r)
			got := chunk.Tiles[dr][dq]
			if got.Terrain != want.Terrain {
				t.Fatalf("chunk.Tiles[%d][%d].Terrain = %q, TileAt(%d,%d).Terrain = %q",
					dr, dq, got.Terrain, q, r, want.Terrain)
			}
		}
	}
}

func TestGeneratorChunkAllTilesPopulated(t *testing.T) {
	g := NewWorldGenerator(7)
	chunk := g.Chunk(ChunkCoord{X: 0, Y: 0})

	count := 0
	for dr := 0; dr < ChunkSize; dr++ {
		for dq := 0; dq < ChunkSize; dq++ {
			if chunk.Tiles[dr][dq].Terrain == "" {
				t.Fatalf("chunk tile at [%d][%d] has empty terrain", dr, dq)
			}
			count++
		}
	}
	if want := ChunkSize * ChunkSize; count != want {
		t.Fatalf("populated %d tiles, want %d", count, want)
	}
}

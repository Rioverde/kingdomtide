package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestObjectsInChunkDeterministic verifies that calling ObjectsInChunk twice with the same
// seed and chunk coordinate produces identical results.
func TestObjectsInChunkDeterministic(t *testing.T) {
	g := NewWorldGenerator(12345)
	cc := ChunkCoord{X: 3, Y: -2}

	a := g.ObjectsInChunk(cc)
	b := g.ObjectsInChunk(cc)

	if len(a) != len(b) {
		t.Fatalf("non-deterministic: first call returned %d POIs, second returned %d", len(a), len(b))
	}
	for key, kindA := range a {
		kindB, ok := b[key]
		if !ok {
			t.Errorf("key %v present in first call but missing in second", key)
			continue
		}
		if kindA != kindB {
			t.Errorf("key %v: first call=%q second call=%q", key, kindA, kindB)
		}
	}
}

// TestPOIMinDistance collects all POIs from a 5x5 grid of chunks and checks that no two
// POIs are closer than poiMinDistance tiles apart.
func TestPOIMinDistance(t *testing.T) {
	g := NewWorldGenerator(99999)

	type worldPOI struct{ x, y int }
	var all []worldPOI

	for cy := -2; cy <= 2; cy++ {
		for cx := -2; cx <= 2; cx++ {
			cc := ChunkCoord{X: cx, Y: cy}
			minX, _, minY, _ := cc.Bounds()
			for key := range g.ObjectsInChunk(cc) {
				all = append(all, worldPOI{
					x: minX + key[0],
					y: minY + key[1],
				})
			}
		}
	}

	for i := range all {
		for j := i + 1; j < len(all); j++ {
			d := hexDistance(all[i].x, all[i].y, all[j].x, all[j].y)
			if d < poiMinDistance {
				t.Errorf("POI at (%d,%d) and (%d,%d) are only %d apart (min %d)",
					all[i].x, all[i].y, all[j].x, all[j].y, d, poiMinDistance)
			}
		}
	}
}

// TestPOIRespectsBiome checks that no village appears on ocean tiles and no castle
// appears on snowy-peak tiles, across a broad sample of chunks.
func TestPOIRespectsBiome(t *testing.T) {
	g := NewWorldGenerator(777)

	for cy := -5; cy <= 5; cy++ {
		for cx := -5; cx <= 5; cx++ {
			cc := ChunkCoord{X: cx, Y: cy}
			minX, _, minY, _ := cc.Bounds()
			for key, kind := range g.ObjectsInChunk(cc) {
				wx := minX + key[0]
				wy := minY + key[1]
				tile := g.TileAt(wx, wy)

				switch kind {
				case game.ObjectVillage:
					if tile.Terrain == game.TerrainOcean || tile.Terrain == game.TerrainDeepOcean {
						t.Errorf("village at (%d,%d) on water terrain %q", wx, wy, tile.Terrain)
					}
				case game.ObjectCastle:
					if tile.Terrain == game.TerrainSnowyPeak {
						t.Errorf("castle at (%d,%d) on snowy_peak terrain", wx, wy)
					}
					if tile.Terrain == game.TerrainDeepOcean || tile.Terrain == game.TerrainOcean {
						t.Errorf("castle at (%d,%d) on water terrain %q", wx, wy, tile.Terrain)
					}
				}
			}
		}
	}
}

// TestChunkContainsSomePOIs is a loose sanity check: with enough chunks around the origin
// at least one should have a POI, given that spawn chances are non-trivial.
func TestChunkContainsSomePOIs(t *testing.T) {
	g := NewWorldGenerator(42)
	total := 0

	for cy := -5; cy <= 5; cy++ {
		for cx := -5; cx <= 5; cx++ {
			total += len(g.ObjectsInChunk(ChunkCoord{X: cx, Y: cy}))
		}
	}

	if total == 0 {
		t.Error("expected at least one POI across a 10x10 chunk neighbourhood but got none")
	}
}

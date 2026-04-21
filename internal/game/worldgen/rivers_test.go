package worldgen

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// TestRiverTilesInChunkDeterministic calls RiverTilesInChunk twice for the same
// chunk on the same generator and asserts the returned sets are identical.
// Determinism here is what lets separate HTTP requests for the same chunk
// produce the same river layout.
func TestRiverTilesInChunkDeterministic(t *testing.T) {
	g := NewWorldGenerator(7)
	cc := ChunkCoord{X: 3, Y: -2}

	s1 := g.RiverTilesInChunk(cc)
	s2 := g.RiverTilesInChunk(cc)

	if !reflect.DeepEqual(s1, s2) {
		t.Fatal("RiverTilesInChunk returned different sets on two calls for the same chunk")
	}
}

// TestChunkHasRivers is a sanity floor: across a sweep of seeds some chunk in a
// 21×21 window must contain at least one river tile. Catches a regression
// where the accum threshold, moisture gate, or mountain-source propagation
// accidentally suppresses all rivers.
func TestChunkHasRivers(t *testing.T) {
	const seedCount = 20

	for s := int64(1); s <= seedCount; s++ {
		g := NewWorldGenerator(s)
		for cx := -10; cx <= 10; cx++ {
			for cy := -10; cy <= 10; cy++ {
				c := g.Chunk(ChunkCoord{X: cx, Y: cy})
				for dy := range ChunkSize {
					for dx := range ChunkSize {
						if c.Tiles[dy][dx].Overlays.Has(game.OverlayRiver) {
							return
						}
					}
				}
			}
		}
	}

	t.Fatal("no river tiles found across 20 seeds × 21×21 chunk windows — river generation likely broken")
}

// TestRiverTilesFlowToSink asserts the "rivers end in ocean or lake" property:
// every river tile walked along D8 steepest descent terminates at an ocean
// cell, a lake cell, or the buffer boundary within hydrologyBufferSide steps.
// The buffer-boundary termination is acceptable because a river crossing out
// of the buffer is another chunk's responsibility to render (its own buffer
// covers that cell).
func TestRiverTilesFlowToSink(t *testing.T) {
	g := NewWorldGenerator(42)

	for cx := -4; cx <= 4; cx++ {
		for cy := -4; cy <= 4; cy++ {
			cc := ChunkCoord{X: cx, Y: cy}
			f := g.hydrologyFor(cc)
			for y := range hydrologyBufferSide {
				for x := range hydrologyBufferSide {
					if !f.river[y][x] {
						continue
					}
					if !walkReachesSink(f, x, y) {
						t.Errorf("chunk (%d,%d) buf (%d,%d): river tile never reached ocean/lake/boundary",
							cx, cy, x, y)
					}
				}
			}
		}
	}
}

// walkReachesSink follows steepest descent from (x, y) on f for at most
// hydrologyBufferSide steps and returns true if the walk ends at an ocean
// cell, a lake cell, or the buffer boundary (dx=dy=0 from bestDownhill).
func walkReachesSink(f *hydrologyField, x, y int) bool {
	for range hydrologyBufferSide {
		if f.fillElev[y][x] < elevationOcean {
			return true
		}
		if f.wasRaised[y][x] {
			return true
		}
		dx, dy := f.bestDownhill(x, y)
		if dx == 0 && dy == 0 {
			return true
		}
		x += int(dx)
		y += int(dy)
	}
	return false
}

// TestNoRiverWithoutMountainSource builds a synthetic field with zero mountain
// sources and verifies no cell is classified as a river — even where
// accumulation would otherwise pass the threshold. This pins the headline
// rule: rivers must originate in mountains.
func TestNoRiverWithoutMountainSource(t *testing.T) {
	var raw [hydrologyBufferSide][hydrologyBufferSide]float64
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			raw[y][x] = 0.55 - float64(x)*0.0005
		}
	}

	var zeroSource [hydrologyBufferSide][hydrologyBufferSide]bool
	f := buildSyntheticField(&raw, &zeroSource)

	highAccum := false
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			if f.accum[y][x] >= riverAccumThreshold {
				highAccum = true
			}
			if f.river[y][x] {
				t.Errorf("river tile at (%d,%d) despite zero mountain-sourced cells", x, y)
			}
		}
	}
	if !highAccum {
		t.Fatal("test preconditions broken: no cell accumulated past threshold, so the "+
			"no-rivers assertion is vacuous — adjust the ramp gradient")
	}
}

// TestRiverRequiresSourceToProduceRiver is the complement to
// TestNoRiverWithoutMountainSource: the same topology with at least one
// mountain-sourced cell upstream MUST produce river tiles where accum passes
// the threshold. Together the two tests pin the source gate as necessary and
// (given accum) sufficient.
func TestRiverRequiresSourceToProduceRiver(t *testing.T) {
	var raw [hydrologyBufferSide][hydrologyBufferSide]float64
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			raw[y][x] = 0.55 - float64(x)*0.0005
		}
	}

	var source [hydrologyBufferSide][hydrologyBufferSide]bool
	source[hydrologyBufferSide/2][1] = true

	f := buildSyntheticField(&raw, &source)

	found := false
	for y := range hydrologyBufferSide {
		for x := range hydrologyBufferSide {
			if f.river[y][x] {
				found = true
			}
		}
	}
	if !found {
		t.Error("no river tiles despite a mountain-sourced cell upstream of a descending ramp")
	}
}

// TestRiverDoesNotOverpaintLake asserts rivers and lakes are mutually exclusive
// on the overlay layer — a lake cell is never also marked as river.
func TestRiverDoesNotOverpaintLake(t *testing.T) {
	g := NewWorldGenerator(42)
	for cx := -4; cx <= 4; cx++ {
		for cy := -4; cy <= 4; cy++ {
			cc := ChunkCoord{X: cx, Y: cy}
			f := g.hydrologyFor(cc)
			for y := range hydrologyBufferSide {
				for x := range hydrologyBufferSide {
					if f.river[y][x] && f.wasRaised[y][x] {
						t.Errorf("chunk (%d,%d) buf (%d,%d): cell marked both river and lake",
							cx, cy, x, y)
					}
				}
			}
		}
	}
}

// TestRiverDensityRealistic calibrates the accum threshold + source gate: over
// 8 seeds × 16×16 chunks the fraction of river tiles among land tiles must
// sit in a realistic band. The ceiling catches an over-wet map (every creek
// is a river); the floor catches a gate that suppresses all rivers.
func TestRiverDensityRealistic(t *testing.T) {
	var totalLand, totalRiver int

	for s := int64(1); s <= 8; s++ {
		g := NewWorldGenerator(s)
		for cx := -8; cx < 8; cx++ {
			for cy := -8; cy < 8; cy++ {
				c := g.Chunk(ChunkCoord{X: cx, Y: cy})
				for dy := range ChunkSize {
					for dx := range ChunkSize {
						tile := c.Tiles[dy][dx]
						if isLandTerrain(tile.Terrain) {
							totalLand++
							if tile.Overlays.Has(game.OverlayRiver) {
								totalRiver++
							}
						}
					}
				}
			}
		}
	}

	if totalLand == 0 {
		t.Fatal("no land tiles sampled")
	}
	density := float64(totalRiver) / float64(totalLand)
	t.Logf("river density = %.4f (%d rivers / %d land)", density, totalRiver, totalLand)
	if density < 0.001 {
		t.Errorf("river density %.4f below 0.1%% floor — threshold too strict or source gate over-blocks",
			density)
	}
	if density > 0.04 {
		t.Errorf("river density %.4f above 4%% ceiling — threshold too loose", density)
	}
}

// isLandTerrain reports whether a terrain is on land (not ocean).
func isLandTerrain(t game.Terrain) bool {
	return t != game.TerrainDeepOcean && t != game.TerrainOcean
}

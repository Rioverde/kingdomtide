package worldgen

import (
	"strings"
	"testing"
	"time"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/voronoi"
)

// TestVisualGen generates a Standard world and dumps an ASCII
// rendering plus per-pass statistics. Not a unit test — a hands-on
// diagnostic to eyeball pipeline output without launching the
// explorer TUI. Standard size used because Tiny only has ~200 cells
// and ~9 river heads, which is statistically too small to judge
// distributions.
func TestVisualGen(t *testing.T) {
	if testing.Short() {
		t.Skip("standard-world generation: too slow for -short")
	}
	const (
		seed         int64 = 42
		stepX, stepY       = 32, 32 // 2560/32 = 80 cols, 1024/32 = 32 rows
	)
	type stage struct {
		name string
		dur  time.Duration
	}
	var stages []stage
	GenStageHook = func(name string, d time.Duration) {
		stages = append(stages, stage{name, d})
	}
	voronoi.SubStageHook = func(name string, d time.Duration) {
		stages = append(stages, stage{"  voronoi/" + name, d})
	}
	defer func() {
		GenStageHook = nil
		voronoi.SubStageHook = nil
	}()

	t0 := time.Now()
	w := Generate(seed, WorldSizeStandard)
	genDur := time.Since(t0)

	t.Logf("\n=== STATS (seed=%d, size=%s %dx%d, gen=%v) ===",
		seed, w.Size.Label(), w.Width, w.Height, genDur.Round(time.Millisecond))
	for _, s := range stages {
		t.Logf("  %-22s %v", s.name, s.dur.Round(time.Microsecond))
	}
	dumpStats(t, w)

	t.Logf("\n=== BIOME (sampled stepX=%d stepY=%d) ===", stepX, stepY)
	dumpAscii(t, w, stepX, stepY, false)

	t.Logf("\n=== LAND/OCEAN + RIVERS ===")
	dumpAscii(t, w, stepX, stepY, true)
}

func dumpStats(t *testing.T, w *World) {
	cells := len(w.Voronoi.Cells)
	var ocean, land, lakes int
	for i := range cells {
		switch w.Terrain[i] {
		case gworld.TerrainOcean, gworld.TerrainDeepOcean:
			ocean++
		default:
			land++
		}
	}

	// Cells classified water but not surfaced as ocean — those are
	// our lake cells (interior water + river-blocked lakes).
	for i := range cells {
		if w.Terrain[i] == gworld.TerrainOcean {
			// May be lake (interior) or shallow ocean. Differentiate
			// by checking if any neighbour is deep ocean.
			isShoreOcean := false
			for _, n := range w.Voronoi.Cells[i].Neighbors {
				if w.Terrain[n] == gworld.TerrainDeepOcean {
					isShoreOcean = true
					break
				}
			}
			if !isShoreOcean {
				lakes++
			}
		}
	}

	var landElevSum, landMoistSum, landTempSum float32
	var landElevMin, landElevMax float32 = 1, 0
	var landMoistMin, landMoistMax float32 = 1, 0
	var landTempMin, landTempMax float32 = 1, 0
	var landN int
	for i := range cells {
		if w.Terrain[i] == gworld.TerrainOcean || w.Terrain[i] == gworld.TerrainDeepOcean {
			continue
		}
		landN++
		e, m, tt := w.Elevation[i], w.Moisture[i], w.Temperature[i]
		landElevSum += e
		landMoistSum += m
		landTempSum += tt
		if e < landElevMin {
			landElevMin = e
		}
		if e > landElevMax {
			landElevMax = e
		}
		if m < landMoistMin {
			landMoistMin = m
		}
		if m > landMoistMax {
			landMoistMax = m
		}
		if tt < landTempMin {
			landTempMin = tt
		}
		if tt > landTempMax {
			landTempMax = tt
		}
	}

	// Watershed basin count.
	basins := make(map[int32]int)
	for i := range cells {
		if w.Watershed[i] >= 0 {
			basins[w.Watershed[i]]++
		}
	}

	// River tile count.
	var rivers int
	if w.riverBits != nil {
		rivers = w.riverBits.Count()
	}

	t.Logf("cells: %d   land: %d (%.1f%%)   ocean+lake: %d (%.1f%%)   lakes (≈): %d",
		cells, land, 100*float32(land)/float32(cells),
		ocean, 100*float32(ocean)/float32(cells),
		lakes,
	)
	if landN > 0 {
		fN := float32(landN)
		t.Logf("LAND elev:  [%.2f .. %.2f]  mean %.2f", landElevMin, landElevMax, landElevSum/fN)
		t.Logf("LAND moist: [%.2f .. %.2f]  mean %.2f", landMoistMin, landMoistMax, landMoistSum/fN)
		t.Logf("LAND temp:  [%.2f .. %.2f]  mean %.2f", landTempMin, landTempMax, landTempSum/fN)
	}
	t.Logf("watersheds: %d distinct basins", len(basins))
	t.Logf("river tiles: %d (%.2f%% of map)", rivers,
		100*float32(rivers)/float32(w.Width*w.Height))
}

// terrainGlyph picks a one-char ASCII for each terrain so the dump
// is readable in plain logs (no colours).
func terrainGlyph(t gworld.Terrain) byte {
	switch t {
	case gworld.TerrainDeepOcean:
		return '~'
	case gworld.TerrainOcean:
		return '-'
	case gworld.TerrainBeach:
		return '.'
	case gworld.TerrainDesert:
		return ':'
	case gworld.TerrainSavanna:
		return ';'
	case gworld.TerrainPlains:
		return ','
	case gworld.TerrainGrassland:
		return '"'
	case gworld.TerrainMeadow:
		return '*'
	case gworld.TerrainForest:
		return 'F'
	case gworld.TerrainJungle:
		return 'J'
	case gworld.TerrainTaiga:
		return 'T'
	case gworld.TerrainTundra:
		return 'u'
	case gworld.TerrainSnow:
		return 'S'
	case gworld.TerrainHills:
		return 'h'
	case gworld.TerrainMountain:
		return 'M'
	case gworld.TerrainSnowyPeak:
		return '^'
	}
	return '?'
}

func dumpAscii(t *testing.T, w *World, stepX, stepY int, landOcean bool) {
	var sb strings.Builder
	for y := 0; y < w.Height; y += stepY {
		for x := 0; x < w.Width; x += stepX {
			cellID := w.Voronoi.CellIDAt(x, y)
			// Scan the whole sample block so a single-tile river still
			// shows up at this resolution.
			hasRiver := false
			for dy := 0; dy < stepY && !hasRiver; dy++ {
				for dx := 0; dx < stepX && !hasRiver; dx++ {
					if w.IsRiver(x+dx, y+dy) {
						hasRiver = true
					}
				}
			}
			if landOcean {
				if w.IsOcean(cellID) {
					sb.WriteByte('~')
				} else if hasRiver {
					sb.WriteByte('=')
				} else if w.IsCoast(cellID) {
					sb.WriteByte('+')
				} else {
					sb.WriteByte('#')
				}
			} else {
				if hasRiver && !w.IsOcean(cellID) {
					sb.WriteByte('=')
				} else {
					sb.WriteByte(terrainGlyph(w.Terrain[cellID]))
				}
			}
		}
		sb.WriteByte('\n')
	}
	t.Log("\n" + sb.String())
}


package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// viewportBuf is a single shared builder reused across frames.
// Bubbletea is single-threaded so no synchronisation is needed; this
// saves an ~80KB Builder allocation + free per frame, the largest
// per-frame allocation in the explorer.
var viewportBuf strings.Builder

// renderViewport draws the viewport — one cell per world tile
// (mapped through cell-ID) for the currently-selected layer.
func renderViewport(w *worldgen.World, l layer, zoom, vpX, vpY, cols, rows int) string {
	viewportBuf.Reset()
	viewportBuf.Grow(cols * rows * 16)
	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			wx := vpX + rx*zoom
			wy := vpY + ry*zoom
			viewportBuf.WriteString(renderCell(w, l, wx, wy))
		}
		if ry < rows-1 {
			viewportBuf.WriteByte('\n')
		}
	}
	return viewportBuf.String()
}

// renderCell returns a precomputed cell string for the given layer.
// Every styled string is built once at init() — no lipgloss.Render
// calls happen per frame. With an 80×30 viewport at 60 fps that
// saves ~144K Render() calls/sec, which was the dev-tool's
// bottleneck.
func renderCell(w *worldgen.World, l layer, wx, wy int) string {
	if wx < 0 || wy < 0 || wx >= w.Width || wy >= w.Height {
		return oobCell
	}
	cellID := w.Voronoi.CellIDAt(wx, wy)
	switch l {
	case layerBiome:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		return terrainCell[w.Terrain[cellID]]
	case layerCells:
		return cellsPaletteCell[int(cellID)%len(cellsPaletteCell)]
	case layerLand:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return landOceanCell
		}
		return landLandCell
	case layerElevation:
		return scalarCell[scalarBucket(w.Elevation[cellID])]
	case layerMoisture:
		return scalarCell[scalarBucket(w.Moisture[cellID])]
	case layerCoast:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return coastOceanCell
		}
		if w.IsCoast(cellID) {
			return coastCoastCell
		}
		return coastInlandCell
	case layerWatershed:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return watershedOceanCell
		}
		ws := w.Watershed[cellID]
		if ws < 0 {
			return watershedEndorheicCell
		}
		return watershedCell[int(ws)%len(watershedCell)]
	}
	return "  "
}

// === Caches built once at init() ============================

var (
	oobStyle = lipgloss.NewStyle().Background(lipgloss.Color("234"))
	oobCell  string

	// riverCell — bold ocean style with the ocean glyph, used as the
	// river overlay across all biome-revealing layers.
	riverCell string

	// terrainCell[t] — fully-styled "glyph " string for terrain t.
	// world.Terrain is a string-typed enum, so this is a map; the
	// per-frame lookup is still cheap (sub-100ns) compared to the
	// alternative styling chain it replaces.
	terrainCell = map[gworld.Terrain]string{}

	// cellsPaletteCell — 15-entry debug palette for the cells layer.
	cellsPaletteCell = make([]string, 15)

	// landLayer — binary land/ocean.
	landOceanCell string
	landLandCell  string

	// coastLayer — ocean / coast / inland.
	coastOceanCell  string
	coastCoastCell  string
	coastInlandCell string

	// scalarCell — quantised gradient (32 buckets) for elevation /
	// moisture layers. Continuous values get bucketed via
	// scalarBucket() so we precompute styling once per bucket.
	scalarCell [scalarBuckets]string

	// watershedCell — same hand-picked palette as before, but
	// pre-rendered.
	watershedCell         []string
	watershedOceanCell    string
	watershedEndorheicCell string
)

const scalarBuckets = 32

// watershedPalette stays as terrain-typed list; we render each entry
// once at init.
var watershedPalette = []gworld.Terrain{
	gworld.TerrainGrassland,
	gworld.TerrainDesert,
	gworld.TerrainTaiga,
	gworld.TerrainSavanna,
	gworld.TerrainJungle,
	gworld.TerrainBeach,
	gworld.TerrainHills,
	gworld.TerrainMeadow,
	gworld.TerrainTundra,
	gworld.TerrainForest,
}

func init() {
	oobCell = oobStyle.Render("  ")

	oceanStyle := tilestyle.StyleFor(gworld.TerrainOcean).Bold(true)
	riverCell = oceanStyle.Render(tilestyle.GlyphFor(gworld.TerrainOcean) + " ")

	for terrain, glyph := range tilestyle.TerrainRunes {
		terrainCell[terrain] = tilestyle.StyleFor(terrain).Render(glyph + " ")
	}

	cellsPalette := []string{
		"#a06040", "#507040", "#603060", "#406080", "#805030",
		"#304860", "#708030", "#503070", "#604020", "#204870",
		"#708040", "#a08040", "#4060a0", "#30a060", "#906020",
	}
	for i, hex := range cellsPalette {
		cellsPaletteCell[i] = lipgloss.NewStyle().
			Background(lipgloss.Color(hex)).Render("  ")
	}

	landOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	landLandCell = tilestyle.StyleFor(gworld.TerrainGrassland).Reverse(true).Render("  ")

	coastOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	coastCoastCell = tilestyle.StyleFor(gworld.TerrainBeach).Reverse(true).Render("  ")
	coastInlandCell = tilestyle.StyleFor(gworld.TerrainForest).Reverse(true).Render("  ")

	for i := 0; i < scalarBuckets; i++ {
		v := float64(i) / float64(scalarBuckets-1)
		r, g, b := lerpRGB(0x23, 0x23, 0x23, 0xe0, 0xe0, 0xe0, v)
		bg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
		scalarCell[i] = lipgloss.NewStyle().Background(bg).Render("  ")
	}

	watershedCell = make([]string, len(watershedPalette))
	for i, t := range watershedPalette {
		watershedCell[i] = tilestyle.StyleFor(t).Reverse(true).Render("  ")
	}
	watershedOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	watershedEndorheicCell = oobStyle.Render("  ")
}

func scalarBucket(v float32) int {
	if v < 0 {
		return 0
	}
	if v > 1 {
		v = 1
	}
	idx := int(v * float32(scalarBuckets-1))
	if idx >= scalarBuckets {
		return scalarBuckets - 1
	}
	return idx
}

func lerpRGB(r0, g0, b0, r1, g1, b1 int, t float64) (int, int, int) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return int(float64(r0) + (float64(r1)-float64(r0))*t),
		int(float64(g0) + (float64(g1)-float64(g0))*t),
		int(float64(b0) + (float64(b1)-float64(b0))*t)
}

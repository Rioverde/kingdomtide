package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// renderViewport draws the viewport — one cell per world tile
// (mapped through cell-ID) for the currently-selected layer.
func renderViewport(w *worldgen.World, l layer, zoom, vpX, vpY, cols, rows int) string {
	var b strings.Builder
	b.Grow(cols * rows * 16)
	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			wx := vpX + rx*zoom
			wy := vpY + ry*zoom
			b.WriteString(renderCell(w, l, wx, wy))
		}
		if ry < rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderCell(w *worldgen.World, l layer, wx, wy int) string {
	if wx < 0 || wy < 0 || wx >= w.Width || wy >= w.Height {
		return oobStyle.Render("  ")
	}
	cellID := w.Voronoi.CellIDAt(wx, wy)
	switch l {
	case layerBiome:
		return renderTerrainCell(w.Terrain[cellID])
	case layerCells:
		return renderCellsLayer(cellID)
	case layerLand:
		return renderLandLayer(w, cellID)
	case layerElevation:
		return renderScalarLayer(w.Elevation[cellID])
	case layerMoisture:
		return renderScalarLayer(w.Moisture[cellID])
	case layerCoast:
		return renderCoastLayer(w, cellID)
	}
	return "  "
}

// renderTerrainCell reuses the shared tilestyle palette — same
// glyphs and colours as the main client UI. Glyph goes in the left
// column, a padding space in the right so the cell is two terminal
// columns wide for aspect correction.
func renderTerrainCell(t gworld.Terrain) string {
	glyph := tilestyle.GlyphFor(t)
	if glyph == "" {
		glyph = "?"
	}
	return tilestyle.StyleFor(t).Render(glyph + " ")
}

// renderCellsLayer paints one colour per cell ID — lets the dev
// verify the Voronoi partition.
func renderCellsLayer(cellID uint16) string {
	palette := []string{
		"#a06040", "#507040", "#603060", "#406080", "#805030",
		"#304860", "#708030", "#503070", "#604020", "#204870",
		"#708040", "#a08040", "#4060a0", "#30a060", "#906020",
	}
	bg := lipgloss.Color(palette[int(cellID)%len(palette)])
	return lipgloss.NewStyle().Background(bg).Render("  ")
}

// renderLandLayer paints binary land / ocean so the developer can
// see continent outlines without biome variation.
func renderLandLayer(w *worldgen.World, cellID uint16) string {
	if w.IsOcean(cellID) {
		return lipgloss.NewStyle().Background(lipgloss.Color("#0d3a70")).Render("  ")
	}
	return lipgloss.NewStyle().Background(lipgloss.Color("#6a8040")).Render("  ")
}

// renderScalarLayer draws a greyscale bar for a [0, 1] scalar —
// used by elevation and moisture layers.
func renderScalarLayer(v float32) string {
	r, g, b := lerpRGB(0x23, 0x23, 0x23, 0xe0, 0xe0, 0xe0, float64(v))
	bg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
	return lipgloss.NewStyle().Background(bg).Render("  ")
}

// renderCoastLayer highlights coast cells in yellow, ocean in dark
// blue, inland in green.
func renderCoastLayer(w *worldgen.World, cellID uint16) string {
	if w.IsOcean(cellID) {
		return lipgloss.NewStyle().Background(lipgloss.Color("#0d3a70")).Render("  ")
	}
	if w.IsCoast(cellID) {
		return lipgloss.NewStyle().Background(lipgloss.Color("#e6c64a")).Render("  ")
	}
	return lipgloss.NewStyle().Background(lipgloss.Color("#334a26")).Render("  ")
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

var oobStyle = lipgloss.NewStyle().Background(lipgloss.Color("234"))

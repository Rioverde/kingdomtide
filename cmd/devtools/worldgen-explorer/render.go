package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// renderViewport builds the main grid of coloured cells for the current
// layer. Each rendered cell is two terminal columns wide so the aspect
// approximates 1:1 despite terminal cells being taller than wide. The
// whole viewport is one lipgloss-built string — a handful of allocations
// per frame rather than per-cell style objects.
func renderViewport(w *worldgen.DemoWorld, l layer, zoom, vpX, vpY, cols, rows int) string {
	var b strings.Builder
	b.Grow(cols * rows * 16)

	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			wx := vpX + rx*zoom
			wy := vpY + ry*zoom
			val := sampleLayer(w, l, wx, wy, zoom)
			b.WriteString(cellForValue(l, val))
		}
		if ry < rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// sampleLayer returns one scalar sample from the chosen layer at world
// coord (wx, wy). At zoom > 1 it averages a zoom x zoom block so the
// dev sees an aggregate rather than one-in-N undersampling that would
// hide small features.
func sampleLayer(w *worldgen.DemoWorld, l layer, wx, wy, zoom int) float32 {
	if wx < 0 || wy < 0 || wx >= w.Width || wy >= w.Height {
		return -1 // sentinel for out-of-bounds padding cells
	}
	if zoom == 1 {
		return layerValueAt(w, l, wx, wy)
	}
	var sum float32
	var n int
	for dy := 0; dy < zoom; dy++ {
		for dx := 0; dx < zoom; dx++ {
			x, y := wx+dx, wy+dy
			if x >= w.Width || y >= w.Height {
				continue
			}
			sum += layerValueAt(w, l, x, y)
			n++
		}
	}
	if n == 0 {
		return -1
	}
	return sum / float32(n)
}

// layerValueAt picks the grid slice the current layer points at. One
// switch so the hot path stays trivially inlined.
func layerValueAt(w *worldgen.DemoWorld, l layer, x, y int) float32 {
	idx := y*w.Width + x
	switch l {
	case layerElevation:
		return w.Elevation[idx]
	case layerTemperature:
		return w.Temperature[idx]
	case layerMoisture:
		return w.Moisture[idx]
	}
	return 0
}

// cellForValue renders one 2-column viewport cell for a sampled scalar
// in [0, 1]. Out-of-bounds samples (val < 0) render as dim grey so the
// viewport border is visually distinct from deep-ocean tiles.
func cellForValue(l layer, val float32) string {
	if val < 0 {
		return oobStyle.Render("  ")
	}
	return palette(l, val).Render("  ")
}

// palette maps a scalar in [0, 1] to a lipgloss.Style with an RGB
// background. Per-layer tuning so elevation reads as land-tint,
// temperature as hot/cold, moisture as dry/wet. Foreground is unset;
// every cell renders as two blank spaces on a coloured background,
// which keeps glyph rendering cheap and avoids font-specific quirks.
func palette(l layer, val float32) lipgloss.Style {
	switch l {
	case layerElevation:
		return lipgloss.NewStyle().Background(elevationColor(val))
	case layerTemperature:
		return lipgloss.NewStyle().Background(temperatureColor(val))
	case layerMoisture:
		return lipgloss.NewStyle().Background(moistureColor(val))
	}
	return lipgloss.NewStyle()
}

// elevationColor is a five-band ramp — deep ocean, ocean, beach,
// lowland, highland, peak — matching the Whittaker-ish classification
// the real biome stage will eventually apply. Bands give the eye
// anchors; a purely smooth gradient would make it hard to spot the
// sea level threshold.
func elevationColor(v float32) lipgloss.TerminalColor {
	switch {
	case v < 0.36:
		return lipgloss.Color("#0a2a6b") // deep ocean
	case v < 0.44:
		return lipgloss.Color("#1e5ba6") // ocean
	case v < 0.48:
		return lipgloss.Color("#d9c37c") // beach
	case v < 0.58:
		return lipgloss.Color("#5c8a3a") // lowland
	case v < 0.68:
		return lipgloss.Color("#8a6b3a") // hills
	case v < 0.80:
		return lipgloss.Color("#6b5a4a") // mountain
	}
	return lipgloss.Color("#efefef") // snow peak
}

// temperatureColor is a blue-to-red heatmap. Saturated endpoints keep
// the eye drawn to extremes where climate anomalies show up.
func temperatureColor(v float32) lipgloss.TerminalColor {
	r, g, b := lerpRGB(0x1f, 0x77, 0xb4, 0xff, 0x37, 0x2e, float64(v))
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

// moistureColor is a tan-to-blue ramp. Drier tiles (deserts, rain
// shadows) read as warm khaki; wetter tiles (jungle, swamp) read as
// deep blue — distinct from the ocean blue used by elevation.
func moistureColor(v float32) lipgloss.TerminalColor {
	r, g, b := lerpRGB(0xc5, 0xa3, 0x6e, 0x1b, 0x4d, 0x8a, float64(v))
	return lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
}

// lerpRGB linearly interpolates between two RGB triples. Kept inline in
// each palette helper would duplicate the math; factoring it out keeps
// per-layer colour definitions to two sRGB endpoints each.
func lerpRGB(r0, g0, b0, r1, g1, b1 int, t float64) (int, int, int) {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	r := int(float64(r0) + (float64(r1)-float64(r0))*t)
	g := int(float64(g0) + (float64(g1)-float64(g0))*t)
	b := int(float64(b0) + (float64(b1)-float64(b0))*t)
	return r, g, b
}

// oobStyle paints out-of-bounds viewport cells. Keeping the colour in
// a package-level style avoids allocating a fresh lipgloss.Style for
// every border cell during render.
var oobStyle = lipgloss.NewStyle().Background(lipgloss.Color("234"))

package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// viewerZoomLevels enumerates the zoom factors the dev can cycle
// through with + / -. 1 means one terminal cell per tile (after the
// 2-column-per-tile aspect correction); 2 means a 2x2 block of world
// tiles folds into one rendered cell; and so on. 64 is enough to fit a
// 4096x4096 world on an 80-col terminal as a thumbnail (4096/64 = 64
// cells wide).
var viewerZoomLevels = []int{1, 2, 4, 8, 16, 32, 64}

// updateViewer is the phaseViewer key handler. Covers scrolling, layer
// cycling, zoom, the info-panel toggle, and the escape hatches back to
// the menu or out of the program.
func (m Model) updateViewer(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "n", "esc":
		// Return to menu so the dev can pick a different size / seed.
		m.phase = phaseMenu
		m.world = nil
		return m, nil

	case "up":
		m.vpY -= 1
	case "down":
		m.vpY += 1
	case "left":
		m.vpX -= 1
	case "right":
		m.vpX += 1
	case "shift+up", "K":
		m.vpY -= 10
	case "shift+down", "J":
		m.vpY += 10
	case "shift+left", "H":
		m.vpX -= 10
	case "shift+right", "L":
		m.vpX += 10
	case "pgup":
		_, visRows := m.viewportSize()
		m.vpY -= visRows
	case "pgdown":
		_, visRows := m.viewportSize()
		m.vpY += visRows
	case "home":
		m.vpX = 0
		m.vpY = 0
	case "end":
		visCols, visRows := m.viewportSize()
		m.vpX = m.world.Width - visCols*m.zoom
		m.vpY = m.world.Height - visRows*m.zoom

	case "l":
		m.layer = m.layer.next()
	case "+", "=":
		m.zoom = prevZoom(m.zoom)
	case "-", "_":
		m.zoom = nextZoom(m.zoom)

	case "i":
		m.showInfo = !m.showInfo

	case "1":
		m.layer = layerElevation
	case "2":
		m.layer = layerTemperature
	case "3":
		m.layer = layerMoisture
	}

	m.clampViewport()
	return m, nil
}

// viewViewer assembles the main scrollable screen: a grid of coloured
// cells rendered from the current layer, a one-line status bar, and
// optional info footer showing the sampled values at viewport centre.
func (m Model) viewViewer() string {
	if m.world == nil {
		return "no world generated"
	}
	visCols, visRows := m.viewportSize()
	body := renderViewport(m.world, m.layer, m.zoom, m.vpX, m.vpY, visCols, visRows)
	status := m.renderStatusBar(visCols)
	hints := m.renderHints()

	if m.showInfo {
		return body + "\n" + status + "\n" + m.renderInfo() + "\n" + hints
	}
	return body + "\n" + status + "\n" + hints
}

// viewportSize returns the (cols, rows) count in logical viewport cells.
// Each cell is 2 terminal columns wide so world maps stay roughly 1:1
// in aspect despite terminal cells being taller than they are wide.
func (m Model) viewportSize() (int, int) {
	cols := (m.termW - 2) / 2
	rows := m.termH - 4 // status bar + hints + optional info
	if m.showInfo {
		rows -= 1
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// clampViewport keeps (vpX, vpY) inside a safe range for the current
// zoom. Allows empty-band padding up to but not past the world edges so
// the dev cannot scroll the map completely off-screen.
func (m *Model) clampViewport() {
	visCols, visRows := m.viewportSize()
	if m.vpX < 0 {
		m.vpX = 0
	}
	if m.vpY < 0 {
		m.vpY = 0
	}
	maxX := m.world.Width - visCols*m.zoom
	maxY := m.world.Height - visRows*m.zoom
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	if m.vpX > maxX {
		m.vpX = maxX
	}
	if m.vpY > maxY {
		m.vpY = maxY
	}
}

// renderStatusBar shows what the dev is looking at: layer name, zoom,
// seed, size, viewport origin. Fits on one line at 80-col terminals.
func (m Model) renderStatusBar(visCols int) string {
	left := fmt.Sprintf(" %s  zoom %dx  seed %s  size %s (%dx%d)  vp %d,%d ",
		strings.ToUpper(m.layer.String()), m.zoom, formatSeed(m.world.Seed),
		m.world.Size.Label(), m.world.Width, m.world.Height,
		m.vpX, m.vpY)
	return statusBarStyle.Render(left)
}

// renderHints renders the dim controls line at the bottom of the view.
func (m Model) renderHints() string {
	s := "arrows: scroll  ·  shift+arrows: fast  ·  l: layer  ·  1/2/3: direct  ·  +/-: zoom  ·  i: info  ·  n: new world  ·  q: quit"
	return hintsStyle.Render(s)
}

// renderInfo dumps per-layer values at the centre of the viewport. Cheap
// single-tile sample; the info panel is for spot-checking specific sites
// rather than aggregate statistics.
func (m Model) renderInfo() string {
	visCols, visRows := m.viewportSize()
	cx := m.vpX + (visCols*m.zoom)/2
	cy := m.vpY + (visRows*m.zoom)/2
	if cx < 0 || cy < 0 || cx >= m.world.Width || cy >= m.world.Height {
		return hintsStyle.Render("(out of bounds)")
	}
	idx := cy*m.world.Width + cx
	line := fmt.Sprintf(" @(%d,%d)  elev=%.3f  temp=%.3f  moist=%.3f ",
		cx, cy,
		m.world.Elevation[idx], m.world.Temperature[idx], m.world.Moisture[idx])
	return infoStyle.Render(line)
}

// nextZoom and prevZoom walk the viewerZoomLevels table. nextZoom moves
// toward bigger factors (less detail, more world visible); prevZoom
// toward smaller (more detail, less world).
func nextZoom(z int) int {
	for i, v := range viewerZoomLevels {
		if v == z && i+1 < len(viewerZoomLevels) {
			return viewerZoomLevels[i+1]
		}
	}
	return z
}
func prevZoom(z int) int {
	for i, v := range viewerZoomLevels {
		if v == z && i-1 >= 0 {
			return viewerZoomLevels[i-1]
		}
	}
	return z
}

// Status-bar / hints / info styling. Kept out of the top of the file so
// the hot render path stays close to the dispatch logic. Lipgloss styles
// are lightweight, constructing them at package init is cheap.
var (
	statusBarStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("236")).
			Foreground(lipgloss.Color("15"))
	hintsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
	infoStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("237")).
			Foreground(lipgloss.Color("11"))
)

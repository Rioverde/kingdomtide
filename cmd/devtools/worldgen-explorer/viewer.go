package main

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/naming/markov"
	"github.com/Rioverde/gongeons/internal/game/naming/parts"
	"github.com/Rioverde/gongeons/internal/game/geom"
)

var viewerZoomLevels = []int{1, 2, 4, 8, 16, 32, 64}

func (m Model) updateViewer(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "n", "esc":
		m.phase = phaseMenu
		m.world = nil
		m.seedInput.SetValue(randomSeedString())
		return m, nil
	case "up":
		m.vpY -= m.arrowStep()
	case "down":
		m.vpY += m.arrowStep()
	case "left":
		m.vpX -= m.arrowStep()
	case "right":
		m.vpX += m.arrowStep()
	case "shift+up", "K":
		m.vpY -= m.arrowStep() * 5
	case "shift+down", "J":
		m.vpY += m.arrowStep() * 5
	case "shift+left", "H":
		m.vpX -= m.arrowStep() * 5
	case "shift+right", "L":
		m.vpX += m.arrowStep() * 5
	case "pgup":
		_, visRows := m.viewportSize()
		m.vpY -= visRows * m.zoom
	case "pgdown":
		_, visRows := m.viewportSize()
		m.vpY += visRows * m.zoom
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
	case "?":
		m.showLegend = !m.showLegend
	case "1":
		// Press once: switch to biome. Press again: toggle region tint.
		if m.layer == layerBiome {
			m.showRegionTint = !m.showRegionTint
		} else {
			m.layer = layerBiome
		}
	case "2":
		m.layer = layerCells
	case "3":
		m.layer = layerLand
	case "4":
		m.layer = layerElevation
	case "5":
		m.layer = layerMoisture
	case "6":
		m.layer = layerCoast
	case "7":
		m.layer = layerVolcanoes
	case "8":
		m.layer = layerRegions
	case "9":
		m.layer = layerLandmarks
	case "0":
		m.layer = layerDeposits
	case "c":
		// Press once: switch to camps. Press again: toggle faith background.
		if m.layer == layerCamps {
			m.showCampFaithBg = !m.showCampFaithBg
		} else {
			m.layer = layerCamps
		}
	}
	m.clampViewport()
	return m, nil
}

func (m Model) arrowStep() int {
	s := m.zoom * 4
	if s < 1 {
		return 1
	}
	return s
}

// viewBuf is the assembly buffer for viewViewer's final string.
// Reused across frames since bubbletea is single-threaded.
var viewBuf strings.Builder

func (m Model) viewViewer() string {
	if m.world == nil {
		return "no world generated"
	}
	visCols, visRows := m.viewportSize()
	body := renderViewport(&m, m.zoom, m.vpX, m.vpY, visCols, visRows)

	viewBuf.Reset()
	viewBuf.Grow(len(body) + 512)
	viewBuf.WriteString(body)
	viewBuf.WriteByte('\n')
	viewBuf.WriteString(m.renderStatusBar())
	if m.showInfo {
		viewBuf.WriteByte('\n')
		viewBuf.WriteString(m.renderInfo())
	}
	if m.showLegend {
		viewBuf.WriteByte('\n')
		viewBuf.WriteString(m.renderLegend())
	}
	viewBuf.WriteByte('\n')
	viewBuf.WriteString(hintsCached)
	return viewBuf.String()
}

func (m Model) viewportSize() (int, int) {
	cols := (m.termW - 2) / 2
	rows := m.termH - 4
	if m.showInfo {
		rows -= 2
	}
	if m.showLegend {
		rows -= legendLineCount(m.layer)
	}
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

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

func (m Model) renderStatusBar() string {
	left := fmt.Sprintf(" %s  zoom %dx  seed %s  size %s (%dx%d)  cells %d  vp %d,%d ",
		strings.ToUpper(m.layer.String()), m.zoom, formatSeed(m.world.Seed),
		m.world.Size.Label(), m.world.Width, m.world.Height,
		len(m.world.Voronoi.Cells),
		m.vpX, m.vpY)
	return statusBarStyle.Render(left)
}

// hintsCached — the hints line never changes, so render it once and
// reuse. Saves a lipgloss.Render call per frame.
var hintsCached = hintsStyle.Render(
	"arrows: scroll  ·  shift+arrows: fast  ·  l: layer  ·  1-9,0,c: direct  ·  +/-: zoom  ·  i: info  ·  ?: legend  ·  n: new  ·  q: quit")

func (m Model) renderInfo() string {
	visCols, visRows := m.viewportSize()
	cx := m.vpX + (visCols*m.zoom)/2
	cy := m.vpY + (visRows*m.zoom)/2
	if cx < 0 || cy < 0 || cx >= m.world.Width || cy >= m.world.Height {
		return infoStyle.Render("(out of bounds)") + "\n" + infoStyle.Render("")
	}
	cellID := m.world.Voronoi.CellIDAt(cx, cy)
	kind := "land"
	if m.world.IsOcean(cellID) {
		kind = "ocean"
	} else if m.world.IsCoast(cellID) {
		kind = "coast"
	}
	line1 := fmt.Sprintf(" @(%d,%d)  cell=%d(%s)  elev=%.2f  moist=%.2f  terrain=%s ",
		cx, cy, cellID, kind,
		m.world.Elevation[cellID], m.world.Moisture[cellID],
		m.world.Terrain[cellID])

	var line2 string
	if m.layer == layerCamps && m.campIndex != nil {
		key := geom.PackPos(geom.Position{X: cx, Y: cy})
		if camp, ok := m.campIndex[key]; ok {
			line2 = fmt.Sprintf(" region=%s  faith=%s  pop=%d  born=%d  footprint=%d tile(s) ",
				camp.Region.Key(), camp.Faiths.Majority(), camp.Population, camp.Founded, len(camp.Footprint))
		} else {
			line2 = " (no camp at cursor) "
		}
	} else if m.regionSrc != nil {
		sc := geom.WorldToSuperChunk(cx, cy)
		region := m.regionSrc.RegionAt(sc)
		name := renderRegionName(region.Name)
		top := formatTopInfluence(region.Influence)
		line2 = fmt.Sprintf(" region=%s  char=%s  top=%s ", name, region.Character.Key(), top)
	}

	return infoStyle.Render(line1) + "\n" + infoStyle.Render(line2)
}

// renderRegionName generates the Markov body string for a region's Parts
// record using the English corpus. Falls back to a hex representation of
// BodySeed so the panel always shows something distinctive.
func renderRegionName(p parts.Parts) string {
	if p.BodySeed == 0 {
		return "(unnamed)"
	}
	chain, err := markov.ChainFor("en", p.Character)
	if err != nil || chain == nil {
		return fmt.Sprintf("%#x", p.BodySeed)
	}
	rng := rand.New(rand.NewPCG(uint64(p.BodySeed), uint64(p.BodySeed)))
	body := chain.Generate(rng, 4, 10)
	if body == "" {
		return fmt.Sprintf("%#x", p.BodySeed)
	}
	return body
}

// formatTopInfluence formats up to three influence fields whose value is
// >= 0.1, sorted descending. Returns an empty string when all fields are
// below the threshold.
func formatTopInfluence(inf gworld.RegionInfluence) string {
	type kv struct {
		name string
		v    float32
	}
	fields := []kv{
		{"Blight", inf.Blight},
		{"Fae", inf.Fae},
		{"Ancient", inf.Ancient},
		{"Savage", inf.Savage},
		{"Holy", inf.Holy},
		{"Wild", inf.Wild},
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].v > fields[j].v })
	var sb strings.Builder
	for i, f := range fields {
		if i >= 3 || f.v < 0.1 {
			break
		}
		if sb.Len() > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%s=%.2f", f.name, f.v)
	}
	if sb.Len() == 0 {
		return "none"
	}
	return sb.String()
}

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

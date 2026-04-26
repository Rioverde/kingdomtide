package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/simulation"
)

// viewerZoomLevels is the ordered set of supported tile-sampling steps.
var viewerZoomLevels = []int{1, 2, 4, 8, 16, 32, 64}

// tickMsg is emitted by the playback ticker each time the year should advance.
type tickMsg time.Time

// playbackTick returns a tea.Cmd that fires after the appropriate delay for
// the current playback speed.
func playbackTick(speed int) tea.Cmd {
	if speed <= 0 {
		speed = 1
	}
	d := time.Duration(1000/speed) * time.Millisecond
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) updateViewer(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		if m.playing && len(m.snaps) > 0 {
			if m.year < len(m.snaps)-1 {
				m.year++
				m.rebuildPlaceIndex()
			} else {
				m.playing = false
			}
			if m.playing {
				return m, playbackTick(m.speed)
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit

		case "n", "esc":
			m.phase = phaseMenu
			m.world = nil
			m.seedInput.SetValue(randomSeedString())
			return m, nil

		// Playback controls.
		case " ":
			m.playing = !m.playing
			if m.playing {
				return m, playbackTick(m.speed)
			}
			return m, nil

		case ".":
			if m.year < len(m.snaps)-1 {
				m.year++
				m.rebuildPlaceIndex()
			}

		case ",":
			if m.year > 0 {
				m.year--
				m.rebuildPlaceIndex()
			}

		case "g":
			m.year = 0
			m.rebuildPlaceIndex()

		// Navigation.
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

		// Zoom.
		case "+", "=":
			m.zoom = prevZoom(m.zoom)
		case "-", "_":
			m.zoom = nextZoom(m.zoom)

		// Layer / display toggles.
		case "tab":
			m.layer = m.layer.next()
		case "i":
			m.showInfo = !m.showInfo
		case "?":
			m.showLegend = !m.showLegend
		case "M":
			m.showLog = !m.showLog

		// Layer direct keys: s=settlements, r=resources (mnemonic).
		case "s":
			m.layer = layerSettlements
		case "r":
			m.layer = layerResources

		// Speed / year keys: 1-9 set playback speed; 0 jumps to last year.
		case "0":
			if len(m.snaps) > 0 {
				m.year = len(m.snaps) - 1
				m.rebuildPlaceIndex()
			}
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			m.speed = int(msg.String()[0] - '0')
			if m.playing {
				return m, playbackTick(m.speed)
			}
		}
		m.clampViewport()
	}
	return m, nil
}

func (m Model) arrowStep() int {
	s := m.zoom * 4
	if s < 1 {
		return 1
	}
	return s
}

// viewBuf is the assembly buffer for viewViewer. Reused across frames since
// bubbletea is single-threaded.
var viewBuf strings.Builder

func (m Model) viewViewer() string {
	if m.world == nil || len(m.snaps) == 0 {
		return "no simulation data\n"
	}
	snap := &m.snaps[m.year]
	visCols, visRows := m.viewportSize()
	body := renderViewport(&m, m.zoom, m.vpX, m.vpY, visCols, visRows)

	viewBuf.Reset()
	viewBuf.Grow(len(body) + 512)
	viewBuf.WriteString(body)
	viewBuf.WriteByte('\n')
	viewBuf.WriteString(m.renderStatusBar(snap))
	if m.showInfo {
		viewBuf.WriteByte('\n')
		viewBuf.WriteString(m.renderInfo(snap))
	}
	if m.showLegend {
		viewBuf.WriteByte('\n')
		viewBuf.WriteString(m.renderLegend())
	}
	viewBuf.WriteByte('\n')
	viewBuf.WriteString(simHintsCached)

	mainStr := viewBuf.String()

	if m.showLog {
		logPanel := renderLogPanel(m.logBuf.String(), visRows)
		mainStr = lipgloss.JoinHorizontal(lipgloss.Top, mainStr, "  ", logPanel)
	}

	return mainStr
}

// viewportSize returns the visible (cols, rows) in screen-cell units.
// Each cell is 2 chars wide. Mirrors worldgen-explorer's viewportSize exactly.
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
	if m.world == nil {
		return
	}
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

func (m Model) renderStatusBar(snap *simulation.Snapshot) string {
	playIcon := "||"
	if m.playing {
		playIcon = ">>"
	}
	totalYears := len(m.snaps) - 1
	left := fmt.Sprintf(
		" %s  zoom %dx  seed %s  size %s (%dx%d)  year %d/%d  speed %dx %s  camps %d  hamlets %d  villages %d  vp %d,%d ",
		strings.ToUpper(m.layer.String()),
		m.zoom,
		formatSeed(m.pendingSeed),
		m.world.Size.Label(), m.world.Width, m.world.Height,
		snap.Year, totalYears,
		m.speed, playIcon,
		len(snap.Camps), len(snap.Hamlets), len(snap.Villages),
		m.vpX, m.vpY,
	)
	return statusBarStyle.Render(left)
}

// simHintsCached — rendered once; never changes between frames.
var simHintsCached = hintsStyle.Render(
	"arrows: scroll  ·  shift+arrows: fast  ·  s: settlements  r: resources  ·  tab: layer  ·  +/-: zoom  ·  space: play/pause  ·  ./,: step  ·  1-9: speed  ·  0: end  ·  g: start  ·  i: info  ·  ?: legend  ·  n: new  ·  q: quit")

func (m Model) renderInfo(snap *simulation.Snapshot) string {
	visCols, visRows := m.viewportSize()
	cx := m.vpX + (visCols*m.zoom)/2
	cy := m.vpY + (visRows*m.zoom)/2
	if m.world == nil || cx < 0 || cy < 0 || cx >= m.world.Width || cy >= m.world.Height {
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
	if m.placeIndex != nil {
		key := geom.PackPos(geom.Position{X: cx, Y: cy})
		if entry, ok := m.placeIndex[key]; ok {
			var base *polity.Settlement
			switch entry.tier {
			case polity.TierCamp:
				for i := range snap.Camps {
					if snap.Camps[i].Base().ID == entry.id {
						base = snap.Camps[i].Base()
						break
					}
				}
			case polity.TierHamlet:
				for i := range snap.Hamlets {
					if snap.Hamlets[i].Base().ID == entry.id {
						base = snap.Hamlets[i].Base()
						break
					}
				}
			case polity.TierVillage:
				for i := range snap.Villages {
					if snap.Villages[i].Base().ID == entry.id {
						base = snap.Villages[i].Base()
						break
					}
				}
			}
			if base != nil {
				tierName := tierLabel(entry.tier)
				line2 = fmt.Sprintf(" %s '%s' (id %04d)  region=%s  pop=%d  founded=%d  footprint=%d tiles ",
					tierName, base.Name, base.ID,
					base.Region.Key(),
					base.Population, base.Founded, len(base.Footprint))
			}
		}
	}
	if line2 == "" {
		if m.regionSrc != nil {
			sc := geom.WorldToSuperChunk(cx, cy)
			region := m.regionSrc.RegionAt(sc)
			line2 = fmt.Sprintf(" region-char=%s ", region.Character.Key())
		} else {
			line2 = " (no settlement at cursor) "
		}
	}

	return infoStyle.Render(line1) + "\n" + infoStyle.Render(line2)
}

// tierLabel returns the display name for a settlement tier.
func tierLabel(t polity.SettlementTier) string {
	switch t {
	case polity.TierCamp:
		return "Camp"
	case polity.TierHamlet:
		return "Hamlet"
	case polity.TierVillage:
		return "Village"
	default:
		return "Settlement"
	}
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

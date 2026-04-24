package main

import (
	"math/rand/v2"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// phase is the top-level state-machine slot the TUI lives in. Each phase
// owns a disjoint subset of Model fields and a single dispatch path in
// Update.
type phase int

const (
	phaseMenu phase = iota
	phaseBuilding
	phaseViewer
)

// layer picks which demo grid the viewer renders. Iteration order drives
// the L / shift-L cycling.
type layer int

const (
	layerElevation layer = iota
	layerTemperature
	layerMoisture
	layerCount
)

func (l layer) String() string {
	switch l {
	case layerElevation:
		return "elevation"
	case layerTemperature:
		return "temperature"
	case layerMoisture:
		return "moisture"
	}
	return "unknown"
}

func (l layer) next() layer { return (l + 1) % layerCount }
func (l layer) prev() layer { return (l + layerCount - 1) % layerCount }

// buildDoneMsg is emitted on the tea event loop when GenerateDemoWorld
// returns. It carries the built world so the Update handler can flip
// into phaseViewer without blocking Init.
type buildDoneMsg struct{ world *worldgen.DemoWorld }

// Model is the entire explorer state. Fields are grouped by phase; only
// the slice used by the active phase needs to be valid at any given
// moment.
type Model struct {
	phase phase

	// Menu phase.
	sizes        []worldgen.WorldSize
	sizeIdx      int
	seedInput    textinput.Model
	editingSeed  bool
	menuErr      string

	// Building phase.
	pendingSize worldgen.WorldSize
	pendingSeed int64

	// Viewer phase.
	world    *worldgen.DemoWorld
	layer    layer
	zoom     int // 1, 2, 4, 8, 16 — world tiles per rendered cell
	vpX, vpY int // top-left viewport in world-tile coordinates
	termW    int
	termH    int
	showInfo bool
	cursorX  int
	cursorY  int
}

// initialModel returns a Model parked in phaseMenu with the Standard
// preset pre-selected and a random seed pre-filled. The dev accepts the
// defaults with Enter or tweaks them first.
func initialModel() Model {
	ti := textinput.New()
	ti.Placeholder = "seed"
	ti.CharLimit = 19
	ti.Width = 20
	ti.SetValue(randomSeedString())

	return Model{
		phase:     phaseMenu,
		sizes:     worldgen.AllSizes(),
		sizeIdx:   int(worldgen.WorldSizeStandard),
		seedInput: ti,
		zoom:      1,
	}
}

// modelStartingBuild shortcuts the menu when both --size and --seed were
// supplied on the CLI. The build goroutine fires from Init so the user
// sees the progress screen immediately.
func modelStartingBuild(size worldgen.WorldSize, seed int64) Model {
	m := initialModel()
	m.phase = phaseBuilding
	m.pendingSize = size
	m.pendingSeed = seed
	return m
}

// Init kicks off the build if we started in phaseBuilding; otherwise the
// menu is already interactive and nothing async needs to happen.
func (m Model) Init() tea.Cmd {
	if m.phase == phaseBuilding {
		return buildCmd(m.pendingSize, m.pendingSeed)
	}
	return textinput.Blink
}

// Update dispatches the tea event to the handler for the current phase.
// Global keys (Ctrl+C / q) live at the top because the dev expects them
// to work regardless of which phase is on screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		}
	case buildDoneMsg:
		m.world = msg.world
		m.phase = phaseViewer
		m.layer = layerElevation
		m.zoom = pickInitialZoom(msg.world.Width, msg.world.Height, m.termW, m.termH)
		m.vpX = 0
		m.vpY = 0
		m.cursorX = msg.world.Width / 2
		m.cursorY = msg.world.Height / 2
		return m, nil
	}

	switch m.phase {
	case phaseMenu:
		return m.updateMenu(msg)
	case phaseViewer:
		return m.updateViewer(msg)
	}
	return m, nil
}

// View picks the renderer for the active phase.
func (m Model) View() string {
	switch m.phase {
	case phaseMenu:
		return m.viewMenu()
	case phaseBuilding:
		return m.viewBuilding()
	case phaseViewer:
		return m.viewViewer()
	}
	return ""
}

// buildCmd returns a tea.Cmd that runs GenerateDemoWorld off the event
// loop and emits a buildDoneMsg when it finishes. tea schedules the Cmd
// on a worker goroutine so the UI stays responsive during generation.
func buildCmd(size worldgen.WorldSize, seed int64) tea.Cmd {
	return func() tea.Msg {
		w := worldgen.GenerateDemoWorld(seed, size)
		return buildDoneMsg{world: w}
	}
}

// randomSeedString returns a fresh random seed rendered as a decimal
// string suitable for seeding the textinput field. Uses math/rand/v2 —
// the explorer is a dev tool, not a security-sensitive path.
func randomSeedString() string {
	return formatSeed(int64(rand.Uint64()))
}

// pickInitialZoom chooses a zoom level that shows most of the world at
// first glance. Terminal cell aspect is ~2:1 (tall), so every world
// tile takes 2 terminal columns.
func pickInitialZoom(worldW, worldH, termW, termH int) int {
	if termW <= 0 || termH <= 0 {
		return 1
	}
	// Leave room for the info bar + status line.
	visibleCols := (termW - 2) / 2
	visibleRows := termH - 4
	if visibleCols <= 0 || visibleRows <= 0 {
		return 1
	}
	for _, z := range viewerZoomLevels {
		if worldW/z <= visibleCols && worldH/z <= visibleRows {
			return z
		}
	}
	return viewerZoomLevels[len(viewerZoomLevels)-1]
}

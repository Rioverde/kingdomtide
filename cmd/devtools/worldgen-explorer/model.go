package main

import (
	"math/rand/v2"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// phase is the top-level state-machine slot the TUI lives in.
type phase int

const (
	phaseMenu phase = iota
	phaseBuilding
	phaseViewer
)

// menuField identifies which interactive element in the menu is taking input.
type menuField int

const (
	fieldSize menuField = iota
	fieldSeed
	fieldCount
)

func (f menuField) next() menuField { return (f + 1) % fieldCount }
func (f menuField) prev() menuField { return (f + fieldCount - 1) % fieldCount }

// layer picks which overlay the viewer renders.
type layer int

const (
	layerBiome layer = iota
	layerCells
	layerLand
	layerElevation
	layerMoisture
	layerCoast
	layerCount
)

func (l layer) String() string {
	switch l {
	case layerBiome:
		return "biome"
	case layerCells:
		return "cells"
	case layerLand:
		return "land/ocean"
	case layerElevation:
		return "elevation"
	case layerMoisture:
		return "moisture"
	case layerCoast:
		return "coast"
	}
	return "unknown"
}

func (l layer) next() layer { return (l + 1) % layerCount }

// buildDoneMsg is emitted on the tea event loop when Generate returns.
type buildDoneMsg struct{ world *worldgen.World }

// Model is the entire explorer state.
type Model struct {
	phase phase

	// Menu phase.
	sizes       []worldgen.WorldSize
	sizeIdx     int
	seedInput   textinput.Model
	activeField menuField
	menuErr     string

	// Building phase.
	pendingSize worldgen.WorldSize
	pendingSeed int64

	// Viewer phase.
	world    *worldgen.World
	layer    layer
	zoom     int
	vpX, vpY int
	termW    int
	termH    int
	showInfo bool
}

func initialModel() Model {
	ti := textinput.New()
	ti.Placeholder = "seed"
	ti.CharLimit = 19
	ti.Width = 20
	ti.SetValue(randomSeedString())
	return Model{
		phase:       phaseMenu,
		sizes:       worldgen.AllSizes(),
		sizeIdx:     int(worldgen.WorldSizeStandard),
		seedInput:   ti,
		activeField: fieldSize,
		zoom:        1,
	}
}

func modelStartingBuild(size worldgen.WorldSize, seed int64) Model {
	m := initialModel()
	m.phase = phaseBuilding
	m.pendingSize = size
	m.pendingSeed = seed
	return m
}

func (m Model) Init() tea.Cmd {
	if m.phase == phaseBuilding {
		return buildCmd(m.pendingSize, m.pendingSeed)
	}
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case buildDoneMsg:
		m.world = msg.world
		m.phase = phaseViewer
		m.layer = layerBiome
		m.zoom = pickInitialZoom(msg.world.Width, msg.world.Height, m.termW, m.termH)
		m.vpX = 0
		m.vpY = 0
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

func buildCmd(size worldgen.WorldSize, seed int64) tea.Cmd {
	return func() tea.Msg {
		w := worldgen.Generate(seed, size)
		return buildDoneMsg{world: w}
	}
}

func randomSeedString() string {
	return formatSeed(int64(rand.Uint64()))
}

func pickInitialZoom(worldW, worldH, termW, termH int) int {
	if termW <= 0 || termH <= 0 {
		return 1
	}
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

package main

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/simulation"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
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

// simLayer picks which overlay the viewer renders.
type simLayer int

const (
	layerSettlements simLayer = iota
	layerResources
	layerCount
)

func (l simLayer) String() string {
	switch l {
	case layerSettlements:
		return "settlements"
	case layerResources:
		return "resources"
	}
	return "unknown"
}

func (l simLayer) next() simLayer { return (l + 1) % layerCount }

// buildDoneMsg is emitted on the tea event loop when generation and
// simulation have finished.
type buildDoneMsg struct {
	world      *worldgen.Map
	regionSrc  gworld.RegionSource
	depositSrc gworld.DepositSource
	simSrc     *simulation.Source
	snaps      []simulation.Snapshot
	logBuf     *bytes.Buffer
	err        error
}

// Model is the entire sim-explorer state.
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
	world      *worldgen.Map
	regionSrc  gworld.RegionSource
	depositSrc gworld.DepositSource
	simSrc     *simulation.Source
	snaps      []simulation.Snapshot
	year       int
	speed      int
	playing    bool
	logBuf     bytes.Buffer

	// placeIndex maps packed tile position → settlement entry for the current
	// snapshot. Rebuilt every time year changes.
	placeIndex map[uint64]placeTileEntry

	// depositIndex maps packed tile position → Deposit for resources layer.
	depositIndex map[uint64]gworld.Deposit

	layer      simLayer
	zoom       int
	vpX, vpY   int
	termW      int
	termH      int
	showInfo   bool
	showLegend bool
	showLog    bool
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
		sizeIdx:     int(worldgen.WorldSizeSmall),
		seedInput:   ti,
		activeField: fieldSize,
		speed:       1,
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
		if msg.err != nil {
			return m, nil
		}
		m.world = msg.world
		m.regionSrc = msg.regionSrc
		m.depositSrc = msg.depositSrc
		m.simSrc = msg.simSrc
		m.snaps = msg.snaps
		m.year = 0
		m.vpX = 0
		m.vpY = 0
		m.zoom = pickInitialZoom(msg.world.Width, msg.world.Height, m.termW, m.termH)
		m.layer = layerSettlements
		if msg.logBuf != nil {
			m.logBuf = *msg.logBuf
		}
		m.depositIndex = buildDepositIndex(msg.depositSrc, msg.world)
		m.rebuildPlaceIndex()
		m.phase = phaseViewer
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
		var logBuf bytes.Buffer
		w := worldgen.Generate(seed, size)
		volcanoes := worldgen.NewVolcanoSource(w, seed)
		regions := worldgen.NewRegionSource(w, seed)
		landmarks := worldgen.NewLandmarkSource(w, seed, worldgen.LandmarkSourceConfig{
			Regions:   regions,
			Volcanoes: volcanoes,
		})
		deposits := worldgen.NewDepositSource(w, seed, worldgen.DepositSourceConfig{
			Volcanoes: volcanoes,
		})
		camps := worldgen.NewCampSource(w, seed, worldgen.CampSourceConfig{
			Regions:   regions,
			Landmarks: landmarks,
			Volcanoes: volcanoes,
			Deposits:  deposits,
		})

		// Open per-run log file under .omc/logs/. Failure is non-fatal — the
		// dev tool still runs with the in-memory buffer for the side panel.
		logFile, ferr := simulation.OpenLogFile(".", seed)
		var loggerWriter io.Writer = &logBuf
		if ferr == nil {
			// Caller leaks the file descriptor for the life of the process.
			// The dev tool runs once per launch so this is acceptable.
			loggerWriter = io.MultiWriter(&logBuf, logFile)
		}

		result := simulation.Run(seed, camps,
			simulation.WithSnapshotEvery(1),
			simulation.WithLogger(loggerWriter),
		)
		return buildDoneMsg{
			world:      w,
			regionSrc:  regions,
			depositSrc: deposits,
			simSrc:     result.SettlementSource(),
			snaps:      result.Snapshots(),
			logBuf:     &logBuf,
		}
	}
}

// buildDepositIndex flattens a DepositSource into a packed-XY map for
// O(1) per-tile lookups during rendering. Built once at world load.
func buildDepositIndex(src gworld.DepositSource, w *worldgen.Map) map[uint64]gworld.Deposit {
	if src == nil || w == nil {
		return nil
	}
	all := src.DepositsIn(geom.Rect{
		MinX: 0, MinY: 0, MaxX: w.Width, MaxY: w.Height,
	})
	idx := make(map[uint64]gworld.Deposit, len(all))
	for _, d := range all {
		idx[geom.PackPos(d.Position)] = d
	}
	return idx
}

// rebuildPlaceIndex regenerates the per-tile lookup map from the current
// year's snapshot. Called after any year change.
func (m *Model) rebuildPlaceIndex() {
	if m.year < 0 || m.year >= len(m.snaps) {
		m.placeIndex = nil
		return
	}
	snap := &m.snaps[m.year]
	m.placeIndex = buildPlaceIndex(snap)
}

// pickInitialZoom returns the smallest zoom level at which the entire world
// fits in the terminal viewport. Mirrors worldgen-explorer's implementation.
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

func randomSeedString() string {
	return formatSeed(int64(rand.Uint64()))
}

// formatSeed renders a seed int64 as a plain decimal string.
func formatSeed(seed int64) string { return strconv.FormatInt(seed, 10) }

// parseSeed accepts either a decimal int64 or a hex value prefixed with 0x.
func parseSeed(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("seed is empty")
	}
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		u, err := strconv.ParseUint(s[2:], 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hex seed: %w", err)
		}
		return int64(u), nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seed: %w", err)
	}
	return v, nil
}

// Keep polity imported via placeTileEntry.
var _ = polity.TierCamp

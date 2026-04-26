package main

import (
	"math/rand/v2"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
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

// layer picks which overlay the viewer renders.
type layer int

const (
	layerBiome layer = iota
	layerCells
	layerLand
	layerElevation
	layerMoisture
	layerCoast
	layerWatershed
	layerVolcanoes
	layerRegions
	layerLandmarks
	layerDeposits
	layerCamps
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
	case layerWatershed:
		return "watershed"
	case layerVolcanoes:
		return "volcanoes"
	case layerRegions:
		return "regions"
	case layerLandmarks:
		return "landmarks"
	case layerDeposits:
		return "deposits"
	case layerCamps:
		return "camps"
	}
	return "unknown"
}

func (l layer) next() layer { return (l + 1) % layerCount }

// buildDoneMsg is emitted on the tea event loop when Generate returns.
type buildDoneMsg struct{ world *worldgen.Map }

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
	world          *worldgen.Map
	showRegionTint bool // true → biome layer overlays region character tint
	volcanoSrc     gworld.VolcanoSource
	regionSrc      gworld.RegionSource
	landmarkSrc   gworld.LandmarkSource
	landmarkIndex map[uint64]gworld.Landmark
	landmarkList  []gworld.Landmark
	depositSrc    gworld.DepositSource
	depositIndex  map[uint64]gworld.Deposit
	campSrc       polity.CampSource
	campIndex     map[uint64]polity.Camp // packed position/footprint pos → Camp
	showCampFaithBg bool
	layer         layer
	zoom          int
	vpX, vpY      int
	termW         int
	termH         int
	showInfo   bool
	showLegend bool
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
		w := msg.world
		volcSrc := worldgen.NewVolcanoSource(w, w.Seed)
		regionSrc := worldgen.NewRegionSource(w, w.Seed)
		landmarkSrc := worldgen.NewLandmarkSource(w, w.Seed, worldgen.LandmarkSourceConfig{
			Regions:   regionSrc,
			Volcanoes: volcSrc,
		})
		depositSrc := worldgen.NewDepositSource(w, w.Seed, worldgen.DepositSourceConfig{
			Volcanoes: volcSrc,
		})
		campSrc := worldgen.NewCampSource(w, w.Seed, worldgen.CampSourceConfig{
			Regions:   regionSrc,
			Landmarks: landmarkSrc,
			Volcanoes: volcSrc,
			Deposits:  depositSrc,
		})
		m.volcanoSrc = volcSrc
		m.regionSrc = regionSrc
		m.landmarkSrc = landmarkSrc
		m.landmarkIndex, m.landmarkList = buildLandmarkIndex(w, landmarkSrc)
		m.depositSrc = depositSrc
		m.depositIndex = buildDepositIndex(depositSrc)
		m.campSrc = campSrc
		m.campIndex = buildCampIndex(campSrc)
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

// buildLandmarkIndex walks every super-chunk that intersects the world bounds
// and accumulates all landmark coords into a flat map keyed by packed XY so
// per-tile lookups are O(1) during rendering. It also returns a flat slice for
// bbox-scan at high zoom levels. Built once at world load.
func buildLandmarkIndex(w *worldgen.Map, src gworld.LandmarkSource) (map[uint64]gworld.Landmark, []gworld.Landmark) {
	idx := make(map[uint64]gworld.Landmark)
	var list []gworld.Landmark
	scW := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scH := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < scH; sy++ {
		for sx := 0; sx < scW; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			for _, lm := range src.LandmarksIn(sc) {
				key := geom.PackPos(lm.Coord)
				idx[key] = lm
				list = append(list, lm)
			}
		}
	}
	return idx, list
}

// buildDepositIndex flattens a DepositSource into a packed-XY map for O(1)
// per-tile lookups during rendering. Built once at world load.
func buildDepositIndex(src gworld.DepositSource) map[uint64]gworld.Deposit {
	if src == nil {
		return nil
	}
	// Query the entire world by using an enormous rect — DepositSource.sorted
	// is already a complete list; DepositsIn with a huge rect returns everything.
	all := src.DepositsIn(geom.Rect{MinX: -1 << 20, MinY: -1 << 20, MaxX: 1 << 20, MaxY: 1 << 20})
	idx := make(map[uint64]gworld.Deposit, len(all))
	for _, d := range all {
		key := geom.PackPos(d.Position)
		idx[key] = d
	}
	return idx
}

// buildCampIndex flattens a CampSource into a packed-XY map keyed by every
// footprint position (including anchor) for O(1) per-tile lookups during
// rendering. Built once at world load.
func buildCampIndex(src polity.CampSource) map[uint64]polity.Camp {
	if src == nil {
		return nil
	}
	camps := src.All()
	idx := make(map[uint64]polity.Camp, len(camps)*3)
	for _, c := range camps {
		for _, fp := range c.Footprint {
			idx[geom.PackPos(fp)] = c
		}
	}
	return idx
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

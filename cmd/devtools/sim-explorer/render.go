package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/simulation"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// regionTintBg mirrors worldgen-explorer's biome region overlay palette.
// Index matches polity.RegionCharacter iota.
var regionTintBg = [7]lipgloss.Color{
	"",    // RegionNormal   — no tint
	"236", // RegionBlighted — dark grey
	"53",  // RegionFey      — dark purple
	"94",  // RegionAncient  — dark brown
	"88",  // RegionSavage   — dark red
	"237", // RegionHoly     — soft grey
	"22",  // RegionWild     — dark green
}

// regionFgColors provides themed foreground hues per RegionCharacter for
// settlement glyphs. Mirrors worldgen-explorer's camp fg palette.
var regionFgColors = [7]lipgloss.Color{
	"250", // RegionNormal   — light grey
	"245", // RegionBlighted — medium grey
	"135", // RegionFey      — purple
	"130", // RegionAncient  — brown
	"160", // RegionSavage   — red
	"231", // RegionHoly     — white
	"34",  // RegionWild     — green
}

// simViewportBuf is a single shared builder reused across frames.
// Bubbletea is single-threaded so no synchronisation is needed; this
// saves a large Builder allocation + free per frame.
var simViewportBuf strings.Builder

// renderViewport draws the viewport — one 2-char cell per world tile
// (mapped through zoom stride) for the currently-selected layer.
// Mirrors worldgen-explorer's renderViewport signature exactly.
func renderViewport(m *Model, zoom, vpX, vpY, cols, rows int) string {
	simViewportBuf.Reset()
	simViewportBuf.Grow(cols * rows * 16)
	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			wx := vpX + rx*zoom
			wy := vpY + ry*zoom
			simViewportBuf.WriteString(renderCell(m, wx, wy, zoom))
		}
		if ry < rows-1 {
			simViewportBuf.WriteByte('\n')
		}
	}
	return simViewportBuf.String()
}

// renderCell returns a precomputed 2-char cell string for the given layer.
// All styled strings in the caches are built once at init() — no
// lipgloss.Render calls happen per frame.
func renderCell(m *Model, wx, wy, zoom int) string {
	w := m.world
	if wx < 0 || wy < 0 || wx >= w.Width || wy >= w.Height {
		return simOobCell
	}

	switch m.layer {
	case layerSettlements:
		return renderSettlementCell(m, wx, wy, zoom)
	case layerResources:
		cellID := w.Voronoi.CellIDAt(wx, wy)
		return renderDepositCell(m, wx, wy, zoom, cellID)
	}
	return "  "
}

// renderSettlementCell renders one cell on the settlements layer. At zoom=1
// the exact tile key is checked; at zoom>1 the entire zoom×zoom sample block
// is scanned for the highest-tier settlement present. Terrain (with region
// tint) shows beneath tiles with no settlement.
func renderSettlementCell(m *Model, wx, wy, zoom int) string {
	w := m.world

	if zoom == 1 {
		key := geom.PackPos(geom.Position{X: wx, Y: wy})
		if p, ok := m.placeIndex[key]; ok {
			return settlementCell(p, wx, wy)
		}
	} else {
		// Zoom > 1: scan the block; highest-tier settlement wins.
		var best *placeTileEntry
		for dy := 0; dy < zoom; dy++ {
			for dx := 0; dx < zoom; dx++ {
				key := geom.PackPos(geom.Position{X: wx + dx, Y: wy + dy})
				if p, ok := m.placeIndex[key]; ok {
					if best == nil || p.tier > best.tier {
						cp := p
						best = &cp
					}
				}
			}
		}
		if best != nil {
			return settlementCell(*best, wx, wy)
		}
	}

	// Terrain fallback — dimmed biome layer so settlements stand out as the focus.
	// Mirrors worldgen-explorer's deposit layer dimming pattern.
	cellID := w.Voronoi.CellIDAt(wx, wy)
	t := w.Terrain[cellID]
	baseGlyph := tilestyle.GlyphFor(t)
	if baseGlyph == "" {
		baseGlyph = "·"
	}
	return settlementDimStyle.Render(baseGlyph + " ")
}

// renderDepositCell renders one cell on the resources layer. The base tile is
// shown in a dimmed biome style; if any deposit falls within the zoom×zoom
// sample block it is overlaid with a kind-specific glyph. Mirrors
// worldgen-explorer's renderDepositCell exactly.
func renderDepositCell(m *Model, wx, wy, zoom int, cellID uint32) string {
	t := m.world.Terrain[cellID]
	baseGlyph := tilestyle.GlyphFor(t)
	if baseGlyph == "" {
		baseGlyph = "·"
	}
	base := simDepositDimStyle.Render(baseGlyph + " ")

	if m.depositIndex == nil {
		return base
	}

	if zoom == 1 {
		key := geom.PackPos(geom.Position{X: wx, Y: wy})
		if d, ok := m.depositIndex[key]; ok {
			idx := int(d.Kind)
			if idx > 0 && idx < len(simDepositKindCell) {
				return simDepositKindCell[idx]
			}
		}
		return base
	}

	// Zoom > 1: scan the sample block for any deposit.
	for dy := 0; dy < zoom; dy++ {
		for dx := 0; dx < zoom; dx++ {
			key := geom.PackPos(geom.Position{X: wx + dx, Y: wy + dy})
			if d, ok := m.depositIndex[key]; ok {
				idx := int(d.Kind)
				if idx > 0 && idx < len(simDepositKindCell) {
					return simDepositKindCell[idx]
				}
			}
		}
	}
	return base
}

// settlementCell returns the styled 2-char glyph cell for a place tile.
// At zoom>1 the anchor distinction is dropped — the block shows the tier glyph.
func settlementCell(p placeTileEntry, wx, wy int) string {
	regionIdx := int(p.region)
	if regionIdx < 0 || regionIdx >= 7 {
		regionIdx = 0
	}
	isAnchor := p.anchorX == wx && p.anchorY == wy
	switch p.tier {
	case polity.TierCamp:
		if isAnchor {
			return campAnchorCellSim[regionIdx]
		}
		return campFootCellSim[regionIdx]
	case polity.TierHamlet:
		if isAnchor {
			return hamletAnchorCellSim[regionIdx]
		}
		return hamletFootCellSim[regionIdx]
	case polity.TierVillage:
		if isAnchor {
			return villageAnchorCellSim[regionIdx]
		}
		return villageFootCellSim[regionIdx]
	}
	return "  "
}

// placeTileEntry holds the per-tile settlement data stored in the place index.
type placeTileEntry struct {
	tier    polity.SettlementTier
	region  polity.RegionCharacter
	anchorX int
	anchorY int
	id      polity.SettlementID
}

// buildPlaceIndex constructs a packed-XY → placeTileEntry map for O(1)
// per-tile lookups during rendering. Rebuilt each time the displayed snapshot
// changes. Every footprint tile (including the anchor) maps to its settlement.
func buildPlaceIndex(snap *simulation.Snapshot) map[uint64]placeTileEntry {
	if snap == nil {
		return nil
	}
	idx := make(map[uint64]placeTileEntry)
	for i := range snap.Camps {
		c := &snap.Camps[i]
		base := c.Base()
		for _, fp := range base.Footprint {
			idx[geom.PackPos(fp)] = placeTileEntry{
				tier:    polity.TierCamp,
				region:  base.Region,
				anchorX: base.Position.X,
				anchorY: base.Position.Y,
				id:      base.ID,
			}
		}
	}
	for i := range snap.Hamlets {
		h := &snap.Hamlets[i]
		base := h.Base()
		for _, fp := range base.Footprint {
			idx[geom.PackPos(fp)] = placeTileEntry{
				tier:    polity.TierHamlet,
				region:  base.Region,
				anchorX: base.Position.X,
				anchorY: base.Position.Y,
				id:      base.ID,
			}
		}
	}
	for i := range snap.Villages {
		v := &snap.Villages[i]
		base := v.Base()
		for _, fp := range base.Footprint {
			idx[geom.PackPos(fp)] = placeTileEntry{
				tier:    polity.TierVillage,
				region:  base.Region,
				anchorX: base.Position.X,
				anchorY: base.Position.Y,
				id:      base.ID,
			}
		}
	}
	return idx
}

// === Pre-rendered cell caches (built once at init) ===

var (
	simOobCell   string
	simRiverCell string

	terrainCellSim         = map[gworld.Terrain]string{}
	terrainCellVariantsSim = map[gworld.Terrain][]string{}

	// biomeRegionCellSim — per (terrain, character) cell with region tint bg.
	biomeRegionCellSim map[gworld.Terrain][7]string

	// Camp cells: [regionIdx] — "C " anchor (bold) / "c " footprint.
	campAnchorCellSim [7]string
	campFootCellSim   [7]string

	// Hamlet cells: slightly brighter.
	hamletAnchorCellSim [7]string
	hamletFootCellSim   [7]string

	// Village cells: brightest.
	villageAnchorCellSim [7]string
	villageFootCellSim   [7]string

	// Deposit layer: styled glyph cell per DepositKind (index == kind uint8).
	// Matches worldgen-explorer's depositKindCell order and palette exactly.
	// Length 13: index 0 (DepositNone) unused; 1-12 = Iron…Sulfur.
	simDepositKindCell [13]string
	// simDepositDimStyle — dim style for base tiles on the resources layer.
	simDepositDimStyle lipgloss.Style

	// settlementDimStyle — dim style for base tiles on the settlements layer.
	settlementDimStyle lipgloss.Style
)

// hamletBrightFg maps RegionCharacter index to a brighter foreground color
// for hamlet glyphs, giving visual differentiation from camps.
var hamletBrightFg = [7]lipgloss.Color{
	"253", // RegionNormal
	"248", // RegionBlighted
	"141", // RegionFey
	"136", // RegionAncient
	"196", // RegionSavage
	"231", // RegionHoly
	"82",  // RegionWild
}

// villageBrightestFg maps RegionCharacter index to the brightest foreground
// color for village glyphs.
var villageBrightestFg = [7]lipgloss.Color{
	"255", // RegionNormal
	"251", // RegionBlighted
	"147", // RegionFey
	"172", // RegionAncient
	"203", // RegionSavage
	"231", // RegionHoly
	"118", // RegionWild
}

func init() {
	oobSty := lipgloss.NewStyle().Background(lipgloss.Color("234"))
	simOobCell = oobSty.Render("  ")

	oceanStyle := tilestyle.StyleFor(gworld.TerrainOcean).Bold(true)
	simRiverCell = oceanStyle.Render(tilestyle.GlyphFor(gworld.TerrainOcean) + " ")

	for terrain, glyph := range tilestyle.TerrainRunes {
		terrainCellSim[terrain] = tilestyle.StyleFor(terrain).Render(glyph + " ")
	}
	for terrain, variants := range tilestyle.TerrainRuneVariants {
		style := tilestyle.StyleFor(terrain)
		rendered := make([]string, len(variants))
		for i, glyph := range variants {
			rendered[i] = style.Render(glyph + " ")
		}
		terrainCellVariantsSim[terrain] = rendered
	}

	biomeRegionCellSim = make(map[gworld.Terrain][7]string, len(tilestyle.TerrainRunes))
	for terrain, glyph := range tilestyle.TerrainRunes {
		var arr [7]string
		for char := 1; char < 7; char++ {
			if regionTintBg[char] == "" {
				continue
			}
			style := tilestyle.StyleFor(terrain).Background(regionTintBg[char])
			arr[char] = style.Render(glyph + " ")
		}
		biomeRegionCellSim[terrain] = arr
	}

	for i, fg := range regionFgColors {
		campAnchorCellSim[i] = lipgloss.NewStyle().Foreground(fg).Bold(true).Render("C ")
		campFootCellSim[i] = lipgloss.NewStyle().Foreground(fg).Render("c ")

		hfg := hamletBrightFg[i]
		hamletAnchorCellSim[i] = lipgloss.NewStyle().Foreground(hfg).Bold(true).Render("H ")
		hamletFootCellSim[i] = lipgloss.NewStyle().Foreground(hfg).Render("h ")

		vfg := villageBrightestFg[i]
		villageAnchorCellSim[i] = lipgloss.NewStyle().Foreground(vfg).Bold(true).Render("V ")
		villageFootCellSim[i] = lipgloss.NewStyle().Foreground(vfg).Render("v ")
	}

	// Deposit layer: matches worldgen-explorer's depositKindCell exactly.
	type depositEntry struct {
		glyph string
		style lipgloss.Style
	}
	depositEntries := [13]depositEntry{
		0:  {"  ", lipgloss.NewStyle()},
		1:  {"* ", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)}, // Iron
		2:  {"▪ ", lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)}, // Stone
		3:  {"T ", lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true)}, // Timber
		4:  {"~ ", lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)},  // Fertile
		5:  {"~ ", lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)},  // Fish
		6:  {"^ ", lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)}, // Game
		7:  {"· ", lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)}, // Salt
		8:  {"$ ", lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)}, // Gold
		9:  {"$ ", lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Bold(true)}, // Silver
		10: {"◆ ", lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true)}, // Gems
		11: {"▲ ", lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)}, // Obsidian
		12: {"% ", lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)}, // Sulfur
	}
	for i, e := range depositEntries {
		simDepositKindCell[i] = e.style.Render(e.glyph)
	}
	simDepositDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))

	// Settlements layer: dim style for base tiles so settlements pop.
	settlementDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("237"))
}

// renderLogPanel returns a side panel string containing the last n lines of
// the simulation log. Fixed 35 chars wide.
func renderLogPanel(logContent string, panelRows int) string {
	const (
		panelWidth = 35
		prefix     = " "
	)
	logPanelBorderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(panelWidth)
	logPanelTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Bold(true)
	logPanelLineStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("250"))

	lines := splitLogTail(logContent, panelRows)

	var sb strings.Builder
	sb.WriteString(logPanelTitleStyle.Render("simulation log"))
	sb.WriteByte('\n')
	for _, line := range lines {
		if len(line) > panelWidth-2 {
			line = line[:panelWidth-2]
		}
		sb.WriteString(prefix)
		sb.WriteString(logPanelLineStyle.Render(line))
		sb.WriteByte('\n')
	}
	return logPanelBorderStyle.Render(sb.String())
}

// splitLogTail returns the last n non-empty lines from the log string.
func splitLogTail(log string, n int) []string {
	if log == "" {
		return nil
	}
	log = strings.TrimRight(log, "\n")
	all := strings.Split(log, "\n")
	if len(all) <= n {
		return all
	}
	return all[len(all)-n:]
}

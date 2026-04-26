package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// regionTintBg maps RegionCharacter index to the background colour used
// for the subtle overlay on the biome layer. Index 0 (RegionNormal) is
// empty — no tint applied so the plain biome cell shows instead.
var regionTintBg = [7]lipgloss.Color{
	"",    // RegionNormal   — no tint
	"236", // RegionBlighted — dark grey
	"53",  // RegionFey      — dark purple
	"94",  // RegionAncient  — dark brown
	"88",  // RegionSavage   — dark red
	"237", // RegionHoly     — soft grey
	"22",  // RegionWild     — dark green
}

// viewportBuf is a single shared builder reused across frames.
// Bubbletea is single-threaded so no synchronisation is needed; this
// saves an ~80KB Builder allocation + free per frame, the largest
// per-frame allocation in the explorer.
var viewportBuf strings.Builder

// renderViewport draws the viewport — one cell per world tile
// (mapped through cell-ID) for the currently-selected layer.
func renderViewport(m *Model, zoom, vpX, vpY, cols, rows int) string {
	viewportBuf.Reset()
	viewportBuf.Grow(cols * rows * 16)
	for ry := 0; ry < rows; ry++ {
		for rx := 0; rx < cols; rx++ {
			wx := vpX + rx*zoom
			wy := vpY + ry*zoom
			viewportBuf.WriteString(renderCell(m, wx, wy, zoom))
		}
		if ry < rows-1 {
			viewportBuf.WriteByte('\n')
		}
	}
	return viewportBuf.String()
}

// renderCell returns a precomputed cell string for the given layer.
// Every styled string is built once at init() — no lipgloss.Render
// calls happen per frame. With an 80×30 viewport at 60 fps that
// saves ~144K Render() calls/sec, which was the dev-tool's
// bottleneck.
func renderCell(m *Model, wx, wy, zoom int) string {
	w := m.world
	if wx < 0 || wy < 0 || wx >= w.Width || wy >= w.Height {
		return oobCell
	}
	cellID := w.Voronoi.CellIDAt(wx, wy)
	switch m.layer {
	case layerBiome:
		// Landmark overlay — point features, highest priority on biome
		// view so users see "where stuff is" without switching layers.
		if m.landmarkIndex != nil {
			for dy := 0; dy < zoom; dy++ {
				for dx := 0; dx < zoom; dx++ {
					key := geom.PackPos(geom.Position{X: wx + dx, Y: wy + dy})
					if lm, ok := m.landmarkIndex[key]; ok {
						idx := int(lm.Kind)
						if idx > 0 && idx < len(landmarkKindCell) {
							return landmarkKindCell[idx]
						}
					}
				}
			}
		}
		// Volcano terrain overlay — replaces base biome on volcanic
		// tiles. Scan zoom block so zones stay visible at high zoom.
		if m.volcanoSrc != nil {
			bestP := 0
			var bestT gworld.Terrain
			for dy := 0; dy < zoom; dy++ {
				for dx := 0; dx < zoom; dx++ {
					if vt, ok := m.volcanoSrc.TerrainOverrideAt(geom.Position{X: wx + dx, Y: wy + dy}); ok {
						if p := volcanoBiomePriority[vt]; p > bestP {
							bestP = p
							bestT = vt
						}
					}
				}
			}
			if bestP > 0 {
				if cell, found := volcanoTerrainCell[bestT]; found {
					return cell
				}
			}
		}
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		t := w.Terrain[cellID]
		// Region tint overlay — non-Normal characters paint a subtle background
		// so users see regional character on the biome layer without switching.
		// Skipped for ocean and river tiles (handled above).
		if m.showRegionTint && !w.IsOcean(cellID) && m.regionSrc != nil {
			sc := geom.WorldToSuperChunk(wx, wy)
			char := m.regionSrc.RegionAt(sc).Character
			if char != gworld.RegionNormal {
				idx := int(char)
				if idx > 0 && idx < 7 {
					if arr, ok := biomeRegionCell[t]; ok {
						if cell := arr[idx]; cell != "" {
							return cell
						}
					}
				}
			}
		}
		if variants := terrainCellVariants[t]; len(variants) > 0 {
			return variants[int(cellID)%len(variants)]
		}
		return terrainCell[t]
	case layerCells:
		return cellsPaletteCell[int(cellID)%len(cellsPaletteCell)]
	case layerLand:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return landOceanCell
		}
		return landLandCell
	case layerElevation:
		return scalarCell[scalarBucket(w.Elevation[cellID])]
	case layerMoisture:
		return scalarCell[scalarBucket(w.Moisture[cellID])]
	case layerCoast:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return coastOceanCell
		}
		if w.IsCoast(cellID) {
			return coastCoastCell
		}
		return coastInlandCell
	case layerWatershed:
		if w.IsRiver(wx, wy) && !w.IsOcean(cellID) {
			return riverCell
		}
		if w.IsOcean(cellID) {
			return watershedOceanCell
		}
		ws := w.Watershed[cellID]
		if ws < 0 {
			return watershedEndorheicCell
		}
		return watershedCell[int(ws)%len(watershedCell)]
	case layerVolcanoes:
		return renderVolcanoCell(m, wx, wy, zoom)
	case layerRegions:
		return renderRegionCell(m, wx, wy, cellID)
	case layerLandmarks:
		return renderLandmarkCell(m, wx, wy, zoom, cellID)
	case layerDeposits:
		return renderDepositCell(m, wx, wy, zoom, cellID)
	case layerCamps:
		return renderCampCell(m, wx, wy, zoom)
	}
	return "  "
}

// === Caches built once at init() ============================

var (
	oobStyle = lipgloss.NewStyle().Background(lipgloss.Color("234"))
	oobCell  string

	// riverCell — bold ocean style with the ocean glyph, used as the
	// river overlay across all biome-revealing layers.
	riverCell string

	// terrainCell[t] — fully-styled "glyph " string for terrain t.
	// Used as fallback for terrains with no variants defined.
	terrainCell = map[gworld.Terrain]string{}

	// terrainCellVariants[t] — pre-rendered variant strings for terrain t.
	// Each entry is a fully-styled "glyph " string; index selected per
	// cell via cellID%len(variants) so the same cell always shows the
	// same glyph across redraws.
	terrainCellVariants = map[gworld.Terrain][]string{}

	// cellsPaletteCell — 15-entry debug palette for the cells layer.
	cellsPaletteCell = make([]string, 15)

	// landLayer — binary land/ocean.
	landOceanCell string
	landLandCell  string

	// coastLayer — ocean / coast / inland.
	coastOceanCell  string
	coastCoastCell  string
	coastInlandCell string

	// volcanoBiomePriority orders the 5 volcanic terrains so the most
	// distinctive feature wins when a sample block straddles multiple
	// zones (core/dormant/crater > slope > ashland). Zero entry =
	// non-volcanic terrain, never picked.
	volcanoBiomePriority = map[gworld.Terrain]int{
		gworld.TerrainVolcanoCore:        4,
		gworld.TerrainVolcanoCoreDormant: 4,
		gworld.TerrainCraterLake:         3,
		gworld.TerrainVolcanoSlope:       2,
		gworld.TerrainAshland:            1,
	}

	// scalarCell — quantised gradient (32 buckets) for elevation /
	// moisture layers. Continuous values get bucketed via
	// scalarBucket() so we precompute styling once per bucket.
	scalarCell [scalarBuckets]string

	// watershedCell — same hand-picked palette as before, but
	// pre-rendered.
	watershedCell          []string
	watershedOceanCell     string
	watershedEndorheicCell string

	// volcanoTerrainCell — styled cell strings for each volcanic terrain type.
	// Index by gworld.Terrain; missing keys fall back to volcanoDimCell.
	volcanoTerrainCell = map[gworld.Terrain]string{}
	// volcanoDimCell is the dimmed background shown for non-volcanic tiles
	// in the volcano layer so hotspots stand out.
	volcanoDimCell string

	// regionCharCell — one pre-rendered "  " cell per RegionCharacter value,
	// reverse-mode background tint matching the character's thematic terrain.
	regionCharCell [7]string

	// biomeRegionCell — per (terrain, character) pre-rendered "glyph " cell
	// with a subtle background tint for non-Normal characters. Used on the
	// biome layer to show regional character without hiding the terrain glyph.
	// An empty string sentinel means "use the plain biome cell instead" (Normal
	// or any terrain not in TerrainRunes).
	biomeRegionCell map[gworld.Terrain][7]string

	// landmarkKindCell — styled glyph cell per LandmarkKind (index == kind
	// uint8). Index 0 (LandmarkNone) is unused; callers check Kind != None.
	landmarkKindCell [7]string
	// landmarkBaseStyle — dim style for the base tile shown beneath landmarks.
	landmarkDimStyle lipgloss.Style

	// depositKindCell — styled glyph cell per DepositKind (index == kind uint8).
	// Index 0 (DepositNone) is unused; callers skip Kind == None.
	// Length matches the DepositSulfur iota value + 1 = 13.
	depositKindCell [13]string
	// depositDimStyle — dim style for the base tile shown beneath deposits.
	depositDimStyle lipgloss.Style

	// campAnchorCell[regionIdx] — styled "c " cell for a camp anchor tile,
	// foreground tinted by region (7 entries matching regionTintBg palette).
	campAnchorCell [7]string
	// campFootCell[regionIdx] — styled "o " cell for a non-anchor footprint tile.
	campFootCell [7]string
	// campFaithBg — background colour per Faith for the optional faith overlay.
	// Index matches polity.Faith iota order (0=OldGods … 4=StormPact).
	campFaithBg [5]lipgloss.Color
	// campFaithBgCell[regionIdx][faithIdx][anchorBit] — pre-rendered cell for the
	// faith-background overlay path. anchorBit: 0=footprint tile, 1=anchor tile.
	// 7 regions × 5 faiths × 2 anchor states = 70 strings, built once at init().
	campFaithBgCell [7][5][2]string
	// campDimCell — fallback dim cell for tiles with no camp on this layer.
	campDimCell string
)

const scalarBuckets = 32

// watershedPalette stays as terrain-typed list; we render each entry
// once at init.
var watershedPalette = []gworld.Terrain{
	gworld.TerrainGrassland,
	gworld.TerrainDesert,
	gworld.TerrainTaiga,
	gworld.TerrainSavanna,
	gworld.TerrainJungle,
	gworld.TerrainBeach,
	gworld.TerrainHills,
	gworld.TerrainMeadow,
	gworld.TerrainTundra,
	gworld.TerrainForest,
}

func init() {
	oobCell = oobStyle.Render("  ")

	oceanStyle := tilestyle.StyleFor(gworld.TerrainOcean).Bold(true)
	riverCell = oceanStyle.Render(tilestyle.GlyphFor(gworld.TerrainOcean) + " ")

	for terrain, glyph := range tilestyle.TerrainRunes {
		terrainCell[terrain] = tilestyle.StyleFor(terrain).Render(glyph + " ")
	}

	for terrain, variants := range tilestyle.TerrainRuneVariants {
		style := tilestyle.StyleFor(terrain)
		rendered := make([]string, len(variants))
		for i, glyph := range variants {
			rendered[i] = style.Render(glyph + " ")
		}
		terrainCellVariants[terrain] = rendered
	}

	// biomeRegionCell: combine each terrain's foreground style with each
	// non-Normal character's background tint. Empty string sentinel for
	// Normal (index 0) so renderCell falls through to the plain biome cell.
	biomeRegionCell = make(map[gworld.Terrain][7]string, len(tilestyle.TerrainRunes))
	for terrain, glyph := range tilestyle.TerrainRunes {
		var arr [7]string
		for char := 1; char < 7; char++ {
			if regionTintBg[char] == "" {
				continue
			}
			style := tilestyle.StyleFor(terrain).Background(regionTintBg[char])
			arr[char] = style.Render(glyph + " ")
		}
		biomeRegionCell[terrain] = arr
	}

	cellsPalette := []string{
		"#a06040", "#507040", "#603060", "#406080", "#805030",
		"#304860", "#708030", "#503070", "#604020", "#204870",
		"#708040", "#a08040", "#4060a0", "#30a060", "#906020",
	}
	for i, hex := range cellsPalette {
		cellsPaletteCell[i] = lipgloss.NewStyle().
			Background(lipgloss.Color(hex)).Render("  ")
	}

	landOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	landLandCell = tilestyle.StyleFor(gworld.TerrainGrassland).Reverse(true).Render("  ")

	coastOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	coastCoastCell = tilestyle.StyleFor(gworld.TerrainBeach).Reverse(true).Render("  ")
	coastInlandCell = tilestyle.StyleFor(gworld.TerrainForest).Reverse(true).Render("  ")

	for i := 0; i < scalarBuckets; i++ {
		v := float64(i) / float64(scalarBuckets-1)
		r, g, b := lerpRGB(0x23, 0x23, 0x23, 0xe0, 0xe0, 0xe0, v)
		bg := lipgloss.Color(fmt.Sprintf("#%02x%02x%02x", r, g, b))
		scalarCell[i] = lipgloss.NewStyle().Background(bg).Render("  ")
	}

	watershedCell = make([]string, len(watershedPalette))
	for i, t := range watershedPalette {
		watershedCell[i] = tilestyle.StyleFor(t).Reverse(true).Render("  ")
	}
	watershedOceanCell = tilestyle.StyleFor(gworld.TerrainDeepOcean).Reverse(true).Render("  ")
	watershedEndorheicCell = oobStyle.Render("  ")

	// Volcano layer: pre-render each volcanic terrain using its tilestyle entry.
	volcanicTerrains := []gworld.Terrain{
		gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainCraterLake,
		gworld.TerrainVolcanoSlope,
		gworld.TerrainAshland,
	}
	for _, t := range volcanicTerrains {
		glyph := tilestyle.GlyphFor(t)
		volcanoTerrainCell[t] = tilestyle.StyleFor(t).Render(glyph + " ")
	}
	volcanoDimCell = lipgloss.NewStyle().
		Foreground(lipgloss.Color("239")).
		Background(lipgloss.Color("233")).
		Render("· ")

	// Region layer: one reverse-mode background cell per character.
	regionPalette := []gworld.Terrain{
		gworld.TerrainGrassland,  // RegionNormal
		gworld.TerrainTundra,     // RegionBlighted
		gworld.TerrainMeadow,     // RegionFey
		gworld.TerrainHills,      // RegionAncient
		gworld.TerrainDesert,     // RegionSavage
		gworld.TerrainSnow,       // RegionHoly
		gworld.TerrainForest,     // RegionWild
	}
	for i, t := range regionPalette {
		regionCharCell[i] = tilestyle.StyleFor(t).Reverse(true).Render("  ")
	}

	// Landmark layer: bright foreground glyph per kind.
	// Index order matches LandmarkKind iota: 0=None, 1=Tower…6=Shrine.
	type landmarkEntry struct {
		glyph string
		style lipgloss.Style
	}
	landmarkEntries := [7]landmarkEntry{
		0: {"  ", lipgloss.NewStyle()}, // LandmarkNone — unused
		1: {"T ", lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)}, // Tower — gold
		2: {"Y ", lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true)},  // GiantTree — bright green
		3: {"o ", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)}, // StandingStones — light grey
		4: {"I ", lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Bold(true)}, // Obelisk — purple
		5: {"V ", lipgloss.NewStyle().Foreground(lipgloss.Color("124")).Bold(true)}, // Chasm — dark red
		6: {"+ ", lipgloss.NewStyle().Foreground(lipgloss.Color("87")).Bold(true)}, // Shrine — cyan
	}
	for i, e := range landmarkEntries {
		landmarkKindCell[i] = e.style.Render(e.glyph)
	}
	landmarkDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	// Deposit layer: bright foreground glyph per kind.
	// Index order matches DepositKind iota:
	//   0=None, 1=Iron, 2=Stone, 3=Timber, 4=Fertile, 5=Fish,
	//   6=Game, 7=Salt, 8=Gold, 9=Silver, 10=Gems, 11=Obsidian, 12=Sulfur.
	type depositEntry struct {
		glyph string
		style lipgloss.Style
	}
	depositEntries := [13]depositEntry{
		0:  {"  ", lipgloss.NewStyle()},                                                           // None — unused
		1:  {"* ", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true)},              // Iron — silver
		2:  {"▪ ", lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true)},              // Stone — grey
		3:  {"T ", lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true)},              // Timber — brown
		4:  {"~ ", lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true)},               // Fertile — green
		5:  {"~ ", lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)},               // Fish — light blue
		6:  {"^ ", lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)},              // Game — orange
		7:  {"· ", lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true)},              // Salt — white
		8:  {"$ ", lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true)},              // Gold — yellow
		9:  {"$ ", lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Bold(true)},              // Silver — light grey
		10: {"◆ ", lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true)},              // Gems — magenta
		11: {"▲ ", lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true)},              // Obsidian — dark grey
		12: {"% ", lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true)},              // Sulfur — bright yellow
	}
	for i, e := range depositEntries {
		depositKindCell[i] = e.style.Render(e.glyph)
	}
	depositDimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("236"))

	// Camp layer: faith background palette.
	// Index matches polity.Faith iota: 0=OldGods, 1=SunCovenant, 2=GreenSage,
	// 3=OneOath, 4=StormPact.
	campFaithBg = [5]lipgloss.Color{
		"#4a4a4a", // FaithOldGods     — dark grey (ancient, neutral)
		"#b8860b", // FaithSunCovenant — dark gold (solar)
		"#2e7d32", // FaithGreenSage   — forest green (nature)
		"#8b0000", // FaithOneOath     — dark crimson (martial)
		"#4682b4", // FaithStormPact   — steel blue (sea/storm)
	}

	// Camp layer: region-tinted foreground for anchor ("c") and foot ("o") glyphs.
	// regionFgColors mirrors the thematic palette used by regionTintBg but as
	// foreground hues so they read against any background.
	regionFgColors := [7]lipgloss.Color{
		"250", // RegionNormal   — light grey
		"245", // RegionBlighted — medium grey
		"135", // RegionFey      — purple
		"130", // RegionAncient  — brown
		"160", // RegionSavage   — red
		"231", // RegionHoly     — white
		"34",  // RegionWild     — green
	}
	for i, fg := range regionFgColors {
		anchorStyle := lipgloss.NewStyle().Foreground(fg).Bold(true)
		footStyle := lipgloss.NewStyle().Foreground(fg)
		campAnchorCell[i] = anchorStyle.Render("c ")
		campFootCell[i] = footStyle.Render("o ")
	}

	// Pre-render the faith-background overlay table: 7 regions × 5 faiths × 2
	// anchor states. Eliminates per-frame style construction when showCampFaithBg
	// is true.
	for regionIdx, fg := range regionFgColors {
		for faithIdx, bg := range campFaithBg {
			footStyle := lipgloss.NewStyle().Foreground(fg).Background(bg)
			anchorStyle := lipgloss.NewStyle().Foreground(fg).Background(bg).Bold(true)
			campFaithBgCell[regionIdx][faithIdx][0] = footStyle.Render("o ")
			campFaithBgCell[regionIdx][faithIdx][1] = anchorStyle.Render("c ")
		}
	}

	campDimCell = lipgloss.NewStyle().
		Foreground(lipgloss.Color("239")).
		Background(lipgloss.Color("233")).
		Render("  ")
}

func scalarBucket(v float32) int {
	if v < 0 {
		return 0
	}
	if v > 1 {
		v = 1
	}
	idx := int(v * float32(scalarBuckets-1))
	if idx >= scalarBuckets {
		return scalarBuckets - 1
	}
	return idx
}

// volcanoPriority maps volcanic terrain types to render priority.
// Core/dormant-core/crater are highest; slope mid; ashland low.
// Built once — not per call.
var volcanoPriority = map[gworld.Terrain]int{
	gworld.TerrainVolcanoCore:        4,
	gworld.TerrainVolcanoCoreDormant: 4,
	gworld.TerrainCraterLake:         3,
	gworld.TerrainVolcanoSlope:       2,
	gworld.TerrainAshland:            1,
}

// renderVolcanoCell returns a styled cell for the volcano layer. At zoom > 1
// the entire zoom×zoom sample block is scanned so volcanic tiles that fall
// between sample points are still visible. The highest-priority terrain hit
// wins (core > crater > slope > ashland).
func renderVolcanoCell(m *Model, wx, wy, zoom int) string {
	if m.volcanoSrc == nil {
		return volcanoDimCell
	}
	var bestT gworld.Terrain
	bestP := 0
	for dy := 0; dy < zoom; dy++ {
		for dx := 0; dx < zoom; dx++ {
			t, ok := m.volcanoSrc.TerrainOverrideAt(geom.Position{X: wx + dx, Y: wy + dy})
			if !ok {
				continue
			}
			if p, found := volcanoPriority[t]; found && p > bestP {
				bestP = p
				bestT = t
			}
		}
	}
	if bestP > 0 {
		if cell, found := volcanoTerrainCell[bestT]; found {
			return cell
		}
	}
	return volcanoDimCell
}

// renderRegionCell returns a styled cell for the regions layer. Each
// super-chunk cell gets a solid background tint tied to its character.
// Ocean tiles render as deep-ocean (no region tint over water) so only
// land shows character variation. Rivers are overlaid on top for non-ocean
// tiles so water is still visible.
func renderRegionCell(m *Model, wx, wy int, cellID uint32) string {
	if m.regionSrc == nil {
		return "  "
	}
	if m.world.IsOcean(cellID) {
		return landOceanCell
	}
	if m.world.IsRiver(wx, wy) {
		return riverCell
	}
	sc := geom.WorldToSuperChunk(wx, wy)
	char := m.regionSrc.RegionAt(sc).Character
	idx := int(char)
	if idx < 0 || idx >= len(regionCharCell) {
		idx = 0
	}
	return regionCharCell[idx]
}

// renderLandmarkCell returns a styled cell for the landmarks layer. The base
// tile is rendered in a dimmed biome style; if any landmark falls within the
// zoom×zoom sample block it is overlaid with a bright kind-specific glyph.
// At zoom=1 this degenerates to an exact-tile check. At zoom>1 the flat
// landmarkList (71 entries) is iterated — far cheaper than a 64×64 map scan.
func renderLandmarkCell(m *Model, wx, wy, zoom int, cellID uint32) string {
	if zoom == 1 {
		// Fast path: exact tile lookup.
		if m.landmarkIndex != nil {
			key := geom.PackPos(geom.Position{X: wx, Y: wy})
			if lm, ok := m.landmarkIndex[key]; ok {
				idx := int(lm.Kind)
				if idx > 0 && idx < len(landmarkKindCell) {
					return landmarkKindCell[idx]
				}
			}
		}
	} else {
		// Zoom > 1: bbox scan over the small landmark list.
		for _, lm := range m.landmarkList {
			if lm.Coord.X >= wx && lm.Coord.X < wx+zoom &&
				lm.Coord.Y >= wy && lm.Coord.Y < wy+zoom {
				idx := int(lm.Kind)
				if idx > 0 && idx < len(landmarkKindCell) {
					return landmarkKindCell[idx]
				}
			}
		}
	}
	// Fallback: dim version of the underlying terrain.
	t := m.world.Terrain[cellID]
	baseGlyph := tilestyle.GlyphFor(t)
	if baseGlyph == "" {
		baseGlyph = "·"
	}
	return landmarkDimStyle.Render(baseGlyph + " ")
}

// renderDepositCell returns a styled cell for the deposits layer. The base tile
// is rendered in a dimmed biome style; if any deposit falls within the zoom×zoom
// sample block it is overlaid with a kind-specific glyph. Ocean tiles render as
// the dim ocean base with no overlay.
func renderDepositCell(m *Model, wx, wy, zoom int, cellID uint32) string {
	// Dim base from underlying terrain.
	t := m.world.Terrain[cellID]
	baseGlyph := tilestyle.GlyphFor(t)
	if baseGlyph == "" {
		baseGlyph = "·"
	}
	base := depositDimStyle.Render(baseGlyph + " ")

	if m.depositIndex == nil {
		return base
	}

	if zoom == 1 {
		key := geom.PackPos(geom.Position{X: wx, Y: wy})
		if d, ok := m.depositIndex[key]; ok {
			idx := int(d.Kind)
			if idx > 0 && idx < len(depositKindCell) {
				return depositKindCell[idx]
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
				if idx > 0 && idx < len(depositKindCell) {
					return depositKindCell[idx]
				}
			}
		}
	}
	return base
}

// renderCampCell returns a styled cell for the camps layer. Anchor tiles
// render as "c", other footprint tiles as "o"; both are tinted by the
// camp's RegionCharacter. When showCampFaithBg is true the background is
// coloured by the camp's Faith. Non-camp tiles render as campDimCell.
func renderCampCell(m *Model, wx, wy, zoom int) string {
	if m.campIndex == nil {
		return campDimCell
	}

	// At zoom=1 use the exact-tile fast path; at zoom>1 scan the block.
	var found bool
	var camp polity.Camp
	if zoom == 1 {
		key := geom.PackPos(geom.Position{X: wx, Y: wy})
		camp, found = m.campIndex[key]
	} else {
		for dy := 0; dy < zoom && !found; dy++ {
			for dx := 0; dx < zoom && !found; dx++ {
				key := geom.PackPos(geom.Position{X: wx + dx, Y: wy + dy})
				if c, ok := m.campIndex[key]; ok {
					camp = c
					found = true
				}
			}
		}
	}
	if !found {
		return campDimCell
	}

	regionIdx := int(camp.Region)
	if regionIdx < 0 || regionIdx >= len(campAnchorCell) {
		regionIdx = 0
	}

	isAnchor := (zoom == 1 && geom.PackPos(geom.Position{X: wx, Y: wy}) == geom.PackPos(camp.Position)) ||
		(zoom > 1 && wx <= camp.Position.X && camp.Position.X < wx+zoom &&
			wy <= camp.Position.Y && camp.Position.Y < wy+zoom)

	if !m.showCampFaithBg {
		if isAnchor {
			return campAnchorCell[regionIdx]
		}
		return campFootCell[regionIdx]
	}

	// Faith background overlay: look up the pre-rendered cell (built once at
	// init) to avoid per-frame style construction.
	faithIdx := int(camp.Faiths.Majority())
	if faithIdx < 0 || faithIdx >= len(campFaithBg) {
		faithIdx = 0
	}
	anchorBit := 0
	if isAnchor {
		anchorBit = 1
	}
	return campFaithBgCell[regionIdx][faithIdx][anchorBit]
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

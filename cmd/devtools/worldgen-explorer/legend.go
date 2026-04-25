package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// Static legend strings built once at init — all rendering in the legend is
// pre-computed so renderLegend() does nothing but string concatenation.
var (
	legendBiomes    string
	legendLandmarks string
	legendDeposits  string
	legendVolcanic  string
	legendRegions   string
	legendCamps     string
)

// legendLabelStyle renders the category label that prefixes each legend row.
var legendLabelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("243")).
	Bold(true)

func init() {
	legendBiomes = buildBiomeLegend()
	legendLandmarks = buildLandmarkLegend()
	legendDeposits = buildDepositLegend()
	legendVolcanic = buildVolcanicLegend()
	legendRegions = buildRegionLegend()
	legendCamps = buildCampLegend()
}

// terrainLegendOrder is the display sequence for the biome legend row.
// Ocean/deep-ocean are omitted — they never appear on land layers and
// would clutter the line.
var terrainLegendOrder = []struct {
	t    gworld.Terrain
	name string
}{
	{gworld.TerrainGrassland, "Grass"},
	{gworld.TerrainPlains, "Plains"},
	{gworld.TerrainMeadow, "Meadow"},
	{gworld.TerrainForest, "Forest"},
	{gworld.TerrainJungle, "Jungle"},
	{gworld.TerrainTaiga, "Taiga"},
	{gworld.TerrainSnow, "Snow"},
	{gworld.TerrainTundra, "Tundra"},
	{gworld.TerrainDesert, "Desert"},
	{gworld.TerrainSavanna, "Savanna"},
	{gworld.TerrainBeach, "Beach"},
	{gworld.TerrainHills, "Hills"},
	{gworld.TerrainMountain, "Mountain"},
	{gworld.TerrainSnowyPeak, "Peak"},
	{gworld.TerrainOcean, "Ocean"},
	{gworld.TerrainDeepOcean, "Deep"},
}

func buildBiomeLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("BIOMES: "))
	for _, e := range terrainLegendOrder {
		glyph := tilestyle.GlyphFor(e.t)
		if glyph == "" {
			glyph = "?"
		}
		sb.WriteString(tilestyle.StyleFor(e.t).Render(glyph))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// landmarkLegendEntries lists every non-None landmark in iota order.
var landmarkLegendEntries = []struct {
	glyph string
	style lipgloss.Style
	name  string
}{
	{"T", lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true), "Tower"},
	{"Y", lipgloss.NewStyle().Foreground(lipgloss.Color("82")).Bold(true), "Giant Tree"},
	{"o", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true), "Stones"},
	{"I", lipgloss.NewStyle().Foreground(lipgloss.Color("135")).Bold(true), "Obelisk"},
	{"V", lipgloss.NewStyle().Foreground(lipgloss.Color("124")).Bold(true), "Chasm"},
	{"+", lipgloss.NewStyle().Foreground(lipgloss.Color("87")).Bold(true), "Shrine"},
}

func buildLandmarkLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("LANDMARKS: "))
	for _, e := range landmarkLegendEntries {
		sb.WriteString(e.style.Render(e.glyph))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// depositLegendEntries lists every non-None deposit kind in iota order,
// mirroring the styles in render.go exactly.
var depositLegendEntries = []struct {
	glyph string
	style lipgloss.Style
	name  string
}{
	{"*", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true), "Iron"},
	{"▪", lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true), "Stone"},
	{"T", lipgloss.NewStyle().Foreground(lipgloss.Color("130")).Bold(true), "Timber"},
	{"~", lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true), "Fertile"},
	{"~", lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true), "Fish"},
	{"^", lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true), "Game"},
	{"·", lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true), "Salt"},
	{"$", lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true), "Gold"},
	{"$", lipgloss.NewStyle().Foreground(lipgloss.Color("247")).Bold(true), "Silver"},
	{"◆", lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true), "Gems"},
	{"▲", lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Bold(true), "Obsidian"},
	{"%", lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Bold(true), "Sulfur"},
}

func buildDepositLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("DEPOSITS: "))
	for _, e := range depositLegendEntries {
		sb.WriteString(e.style.Render(e.glyph))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// volcanicLegendEntries lists the 5 volcanic terrain types in priority order.
var volcanicLegendEntries = []struct {
	t    gworld.Terrain
	name string
}{
	{gworld.TerrainVolcanoCore, "Core"},
	{gworld.TerrainVolcanoCoreDormant, "Dormant"},
	{gworld.TerrainCraterLake, "Crater"},
	{gworld.TerrainVolcanoSlope, "Slope"},
	{gworld.TerrainAshland, "Ashland"},
}

func buildVolcanicLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("VOLCANIC: "))
	for _, e := range volcanicLegendEntries {
		glyph := tilestyle.GlyphFor(e.t)
		if glyph == "" {
			glyph = "?"
		}
		sb.WriteString(tilestyle.StyleFor(e.t).Render(glyph))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// regionLegendEntries lists all 7 region characters in iota order,
// mirroring the regionPalette in render.go exactly.
var regionLegendEntries = []struct {
	t    gworld.Terrain
	name string
}{
	{gworld.TerrainGrassland, "Normal"},
	{gworld.TerrainTundra, "Blighted"},
	{gworld.TerrainMeadow, "Fey"},
	{gworld.TerrainHills, "Ancient"},
	{gworld.TerrainDesert, "Savage"},
	{gworld.TerrainSnow, "Holy"},
	{gworld.TerrainForest, "Wild"},
}

func buildRegionLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("REGIONS: "))
	for _, e := range regionLegendEntries {
		sb.WriteString(tilestyle.StyleFor(e.t).Reverse(true).Render("  "))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// campLegendGlyphEntries lists the two camp glyph types with their styles.
var campLegendGlyphEntries = []struct {
	glyph string
	style lipgloss.Style
	name  string
}{
	{"c", lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Bold(true), "anchor"},
	{"o", lipgloss.NewStyle().Foreground(lipgloss.Color("250")), "footprint"},
}

// campFaithLegendEntries lists every faith with its background colour.
var campFaithLegendEntries = []struct {
	name string
	bg   lipgloss.Color
}{
	{"OldGods", "#4a4a4a"},
	{"SunCovenant", "#b8860b"},
	{"GreenSage", "#2e7d32"},
	{"OneOath", "#8b0000"},
	{"StormPact", "#4682b4"},
}

func buildCampLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("CAMPS: "))
	for _, e := range campLegendGlyphEntries {
		sb.WriteString(e.style.Render(e.glyph))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	sb.WriteString(legendLabelStyle.Render(" fg=Region "))
	for i, e := range regionLegendEntries {
		sb.WriteString(tilestyle.StyleFor(e.t).Reverse(true).Render(" "))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		if i < len(regionLegendEntries)-1 {
			sb.WriteString("  ")
		}
	}
	sb.WriteString(legendLabelStyle.Render("  bg=Faith(c to toggle): "))
	for _, e := range campFaithLegendEntries {
		sb.WriteString(lipgloss.NewStyle().Background(e.bg).Render("  "))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// renderLegend assembles the context-aware legend for the current layer.
// Always shows the biome row; appends layer-specific rows below it.
func (m Model) renderLegend() string {
	var sb strings.Builder
	sb.WriteString(legendBiomes)
	sb.WriteByte('\n')

	switch m.layer {
	case layerLandmarks, layerBiome:
		sb.WriteString(legendLandmarks)
		sb.WriteByte('\n')
		if m.layer == layerBiome {
			sb.WriteString(legendVolcanic)
			sb.WriteByte('\n')
		}
	case layerDeposits:
		sb.WriteString(legendDeposits)
		sb.WriteByte('\n')
	case layerVolcanoes:
		sb.WriteString(legendVolcanic)
		sb.WriteByte('\n')
	case layerRegions:
		sb.WriteString(legendRegions)
		sb.WriteByte('\n')
	case layerCamps:
		sb.WriteString(legendCamps)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// legendLineCount returns how many terminal lines the legend occupies for
// the given layer, so viewportSize can shrink the viewport accordingly.
func legendLineCount(l layer) int {
	switch l {
	case layerBiome:
		// biomes + landmarks + volcanic
		return 3
	case layerLandmarks:
		// biomes + landmarks
		return 2
	case layerDeposits, layerVolcanoes, layerRegions, layerCamps:
		// biomes + one layer row
		return 2
	default:
		// biomes only
		return 1
	}
}

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
	legendSettlements string
	legendDeposits    string
	legendBiomes      string
)

// legendLabelStyle renders the category label that prefixes each legend row.
var legendLabelStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("243")).
	Bold(true)

func init() {
	legendSettlements = buildSettlementsLegend()
	legendDeposits = buildDepositsLegend()
	legendBiomes = buildBiomeLegend()
}

// terrainLegendOrder is the display sequence for the biome legend row.
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

// settlementLegendEntries lists the three tiers with anchor + footprint glyphs.
var settlementLegendEntries = []struct {
	anchor string
	foot   string
	fg     lipgloss.Color
	name   string
}{
	{"C", "c", "250", "Camp (anchor / footprint)"},
	{"H", "h", "253", "Hamlet (anchor / footprint)"},
	{"V", "v", "255", "Village (anchor / footprint)"},
}

func buildSettlementsLegend() string {
	var sb strings.Builder
	sb.WriteString(legendLabelStyle.Render("SETTLEMENTS: "))
	for _, e := range settlementLegendEntries {
		anchorSty := lipgloss.NewStyle().Foreground(e.fg).Bold(true)
		footSty := lipgloss.NewStyle().Foreground(e.fg)
		sb.WriteString(anchorSty.Render(e.anchor))
		sb.WriteString("/")
		sb.WriteString(footSty.Render(e.foot))
		sb.WriteByte(' ')
		sb.WriteString(e.name)
		sb.WriteString("  ")
	}
	return sb.String()
}

// depositLegendEntries mirrors worldgen-explorer's deposit palette exactly.
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

func buildDepositsLegend() string {
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

// renderLegend assembles the context-aware legend for the current layer.
// Always shows the biome row; appends a layer-specific row below it.
func (m Model) renderLegend() string {
	var sb strings.Builder
	sb.WriteString(legendBiomes)
	sb.WriteByte('\n')
	switch m.layer {
	case layerSettlements:
		sb.WriteString(legendSettlements)
		sb.WriteByte('\n')
	case layerResources:
		sb.WriteString(legendDeposits)
		sb.WriteByte('\n')
	}
	return sb.String()
}

// legendLineCount returns how many terminal lines the legend occupies for
// the given layer, so viewportSize can shrink the viewport accordingly.
func legendLineCount(l simLayer) int {
	// biomes row + one layer-specific row for every layer.
	return 2
}

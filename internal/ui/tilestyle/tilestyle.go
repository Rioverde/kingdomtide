// Package tilestyle is the single source of truth for rendering
// conventions — glyph + colour per world.Terrain value.
//
// It sits between the game's domain types (internal/game/world) and
// the display layer. The main client UI (internal/ui) consumes it
// after converting wire-protocol Terrain to world.Terrain via FromPB;
// developer tools (cmd/devtools/worldgen-explorer) consume it
// directly with world.Terrain keys. Both paths render the same glyph
// and colour — no duplication.
//
// Edit the TerrainRunes table alongside TerrainStyles to re-skin the
// map. Runes and styles are kept in separate tables because they are
// tuned independently (glyph width vs. ANSI palette).
package tilestyle

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/world"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// TerrainRunes maps every world.Terrain value to the rune shown for
// that biome. Missing keys should fall back to a caller-chosen default
// (e.g. "?" for unspecified).
//
// Glyphs are single display-cell wide in well-behaved terminals. Avoid
// double-width characters (emoji variants with VS16) — those skew the
// grid.
var TerrainRunes = map[world.Terrain]string{
	world.TerrainPlains:    "·",
	world.TerrainGrassland: "„",
	world.TerrainMeadow:    "❀",

	world.TerrainBeach:   "░",
	world.TerrainSavanna: "⁖",
	world.TerrainDesert:  "∙",

	world.TerrainForest: "♣",
	world.TerrainJungle: "♠",
	world.TerrainTaiga:  "♤",

	world.TerrainTundra: "‥",
	world.TerrainSnow:   "∗",

	world.TerrainHills:      "∩",
	world.TerrainMountain:   "▲",
	world.TerrainSnowyPeak:  "△",

	world.TerrainOcean:     "≈",
	world.TerrainDeepOcean: "≋",

	world.TerrainVolcanoCore:        "▼",
	world.TerrainVolcanoCoreDormant: "○",
	world.TerrainCraterLake:         "⊙",
	world.TerrainVolcanoSlope:       "◭",
	world.TerrainAshland:            "▒",
}

// TerrainStyles pairs each biome with its lipgloss style (foreground
// and optional background). Palette matches the main client UI.
var TerrainStyles = map[world.Terrain]lipgloss.Style{
	world.TerrainPlains:    lipgloss.NewStyle().Foreground(lipgloss.Color("108")),
	world.TerrainGrassland: lipgloss.NewStyle().Foreground(lipgloss.Color("70")),
	world.TerrainMeadow:    lipgloss.NewStyle().Foreground(lipgloss.Color("119")),
	world.TerrainBeach:     lipgloss.NewStyle().Foreground(lipgloss.Color("221")),
	world.TerrainDesert:    lipgloss.NewStyle().Foreground(lipgloss.Color("222")),
	world.TerrainSavanna:   lipgloss.NewStyle().Foreground(lipgloss.Color("143")),
	world.TerrainForest:    lipgloss.NewStyle().Foreground(lipgloss.Color("22")),
	world.TerrainJungle:    lipgloss.NewStyle().Foreground(lipgloss.Color("28")),
	world.TerrainTaiga:     lipgloss.NewStyle().Foreground(lipgloss.Color("30")),
	world.TerrainTundra:    lipgloss.NewStyle().Foreground(lipgloss.Color("152")),
	world.TerrainSnow:      lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
	world.TerrainHills:     lipgloss.NewStyle().Foreground(lipgloss.Color("94")),
	world.TerrainMountain:  lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true),
	world.TerrainSnowyPeak: lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true),
	world.TerrainOcean:     lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
	world.TerrainDeepOcean: lipgloss.NewStyle().Foreground(lipgloss.Color("18")),

	world.TerrainVolcanoCore: lipgloss.NewStyle().
		Foreground(lipgloss.Color("202")).
		Background(lipgloss.Color("52")).
		Bold(true),
	world.TerrainVolcanoCoreDormant: lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Background(lipgloss.Color("235")),
	world.TerrainCraterLake: lipgloss.NewStyle().
		Foreground(lipgloss.Color("39")).
		Background(lipgloss.Color("24")),
	world.TerrainVolcanoSlope: lipgloss.NewStyle().
		Foreground(lipgloss.Color("130")).
		Background(lipgloss.Color("237")),
	world.TerrainAshland: lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Background(lipgloss.Color("234")),
}

// GlyphFor returns the rune registered for a terrain, or empty string
// when the terrain is unknown. Callers supply their own fallback.
func GlyphFor(t world.Terrain) string {
	return TerrainRunes[t]
}

// StyleFor returns the style registered for a terrain, or a zero
// lipgloss.Style when the terrain is unknown.
func StyleFor(t world.Terrain) lipgloss.Style {
	return TerrainStyles[t]
}

// pbToTerrain translates a wire-protocol terrain enum back to its
// domain value so the UI package (which holds pb.Terrain from the
// wire) can feed into the world-keyed tables above.
var pbToTerrain = map[pb.Terrain]world.Terrain{
	pb.Terrain_TERRAIN_PLAINS:               world.TerrainPlains,
	pb.Terrain_TERRAIN_GRASSLAND:            world.TerrainGrassland,
	pb.Terrain_TERRAIN_MEADOW:               world.TerrainMeadow,
	pb.Terrain_TERRAIN_BEACH:                world.TerrainBeach,
	pb.Terrain_TERRAIN_DESERT:               world.TerrainDesert,
	pb.Terrain_TERRAIN_SAVANNA:              world.TerrainSavanna,
	pb.Terrain_TERRAIN_FOREST:               world.TerrainForest,
	pb.Terrain_TERRAIN_JUNGLE:               world.TerrainJungle,
	pb.Terrain_TERRAIN_TAIGA:                world.TerrainTaiga,
	pb.Terrain_TERRAIN_TUNDRA:               world.TerrainTundra,
	pb.Terrain_TERRAIN_SNOW:                 world.TerrainSnow,
	pb.Terrain_TERRAIN_HILLS:                world.TerrainHills,
	pb.Terrain_TERRAIN_MOUNTAIN:             world.TerrainMountain,
	pb.Terrain_TERRAIN_SNOWY_PEAK:           world.TerrainSnowyPeak,
	pb.Terrain_TERRAIN_OCEAN:                world.TerrainOcean,
	pb.Terrain_TERRAIN_DEEP_OCEAN:           world.TerrainDeepOcean,
	pb.Terrain_TERRAIN_VOLCANO_CORE:         world.TerrainVolcanoCore,
	pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT: world.TerrainVolcanoCoreDormant,
	pb.Terrain_TERRAIN_CRATER_LAKE:          world.TerrainCraterLake,
	pb.Terrain_TERRAIN_VOLCANO_SLOPE:        world.TerrainVolcanoSlope,
	pb.Terrain_TERRAIN_ASHLAND:              world.TerrainAshland,
}

// FromPB translates a wire-protocol terrain to its domain value.
// Unknown values return the empty world.Terrain ("").
func FromPB(p pb.Terrain) world.Terrain {
	return pbToTerrain[p]
}

// GlyphForPB looks up the glyph for a wire-protocol terrain — a
// convenience wrapper for callers holding pb.Terrain.
func GlyphForPB(p pb.Terrain) string {
	return GlyphFor(FromPB(p))
}

// StyleForPB looks up the style for a wire-protocol terrain.
func StyleForPB(p pb.Terrain) lipgloss.Style {
	return StyleFor(FromPB(p))
}

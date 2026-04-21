package ui

import (
	"github.com/charmbracelet/lipgloss"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// Styles are the lipgloss decorations (colour, bold, reverse…) applied to
// rendered cells. Runes live in runes.go; styles live here. Separating
// them lets you retune colour without touching the glyph table.
var styles = struct {
	selfPlayer  lipgloss.Style
	otherPlayer lipgloss.Style
	river       lipgloss.Style
	village     lipgloss.Style
	castle      lipgloss.Style
	unknownTile lipgloss.Style

	title   lipgloss.Style
	box     lipgloss.Style
	status  lipgloss.Style
	prompt  lipgloss.Style
	input   lipgloss.Style
	cursor  lipgloss.Style
	log     lipgloss.Style
	playerL lipgloss.Style
	errBox  lipgloss.Style
}{
	selfPlayer:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
	otherPlayer: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
	river:       lipgloss.NewStyle().Foreground(lipgloss.Color("45")),
	village:     lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true), // warm orange
	castle:      lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true), // red
	unknownTile: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

	title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1),
	box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 2),
	status: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
	prompt: lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
	input:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
	cursor: lipgloss.NewStyle().Reverse(true),
	log: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).Padding(0, 1),
	playerL: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).Padding(0, 1),
	errBox: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9")).Foreground(lipgloss.Color("9")).Padding(1, 2),
}

// terrainStyles pairs each biome with its foreground colour. Edit this
// table alongside runes.go to re-skin the map.
var terrainStyles = map[pb.Terrain]lipgloss.Style{
	pb.Terrain_TERRAIN_PLAINS:     lipgloss.NewStyle().Foreground(lipgloss.Color("108")),
	pb.Terrain_TERRAIN_GRASSLAND:  lipgloss.NewStyle().Foreground(lipgloss.Color("70")),
	pb.Terrain_TERRAIN_MEADOW:     lipgloss.NewStyle().Foreground(lipgloss.Color("119")),
	pb.Terrain_TERRAIN_BEACH:      lipgloss.NewStyle().Foreground(lipgloss.Color("221")),
	pb.Terrain_TERRAIN_DESERT:     lipgloss.NewStyle().Foreground(lipgloss.Color("222")),
	pb.Terrain_TERRAIN_SAVANNA:    lipgloss.NewStyle().Foreground(lipgloss.Color("143")),
	pb.Terrain_TERRAIN_FOREST:     lipgloss.NewStyle().Foreground(lipgloss.Color("22")),
	pb.Terrain_TERRAIN_JUNGLE:     lipgloss.NewStyle().Foreground(lipgloss.Color("28")),
	pb.Terrain_TERRAIN_TAIGA:      lipgloss.NewStyle().Foreground(lipgloss.Color("30")),
	pb.Terrain_TERRAIN_TUNDRA:     lipgloss.NewStyle().Foreground(lipgloss.Color("152")),
	pb.Terrain_TERRAIN_SNOW:       lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
	pb.Terrain_TERRAIN_HILLS:      lipgloss.NewStyle().Foreground(lipgloss.Color("94")),
	pb.Terrain_TERRAIN_MOUNTAIN:   lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true),
	pb.Terrain_TERRAIN_SNOWY_PEAK: lipgloss.NewStyle().Foreground(lipgloss.Color("231")).Bold(true),
	pb.Terrain_TERRAIN_OCEAN:      lipgloss.NewStyle().Foreground(lipgloss.Color("33")),
	pb.Terrain_TERRAIN_DEEP_OCEAN: lipgloss.NewStyle().Foreground(lipgloss.Color("18")),
}

// lookTile returns the rune + style for a wire tile. Rivers override the
// biome rune with a cyan stroke so water routes stand out against land.
func lookTile(t *pb.Tile) (string, lipgloss.Style) {
	if t == nil {
		return RuneUnspecified, styles.unknownTile
	}
	if t.GetRiver() {
		return RiverRune, styles.river
	}
	r, ok := TerrainRunes[t.GetTerrain()]
	if !ok {
		return RuneUnspecified, styles.unknownTile
	}
	style, ok := terrainStyles[t.GetTerrain()]
	if !ok {
		return r, styles.unknownTile
	}
	return r, style
}

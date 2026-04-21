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

	// logJoin and logLeave style typed event entries in the events panel.
	// logJoin uses ANSI green (#5fd75f) for join events; logLeave uses a
	// muted grey (#d0d0d0) for leave events. logDefault is the uncoloured
	// fallback for all other log lines.
	logJoin    lipgloss.Style
	logLeave   lipgloss.Style
	logDefault lipgloss.Style
}{
	selfPlayer:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
	otherPlayer: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
	river:       lipgloss.NewStyle().Foreground(lipgloss.Color("45")),
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

	logJoin:    lipgloss.NewStyle().Foreground(lipgloss.Color("#5fd75f")),
	logLeave:   lipgloss.NewStyle().Foreground(lipgloss.Color("#d0d0d0")),
	logDefault: lipgloss.NewStyle(),
}

// landmarkStyles pairs each LandmarkKind with its foreground style. Landmarks
// are rendered without region tint — their distinctive colours are already
// visually salient enough that tinting would muddy them.
var landmarkStyles = map[pb.LandmarkKind]lipgloss.Style{
	pb.LandmarkKind_LANDMARK_KIND_TOWER:           lipgloss.NewStyle().Foreground(lipgloss.Color("201")).Bold(true),
	pb.LandmarkKind_LANDMARK_KIND_GIANT_TREE:      lipgloss.NewStyle().Foreground(lipgloss.Color("34")).Bold(true),
	pb.LandmarkKind_LANDMARK_KIND_STANDING_STONES: lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Bold(true),
	pb.LandmarkKind_LANDMARK_KIND_OBELISK:         lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Bold(true),
	pb.LandmarkKind_LANDMARK_KIND_CHASM:           lipgloss.NewStyle().Foreground(lipgloss.Color("160")),
	pb.LandmarkKind_LANDMARK_KIND_SHRINE:          lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Bold(true),
}

// structureStyles pairs each wire Structure with its foreground style.
// Edit alongside structureRunes to re-skin village/castle overlays.
var structureStyles = map[pb.Structure]lipgloss.Style{
	pb.Structure_STRUCTURE_VILLAGE: lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true),
	pb.Structure_STRUCTURE_CASTLE:  lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true),
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

// lookTile returns the rune + style for a wire tile's terrain. Overlay
// handling (river, road, bridge, ...) lives in renderCell — lookTile is a
// pure terrain → (rune, style) lookup with a graceful fallback for
// version-skew biomes the client doesn't know about.
func lookTile(t *pb.Tile) (string, lipgloss.Style) {
	if t == nil {
		return runeUnspecified, styles.unknownTile
	}
	r, ok := terrainRunes[t.GetTerrain()]
	if !ok {
		return runeUnspecified, styles.unknownTile
	}
	style, ok := terrainStyles[t.GetTerrain()]
	if !ok {
		return r, styles.unknownTile
	}
	return r, style
}

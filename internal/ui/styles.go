package ui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/tilestyle"
)

// Styles are the lipgloss decorations (colour, bold, reverse…) applied to
// rendered cells. Runes live in runes.go; styles live here. Separating
// them lets you retune colour without touching the glyph table.
var styles = struct {
	selfPlayer  lipgloss.Style
	otherPlayer lipgloss.Style
	river       lipgloss.Style
	unknownTile lipgloss.Style

	title  lipgloss.Style
	box    lipgloss.Style
	status lipgloss.Style
	// rule styles the horizontal divider lines inside the map box
	// (above the grid, above the status strip). Neutral soft-white so
	// the rules read as secondary chrome without pulling attention
	// like the yellow status tint.
	rule    lipgloss.Style
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

	// hpBar and mpBar tint the HP/MP progress bars in the stats panel.
	// Red for hit points, blue for mana — the conventional roguelike
	// colour pairing so the player can parse the resource at a glance
	// without reading the label.
	hpBar lipgloss.Style
	mpBar lipgloss.Style
}{
	selfPlayer:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true),
	otherPlayer: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true),
	river:       lipgloss.NewStyle().Foreground(lipgloss.Color("45")),
	unknownTile: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),

	title:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Padding(0, 1),
	box:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(1, 2),
	status: lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
	rule:   lipgloss.NewStyle().Foreground(lipgloss.Color("250")), // #bcbcbc soft white
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

	hpBar: lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")),
	mpBar: lipgloss.NewStyle().Foreground(lipgloss.Color("#5fafff")),
}

// seasonStyles tints the top-bar calendar date block with a foreground
// colour that reads as the current season at a glance. Hex comments
// document the 256-colour xterm codes so a reader does not have to
// reverse-lookup the palette.
//
//	Winter (153) pale blue   — cold air mnemonic
//	Spring (120) pale green  — new growth mnemonic
//	Summer (220) warm yellow — sun mnemonic
//	Autumn (166) burnt orange — falling leaves mnemonic
var seasonStyles = map[calendar.Season]lipgloss.Style{
	calendar.SeasonWinter: lipgloss.NewStyle().Foreground(lipgloss.Color("153")),
	calendar.SeasonSpring: lipgloss.NewStyle().Foreground(lipgloss.Color("120")),
	calendar.SeasonSummer: lipgloss.NewStyle().Foreground(lipgloss.Color("220")),
	calendar.SeasonAutumn: lipgloss.NewStyle().Foreground(lipgloss.Color("166")),
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

// Terrain styles live in internal/ui/tilestyle (keyed by world.Terrain
// so both this package and developer tools share one source of truth).
// Look them up via tilestyle.StyleForPB when you have a pb.Terrain.

// lookTile returns the rune + style for a wire tile's terrain. Overlay
// handling (river, road, bridge, ...) lives in renderCell — lookTile is a
// pure terrain → (rune, style) lookup with a graceful fallback for
// version-skew biomes the client doesn't know about.
func lookTile(t *pb.Tile) (string, lipgloss.Style) {
	if t == nil {
		return runeUnspecified, styles.unknownTile
	}
	r := tilestyle.GlyphForPB(t.GetTerrain())
	if r == "" {
		return runeUnspecified, styles.unknownTile
	}
	style := tilestyle.StyleForPB(t.GetTerrain())
	return r, style
}

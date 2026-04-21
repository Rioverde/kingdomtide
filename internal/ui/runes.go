package ui

import pb "github.com/Rioverde/gongeons/internal/proto"

// This file is the single source of truth for every glyph that reaches the
// screen. Colours live in styles.go; rune choice lives here so you can
// rebalance the visual vocabulary (swap to Unicode block characters, or to
// a fantasy-roguelike alternative set) without touching anything else.

// Occupant overlay runes. Occupants win over terrain when a tile is
// rendered — they sit "on top of" the biome glyph.
const (
	RuneSelf        = "@"
	RuneOther       = "P"
	RuneUnspecified = "?"
)

// RiverRune is painted on tiles the procedural generator marked as part of
// a river. It sits over the underlying biome.
const RiverRune = "~"

// UI chrome runes — every string literal that shows up outside the map
// grid. Centralised here so a redesign touches one file.
const (
	InputPrompt    = "> "
	LogBullet      = "*"
	StatusDivider  = "  |  "
	EmptyLogLabel  = "(quiet)"
	EmptyMapLabel  = "(no map yet)"
	EmptyListLabel = "(none)"
	QuitHint       = "q to quit"
	QuitLongHint   = "press Enter to connect, q to quit"
	PlayersHeader  = "Players"
	EventsHeader   = "Events"
	TitleText      = "Gongeons"
	DisconnectHint = "press q to quit"
)

// TerrainRunes maps every wire-level Terrain value to the rune shown for
// that biome. Missing keys fall back to RuneUnspecified in the renderer.
var TerrainRunes = map[pb.Terrain]string{
	// Grass family: rune weight grows with vegetation density.
	pb.Terrain_TERRAIN_PLAINS:    ".",
	pb.Terrain_TERRAIN_GRASSLAND: ",",
	pb.Terrain_TERRAIN_MEADOW:    "\"",

	// Arid family: grainy punctuation, beach to dune horizon.
	pb.Terrain_TERRAIN_BEACH:   ":",
	pb.Terrain_TERRAIN_SAVANNA: ";",
	pb.Terrain_TERRAIN_DESERT:  "-",

	// Forest family: deciduous / tangled canopy / conifer.
	pb.Terrain_TERRAIN_FOREST: "T",
	pb.Terrain_TERRAIN_JUNGLE: "&",
	pb.Terrain_TERRAIN_TAIGA:  "Y",

	// Cold flats.
	pb.Terrain_TERRAIN_TUNDRA: "'",
	pb.Terrain_TERRAIN_SNOW:   "*",

	// Relief ladder: bump -> peak -> snow-capped peak.
	pb.Terrain_TERRAIN_HILLS:      "n",
	pb.Terrain_TERRAIN_MOUNTAIN:   "^",
	pb.Terrain_TERRAIN_SNOWY_PEAK: "A",

	// Water: wave -> flat depth.
	pb.Terrain_TERRAIN_OCEAN:      "~",
	pb.Terrain_TERRAIN_DEEP_OCEAN: "=",
}

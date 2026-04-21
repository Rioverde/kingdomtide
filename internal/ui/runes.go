package ui

import pb "github.com/Rioverde/gongeons/internal/proto"

// This file is the single source of truth for every glyph that reaches the
// screen. Colours live in styles.go; rune choice lives here so you can
// rebalance the visual vocabulary without touching anything else.
//
// Everything below is a single display-cell wide in well-behaved terminals.
// If a glyph renders as double-width (most commonly emoji variants with the
// VS16 selector), the grid will visually skew — swap to a narrower one.

// Occupant overlay runes. Occupants win over terrain when a tile is
// rendered — they sit "on top of" the biome glyph.
const (
	runeSelf        = "@" // classic roguelike player — universal, unambiguous
	runeOther       = "♟" // BMP U+265F BLACK CHESS PAWN, single-cell, pairs with castle rook
	runeUnspecified = "?"
)

// riverRune is painted on tiles the procedural generator marked as part of
// a river. It sits over the underlying biome.
const riverRune = "≈"

// lakeRune is painted on tiles the worldgen river tracer marked as standing
// water (OverlayLake) — depressions the trace filled as lake basins. Triple
// tilde matches the deep-ocean glyph to signal "still water" visually: no
// flow, no wavelets. The underlying biome is preserved under the bit so a
// drained lakebed still reads correctly.
const lakeRune = "≋"

// landmarkRunes maps each LandmarkKind to its single-cell glyph. The zero
// value LANDMARK_KIND_NONE is intentionally absent: a missing key means
// "no landmark here" and the renderer skips the landmark branch entirely.
var landmarkRunes = map[pb.LandmarkKind]string{
	pb.LandmarkKind_LANDMARK_KIND_TOWER:           "▲",
	pb.LandmarkKind_LANDMARK_KIND_GIANT_TREE:      "♣",
	pb.LandmarkKind_LANDMARK_KIND_STANDING_STONES: "◊",
	pb.LandmarkKind_LANDMARK_KIND_OBELISK:         "┃",
	pb.LandmarkKind_LANDMARK_KIND_CHASM:           "╳",
	pb.LandmarkKind_LANDMARK_KIND_SHRINE:          "✦",
}

// structureRunes maps village / castle / etc. to the glyph drawn in place
// of the terrain rune. Rendered below players but above rivers and terrain.
var structureRunes = map[pb.Structure]string{
	pb.Structure_STRUCTURE_VILLAGE: "⌂", // BMP U+2302 HOUSE
	pb.Structure_STRUCTURE_CASTLE:  "♜", // BMP U+265C BLACK CHESS ROOK
}

// terrainRunes maps every wire-level Terrain value to the rune shown for
// that biome. Missing keys fall back to runeUnspecified in the renderer.
var terrainRunes = map[pb.Terrain]string{
	// Grass family: progressively denser vegetation.
	pb.Terrain_TERRAIN_PLAINS:    "·", // middle dot
	pb.Terrain_TERRAIN_GRASSLAND: "„", // low double comma
	pb.Terrain_TERRAIN_MEADOW:    "❀", // flower

	// Arid family: grainy textures, beach → dune.
	pb.Terrain_TERRAIN_BEACH:   "░", // light shade
	pb.Terrain_TERRAIN_SAVANNA: "⁖", // four-dot cluster
	pb.Terrain_TERRAIN_DESERT:  "∙", // bullet operator

	// Forest family: deciduous / tangled / conifer.
	pb.Terrain_TERRAIN_FOREST: "♣", // club (canopy)
	pb.Terrain_TERRAIN_JUNGLE: "♠", // spade (denser canopy)
	pb.Terrain_TERRAIN_TAIGA:  "♤", // white spade (conifer)

	// Cold flats.
	pb.Terrain_TERRAIN_TUNDRA: "‥", // two-dot leader (sparse)
	pb.Terrain_TERRAIN_SNOW:   "∗", // asterisk operator (non-emoji, narrow)

	// Relief ladder: bump → peak → snow-capped peak.
	pb.Terrain_TERRAIN_HILLS:      "∩", // inverted cup
	pb.Terrain_TERRAIN_MOUNTAIN:   "▲", // solid up-triangle
	pb.Terrain_TERRAIN_SNOWY_PEAK: "△", // hollow up-triangle

	// Water: wave → heavy wave.
	pb.Terrain_TERRAIN_OCEAN:      "≈", // approximately equal (wavelets)
	pb.Terrain_TERRAIN_DEEP_OCEAN: "≋", // triple tilde (deeper)
}

// UI chrome glyphs — visual separators and bullets that stay stable across
// locales. User-facing strings (labels, hints, headers) now flow through
// the locale catalog; see locale/active.*.toml.
const (
	// LogBullet precedes every event-log entry. Shared across translations
	// because it is purely visual; changing it is a theme decision.
	LogBullet = "•"

	// StatusDivider sits between fields on the bottom status line.
	StatusDivider = "  │  "
)

// TitleArt is the ASCII sword shown above the title on the name-entry screen.
// The shape is the original hand-drawn sword with a handful of glyphs swapped
// to box-drawing and geometric Unicode so edges look sharp without changing
// any column position — every swap is single-cell wide.
const TitleArt = `              (◉)
              <M
   o          <M
  ╱| ······  ╱:M╲────────────────────────────────────────────────,,,,,,
(◉)[]XXXXXX[]I:K╬}═════◀{H}▶════════════════════════════────────────▶
  ╲| ^^^^^^  ╲:W╱────────────────────────────────────────────────''''''
   o          <W
              <W
              (◉)`

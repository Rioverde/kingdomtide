package world

import (
	"fmt"
	"strings"
)

// TileOverlay is a bitmask of yes/no features that can coexist on a
// single tile — rivers, roads, bridges, paths, built walls, doors. The
// scale is binary presence; orthogonal enums (e.g. RoadKind for
// dirt/stone/railway, if we ever add it) sit on separate fields.
type TileOverlay uint32

// Overlay flag bits. Compose with |: a road crossing a river via a
// bridge is OverlayRiver | OverlayRoad | OverlayBridge. Leaving room up
// to 32 flags total — the current set is intentionally small. Only
// OverlayRiver is used by existing code today; OverlayRoad, OverlayBridge
// and OverlayPath are scaffolding for later features and have no
// producer or renderer hooked up yet.
const (
	OverlayRiver TileOverlay = 1 << iota
	OverlayRoad
	OverlayBridge
	OverlayPath
	// OverlayLake marks tiles the worldgen rivers pass flagged as standing
	// water — a depression a river trace fell into and could not escape
	// without filling first. The underlying Terrain is preserved (plains,
	// forest, etc.) so a drained lakebed would still render correctly;
	// rendering paints the lake glyph over the biome when this bit is set.
	OverlayLake
)

// overlayNames indexes the human name of each bit for logging. Bits
// with no registered name render as the raw bit number (e.g. "bit7")
// to keep debug output readable even before a new flag is named.
var overlayNames = [...]string{
	"OverlayRiver",
	"OverlayRoad",
	"OverlayBridge",
	"OverlayPath",
	"OverlayLake",
}

// Has reports whether every bit in o is set on t. Has(0) returns true
// (the empty mask is trivially contained).
func (t TileOverlay) Has(o TileOverlay) bool {
	return t&o == o
}

// String renders t as a '|'-joined list of set flag names. The empty
// mask renders as "0" rather than "" so debug logs always show a
// non-empty value. Unknown bits render as "bit<N>".
func (t TileOverlay) String() string {
	if t == 0 {
		return "0"
	}
	var parts []string
	for bit := range 32 {
		mask := TileOverlay(1) << bit
		if t&mask == 0 {
			continue
		}
		if bit < len(overlayNames) {
			parts = append(parts, overlayNames[bit])
		} else {
			parts = append(parts, fmt.Sprintf("bit%d", bit))
		}
	}
	return strings.Join(parts, "|")
}

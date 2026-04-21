package game

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
)

// Has reports whether every bit in o is set on t. Has(0) returns true
// (the empty mask is trivially contained).
func (t TileOverlay) Has(o TileOverlay) bool {
	return t&o == o
}

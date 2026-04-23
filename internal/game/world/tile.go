package world

import "github.com/Rioverde/gongeons/internal/game/entity"

// Tile is the atomic map cell. Terrain is the base biome; Overlays is a
// bitmask of binary features that can coexist (river + road + bridge);
// Structure is a single built thing occupying the tile (mutually
// exclusive — a castle and a village don't share a cell); Occupant is
// the runtime entity currently standing on the tile.
type Tile struct {
	Terrain   Terrain
	Overlays  TileOverlay
	Structure StructureKind
	Occupant  entity.Occupant
}

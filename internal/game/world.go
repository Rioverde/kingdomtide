package game

import "sort"

// World is the authoritative in-memory state of a single match: a rectangular
// grid of Tiles plus a registry of active players and their positions. The
// grid is flat, row-major: index y*width + x. World is not safe for
// concurrent use; callers (server) own the lock.
type World struct {
	width, height int
	tiles         []Tile
	players       map[string]*Player
	positions     map[string]Position
}

// NewWorld constructs an empty world of the given dimensions, pre-filled with
// TerrainPlains and no occupants. Panics on non-positive dimensions: the
// caller is building a world at startup and a zero-sized map is a programmer
// error, not a recoverable runtime condition.
func NewWorld(width, height int) *World {
	if width <= 0 || height <= 0 {
		panic("game: world dimensions must be positive")
	}
	tiles := make([]Tile, width*height)
	for i := range tiles {
		tiles[i].Terrain = TerrainPlains
	}
	return &World{
		width:     width,
		height:    height,
		tiles:     tiles,
		players:   make(map[string]*Player),
		positions: make(map[string]Position),
	}
}

// Width returns the world width in tiles.
func (w *World) Width() int { return w.width }

// Height returns the world height in tiles.
func (w *World) Height() int { return w.height }

// InBounds reports whether p is a valid tile coordinate in this world.
func (w *World) InBounds(p Position) bool {
	return p.X >= 0 && p.X < w.width && p.Y >= 0 && p.Y < w.height
}

// TileAt returns the tile at p. The second return is false when p is out of
// bounds, in which case the first return is the zero Tile.
func (w *World) TileAt(p Position) (Tile, bool) {
	if !w.InBounds(p) {
		return Tile{}, false
	}
	return w.tiles[w.index(p)], true
}

// SetTerrain overwrites the terrain at p. Exported so world builders
// (scenario loaders, future procgen) can shape a world without going through
// ApplyCommand. Out-of-bounds writes are silently ignored so builder code can
// stay straight-line.
func (w *World) SetTerrain(p Position, t Terrain) {
	if !w.InBounds(p) {
		return
	}
	w.tiles[w.index(p)].Terrain = t
}

// PlayerByID returns the player with the given id. The second return is
// false when no such player is registered.
func (w *World) PlayerByID(id string) (*Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

// PositionOf returns the position of the player with the given id. The
// second return is false when no such player is registered.
func (w *World) PositionOf(id string) (Position, bool) {
	p, ok := w.positions[id]
	return p, ok
}

// Players returns a snapshot of active players sorted by ID for deterministic
// iteration. The returned slice is a defensive copy: mutating it does not
// affect the world's internal registry.
func (w *World) Players() []*Player {
	ids := make([]string, 0, len(w.players))
	for id := range w.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*Player, 0, len(ids))
	for _, id := range ids {
		out = append(out, w.players[id])
	}
	return out
}

// index converts a position to a row-major tile index. Callers must have
// verified InBounds(p).
func (w *World) index(p Position) int {
	return p.Y*w.width + p.X
}

// Passable reports whether an entity can stand on a tile of this terrain.
// Water and high peaks block movement; the empty string and unknown values
// are treated as impassable so buggy map data fails closed rather than open.
func (t Terrain) Passable() bool {
	switch t {
	case TerrainPlains,
		TerrainGrassland,
		TerrainMeadow,
		TerrainBeach,
		TerrainSavanna,
		TerrainDesert,
		TerrainSnow,
		TerrainTundra,
		TerrainTaiga,
		TerrainForest,
		TerrainJungle,
		TerrainHills:
		return true
	default:
		return false
	}
}

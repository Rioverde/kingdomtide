package game

import "sort"

// TileSource is the read-only pluggable backend that tells a World what
// terrain sits at a given coordinate. Both procedural and hand-painted
// sources implement it; the World does not care which variant it holds.
type TileSource interface {
	TileAt(x, y int) Tile
}

// World is the authoritative in-memory state of a single match. It combines
// an immutable, pluggable TileSource with mutable runtime overlays — the
// player registry and the occupancy map. World is NOT safe for concurrent
// use; callers (server) own the lock.
type World struct {
	source    TileSource
	players   map[string]*Player
	positions map[string]Position
	// occupants shadows the TileSource with runtime occupant info. TileAt
	// merges the two on read so the TileSource stays read-only.
	occupants map[Position]*Player
}

// NewWorld constructs an infinite World around the given TileSource. Use
// worldgen.NewChunkedSource for the procedural production source, or
// NewWorldFromSource with a test-painted source for deterministic unit
// tests.
func NewWorld(source TileSource) *World {
	return NewWorldFromSource(source)
}

// NewWorldFromSource wraps the given TileSource in a World. Production code
// goes through NewWorld; NewWorldFromSource lets tests (or future scenario
// loaders) supply a hand-crafted source without touching the procedural
// pipeline.
func NewWorldFromSource(source TileSource) *World {
	return &World{
		source:    source,
		players:   make(map[string]*Player),
		positions: make(map[string]Position),
		occupants: make(map[Position]*Player),
	}
}

// InBounds reports whether p is a valid tile coordinate. For the current
// infinite world this is always true; the method stays on the API so
// callers are prepared to treat it as a real check when (if) we introduce
// hard world limits.
func (w *World) InBounds(p Position) bool {
	_ = p
	return true
}

// TileAt returns the tile at p with any runtime occupant merged in. The
// second return is always true in an infinite world — kept for API
// compatibility with the previous fixed-grid variant.
func (w *World) TileAt(p Position) (Tile, bool) {
	t := w.source.TileAt(p.X, p.Y)
	if occ, ok := w.occupants[p]; ok {
		t.Occupant = occ
	}
	return t, true
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

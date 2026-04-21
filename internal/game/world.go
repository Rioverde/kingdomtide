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
	// seed is the world-level entropy shared with anchor geometry and the
	// region source. Zero when unset; RegionAt tolerates that by returning a
	// placeholder Region.
	seed int64
	// regionSource produces canonical Region data per anchor. May be nil; if
	// nil, RegionAt returns a placeholder RegionNormal region.
	regionSource RegionSource
	// landmarkSource produces the canonical landmark list per super-chunk.
	// May be nil; if nil, LandmarksIn returns nil so callers need not
	// special-case the missing source.
	landmarkSource LandmarkSource
}

// WorldOption configures optional fields on a World during construction.
// Options compose so callers can opt into seed and region source
// independently without widening the primary constructor signatures and
// breaking existing call sites.
type WorldOption func(*World)

// WithSeed records the world seed on the World. The seed is surfaced via
// Seed and is the entropy source AnchorAt uses when resolving a tile to a
// region.
func WithSeed(seed int64) WorldOption {
	return func(w *World) {
		w.seed = seed
	}
}

// WithRegionSource attaches a RegionSource. If nil is passed, the option is
// a no-op and RegionAt continues to return the placeholder region — callers
// that genuinely want to clear an already-set source should build a new
// World.
func WithRegionSource(source RegionSource) WorldOption {
	return func(w *World) {
		if source != nil {
			w.regionSource = source
		}
	}
}

// WithLandmarkSource attaches a LandmarkSource. If nil is passed, the
// option is a no-op and LandmarksIn keeps returning nil — callers that
// genuinely want to clear an already-set source should build a new
// World. Mirrors WithRegionSource so the two optional backends wire up
// through the same functional-option shape.
func WithLandmarkSource(source LandmarkSource) WorldOption {
	return func(w *World) {
		if source != nil {
			w.landmarkSource = source
		}
	}
}

// NewWorld constructs an infinite World around the given TileSource. Use
// worldgen.NewChunkedSource for the procedural production source, or
// NewWorldFromSource with a test-painted source for deterministic unit
// tests. Optional seed and RegionSource configuration arrive as functional
// options; omit them for back-compatible default construction.
func NewWorld(source TileSource, opts ...WorldOption) *World {
	return NewWorldFromSource(source, opts...)
}

// NewWorldFromSource wraps the given TileSource in a World. Production code
// goes through NewWorld; NewWorldFromSource lets tests (or future scenario
// loaders) supply a hand-crafted source without touching the procedural
// pipeline. Accepts the same WorldOptions as NewWorld.
func NewWorldFromSource(source TileSource, opts ...WorldOption) *World {
	w := &World{
		source:    source,
		players:   make(map[string]*Player),
		positions: make(map[string]Position),
		occupants: make(map[Position]*Player),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

// Seed returns the world seed that drives deterministic geometry and
// procedural generation. Zero when the world was constructed without a
// seed option.
func (w *World) Seed() int64 {
	return w.seed
}

// RegionSource returns the configured region source, or nil when the world
// was constructed without one. Callers (e.g. the server's region cache)
// branch on the result rather than calling RegionAt through the World so
// they can cache at the anchor's SuperChunkCoord granularity.
func (w *World) RegionSource() RegionSource {
	return w.regionSource
}

// LandmarkSource returns the configured landmark source, or nil when the
// world was constructed without one. Callers (e.g. the server's landmark
// cache) branch on the result to decide whether to wire a cache. Mirrors
// RegionSource so the two optional backends follow the same accessor shape.
func (w *World) LandmarkSource() LandmarkSource {
	return w.landmarkSource
}

// RegionAt returns the region covering the given world position. It
// resolves the nearest Voronoi anchor for (p.X, p.Y) and delegates to the
// configured RegionSource keyed by that anchor's SuperChunkCoord. When no
// RegionSource is configured, it returns a RegionNormal placeholder so
// callers need not special-case the nil source.
func (w *World) RegionAt(p Position) Region {
	anchor, sc := AnchorAt(w.seed, p.X, p.Y)
	if w.regionSource == nil {
		return Region{Coord: sc, Anchor: anchor, Character: RegionNormal}
	}
	return w.regionSource.RegionAt(sc)
}

// LandmarksIn returns the landmarks inside the super-chunk sc. Delegates
// to whatever LandmarkSource the World was constructed with. Returns
// nil when no LandmarkSource is wired — the server's per-sc cache can
// treat a nil result the same as "no landmarks here" without a separate
// branch for the missing-source case.
func (w *World) LandmarksIn(sc SuperChunkCoord) []Landmark {
	if w.landmarkSource == nil {
		return nil
	}
	return w.landmarkSource.LandmarksIn(sc)
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

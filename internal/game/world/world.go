package world

import (
	"math/rand/v2"
	"sort"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/entity"
	"github.com/Rioverde/gongeons/internal/game/geom"
)

// TileSource is the read-only pluggable backend that tells a World what
// terrain sits at a given coordinate. Both procedural and hand-painted
// sources implement it; the World does not care which variant it holds.
type TileSource interface {
	TileAt(x, y int) Tile
}

// World is the authoritative in-memory state of a single match. It combines
// an immutable, pluggable TileSource with mutable runtime overlays — the
// player registry, monster registry, and the occupancy maps. World is NOT
// safe for concurrent use; callers (server) own the lock.
type World struct {
	source    TileSource
	players   map[string]*entity.Player
	monsters  map[string]*entity.Monster
	positions map[string]geom.Position
	// occupants shadows the TileSource with runtime player occupant info.
	// TileAt merges it in on read so the TileSource stays read-only.
	occupants map[geom.Position]*entity.Player
	// monsterOccupants is the monster-side occupancy map. Kept disjoint
	// from occupants (players) so the wire-level mapper can continue to
	// switch on *entity.Player for player glyphs; monster glyphs will
	// arrive via a dedicated mapping path when AI ships.
	monsterOccupants map[geom.Position]*entity.Monster
	// seed is the world-level entropy shared with anchor geometry and the
	// region source. Zero when unset; RegionAt tolerates that by returning a
	// placeholder Region.
	seed int64
	// rng drives the probabilistic speed rounding in mcalcMove. Seeded
	// deterministically from seed so replays with the same world seed
	// produce identical combat timing. Never nil after NewWorldFromSource.
	rng *rand.Rand
	// regionSource produces canonical Region data per anchor. May be nil; if
	// nil, RegionAt returns a placeholder RegionNormal region.
	regionSource RegionSource
	// landmarkSource produces the canonical landmark list per super-chunk.
	// May be nil; if nil, LandmarksIn returns nil so callers need not
	// special-case the missing source.
	landmarkSource LandmarkSource
	// volcanoSource produces the canonical volcano list per super-chunk
	// and resolves per-tile volcanic terrain overrides. May be nil; if
	// nil, VolcanoAt returns nil and VolcanoTerrainOverride returns
	// ("", false) so callers need not special-case the missing source.
	volcanoSource VolcanoSource
	// depositSource produces the canonical resource deposit lookups.
	// May be nil; all three DepositAt / DepositsIn / DepositsNear
	// methods return the empty result when unset so callers need not
	// special-case the missing source.
	depositSource DepositSource
	// currentTick is the monotonic tick counter — advances by exactly 1
	// per Tick() call. Zero on a freshly constructed world. Calendar-
	// derived GameTime reads from this counter through w.cal.
	currentTick int64
	// cal is the tick-to-GameTime derivation config. The zero value
	// (TicksPerDay() == 0) is a sentinel for "no calendar wired" — in
	// that mode Tick() still increments currentTick but emits no calendar
	// boundary events and GameTime() returns the zero-value GameTime.
	// Production paths go through WithCalendar(calendar.NewCalendar(...)).
	cal calendar.Calendar
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

// WithVolcanoSource attaches a VolcanoSource. If nil is passed, the
// option is a no-op and VolcanoAt keeps returning nil while
// VolcanoTerrainOverride keeps returning ("", false) — callers that
// genuinely want to clear an already-set source should build a new
// World. Mirrors WithLandmarkSource so the two optional backends wire
// up through the same functional-option shape.
func WithVolcanoSource(source VolcanoSource) WorldOption {
	return func(w *World) {
		if source != nil {
			w.volcanoSource = source
		}
	}
}

// WithDepositSource attaches a DepositSource. If nil is passed, the
// option is a no-op and the deposit accessors keep returning their
// empty results — callers that genuinely want to clear an already-set
// source should build a new World. Mirrors the other resource-layer
// options so every backend wires through the same functional-option
// shape.
func WithDepositSource(source DepositSource) WorldOption {
	return func(w *World) {
		if source != nil {
			w.depositSource = source
		}
	}
}

// WithCalendar attaches a Calendar. A zero-value Calendar is treated
// the same as "no calendar wired" — Tick() still advances the internal
// counter but emits no boundary events and GameTime() returns the zero
// value. Production code always supplies a valid Calendar via
// calendar.NewCalendar; tests can skip the option to exercise the no-calendar
// path.
func WithCalendar(c calendar.Calendar) WorldOption {
	return func(w *World) { w.cal = c }
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
		source:           source,
		players:          make(map[string]*entity.Player),
		monsters:         make(map[string]*entity.Monster),
		positions:        make(map[string]geom.Position),
		occupants:        make(map[geom.Position]*entity.Player),
		monsterOccupants: make(map[geom.Position]*entity.Monster),
	}
	for _, opt := range opts {
		opt(w)
	}
	// Seed the rng deterministically from the world seed. When seed is
	// zero (tests that skip WithSeed) we still need a non-degenerate PCG
	// stream, so XOR-fold a non-zero constant into the second word.
	s1 := uint64(w.seed)
	s2 := uint64(w.seed) ^ 0x5f3759df
	if s1 == 0 && s2 == 0 {
		s2 = 0x5f3759df
	}
	w.rng = rand.New(rand.NewPCG(s1, s2))
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

// VolcanoSource returns the configured volcano source, or nil when the
// world was constructed without one. Callers (e.g. the server's volcano
// cache) branch on the result to decide whether to wire a cache. Mirrors
// LandmarkSource so the two optional backends follow the same accessor shape.
func (w *World) VolcanoSource() VolcanoSource {
	return w.volcanoSource
}

// DepositSource returns the configured deposit source, or nil when the
// world was constructed without one. Callers branch on the result to
// decide whether to wire caching or skip deposit-aware code paths.
func (w *World) DepositSource() DepositSource {
	return w.depositSource
}

// CurrentTick returns the monotonic tick counter. Zero on a freshly
// constructed world; incremented by exactly 1 per Tick() call. Exposed
// so the server can seed wire messages, diagnostics, and persistence
// snapshots with the authoritative counter.
//
// Threading: World is not internally synchronised — callers that also
// mutate world state (the server via s.mu) must hold their mutex while
// calling CurrentTick so Tick() cannot interleave a half-written
// counter update. Read-only callers in test contexts hold no mutex but
// also never race with Tick().
func (w *World) CurrentTick() int64 {
	return w.currentTick
}

// Calendar returns the configured Calendar. A zero value indicates no
// calendar is wired — callers should check `cal.TicksPerDay() > 0`
// before calling Derive on the result. The server uses this to
// propagate CalendarConfig to clients via JoinAccepted.
func (w *World) Calendar() calendar.Calendar {
	return w.cal
}

// GameTime returns the derived calendar position at the current tick.
// Returns the zero-value GameTime when no calendar is wired so callers
// can treat "missing calendar" as "Year 0 Month 0" without a separate
// nil check. Cheap (<20 ns); call freely on the hot path.
//
// Threading: reads w.currentTick and w.cal. The calendar is
// immutable after construction; currentTick is mutated only by Tick().
// Callers that share the world with a concurrent Tick() caller must
// hold the same mutex (the server path always holds s.mu).
func (w *World) GameTime() calendar.GameTime {
	if w.cal.TicksPerDay() == 0 {
		return calendar.GameTime{}
	}
	return w.cal.Derive(w.currentTick)
}

// RegionAt returns the region covering the given world position. It
// resolves the nearest Voronoi anchor for (p.X, p.Y) and delegates to
// the configured RegionSource keyed by that anchor's SuperChunkCoord.
// When no RegionSource is configured, it returns a RegionNormal
// placeholder so callers need not special-case the nil source. Names
// are emitted as structured Parts records; the client composes
// localized display text using its own Markov corpora and catalogs.
func (w *World) RegionAt(p geom.Position) Region {
	anchor, sc := geom.AnchorAt(w.seed, p.X, p.Y)
	if w.regionSource == nil {
		return Region{Coord: sc, Anchor: anchor, Character: RegionNormal}
	}
	return w.regionSource.RegionAt(sc)
}

// LandmarksIn returns the landmarks inside the super-chunk sc.
// Delegates to whatever LandmarkSource the World was constructed with.
// Returns nil when no LandmarkSource is wired — the server's per-sc
// cache can treat a nil result the same as "no landmarks here" without
// a separate branch for the missing-source case. Each landmark's
// structured Name is language-agnostic; the client composes the final
// display string.
func (w *World) LandmarksIn(sc geom.SuperChunkCoord) []Landmark {
	if w.landmarkSource == nil {
		return nil
	}
	return w.landmarkSource.LandmarksIn(sc)
}

// VolcanoAt returns the volcanoes inside the super-chunk sc. Delegates
// to whatever VolcanoSource the World was constructed with. Returns nil
// when no VolcanoSource is wired — the server's per-sc cache can treat
// a nil result the same as "no volcanoes here" without a separate
// branch for the missing-source case.
func (w *World) VolcanoAt(sc geom.SuperChunkCoord) []Volcano {
	if w.volcanoSource == nil {
		return nil
	}
	return w.volcanoSource.VolcanoAt(sc)
}

// VolcanoTerrainOverride returns the volcanic terrain that replaces the
// base biome at tile t, or ("", false) when t is not covered by any
// volcano footprint (or no VolcanoSource is wired). Callers blend the
// override on top of the base TileSource so a volcano's core, slope, or
// ashland ring shows the correct terrain without mutating the tile
// backend.
func (w *World) VolcanoTerrainOverride(t geom.Position) (Terrain, bool) {
	if w.volcanoSource == nil {
		return "", false
	}
	return w.volcanoSource.TerrainOverrideAt(t)
}

// DepositAt returns the resource deposit covering tile p, or
// (Deposit{}, false) when no deposit sits on p or no DepositSource is
// wired. Called by the future prospect action, quest generation, and
// tests that need a deterministic lookup.
func (w *World) DepositAt(p geom.Position) (Deposit, bool) {
	if w.depositSource == nil {
		return Deposit{}, false
	}
	return w.depositSource.DepositAt(p)
}

// DepositsIn returns every deposit whose Position lies inside rect.
// Returns nil when no DepositSource is wired. Used by Phase-5a city
// placement to seed the candidate pool with resource-anchor tiles.
func (w *World) DepositsIn(rect geom.Rect) []Deposit {
	if w.depositSource == nil {
		return nil
	}
	return w.depositSource.DepositsIn(rect)
}

// DepositsNear returns every deposit within Chebyshev radius of p,
// sorted by distance ascending. Returns nil when no DepositSource is
// wired. Used by the future contextual info-panel when the player
// approaches a feature.
func (w *World) DepositsNear(p geom.Position, radius int) []Deposit {
	if w.depositSource == nil {
		return nil
	}
	return w.depositSource.DepositsNear(p, radius)
}

// InBounds reports whether p is a valid tile coordinate. For the current
// infinite world this is always true; the method stays on the API so
// callers are prepared to treat it as a real check when (if) we introduce
// hard world limits.
func (w *World) InBounds(p geom.Position) bool {
	_ = p
	return true
}

// TileAt returns the tile at p with any runtime occupant merged in. The
// second return is always true in an infinite world — kept for API
// compatibility with the previous fixed-grid variant.
func (w *World) TileAt(p geom.Position) (Tile, bool) {
	t := w.source.TileAt(p.X, p.Y)
	if occ, ok := w.occupants[p]; ok {
		t.Occupant = occ
	}
	return t, true
}

// PlayerByID returns the player with the given id. The second return is
// false when no such player is registered.
func (w *World) PlayerByID(id string) (*entity.Player, bool) {
	p, ok := w.players[id]
	return p, ok
}

// PositionOf returns the position of the player with the given id. The
// second return is false when no such player is registered.
func (w *World) PositionOf(id string) (geom.Position, bool) {
	p, ok := w.positions[id]
	return p, ok
}

// Players returns a snapshot of active players sorted by ID for deterministic
// iteration. The returned slice is a defensive copy: mutating it does not
// affect the world's internal registry.
func (w *World) Players() []*entity.Player {
	ids := make([]string, 0, len(w.players))
	for id := range w.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]*entity.Player, 0, len(ids))
	for _, id := range ids {
		out = append(out, w.players[id])
	}
	return out
}

// Monsters returns the world's monster map. The returned map is the live
// internal registry — callers must not mutate it directly. Exposed for
// snapshot and tick consumers that hold the server mutex while reading.
func (w *World) Monsters() map[string]*entity.Monster {
	return w.monsters
}

// AddMonster inserts m into the world's monster registry. If a monster
// with the same ID already exists it is replaced (idempotent) and the
// previous monster's occupancy entry is cleared. The new monster lands
// on its Position field's coordinate (origin when left zero-valued) and
// its spot is recorded in monsterOccupants so applyMonsterMoveIntent
// observes the tile as occupied. This remains an admin/fixture entry
// point; geometric legality (passable terrain, no player present) is
// the caller's responsibility at admin-insert time.
func (w *World) AddMonster(m *entity.Monster) {
	if w.monsters == nil {
		w.monsters = make(map[string]*entity.Monster)
	}
	if w.monsterOccupants == nil {
		w.monsterOccupants = make(map[geom.Position]*entity.Monster)
	}
	if prev, ok := w.monsters[m.ID]; ok {
		// Clear the previous occupancy slot only if the stored map entry
		// still points at that monster — guards against replacing a
		// monster that was already moved off its original tile.
		if stored, ok := w.monsterOccupants[prev.Position]; ok && stored == prev {
			delete(w.monsterOccupants, prev.Position)
		}
	}
	w.monsters[m.ID] = m
	w.monsterOccupants[m.Position] = m
}

// RemoveMonster deletes the monster with the given id from the registry
// and clears its occupancy slot. No-op when no such monster exists.
func (w *World) RemoveMonster(id string) {
	m, ok := w.monsters[id]
	if !ok {
		return
	}
	if stored, ok := w.monsterOccupants[m.Position]; ok && stored == m {
		delete(w.monsterOccupants, m.Position)
	}
	delete(w.monsters, id)
}

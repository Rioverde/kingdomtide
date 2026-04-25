package world

import "github.com/Rioverde/gongeons/internal/game/geom"

// VolcanoState is the lifecycle state of a volcano. It governs the core
// terrain variant (active vs dormant vs extinct) and, later, whether the
// volcano emits events. Unknown is the zero value so tests can assert
// that an uninitialized Volcano has not been mistakenly consumed as a
// valid record.
type VolcanoState uint8

// VolcanoStateUnknown is the zero value signalling "state not set" and
// exists so zero-value records fail loudly when routed through code
// that expects a real state. The remaining constants enumerate every
// concrete state a placed volcano can occupy.
const (
	VolcanoStateUnknown VolcanoState = iota
	VolcanoActive
	VolcanoDormant
	VolcanoExtinct
)

// volcanoStateNames maps each state to its lowercase key. Exposed via
// Key and String; kept as a fixed-size array (not a map) because the
// set is small, dense, and indexed by the uint8 value — O(1) lookup,
// no allocation. Order matches the iota declaration above.
var volcanoStateNames = [...]string{
	VolcanoStateUnknown: "unknown",
	VolcanoActive:       "active",
	VolcanoDormant:      "dormant",
	VolcanoExtinct:      "extinct",
}

// Key returns the lowercase identifier used for locale catalog keys
// (e.g. "active", "dormant"). Out-of-range values return the empty
// string rather than panic so debug output on a corrupt value remains
// usable.
func (s VolcanoState) Key() string {
	if int(s) >= len(volcanoStateNames) {
		return ""
	}
	return volcanoStateNames[s]
}

// String implements fmt.Stringer by delegating to Key. The two are the
// same value; Key exists so call sites document their intent when the
// string is consumed as a stable identifier rather than a label.
func (s VolcanoState) String() string {
	return s.Key()
}

// Volcano is the server-facing record for one placed volcano — its
// anchor tile, lifecycle state, and the three footprint rings (core,
// slope, ashland) the placement pipeline filled in. Anchor is the
// conceptual centre of the volcano; it is always included in CoreTiles.
// CoreTiles holds every tile covered by the impassable core (or crater
// lake for an extinct volcano); SlopeTiles and AshlandTiles hold the
// surrounding passable zones. A volcano with a nil or empty CoreTiles
// slice is malformed and should be dropped by callers.
type Volcano struct {
	Anchor       geom.Position
	State        VolcanoState
	CoreTiles    []geom.Position
	SlopeTiles   []geom.Position
	AshlandTiles []geom.Position
}

// VolcanoSource is the consumer-side interface the World delegates to
// when reporting volcanoes inside a super-chunk and when resolving a
// tile's volcanic terrain override. The interface lives in this
// package because World consumes it — per Go interface-design guidance,
// interfaces belong at the consumer. Implementations live outside
// (e.g. worldgen.VolcanoSource).
//
// Implementations must be deterministic: same SuperChunkCoord yields
// the same []Volcano every call (including order), same Position yields
// the same override result, and must be safe for concurrent read.
// Returning nil or an empty slice is the correct way to signal "no
// volcanoes in this super-chunk". TerrainOverrideAt returns ("", false)
// when t is not covered by any volcano footprint. All returns every
// placed volcano in a stable order, with the outer slice and each
// volcano's tile slices cloned so callers may mutate the result
// freely; implementations that do not enumerate volcanoes may return
// nil.
type VolcanoSource interface {
	VolcanoAt(sc geom.SuperChunkCoord) []Volcano
	TerrainOverrideAt(t geom.Position) (Terrain, bool)
	All() []Volcano
}

package game

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

// VolcanoZone identifies which concentric ring of a volcano footprint
// contains a given tile. VolcanoZoneNone is the zero value meaning "not
// inside the volcano" — the safe default so a malformed or missing
// lookup reads as miss rather than a false hit on a real zone.
//
// The enum uses an explicit named type (not bare strings) so callers
// cannot silently misspell zone names — a typo becomes a compile error
// instead of a runtime miss. Order matches the outward growth of a
// volcano footprint: core → slope → ashland.
type VolcanoZone uint8

const (
	VolcanoZoneNone VolcanoZone = iota
	VolcanoZoneCore
	VolcanoZoneSlope
	VolcanoZoneAshland
)

var volcanoZoneNames = [...]string{
	VolcanoZoneNone:    "",
	VolcanoZoneCore:    "core",
	VolcanoZoneSlope:   "slope",
	VolcanoZoneAshland: "ashland",
}

// Key returns the lowercase identifier used for locale catalog keys and
// structured logging (e.g. "core", "slope", "ashland"). VolcanoZoneNone
// and out-of-range values return the empty string so debug output on a
// corrupt value remains usable.
func (z VolcanoZone) Key() string {
	if int(z) >= len(volcanoZoneNames) {
		return ""
	}
	return volcanoZoneNames[z]
}

// String implements fmt.Stringer by delegating to Key.
func (z VolcanoZone) String() string { return z.Key() }

// Volcano is the server-facing record for one placed volcano — its
// anchor tile, lifecycle state, and the three footprint rings (core,
// slope, ashland) the placement pipeline filled in. Anchor is the
// conceptual centre of the volcano; it is always included in CoreTiles.
// CoreTiles holds every tile covered by the impassable core (or crater
// lake for an extinct volcano); SlopeTiles and AshlandTiles hold the
// surrounding passable zones. A volcano with a nil or empty CoreTiles
// slice is malformed and should be dropped by callers.
type Volcano struct {
	Anchor       Position
	State        VolcanoState
	CoreTiles    []Position
	SlopeTiles   []Position
	AshlandTiles []Position
}

// ZoneAt reports which footprint ring contains t, or VolcanoZoneNone
// when t sits outside the volcano entirely. The check is a linear scan
// — footprints are small (dozens of tiles at most) and the call path is
// cold, so the simple form beats building a set.
func (v Volcano) ZoneAt(t Position) VolcanoZone {
	for _, p := range v.CoreTiles {
		if p.Equal(t) {
			return VolcanoZoneCore
		}
	}
	for _, p := range v.SlopeTiles {
		if p.Equal(t) {
			return VolcanoZoneSlope
		}
	}
	for _, p := range v.AshlandTiles {
		if p.Equal(t) {
			return VolcanoZoneAshland
		}
	}
	return VolcanoZoneNone
}

// VolcanoSource is the consumer-side interface the World delegates to
// when reporting volcanoes inside a super-chunk and when resolving a
// tile's volcanic terrain override. The interface lives in this
// package because World consumes it — per Go interface-design guidance,
// interfaces belong at the consumer. Implementations live outside
// (e.g. a future worldgen.NoiseVolcanoSource).
//
// Implementations must be deterministic: same SuperChunkCoord yields
// the same []Volcano every call (including order), same Position yields
// the same override result, and must be safe for concurrent read.
// Returning nil or an empty slice is the correct way to signal "no
// volcanoes in this super-chunk". TerrainOverrideAt returns ("", false)
// when t is not covered by any volcano footprint.
type VolcanoSource interface {
	VolcanoAt(sc SuperChunkCoord) []Volcano
	TerrainOverrideAt(t Position) (Terrain, bool)
}

package game

// LandmarkKind identifies a natural or ancient landmark visible in the
// world. Landmarks live on Layer 1.5 — tied to geography and pre-
// civilization history, independent of any living faction. A ruined
// castle treated as a narrative artifact stays LandmarkNone here and is
// represented through the later civilization layer instead.
type LandmarkKind uint8

// LandmarkNone is the zero value signalling "no landmark on this tile".
// The remaining constants enumerate every concrete landmark kind the
// world can produce.
const (
	LandmarkNone LandmarkKind = iota
	LandmarkTower
	LandmarkGiantTree
	LandmarkStandingStones
	LandmarkObelisk
	LandmarkChasm
	LandmarkShrine
)

// landmarkKindNames maps each kind to its lowercase key. Exposed via Key
// and String; kept as a fixed-size array (not a map) because the set is
// small, dense, and indexed by the uint8 value — O(1) lookup, no
// allocation. Order matches the iota declaration above.
var landmarkKindNames = [...]string{
	LandmarkNone:           "none",
	LandmarkTower:          "tower",
	LandmarkGiantTree:      "giant_tree",
	LandmarkStandingStones: "standing_stones",
	LandmarkObelisk:        "obelisk",
	LandmarkChasm:          "chasm",
	LandmarkShrine:         "shrine",
}

// Key returns the lowercase identifier used for locale catalog keys
// (e.g. "tower", "giant_tree"). Out-of-range values return the empty
// string rather than panic so debug output on a corrupt value remains
// usable.
func (k LandmarkKind) Key() string {
	if int(k) >= len(landmarkKindNames) {
		return ""
	}
	return landmarkKindNames[k]
}

// String implements fmt.Stringer by delegating to Key. The two are the
// same value; Key exists so call sites document their intent when the
// string is consumed as a stable identifier rather than a label.
func (k LandmarkKind) String() string {
	return k.Key()
}

// Landmark is the server-facing record for one placed landmark — its
// world position and the kind visible at that tile. Coord is the exact
// tile the landmark occupies; Kind is never LandmarkNone for a real
// record (a source that has nothing to place returns an empty slice).
type Landmark struct {
	Coord Position
	Kind  LandmarkKind
}

// LandmarkSource is the consumer-side interface the World delegates to
// when reporting landmarks inside a super-chunk. The interface lives in
// this package because World consumes it — per Go interface-design
// guidance, interfaces belong at the consumer. Implementations live
// outside (e.g. worldgen.NoiseLandmarkSource).
//
// Implementations must be deterministic: same SuperChunkCoord yields
// the same []Landmark every call (including order), and must be safe
// for concurrent read. Returning nil or an empty slice is the correct
// way to signal "no landmarks in this super-chunk".
type LandmarkSource interface {
	LandmarksIn(sc SuperChunkCoord) []Landmark
}

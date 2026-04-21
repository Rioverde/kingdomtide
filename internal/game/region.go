package game

// RegionCharacter is the dominant thematic identity of a super-chunk region.
// It is derived at read time from a RegionInfluence vector via Dominant; a
// region's canonical character is simply the Dominant projection of its
// anchor-sampled influence. Callers should not assign RegionCharacter
// directly to a Region except through the RegionSource that produced it.
type RegionCharacter uint8

// Character constants. Order matters for Dominant tie-breaking: the lower
// the value, the higher the priority when two components exceed the
// threshold at the exact same magnitude.
const (
	RegionNormal RegionCharacter = iota
	RegionBlighted
	RegionFey
	RegionAncient
	RegionSavage
	RegionHoly
	RegionWild
)

// regionCharacterNames maps each character to its lowercase key. Exposed
// via String and Key; kept as a slice (not map) because the set is small,
// fixed, and densely indexed — O(1) lookup without allocation.
var regionCharacterNames = [...]string{
	RegionNormal:   "normal",
	RegionBlighted: "blighted",
	RegionFey:      "fey",
	RegionAncient:  "ancient",
	RegionSavage:   "savage",
	RegionHoly:     "holy",
	RegionWild:     "wild",
}

// String returns the lowercase key of the character. Implements fmt.Stringer.
// Unknown values return "unknown" rather than panic so debug output on a
// corrupt value remains usable.
func (c RegionCharacter) String() string {
	if int(c) >= len(regionCharacterNames) {
		return "unknown"
	}
	return regionCharacterNames[c]
}

// Key returns the lowercase identifier used for locale catalog keys
// (e.g. "crossing.blighted"). Same value as String but named explicitly so
// call sites document their intent: this string is a stable identifier,
// not a user-facing label.
func (c RegionCharacter) Key() string {
	return c.String()
}

// RegionInfluence is the per-region accumulator of thematic influences.
// Each component is in [0, 1]. Multiple components can be non-zero — a
// region can be simultaneously Ancient and Fey. Dominant picks the strongest
// above regionDominantThreshold; if all are below, RegionNormal is returned.
//
// Field order matches the RegionCharacter enum (Blight..Wild) and is the
// tie-break order used by Dominant.
type RegionInfluence struct {
	Blight  float32
	Fae     float32
	Ancient float32
	Savage  float32
	Holy    float32
	Wild    float32
}

// regionDominantThreshold is the minimum component magnitude required for a
// character to be considered "dominant". Components strictly greater than
// the threshold qualify; equal or below is treated as background. Chosen so
// that noise fields in [0, 1] with typical peaks around 0.6-0.8 produce a
// rough 40% dominant / 60% Normal mix.
const regionDominantThreshold float32 = 0.45

// Dominant projects the influence vector onto a single character. Returns
// RegionNormal when no component exceeds regionDominantThreshold. Ties are
// broken by field declaration order: Blight > Fae > Ancient > Savage > Holy
// > Wild. The method is pure and allocation-free.
func (r RegionInfluence) Dominant() RegionCharacter {
	best := RegionNormal
	var bestVal float32
	// Walk components in enum order so the first strictly-greater value wins
	// ties. Using a small indexed array keeps the ordering explicit.
	components := [...]struct {
		value float32
		char  RegionCharacter
	}{
		{r.Blight, RegionBlighted},
		{r.Fae, RegionFey},
		{r.Ancient, RegionAncient},
		{r.Savage, RegionSavage},
		{r.Holy, RegionHoly},
		{r.Wild, RegionWild},
	}
	for _, c := range components {
		if c.value > regionDominantThreshold && c.value > bestVal {
			bestVal = c.value
			best = c.char
		}
	}
	return best
}

// Sum returns the total influence magnitude across all six components. Used
// by the client tint formula to modulate tint intensity — higher sum yields
// a stronger visible accent.
func (r RegionInfluence) Sum() float32 {
	return r.Blight + r.Fae + r.Ancient + r.Savage + r.Holy + r.Wild
}

// Region is the server-facing read-only snapshot of one Voronoi cell of the
// region diagram. Coord identifies the anchor's home super-chunk (not the
// player's tile super-chunk) and is the stable identity used for change
// detection on the client. Anchor is the absolute world position of the
// jittered anchor, used by client-side tint falloff and by landmark
// placement in later phases.
type Region struct {
	Coord     SuperChunkCoord
	Anchor    Position
	Influence RegionInfluence
	Character RegionCharacter
	Name      string
}

// RegionSource produces the canonical Region for a given anchor's home
// super-chunk. The interface lives in this package because World consumes
// it — per Go interface-design guidance, interfaces belong at the consumer.
// Implementations live outside (e.g. worldgen.NoiseRegionSource).
//
// Implementations must be deterministic: same SuperChunkCoord yields the
// same Region every call, and must be safe for concurrent read.
type RegionSource interface {
	RegionAt(sc SuperChunkCoord) Region
}

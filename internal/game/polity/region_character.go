package polity

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

	// RegionCharacterCount is the total number of distinct region characters.
	// Used as a fixed array size wherever a per-character slot is needed.
	RegionCharacterCount
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

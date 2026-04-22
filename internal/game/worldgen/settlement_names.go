package worldgen

import (
	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/naming"
)

// SettlementKind enumerates settlement types at the naming level. The
// value participates in the "<culture>.<kind>" half of the catalog key
// that naming.Generate composes under the settlement domain. Phase 5a
// will promote Settlement to a first-class domain struct with placement
// logic; for Phase 3f the kind exists only so the naming pipeline can
// resolve pattern templates.
type SettlementKind uint8

// SettlementKind values. Order is stable — changing it would shift the
// zero value and any enum-indexed catalog lookups downstream would need
// a migration.
const (
	SettlementVillage SettlementKind = iota
	SettlementTown
	SettlementCity
	SettlementKeep
	SettlementRuin
)

// settlementKindNames maps each SettlementKind to its lowercase catalog
// identifier. Slice lookup is O(1) and allocation-free — the set is
// small, fixed, and densely indexed.
var settlementKindNames = [...]string{
	SettlementVillage: "village",
	SettlementTown:    "town",
	SettlementCity:    "city",
	SettlementKeep:    "keep",
	SettlementRuin:    "ruin",
}

// Key returns the lowercase identifier used in catalog keys. Unknown
// values return the empty string rather than panicking so corrupt enum
// values surface as a visible catalog miss instead of a crash.
func (k SettlementKind) Key() string {
	if int(k) >= len(settlementKindNames) {
		return ""
	}
	return settlementKindNames[k]
}

// String implements fmt.Stringer by delegating to Key. Key exists so
// call sites can document that the returned string is a stable
// identifier rather than a user-facing label.
func (k SettlementKind) String() string { return k.Key() }

// Culture identifies the civilization naming flavour that shapes a
// settlement's kind_pattern templates. Four stock cultures ship with
// Phase 3; Phase 5a may subdivide or add more.
type Culture string

// Culture values. The string form is the exact prefix used in the
// "<culture>.<kind>" catalog sub_kind key — editing these strings
// requires editing every matching TOML entry too.
const (
	CultureDrevan Culture = "drevan" // latin/slavic old-world
	CultureWild   Culture = "wild"   // short harsh tribal
	CultureFallen Culture = "fallen" // gothic/ruin-era
	CulturePlain  Culture = "plain"  // common pastoral
)

// Key returns the lowercase identifier used in catalog keys. Provided
// for symmetry with RegionCharacter.Key and LandmarkKind.Key even
// though Culture already stringifies to the same value.
func (c Culture) Key() string { return string(c) }

// String implements fmt.Stringer by returning the raw culture value.
func (c Culture) String() string { return string(c) }

// settlementBounds caps PrefixIndex and PatternIndex draws to the
// number of catalog entries present for each (culture, kind) pair and
// region character. Both locales carry the same counts, enforced by
// TestNamingCatalogCoverage in the locale package. Pattern keys follow
// the "<domain>.<sub_kind>" shape naming.Generate expects — the
// sub_kind is "<culture>.<kind>" (e.g. "drevan.village"), so the full
// PatternCount key is "settlement.<culture>.<kind>".
var settlementBounds = naming.Bounds{
	PatternCount: map[string]int{
		"settlement.drevan.village": 3,
		"settlement.drevan.town":    3,
		"settlement.drevan.city":    3,
		"settlement.drevan.keep":    3,
		"settlement.drevan.ruin":    3,

		"settlement.wild.village": 3,
		"settlement.wild.town":    3,
		"settlement.wild.city":    3,
		"settlement.wild.keep":    3,
		"settlement.wild.ruin":    3,

		"settlement.fallen.village": 3,
		"settlement.fallen.town":    3,
		"settlement.fallen.city":    3,
		"settlement.fallen.keep":    3,
		"settlement.fallen.ruin":    3,

		"settlement.plain.village": 3,
		"settlement.plain.town":    3,
		"settlement.plain.city":    3,
		"settlement.plain.keep":    3,
		"settlement.plain.ruin":    3,
	},
	PrefixCount: map[string]int{
		"normal":   5,
		"blighted": 5,
		"fey":      5,
		"ancient":  5,
		"savage":   5,
		"holy":     5,
		"wild":     5,
	},
}

// SettlementBounds exposes settlementBounds for the catalog-coverage
// tests in other packages. The returned value is a snapshot; callers
// must not mutate the underlying maps.
func SettlementBounds() naming.Bounds {
	return settlementBounds
}

// SettlementName produces a deterministic structured name for a
// settlement. The returned Parts is composed into a display string by
// the client via the locale catalog under
// "settlement.name.<culture>.<kind>.kind_pattern.*" and
// "settlement.prefix.<character>.*" keys.
//
// Settlements inherit the thematic character from the region that
// contains their placement cell; callers are expected to supply that
// character at call time. Phase 5a will wire this into the placement
// source.
func SettlementName(
	culture Culture,
	kind SettlementKind,
	character game.RegionCharacter,
	seed int64,
	coord game.Position,
) naming.Parts {
	return naming.Generate(
		naming.Input{
			Domain:    naming.DomainSettlement,
			Character: character.Key(),
			SubKind:   string(culture) + "." + kind.Key(),
			Seed:      seed,
			CoordX:    coord.X,
			CoordY:    coord.Y,
		},
		settlementBounds,
	)
}

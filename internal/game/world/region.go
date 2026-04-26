package world

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming/parts"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// RegionCharacter is the dominant thematic identity of a super-chunk region.
// The canonical definition lives in the polity package; this alias re-exports
// it so existing call sites that import world continue to compile unchanged.
type RegionCharacter = polity.RegionCharacter

// Region-character constants re-exported from polity via the alias above.
// Existing callers referencing world.RegionNormal, world.RegionBlighted, etc.
// continue to work without modification.
const (
	RegionNormal   = polity.RegionNormal
	RegionBlighted = polity.RegionBlighted
	RegionFey      = polity.RegionFey
	RegionAncient  = polity.RegionAncient
	RegionSavage   = polity.RegionSavage
	RegionHoly     = polity.RegionHoly
	RegionWild     = polity.RegionWild
)

// RegionInfluence is the per-region accumulator of thematic influences.
// Each component is in [0, 1]. Multiple components can be non-zero — a
// region can be simultaneously Ancient and Fey. Dominant picks the
// strongest component; if all are zero, RegionNormal is returned.
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

// Dominant projects the influence vector onto a single character by
// returning the character with the highest component value. Returns
// RegionNormal when all components are zero. Ties are broken by field
// declaration order: Blight > Fae > Ancient > Savage > Holy > Wild.
// The threshold gate has been removed — callers that need a minimum
// cutoff (e.g. worldgen.RegionSource) apply it at their layer.
// The method is pure and allocation-free.
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
		if c.value > bestVal {
			bestVal = c.value
			best = c.char
		}
	}
	return best
}

// Sum returns the total influence magnitude across all six components. The
// result is in [0, N] where N is the number of influence components (currently
// 6), because each component is individually clamped to [0, 1] and all six
// can be non-zero simultaneously.
func (r RegionInfluence) Sum() float32 {
	return r.Blight + r.Fae + r.Ancient + r.Savage + r.Holy + r.Wild
}

// Max returns the largest single influence component. The result is always
// in [0, 1] by construction — each component is individually clamped to
// [0, 1] — making Max a well-bounded strength signal regardless of how many
// sub-dominant components overlap at a point. Used by the client tint
// formula so the dominant-character intensity drives the accent strength
// without the sum of overlapping characters inflating the value past the
// cap on every tile.
func (r RegionInfluence) Max() float32 {
	m := r.Blight
	if r.Fae > m {
		m = r.Fae
	}
	if r.Ancient > m {
		m = r.Ancient
	}
	if r.Savage > m {
		m = r.Savage
	}
	if r.Holy > m {
		m = r.Holy
	}
	if r.Wild > m {
		m = r.Wild
	}
	return m
}

// Region is the server-facing read-only snapshot of one Voronoi cell
// of the region diagram. Coord identifies the anchor's home super-chunk
// (not the player's tile super-chunk) and is the stable identity used
// for change detection on the client. Anchor is the absolute world
// position of the jittered anchor, used by client-side tint falloff and
// by landmark placement in later phases.
//
// Name is the structured, language-agnostic output of the naming
// package. The client composes the final display string from Name via
// locale keys under "region.name.*" and "region.prefix.*" and an
// embedded Markov corpus keyed on Name.BodySeed.
type Region struct {
	Coord     geom.SuperChunkCoord
	Anchor    geom.Position
	Influence RegionInfluence
	Character RegionCharacter
	Name      parts.Parts
}

// RegionSource produces the canonical Region for a given anchor's home
// super-chunk. The interface lives in this package because World
// consumes it — per Go interface-design guidance, interfaces belong at
// the consumer. Implementations live outside (e.g.
// worldgen.RegionSource).
//
// No language argument: names are emitted as structured Parts records
// and the client composes localized display text. Implementations must
// be deterministic (same sc yields the same Region every call) and
// safe for concurrent read.
type RegionSource interface {
	RegionAt(sc geom.SuperChunkCoord) Region
}

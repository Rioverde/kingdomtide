package world

import "github.com/Rioverde/gongeons/internal/game/geom"

// DepositKind enumerates every resource deposit type the game recognizes.
// Order is stable — the iota value participates in any future persistence
// wire encoding, and reordering shifts every kind-indexed lookup
// downstream. Append new kinds at the end.
//
// The zero value DepositNone is reserved for "no deposit here" and never
// appears on a real Deposit record; tests assert this invariant.
type DepositKind uint8

const (
	DepositNone DepositKind = iota

	// Common — mundane economy.
	DepositIron
	DepositStone
	DepositTimber
	DepositFertile
	DepositFish
	DepositGame
	DepositSalt

	// Rare — prestige economy.
	DepositGold
	DepositSilver
	DepositGems

	// Volcanic — tied to volcano features.
	DepositObsidian
	DepositSulfur
)

// depositKindNames maps each kind to its lowercase key. Fixed-size array
// for O(1) lookup indexed by the uint8 value. Order matches the iota
// declaration above.
var depositKindNames = [...]string{
	DepositNone:     "none",
	DepositIron:     "iron",
	DepositStone:    "stone",
	DepositTimber:   "timber",
	DepositFertile:  "fertile",
	DepositFish:     "fish",
	DepositGame:     "game",
	DepositSalt:     "salt",
	DepositGold:     "gold",
	DepositSilver:   "silver",
	DepositGems:     "gems",
	DepositObsidian: "obsidian",
	DepositSulfur:   "sulfur",
}

// Key returns the lowercase identifier used for locale catalog keys and
// structured logging (e.g. "iron", "fertile"). Out-of-range values return
// the empty string rather than panic so debug output on a corrupt value
// remains usable.
func (k DepositKind) Key() string {
	if int(k) >= len(depositKindNames) {
		return ""
	}
	return depositKindNames[k]
}

// String implements fmt.Stringer by delegating to Key.
func (k DepositKind) String() string { return k.Key() }

// AllDepositKinds returns every valid kind in enum order, excluding
// DepositNone. Used by coverage loops and histogram tests so a new kind
// is exercised automatically.
func AllDepositKinds() []DepositKind {
	return []DepositKind{
		DepositIron, DepositStone, DepositTimber, DepositFertile,
		DepositFish, DepositGame, DepositSalt,
		DepositGold, DepositSilver, DepositGems,
		DepositObsidian, DepositSulfur,
	}
}

// DepositCategory groups kinds by sampling strategy. Placement code
// dispatches on this value to pick the right algorithm: zonal kinds use
// Perlin + threshold, point-like use Poisson-disk, structural are
// feature-locked (coast / volcano slope / core-adjacent).
type DepositCategory uint8

const (
	CategoryZonal DepositCategory = iota
	CategoryPointLike
	CategoryStructural
)

var depositCategoryNames = [...]string{
	CategoryZonal:      "zonal",
	CategoryPointLike:  "point_like",
	CategoryStructural: "structural",
}

// String implements fmt.Stringer for DepositCategory so debug output
// reads as human text rather than a bare integer.
func (c DepositCategory) String() string {
	if int(c) >= len(depositCategoryNames) {
		return ""
	}
	return depositCategoryNames[c]
}

// CategoryOf returns which sampling strategy applies to k. Every value
// in AllDepositKinds has an entry; DepositNone and out-of-range values
// fall through to CategoryZonal as a safe default — tests assert the
// mapping stays total over AllDepositKinds so a new kind without a
// category assignment fails loudly.
func CategoryOf(k DepositKind) DepositCategory {
	switch k {
	case DepositFertile, DepositTimber, DepositGame:
		return CategoryZonal
	case DepositIron, DepositStone, DepositSalt,
		DepositGold, DepositSilver, DepositGems:
		return CategoryPointLike
	case DepositFish, DepositObsidian, DepositSulfur:
		return CategoryStructural
	}
	return CategoryZonal
}

// Deposit is the server-side record for one placed resource. Position
// is the tile; Kind picks the resource type; MaxAmount is the total
// yield potential at generation time; CurrentAmount equals MaxAmount
// in the static-placement phase (no depletion) and will drift below
// MaxAmount in the later dynamic-resources phase. LastRespawn is zero
// in the static phase and wired in the dynamic phase.
//
// For zonal kinds (Fertile / Timber / Game) a Deposit represents one
// tile inside a richness zone — not the whole zone. Consumers treating
// a zone as a unit must aggregate Deposits with the same Kind in a
// locality.
//
// The zero-value Deposit has Kind == DepositNone and is the documented
// invalid state; DepositAt returns (Deposit{}, false) when no deposit
// covers the queried tile.
type Deposit struct {
	Position      geom.Position
	Kind          DepositKind
	MaxAmount     int32
	CurrentAmount int32
	LastRespawn   int64
}

// DepositSource is the consumer-side interface World delegates to for
// resource queries. The production implementation lives in
// worldgen.NoiseDepositSource.
//
// DepositAt returns the deposit on the exact tile p, or (Deposit{}, false)
// when none exists.
//
// DepositsIn returns every deposit whose Position lies inside rect.
// Iteration order is deterministic across calls with the same inputs
// so consumers (city placer, snapshot mapper) can assume stable output.
//
// DepositsNear returns every deposit within Chebyshev radius of p,
// sorted by distance ascending. Ties break by (X, Y) lex order so the
// result is fully deterministic. Used by the future contextual info-
// panel when a player approaches a feature.
type DepositSource interface {
	DepositAt(p geom.Position) (Deposit, bool)
	DepositsIn(rect geom.Rect) []Deposit
	DepositsNear(p geom.Position, radius int) []Deposit
}

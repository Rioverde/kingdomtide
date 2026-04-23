package stats

import (
	"errors"
	"fmt"
)

// CoreStats holds the six D&D 5e ability scores. Each is an integer raw
// score in the canonical 3-20 range; use Modifier to compute the D&D-style
// +/- bonus that downstream math (HP, Mana, damage, derived Speed) reads.
// A zero value is the empty distribution; callers get a neutral baseline
// via DefaultCoreStats.
type CoreStats struct {
	Strength     int
	Dexterity    int
	Constitution int
	Intelligence int
	Wisdom       int
	Charisma     int
}

// Modifier returns the standard D&D 5e ability modifier for a raw stat
// value: floor((stat - 10) / 2). Works correctly on the negative side too —
// Modifier(5) == -3, Modifier(1) == -5 — because Go integer division
// truncates toward zero; we adjust by one when the pre-division number is
// negative with a non-zero remainder.
func Modifier(stat int) int {
	n := stat - 10
	if n < 0 && n%2 != 0 {
		return n/2 - 1
	}
	return n / 2
}

// Point Buy cost table — price of buying a single ability up to N from
// the 5e PHB baseline of 8 (p.13). The table is non-linear: the cost per
// point jumps from 1 to 2 at 14. The map is read-only and shared.
var pointBuyCosts = map[int]int{
	8:  0,
	9:  1,
	10: 2,
	11: 3,
	12: 4,
	13: 5,
	14: 7,
	15: 9,
}

// Point Buy parameters. pointBuyBudget is the total points available;
// pointBuyMin / pointBuyMax bound a single ability score.
const (
	pointBuyBudget = 27
	pointBuyMin    = 8
	pointBuyMax    = 15
)

// PointBuyBudget is the total number of Point Buy points a character gets
// to distribute across the six abilities. Exported so the client's
// character-creation UI (CS4) displays the same budget the server enforces
// on join without hardcoding the number in two places.
const PointBuyBudget = pointBuyBudget

// PointBuyMin is the minimum Point Buy score for any single ability
// (matches the 5e SRD baseline of 8).
const PointBuyMin = pointBuyMin

// PointBuyMax is the maximum Point Buy score for any single ability
// (matches the 5e SRD cap of 15 before racial bonuses).
const PointBuyMax = pointBuyMax

// PointBuyCost returns the 5e Point Buy cost for a single ability score.
// Scores outside [PointBuyMin, PointBuyMax] return 0 — callers must
// range-check first; returning 0 for out-of-range inputs lets the client
// render the cost column without a branch while the constructor stays
// authoritative about legality. The returned value is the point price of
// holding that single score, not a running total.
func PointBuyCost(score int) int {
	return pointBuyCosts[score]
}

// ErrPointBuyBudget is returned when a distribution does not sum to
// pointBuyBudget. Wrap it with %w so callers can errors.Is against the
// sentinel.
var ErrPointBuyBudget = errors.New("point-buy: distribution must sum to 27")

// ErrPointBuyRange is returned when any single ability is outside the
// allowed [pointBuyMin, pointBuyMax] range.
var ErrPointBuyRange = errors.New("point-buy: stats must be in [8, 15]")

// NewStatsPointBuy validates a full six-ability distribution under the 5e
// Point Buy 27 rules and returns the assembled CoreStats. It rejects
// out-of-range scores with ErrPointBuyRange and budget mismatches with
// ErrPointBuyBudget; on success every derived field (MaxHP, Mana, ...) is
// ready to be read off the returned value.
func NewStatsPointBuy(str, dex, con, intel, wis, cha int) (*CoreStats, error) {
	values := []int{str, dex, con, intel, wis, cha}
	total := 0
	for _, v := range values {
		if v < pointBuyMin || v > pointBuyMax {
			return nil, fmt.Errorf("%w: got %d", ErrPointBuyRange, v)
		}
		total += pointBuyCosts[v]
	}
	if total != pointBuyBudget {
		return nil, fmt.Errorf("%w: got %d", ErrPointBuyBudget, total)
	}
	return &CoreStats{
		Strength:     str,
		Dexterity:    dex,
		Constitution: con,
		Intelligence: intel,
		Wisdom:       wis,
		Charisma:     cha,
	}, nil
}

// DefaultCoreStats returns the neutral baseline distribution (all 10s,
// modifier 0 everywhere). Used for monsters without a registry entry and
// as the fallback when a client joins without a stat payload. The return
// is a value, not a pointer, to keep the call site unambiguous about
// ownership.
func DefaultCoreStats() CoreStats {
	return CoreStats{
		Strength:     10,
		Dexterity:    10,
		Constitution: 10,
		Intelligence: 10,
		Wisdom:       10,
		Charisma:     10,
	}
}

// Derivation constants. Every scale factor that turns ability scores into
// runtime pools lives here so balancing changes touch one file. Values
// are first-pass: baseHP of 10 plus 6 per CON modifier puts a balanced
// character (CON 10) at 10 HP and a CON 14 character at 22 HP.
const (
	baseHP         = 10
	hpPerLevel     = 6
	baseMana       = 5
	manaPerLevel   = 4
	weaponDamage   = 2 // 1d4 average, rounded down
	speedPerDexMod = 1
)

// MaxHP returns the total hit points derived from the CoreStats: the base
// pool plus CON modifier scaled by hpPerLevel. A character with CON 10
// has exactly baseHP; CON 14 adds +2 * hpPerLevel on top.
func (s CoreStats) MaxHP() int {
	return baseHP + Modifier(s.Constitution)*hpPerLevel
}

// Mana returns the mana pool derived from INT: the base plus INT modifier
// scaled by manaPerLevel.
func (s CoreStats) Mana() int {
	return baseMana + Modifier(s.Intelligence)*manaPerLevel
}

// BaseDamage returns the unmodified weapon damage: the weapon's base plus
// STR modifier. Combat.go applies per-slot multipliers on top.
func (s CoreStats) BaseDamage() int {
	return weaponDamage + Modifier(s.Strength)
}

// DerivedSpeed returns the entity's tick-resolution Speed scaled by DEX
// modifier. The baseline SpeedNormal maps to DEX 10; every point of DEX
// modifier shifts Speed by speedPerDexMod so fast characters accumulate
// Energy quicker under the tick model.
func (s CoreStats) DerivedSpeed() int {
	return SpeedNormal + Modifier(s.Dexterity)*speedPerDexMod
}

// DerivedInitiative returns the initiative modifier used as the within-
// tick ordering tiebreaker. In 5e this equals the DEX modifier verbatim.
func (s CoreStats) DerivedInitiative() int {
	return Modifier(s.Dexterity)
}

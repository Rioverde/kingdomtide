package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// primogenitureParentBias pulls the heir's stats toward the
	// outgoing ruler's (kin similarity).
	primogenitureParentBias = 3

	// electiveCharismaBias reflects that faction-voted rulers skew
	// toward the factionally-preferred trait (we use Charisma as
	// the default electoral asset).
	electiveCharismaBias = 2

	// tanistryPoolVariance captures "elected from kin group" — a
	// wider stat distribution than primogeniture since any adult
	// kin member can be chosen.
	tanistryPoolVariance = 4

	// salicMaleOnlyBias adds a Strength preference (male-line heirs
	// historically favored martial stat allocation).
	salicMaleOnlyBias = 2
)

// newHeirFor draws a new ruler whose stats reflect the succession
// law of the kingdom. For MVP the law biases the stat roll only —
// the full model would also resolve kin group, faction vote, and
// legitimacy, but those need data we do not yet track.
func newHeirFor(k *polity.Kingdom, stream *dice.Stream, currentYear int) polity.Ruler {
	base := polity.NewRuler(stream, currentYear)
	switch k.SuccessionLaw {
	case polity.SuccessionPrimogeniture:
		return biasTowardParent(base, k.CurrentRuler, primogenitureParentBias)
	case polity.SuccessionUltimogeniture:
		// Youngest child — same bias as primogeniture in MVP (family
		// similarity), but slightly less (younger children drift more).
		return biasTowardParent(base, k.CurrentRuler, primogenitureParentBias-1)
	case polity.SuccessionTanistry:
		return addVariance(base, stream, tanistryPoolVariance)
	case polity.SuccessionElective:
		return biasStat(base, charismaField, electiveCharismaBias)
	case polity.SuccessionDesignated:
		// Current ruler names heir — stats biased toward ruler's strongest.
		return biasTowardParent(base, k.CurrentRuler, primogenitureParentBias+1)
	case polity.SuccessionSalic:
		return biasStat(
			biasTowardParent(base, k.CurrentRuler, primogenitureParentBias),
			strengthField, salicMaleOnlyBias,
		)
	}
	return base
}

// biasTowardParent pulls each of the heir's stats toward the outgoing
// ruler's by `bias` points (clamped to [3, 20]).
func biasTowardParent(heir, parent polity.Ruler, bias int) polity.Ruler {
	heir.Stats.Strength = pullTowards(heir.Stats.Strength, parent.Stats.Strength, bias)
	heir.Stats.Dexterity = pullTowards(heir.Stats.Dexterity, parent.Stats.Dexterity, bias)
	heir.Stats.Constitution = pullTowards(heir.Stats.Constitution, parent.Stats.Constitution, bias)
	heir.Stats.Intelligence = pullTowards(heir.Stats.Intelligence, parent.Stats.Intelligence, bias)
	heir.Stats.Wisdom = pullTowards(heir.Stats.Wisdom, parent.Stats.Wisdom, bias)
	heir.Stats.Charisma = pullTowards(heir.Stats.Charisma, parent.Stats.Charisma, bias)
	return heir
}

// pullTowards moves `current` toward `target` by up to `step`
// points per call, clamped to the D&D-style 3-20 range.
func pullTowards(current, target, step int) int {
	if current < target {
		current += step
	} else if current > target {
		current -= step
	}
	return min(20, max(3, current))
}

// addVariance jitters each stat by ±`variance` via a D20 roll. Used
// for Tanistry where the elected kin may fall far from parent.
func addVariance(r polity.Ruler, stream *dice.Stream, variance int) polity.Ruler {
	adj := func(s int) int {
		delta := stream.D20() - 10 // [-9, +10] — scaled to variance window
		delta = delta * variance / 10
		return min(20, max(3, s+delta))
	}
	r.Stats.Strength = adj(r.Stats.Strength)
	r.Stats.Dexterity = adj(r.Stats.Dexterity)
	r.Stats.Constitution = adj(r.Stats.Constitution)
	r.Stats.Intelligence = adj(r.Stats.Intelligence)
	r.Stats.Wisdom = adj(r.Stats.Wisdom)
	r.Stats.Charisma = adj(r.Stats.Charisma)
	return r
}

// statField names the stat a biasStat call targets. Using an enum
// rather than a `*int` keeps the bias safely scoped to the value
// copy biasStat owns; a raw pointer would reach back into the
// caller's local and miss the returned copy (the bug that masked
// Elective's Charisma boost pre-Wave 7).
type statField uint8

const (
	strengthField statField = iota
	charismaField
)

// biasStat adds `bias` to the named stat of a value-copy Ruler and
// returns it, clamped to the D&D [3, 20] range. Used for laws that
// favor one trait (Salic → Strength, Elective → Charisma).
func biasStat(r polity.Ruler, field statField, bias int) polity.Ruler {
	switch field {
	case strengthField:
		r.Stats.Strength = min(20, max(3, r.Stats.Strength+bias))
	case charismaField:
		r.Stats.Charisma = min(20, max(3, r.Stats.Charisma+bias))
	}
	return r
}

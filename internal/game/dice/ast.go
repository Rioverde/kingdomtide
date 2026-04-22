package dice

import "math/rand/v2"

// node is the internal AST interface every parsed fragment implements.
// Every node is IMMUTABLE after Parse returns — execute must never
// mutate any field on the receiver. This invariant is what lets a
// single Expression be shared across goroutines safely: concurrent
// callers each bring their own *rand.Rand and their own accumulator
// slices, and the shared tree is read-only.
type node interface {
	execute(
		rng *rand.Rand,
		dice *[]DieRoll,
		terms *[]TermResult,
		warns *[]CapWarning,
	) int
	stats() termStats
}

// termNode covers a single dice group: "2d6", "4d6dl1", "1d6!".
// Reroll and explode fields are present so the execution loop can honour
// them without restructuring the AST; execute honours whichever fields
// the parser sets.
type termNode struct {
	count, sides int
	modifier     int
	keep         keepMode
	keepCount    int
	reroll       *rerollSpec
	explode      *explodeSpec
	fudge        bool
}

// keepMode describes the dice-filtering style applied to a term.
type keepMode uint8

const (
	keepAll keepMode = iota
	keepHigh
	keepLow
	dropHigh
	dropLow
)

// rerollSpec captures the parsed reroll modifier. once distinguishes
// "r" (Roll20: single replacement) from "rr" (Foundry: recursive).
type rerollSpec struct {
	op    comparisonOp
	value int
	once  bool
}

// explodeSpec captures the parsed explode modifier.
type explodeSpec struct {
	op    comparisonOp
	value int
}

// comparisonOp enumerates the predicates used by explode and reroll
// modifiers. cmpEq is the default when no operator is written.
type comparisonOp uint8

const (
	cmpEq comparisonOp = iota
	cmpLt
	cmpLe
	cmpGe
	cmpGt
)

// matches reports whether roll satisfies the op/value predicate.
func (op comparisonOp) matches(roll, value int) bool {
	switch op {
	case cmpEq:
		return roll == value
	case cmpLt:
		return roll < value
	case cmpLe:
		return roll <= value
	case cmpGe:
		return roll >= value
	case cmpGt:
		return roll > value
	}
	return false
}

// exprNode is the compound-expression root node. A single-term input
// like "1d20" still wraps in an exprNode with one term and sign +1 so
// Execute and Stats can assume a uniform shape. Compound forms like
// "1d20+1d4+5" populate multiple terms plus a bareMod.
//
// terms[i] is paired with signs[i] (+1 or -1). bareMod is the sum of
// all leading / trailing numeric constants that were parsed outside a
// term (they fold together at parse time: "5+2d6-3" stores bareMod=2).
type exprNode struct {
	terms   []node
	signs   []int
	bareMod int
}

// termStats is the internal statistics representation shared between
// basic and compound nodes. Exported Stats is derived from this.
type termStats struct {
	mean    float64
	variance float64
	min     int
	max     int
	exact   bool
}

// explodeAlwaysTriggers reports whether every face of a die with the
// given number of sides satisfies the explode predicate. Used by the
// parser to reject unreachable-terminator expressions at Parse time.
func explodeAlwaysTriggers(sides int, spec *explodeSpec) bool {
	if spec == nil || sides <= 0 {
		return false
	}
	for face := 1; face <= sides; face++ {
		if !spec.op.matches(face, spec.value) {
			return false
		}
	}
	return true
}

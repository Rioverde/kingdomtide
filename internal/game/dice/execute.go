package dice

import (
	"math/rand/v2"
	"sort"
)

// execute rolls a single termNode against rng and appends every die to
// the shared dice accumulator along with a TermResult record. Returns
// the term's contribution to the expression total (post keep/drop,
// post modifier). Execute order is: roll N root dice → apply reroll
// (r / rr) per die → apply explode (!) to the live pool → apply
// keep/drop to the full post-explosion pool.
//
// Fudge dice are stored in DieRoll.Value with their signed face value
// in {-1, 0, +1}. Regular dice hold positive faces in [1..sides].
//
// CapWarnings populate when the per-term explode/reroll depth hits
// maxExplodeDepth / maxRerollDepth. The Sign field on the generated
// TermResult is filled by the surrounding exprNode; here we leave it
// zero and let the caller patch it up.
func (t *termNode) execute(
	rng *rand.Rand,
	dice *[]DieRoll,
	terms *[]TermResult,
	warns *[]CapWarning,
) int {
	startIdx := len(*dice)
	termIdx := len(*terms)

	// Root rolls first.
	for i := 0; i < t.count; i++ {
		*dice = append(*dice, DieRoll{
			Value:       t.rollFace(rng),
			Source:      DieSourceRolled,
			ParentIndex: -1,
		})
	}

	// Reroll: inspect the root rolls only. rerollSpec.once replaces
	// the die a single time; the non-once ("rr") form loops until the
	// predicate fails or the per-term cap is hit. Fudge dice never
	// reroll (the spec's value/op semantics don't apply to {-1,0,1}).
	if t.reroll != nil && !t.fudge {
		t.applyReroll(rng, dice, warns, startIdx, termIdx)
	}

	// Explode: scan the current pool (root + any reroll replacements)
	// and spawn exploded children inline. Children can themselves
	// trigger explodes until the per-term depth cap is hit.
	if t.explode != nil && !t.fudge {
		t.applyExplode(rng, dice, warns, startIdx, termIdx)
	}

	endIdx := len(*dice)
	// Keep/drop applies to the full post-explosion pool. Rerolled-away
	// dice (already Dropped=true) are excluded from sorting so they
	// don't compete for the keep slots.
	t.applyKeepDrop(*dice, startIdx, endIdx)

	sum := 0
	for i := startIdx; i < endIdx; i++ {
		if !(*dice)[i].Dropped {
			sum += (*dice)[i].Value
		}
	}
	total := sum + t.modifier

	*terms = append(*terms, TermResult{
		Count:     t.count,
		Sides:     t.sides,
		Modifier:  t.modifier,
		Sign:      1, // patched by exprNode.execute
		DiceStart: startIdx,
		DiceEnd:   endIdx,
		Total:     total,
	})
	return total
}

// rollFace rolls one face for this term. For fudge dice the result is
// picked uniformly from {-1, 0, +1}; for plain dice it is uniformly
// in [1..sides].
func (t *termNode) rollFace(rng *rand.Rand) int {
	if t.fudge {
		switch rng.IntN(3) {
		case 0:
			return -1
		case 1:
			return 0
		default:
			return 1
		}
	}
	return rng.IntN(t.sides) + 1
}

// applyReroll walks the root dice in range [start, start+count) and
// replaces each one matching the reroll predicate. For 'r' (once) the
// original die is marked Dropped=true and a new DieRoll with
// Source=DieSourceRerolled is appended with ParentIndex pointing at
// the original; the replacement stands even if it would match. For
// 'rr' (recursive) the loop continues until a non-matching replacement
// lands or the cap is hit, at which point a CapWarningReroll fires.
func (t *termNode) applyReroll(
	rng *rand.Rand,
	dice *[]DieRoll,
	warns *[]CapWarning,
	start, termIdx int,
) {
	spec := t.reroll
	end := start + t.count
	capHit := false
	for i := start; i < end; i++ {
		original := i
		if !spec.op.matches((*dice)[original].Value, spec.value) {
			continue
		}
		depth := 0
		current := original
		for spec.op.matches((*dice)[current].Value, spec.value) {
			if depth >= maxRerollDepth {
				capHit = true
				break
			}
			(*dice)[current].Dropped = true
			newIdx := len(*dice)
			*dice = append(*dice, DieRoll{
				Value:       t.rollFace(rng),
				Source:      DieSourceRerolled,
				ParentIndex: current,
			})
			depth++
			if spec.once {
				// Roll20 'r': replacement stands.
				current = newIdx
				break
			}
			current = newIdx
		}
	}
	if capHit {
		*warns = append(*warns, CapWarning{
			Kind:  CapWarningReroll,
			Term:  termIdx,
			Limit: maxRerollDepth,
		})
	}
}

// applyExplode scans the live dice pool (non-dropped) and spawns an
// exploded child for each die whose value satisfies the predicate.
// Children are appended at the end of the dice slice and themselves
// become eligible to explode — the scan continues until no new
// explosions occur or maxExplodeDepth total explosions have happened
// for this term, at which point a CapWarningExplode fires.
func (t *termNode) applyExplode(
	rng *rand.Rand,
	dice *[]DieRoll,
	warns *[]CapWarning,
	start, termIdx int,
) {
	spec := t.explode
	cursor := start
	explosions := 0
	capHit := false
	for cursor < len(*dice) {
		die := (*dice)[cursor]
		if !die.Dropped && spec.op.matches(die.Value, spec.value) {
			if explosions >= maxExplodeDepth {
				capHit = true
				break
			}
			*dice = append(*dice, DieRoll{
				Value:       t.rollFace(rng),
				Source:      DieSourceExploded,
				ParentIndex: cursor,
			})
			explosions++
		}
		cursor++
	}
	if capHit {
		*warns = append(*warns, CapWarning{
			Kind:  CapWarningExplode,
			Term:  termIdx,
			Limit: maxExplodeDepth,
		})
	}
}

// applyKeepDrop marks DieRoll.Dropped on the filtered dice in place.
// Already-dropped dice (rerolled-away) are excluded from the sort so
// they don't fight for keep slots. Roll order is preserved in the
// original Dice slice — UI consumers rely on it for display.
func (t *termNode) applyKeepDrop(dice []DieRoll, start, end int) {
	if t.keep == keepAll || t.keepCount <= 0 {
		return
	}
	order := make([]int, 0, end-start)
	for i := start; i < end; i++ {
		if dice[i].Dropped {
			continue
		}
		order = append(order, i)
	}
	n := len(order)
	if n == 0 {
		return
	}
	sort.SliceStable(order, func(a, b int) bool {
		return dice[order[a]].Value < dice[order[b]].Value
	})

	// keepCount may exceed the live pool after reroll dropouts; clamp
	// so we never address out-of-range indices.
	k := t.keepCount
	if k > n {
		k = n
	}

	var dropSet []int
	switch t.keep {
	case keepHigh:
		dropSet = order[:n-k]
	case keepLow:
		dropSet = order[k:]
	case dropHigh:
		dropSet = order[n-k:]
	case dropLow:
		dropSet = order[:k]
	}
	for _, idx := range dropSet {
		dice[idx].Dropped = true
	}
}

// execute on an exprNode sums signed term contributions and threads
// its bareMod through. Patches the Sign field on every TermResult its
// children pushed so callers see the final signed contribution.
func (e *exprNode) execute(
	rng *rand.Rand,
	dice *[]DieRoll,
	terms *[]TermResult,
	warns *[]CapWarning,
) int {
	total := e.bareMod
	for i, n := range e.terms {
		before := len(*terms)
		contrib := n.execute(rng, dice, terms, warns)
		// Patch the Sign on the TermResults that were just appended.
		// A termNode pushes exactly one; nested exprNodes would push
		// more, but the exprNode grammar is flat — a single '+'/'-' chain,
		// no nesting.
		for j := before; j < len(*terms); j++ {
			(*terms)[j].Sign = e.signs[i]
		}
		total += e.signs[i] * contrib
	}
	return total
}

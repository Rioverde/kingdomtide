package dice

import "sync"

// stats returns the closed-form or convolution-derived distribution
// summary for a termNode. Exact=true when every field comes from a
// closed-form expression; Exact=false when the answer relied on the
// cached numerical convolution or an approximation.
//
// Closed-form coverage:
//
//   - Basic NdS+M                     (no modifiers)
//   - Fudge NdF+M
//   - 2d20kh1 / 2d20kl1               (advantage / disadvantage)
//   - Exploding NdS!                  (equality on the max face)
//   - Reroll-once NdS r=V or NdS r<K  (predictable closed form)
//
// Everything else (general keep/drop, combined keep/drop + explode,
// rr recursive reroll, non-uniform explode predicates) is handled by
// enumeration or approximation with Exact=false.
func (t *termNode) stats() termStats {
	// Explode + reroll dominate keep/drop for stats purposes — we
	// currently have closed forms for the common shapes. When those
	// apply, skip the keep/drop convolution path.
	if t.reroll != nil || t.explode != nil {
		return t.modifiedStats()
	}
	if t.keep == keepAll || t.keepCount == 0 {
		return t.basicStats()
	}
	if t.count == 2 && t.keepCount == 1 && !t.fudge {
		switch t.keep {
		case keepHigh:
			ts := keepHighOneOfTwoStats(t.sides)
			return applyModifier(ts, t.modifier)
		case keepLow:
			ts := keepLowOneOfTwoStats(t.sides)
			return applyModifier(ts, t.modifier)
		}
	}
	ts := convolveKeepDropStats(t)
	return applyModifier(ts, t.modifier)
}

// modifiedStats computes the per-die mean/variance/range for a term
// that carries an explode or reroll modifier, multiplies by count,
// adds the flat modifier, and returns. Combined modifiers (reroll +
// explode on the same term) fall back to approximate stats with
// Exact=false — closed-form combined math is future work.
func (t *termNode) modifiedStats() termStats {
	if t.fudge {
		// Fudge dice + explode/reroll are nonsensical in practice but
		// don't blow up: treat them as plain fudge for stats.
		return applyModifier(plainFudgeStats(t.count), t.modifier)
	}

	var perDie termStats

	switch {
	case t.explode != nil && t.reroll == nil:
		perDie = explodeStats(t.sides, t.explode)
	case t.reroll != nil && t.explode == nil:
		perDie = rerollStats(t.sides, t.reroll)
	default:
		// Combined reroll + explode: fall back to a coarse range-based
		// approximation. Mean and variance are left at the plain NdS
		// baseline; Min/Max come from the per-die floor/ceiling.
		s := float64(t.sides)
		mean := (s + 1) / 2
		variance := (s*s - 1) / 12
		perDie = termStats{
			mean:     mean,
			variance: variance,
			min:      1,
			max:      t.sides,
			exact:    false,
		}
	}

	// Scale up for N dice. Per-die min/max scale linearly.
	n := float64(t.count)
	ts := termStats{
		mean:     n * perDie.mean,
		variance: n * perDie.variance,
		min:      t.count * perDie.min,
		max:      t.count * perDie.max,
		exact:    perDie.exact,
	}
	// Explode has no upper bound in theory; surface the cap-scaled
	// ceiling (root × (depth+1) × sides) as a conservative Max.
	if t.explode != nil {
		ts.max = t.count * (maxExplodeDepth + 1) * t.sides
	}
	return applyModifier(ts, t.modifier)
}

// explodeStats returns the closed-form per-die mean and variance of a
// single exploding die. Only the canonical form (explode on the max
// face, op==cmpEq and value==sides) has a tight closed form; other
// predicates fall back to a coarse approximation with Exact=false.
//
// Derivation (explode-on-max):
//
//	Let X be the final value of one die. With probability 1/S the die
//	shows face S and re-rolls (adding another iid X'); with (S-1)/S it
//	lands on a face k ∈ {1..S-1} and stops.
//
//	E[X] = (1/S) sum_{k=1..S-1} k + (1/S)(S + E[X])
//	     = (S-1)/2 + 1 + E[X]/S
//	     = (S+1)/2 + E[X]/S
//	     E[X] (1 - 1/S) = (S+1)/2
//	     E[X] = (S+1)·S / (2·(S-1))
//
//	E[X^2] = (1/S) sum_{k=1..S-1} k^2 + (1/S)(S + X')^2
//	       = (1/S) sum_{k=1..S-1} k^2 + (1/S)(S^2 + 2·S·E[X] + E[X^2])
//	Simplifying via sum_{k=1..S} k^2 = S(S+1)(2S+1)/6:
//	     E[X^2]·(S-1) = S(S+1)(2S+1)/6 + 2·S·E[X]
//	     E[X^2] = [S(S+1)(2S+1)/6 + 2·S·E[X]] / (S-1)
//
//	V[X] = E[X^2] - E[X]^2
func explodeStats(sides int, spec *explodeSpec) termStats {
	s := float64(sides)
	// Canonical closed-form path: explode-on-max (cmpEq S) or the
	// equivalent ">=S" / ">S-1" forms.
	if isExplodeOnMax(sides, spec) {
		mean := (s + 1) * s / (2 * (s - 1))
		sumSq := s * (s + 1) * (2*s + 1) / 6
		eSq := (sumSq + 2*s*mean) / (s - 1)
		variance := eSq - mean*mean
		return termStats{
			mean:     mean,
			variance: variance,
			min:      1,
			max:      sides,
			exact:    true,
		}
	}
	// Non-canonical predicate. Let p = P(trigger). Every face
	// contributes its value k with probability 1/S before any
	// recursion; with probability p a trigger spawns another iid X.
	//
	//   E[X] = (1/S)·Σk·(1 + p_k) + p·E[X]  simplifies via
	//   E[X]·(1 - p) = (S+1)/2
	//   E[X] = ((S+1)/2) / (1 - p)
	//
	// For variance we fall back to a numerical approximation:
	//   Var(X) = Var(base) / (1-p)^2  (coarse — ignores cross-term).
	// Non-canonical predicates are advisory so Exact=false.
	p, _, _ := explodePredicateMoments(sides, spec)
	if p >= 1 {
		p = 1 - 1e-9
	}
	mean := ((s + 1) / 2) / (1 - p)
	baseVar := (s*s - 1) / 12
	variance := baseVar / ((1 - p) * (1 - p))
	return termStats{
		mean:     mean,
		variance: variance,
		min:      1,
		max:      sides,
		exact:    false,
	}
}

// isExplodeOnMax reports whether the spec is equivalent to "trigger
// when the die shows its top face and only its top face".
func isExplodeOnMax(sides int, spec *explodeSpec) bool {
	if spec == nil {
		return false
	}
	switch spec.op {
	case cmpEq:
		return spec.value == sides
	case cmpGe:
		return spec.value == sides
	case cmpGt:
		return spec.value == sides-1
	}
	return false
}

// explodePredicateMoments returns (P(trigger), E[non-trigger contribution
// weighted by probability], E[face^2] weighted by probability of that
// face). Used only by the non-canonical explode approximation.
func explodePredicateMoments(sides int, spec *explodeSpec) (float64, float64, float64) {
	var (
		trigCount int
		sumNon    float64
		sumSq     float64
	)
	for k := 1; k <= sides; k++ {
		if spec.op.matches(k, spec.value) {
			trigCount++
			// Trigger face contributes k itself plus the recursive E[X];
			// the recursion is absorbed into the eNon / (1-p) formula.
			sumSq += float64(k*k) / float64(sides)
			continue
		}
		sumNon += float64(k) / float64(sides)
		sumSq += float64(k*k) / float64(sides)
	}
	p := float64(trigCount) / float64(sides)
	return p, sumNon, sumSq
}

// rerollStats returns the closed-form per-die mean of one reroll die.
// Exact for 'r' (reroll once) on any comparison predicate; for 'rr'
// the answer is the post-reject conditional mean (the reroll loop
// terminates on a non-matching face) which is also exact in theory.
// Variance is treated as exact for 'r' (we enumerate the two-step
// distribution) and approximate for 'rr' (we use the conditional
// variance on non-matching faces).
//
// Derivation (reroll once, r predicate):
//
//	Let T = { faces that trigger reroll }. |T| = m.
//	Let A = { faces that don't trigger }. |A| = S - m.
//	E[X] = P(no trigger)·E[A] + P(trigger)·E[{1..S}]
//	     = ((S-m)/S)·mean(A) + (m/S)·(S+1)/2
//
//	For variance with r we enumerate explicitly (see erollOnceVar).
//
// Derivation (reroll until satisfied, rr predicate):
//
//	The loop terminates as soon as a face in A appears, so the final
//	value is uniform on A:
//	  E[X] = mean(A),  V[X] = V(uniform on A) = mean(A^2) - mean(A)^2
func rerollStats(sides int, spec *rerollSpec) termStats {
	trigCount := 0
	var sumA, sumASq float64
	for k := 1; k <= sides; k++ {
		if spec.op.matches(k, spec.value) {
			trigCount++
			continue
		}
		sumA += float64(k)
		sumASq += float64(k * k)
	}
	nonCount := sides - trigCount

	// Pathological impossible-predicate case (every face triggers):
	// stats are essentially undefined. The runtime cap still
	// terminates the rr loop eventually. Return a uniform-on-sides
	// approximation so callers get a finite number.
	if nonCount == 0 {
		s := float64(sides)
		mean := (s + 1) / 2
		return termStats{
			mean:     mean,
			variance: (s*s - 1) / 12,
			min:      1,
			max:      sides,
			exact:    false,
		}
	}

	if !spec.once {
		// rr: final value uniform on non-triggering faces.
		meanA := sumA / float64(nonCount)
		varA := sumASq/float64(nonCount) - meanA*meanA
		return termStats{
			mean:     meanA,
			variance: varA,
			min:      rerollMin(sides, spec),
			max:      rerollMax(sides, spec),
			exact:    true,
		}
	}

	// r: reroll once.
	s := float64(sides)
	meanAll := (s + 1) / 2
	meanAny := sumA / float64(nonCount)
	pTrig := float64(trigCount) / s
	mean := (1-pTrig)*meanAny + pTrig*meanAll
	// Variance via E[X^2] - mean^2.
	// E[X^2] = P(no trigger) · E[A^2] + P(trigger) · E[{1..S}^2]
	eSqAll := s * (s + 1) * (2*s + 1) / (6 * s) // = (s+1)(2s+1)/6
	eSqA := sumASq / float64(nonCount)
	eSq := (1-pTrig)*eSqA + pTrig*eSqAll
	variance := eSq - mean*mean
	return termStats{
		mean:     mean,
		variance: variance,
		min:      1,
		max:      sides,
		exact:    true,
	}
}

// rerollMin / rerollMax bracket the range of an rr die. For simple
// predicates on {1..S} we return the non-matching face span; for
// unusual predicates we conservatively report [1..S].
func rerollMin(sides int, spec *rerollSpec) int {
	for k := 1; k <= sides; k++ {
		if !spec.op.matches(k, spec.value) {
			return k
		}
	}
	return 1
}

func rerollMax(sides int, spec *rerollSpec) int {
	for k := sides; k >= 1; k-- {
		if !spec.op.matches(k, spec.value) {
			return k
		}
	}
	return sides
}

// plainFudgeStats is the fudge-die baseline used when explode/reroll
// combine with fudge (a degenerate case callers shouldn't reach but
// handled here for robustness).
func plainFudgeStats(count int) termStats {
	n := float64(count)
	return termStats{
		mean:     0,
		variance: n * (2.0 / 3.0),
		min:      -count,
		max:      count,
		exact:    true,
	}
}

// applyModifier shifts a term's mean/min/max by its flat modifier.
func applyModifier(ts termStats, modifier int) termStats {
	ts.mean += float64(modifier)
	ts.min += modifier
	ts.max += modifier
	return ts
}

// basicStats computes the exact distribution of NdS+M (or the fudge
// equivalent). No RNG, no allocation.
func (t *termNode) basicStats() termStats {
	n := float64(t.count)
	if t.fudge {
		return termStats{
			mean:     float64(t.modifier),
			variance: n * (2.0 / 3.0),
			min:      -t.count + t.modifier,
			max:      t.count + t.modifier,
			exact:    true,
		}
	}
	s := float64(t.sides)
	return termStats{
		mean:     n*(s+1)/2 + float64(t.modifier),
		variance: n * (s*s - 1) / 12,
		min:      t.count + t.modifier,
		max:      t.count*t.sides + t.modifier,
		exact:    true,
	}
}

// keepHighOneOfTwoStats returns the closed-form stats for 2dS keep
// highest 1. Mean = Σ k·(2k-1) / S² for k=1..S.
func keepHighOneOfTwoStats(sides int) termStats {
	s := sides
	sumK := 0
	sumK2 := 0
	for k := 1; k <= s; k++ {
		sumK += k * (2*k - 1)
		sumK2 += k * k * (2*k - 1)
	}
	denom := float64(s) * float64(s)
	mean := float64(sumK) / denom
	meanSq := float64(sumK2) / denom
	return termStats{
		mean:     mean,
		variance: meanSq - mean*mean,
		min:      1,
		max:      sides,
		exact:    true,
	}
}

// keepLowOneOfTwoStats returns the closed-form stats for 2dS keep
// lowest 1. Mean = Σ k·(2(S-k)+1) / S² for k=1..S.
func keepLowOneOfTwoStats(sides int) termStats {
	s := sides
	sumK := 0
	sumK2 := 0
	for k := 1; k <= s; k++ {
		w := 2*(s-k) + 1
		sumK += k * w
		sumK2 += k * k * w
	}
	denom := float64(s) * float64(s)
	mean := float64(sumK) / denom
	meanSq := float64(sumK2) / denom
	return termStats{
		mean:     mean,
		variance: meanSq - mean*mean,
		min:      1,
		max:      sides,
		exact:    true,
	}
}

// convolutionCacheKey keys the general keep/drop stats cache.
// keepCount participates in the key because filters with different
// counts yield different distributions.
type convolutionCacheKey struct {
	count, sides int
	mode         keepMode
	keepCount    int
	fudge        bool
}

var convolutionCache sync.Map

// convolveKeepDropStats enumerates the full joint distribution of N
// dice, applies the keep/drop filter, and returns the mean/variance/
// min/max of the resulting sum. Exact but flagged Exact=false because
// the computation is enumeration rather than a formula. Results are
// cached by key so repeated Stats() calls are O(1).
func convolveKeepDropStats(t *termNode) termStats {
	key := convolutionCacheKey{
		count:     t.count,
		sides:     t.sides,
		mode:      t.keep,
		keepCount: t.keepCount,
		fudge:     t.fudge,
	}
	if v, ok := convolutionCache.Load(key); ok {
		return v.(termStats)
	}
	ts := enumerateKeepDrop(t)
	convolutionCache.Store(key, ts)
	return ts
}

// maxEnumerationStates caps the joint search space enumerateKeepDrop
// is willing to walk.
const maxEnumerationStates uint64 = 10_000_000

// enumerateKeepDrop walks every combination of N dice with S faces
// and accumulates mean + variance + min + max of the filtered sum.
func enumerateKeepDrop(t *termNode) termStats {
	states := uint64(1)
	sides := uint64(t.sides)
	if t.fudge {
		sides = 3
	}
	tooLarge := false
	for i := 0; i < t.count; i++ {
		if sides != 0 && states > maxEnumerationStates/sides+1 {
			tooLarge = true
			break
		}
		states *= sides
		if states > maxEnumerationStates {
			tooLarge = true
			break
		}
	}
	if tooLarge {
		return approximateKeepDrop(t)
	}

	faces := make([]int, t.sides)
	if t.fudge {
		faces = []int{-1, 0, 1}
	} else {
		for i := range faces {
			faces[i] = i + 1
		}
	}
	f := len(faces)

	dice := make([]int, t.count)
	for i := range dice {
		dice[i] = faces[0]
	}
	sortedBuf := make([]int, t.count)
	idxs := make([]int, t.count)

	var (
		sumInt   int64
		sumSqInt int64
		combos   uint64
		minSum   int
		maxSum   int
		firstSet bool
	)

	for {
		copy(sortedBuf, dice)
		insertionSort(sortedBuf)
		sum := filteredSum(sortedBuf, t.keep, t.keepCount)
		sumInt += int64(sum)
		sumSqInt += int64(sum) * int64(sum)
		combos++
		if !firstSet {
			minSum = sum
			maxSum = sum
			firstSet = true
		} else {
			if sum < minSum {
				minSum = sum
			}
			if sum > maxSum {
				maxSum = sum
			}
		}

		k := 0
		for k < t.count {
			idxs[k]++
			if idxs[k] < f {
				dice[k] = faces[idxs[k]]
				break
			}
			idxs[k] = 0
			dice[k] = faces[0]
			k++
		}
		if k == t.count {
			break
		}
	}

	mean := float64(sumInt) / float64(combos)
	meanSq := float64(sumSqInt) / float64(combos)
	return termStats{
		mean:     mean,
		variance: meanSq - mean*mean,
		min:      minSum,
		max:      maxSum,
		exact:    false,
	}
}

// approximateKeepDrop returns a conservative closed-form approximation
// for keep/drop expressions whose joint state space exceeds
// maxEnumerationStates.
func approximateKeepDrop(t *termNode) termStats {
	basic := t.basicStats()
	basic.exact = false
	basic.mean -= float64(t.modifier)
	basic.min -= t.modifier
	basic.max -= t.modifier

	kept := keptCount(t.count, t.keep, t.keepCount)
	if kept <= 0 {
		kept = 1
	}
	if t.fudge {
		basic.min = -kept
		basic.max = kept
	} else {
		basic.min = kept
		basic.max = kept * t.sides
	}
	return basic
}

// keptCount returns the number of dice surviving a keep/drop filter of
// the given mode applied to count dice.
func keptCount(count int, mode keepMode, k int) int {
	switch mode {
	case keepHigh, keepLow:
		return k
	case dropHigh, dropLow:
		return count - k
	}
	return count
}

// filteredSum returns the sum of the sorted ascending slice after
// applying the keep/drop filter.
func filteredSum(sorted []int, mode keepMode, k int) int {
	n := len(sorted)
	switch mode {
	case keepAll:
		sum := 0
		for _, v := range sorted {
			sum += v
		}
		return sum
	case keepHigh:
		sum := 0
		for i := n - k; i < n; i++ {
			sum += sorted[i]
		}
		return sum
	case keepLow:
		sum := 0
		for i := 0; i < k; i++ {
			sum += sorted[i]
		}
		return sum
	case dropHigh:
		sum := 0
		for i := 0; i < n-k; i++ {
			sum += sorted[i]
		}
		return sum
	case dropLow:
		sum := 0
		for i := k; i < n; i++ {
			sum += sorted[i]
		}
		return sum
	}
	return 0
}

// insertionSort sorts a small int slice in place.
func insertionSort(s []int) {
	for i := 1; i < len(s); i++ {
		v := s[i]
		j := i - 1
		for j >= 0 && s[j] > v {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = v
	}
}

// stats on exprNode sums signed term stats with variance summed
// unsigned (V(-X) = V(X)) and min/max composed piecewise.
func (e *exprNode) stats() termStats {
	acc := termStats{exact: true}
	acc.mean = float64(e.bareMod)
	acc.min = e.bareMod
	acc.max = e.bareMod
	for i, n := range e.terms {
		ts := n.stats()
		sign := e.signs[i]
		acc.mean += float64(sign) * ts.mean
		acc.variance += ts.variance
		if sign > 0 {
			acc.min += ts.min
			acc.max += ts.max
		} else {
			acc.min -= ts.max
			acc.max -= ts.min
		}
		if !ts.exact {
			acc.exact = false
		}
	}
	return acc
}

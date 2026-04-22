package dice

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
)

// Expression is a parsed dice-notation tree. Immutable after Parse,
// safe to share across goroutines. The zero value is invalid: calling
// Execute on it panics with a clear message so skipping the Parse
// error check fails loudly rather than silently returning a zero
// Result.
type Expression struct {
	src  string
	root node
}

// Parse decodes a dice-notation string into an Expression. The error,
// when non-nil, is always a *ParseError carrying the 1-indexed column
// of the offending token for any recognised syntax problem.
func Parse(expr string) (Expression, error) {
	p := newParser(expr)
	root, err := p.parseExpression()
	if err != nil {
		return Expression{}, err
	}
	return Expression{src: expr, root: root}, nil
}

// MustParse is the panic-on-error sibling of Parse, intended for
// package-level var initializers where the literal is known at
// compile time. It mirrors regexp.MustCompile and template.Must. Do
// NOT use it on user input or test fixtures — tests should use Parse
// with t.Fatal so the failure surface is the test itself.
func MustParse(expr string) Expression {
	e, err := Parse(expr)
	if err != nil {
		panic("dice.MustParse(" + strconv.Quote(expr) + "): " + err.Error())
	}
	return e
}

// String returns the original notation string passed to Parse. Useful
// in error messages and debug output; not a canonicalised form.
func (e Expression) String() string {
	return e.src
}

// Result is one execution of an Expression. Every die rolled (including
// explode and reroll descendants) appears in Dice in left-to-right
// execution order with full provenance. CapWarnings is non-empty when a
// safety cap truncated a roll — callers must check this to expose
// determinism-visible truncation in their UI.
type Result struct {
	Total       int
	Modifier    int
	Dice        []DieRoll
	Terms       []TermResult
	CapWarnings []CapWarning
}

// DieRoll carries the full provenance of a single die rolled during
// Execute. Source distinguishes root rolls from explode and reroll
// descendants; ParentIndex points back into Result.Dice for
// explode/reroll children so UI tooltips can walk the parent chain.
// Dropped is true when a keep/drop modifier filtered this die out of
// the term total; the roll still appears in Dice for transparency.
type DieRoll struct {
	Value       int
	Source      DieSource
	ParentIndex int
	Dropped     bool
}

// DieSource identifies why a specific die was rolled.
type DieSource uint8

// Enumerated DieSource values.
const (
	DieSourceRolled DieSource = iota
	DieSourceExploded
	DieSourceRerolled
)

// String returns a human-readable tag for the source; useful in
// tooltips and debug output.
func (s DieSource) String() string {
	switch s {
	case DieSourceRolled:
		return "rolled"
	case DieSourceExploded:
		return "exploded"
	case DieSourceRerolled:
		return "rerolled"
	}
	return "unknown"
}

// TermResult is one dice-group contribution to a compound expression.
// For "2d6+3" there is a single TermResult with Count=2, Sides=6,
// Modifier=3, Sign=+1. In "2d6-1d4" the second term carries Sign=-1
// so UI tooltips can render the signed contribution directly.
// DiceStart and DiceEnd bracket the slice of Result.Dice belonging to
// this term (half-open, slice semantics); they include explode and
// reroll descendants produced during Execute, in roll order.
type TermResult struct {
	Count, Sides int
	Modifier     int
	Sign         int
	DiceStart    int
	DiceEnd      int
	Total        int
}

// CapWarning surfaces to callers when a safety cap truncated a roll.
// Term indexes into Result.Terms; Limit is the cap value hit.
type CapWarning struct {
	Kind  CapWarningKind
	Term  int
	Limit int
}

// CapWarningKind enumerates the kinds of truncation surfaced in
// Result.CapWarnings.
type CapWarningKind uint8

// Enumerated CapWarningKind values.
const (
	CapWarningExplode CapWarningKind = iota
	CapWarningReroll
)

// String returns the lowercase English identifier for debug and logging
// output. NOT for player-facing rendering — when the UI needs to show a
// cap warning it looks up the localized phrase via locale.Tr with a
// catalog key like "dice.cap.explode". This matches the project-wide
// convention where domain Key/String methods carry stable identifiers
// and player-visible text flows through the locale catalog.
func (k CapWarningKind) String() string {
	switch k {
	case CapWarningExplode:
		return "explode"
	case CapWarningReroll:
		return "reroll"
	}
	return "unknown"
}

// Key returns the stable catalog-key fragment for this warning kind.
// The UI layer composes the full locale key as "dice.cap."+k.Key() and
// passes it to locale.Tr — keeping the key stable here means renaming
// a Go identifier never silently breaks a translation catalog.
func (k CapWarningKind) Key() string {
	return k.String()
}

// Execute rolls the Expression once using rng. rng MUST be non-nil.
// Execute is deterministic: a given Expression and rng state produce
// an identical Result. Panics (with a clear message) if the receiver
// is a zero Expression — that state indicates a missing Parse error
// check upstream.
//
// Negative totals are allowed: "2d6-1d4" can plausibly come out
// negative when the rolls break the wrong way. Callers that need a
// clamped value (e.g. damage floors) apply the clamp themselves.
func (e Expression) Execute(rng *rand.Rand) Result {
	if e.root == nil {
		panic("dice: Execute called on zero Expression (did you skip the Parse error check?)")
	}
	if rng == nil {
		panic("dice: Execute called with nil *rand.Rand")
	}
	var res Result
	total := e.root.execute(rng, &res.Dice, &res.Terms, &res.CapWarnings)
	res.Total = total
	// Expression-level Modifier folds the bareMod (stored on the root
	// exprNode) with every term-local modifier × its sign so callers
	// can display "roll total minus flat modifiers" uniformly.
	if root, ok := e.root.(*exprNode); ok {
		res.Modifier = root.bareMod
	}
	for _, t := range res.Terms {
		res.Modifier += t.Sign * t.Modifier
	}
	return res
}

// Stats is the statistical summary of an Expression. Exact is true
// when every term uses a closed-form formula; false when any term
// required numerical approximation (keep/drop with count > 2,
// exploding dice, reroll). Balance tests should pin Exact=true values
// with tight tolerance (1e-9) and Exact=false values with the looser
// tolerance documented in the stats_test file.
type Stats struct {
	Mean   float64
	StdDev float64
	Min    int
	Max    int
	Exact  bool
}

// Stats returns the distribution summary for the Expression. Pure
// math — no RNG, no allocation in the common closed-form path.
func (e Expression) Stats() Stats {
	if e.root == nil {
		panic("dice: Stats called on zero Expression (did you skip the Parse error check?)")
	}
	ts := e.root.stats()
	return Stats{
		Mean:   ts.mean,
		StdDev: math.Sqrt(ts.variance),
		Min:    ts.min,
		Max:    ts.max,
		Exact:  ts.exact,
	}
}

// ParseError is returned by Parse on any syntactic failure. Column is
// 1-indexed into the original notation string; Msg is the reason, in
// the "dice: ..." style the package reserves for public error text.
type ParseError struct {
	Expr   string
	Column int
	Msg    string
}

// Error returns the formatted error message; column appears when > 0.
func (e *ParseError) Error() string {
	if e.Column > 0 {
		return fmt.Sprintf("dice: %s at column %d in %q", e.Msg, e.Column, e.Expr)
	}
	return fmt.Sprintf("dice: %s in %q", e.Msg, e.Expr)
}

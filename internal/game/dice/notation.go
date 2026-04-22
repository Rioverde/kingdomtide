package dice

// Grammar is the canonical dice-notation grammar accepted by Parse.
// Kept as an exported package-level constant so maintainers can cite
// it verbatim in error messages, documentation, and tests.
//
// The grammar is intentionally small — every supported form fits in a
// few productions. New modifier productions are added without reworking
// existing rules; backwards compatibility matters: every expression that
// parses today must continue to parse tomorrow.
const Grammar = `
expression        = term_or_constant (('+' | '-') term_or_constant)*
term_or_constant  = term | digit+
term              = [count] ('d' | 'D') sides [modifiers]
count             = digit+
sides             = digit+ | '%' | 'F' | 'f'
modifiers         = (keepMod | dropMod | explodeMod | rerollMod)*
keepMod           = ('kh' | 'kl') digit+
dropMod           = ('dh' | 'dl') digit+
explodeMod        = '!' [comparison] [digit+]
rerollMod         = ('r' | 'rr') [comparison] digit+
comparison        = '<' | '<=' | '>=' | '>' | '='
`

// Modifier and RNG conventions (for maintainers extending this package):
//
// Reroll semantics:
//
//	r<N>   reroll ONCE if the die shows <N>. The replacement stands even
//	       if it would also trigger the condition. Matches Roll20.
//	rr<N>  reroll RECURSIVELY until a non-matching value appears, capped
//	       at maxRerollDepth. Matches Foundry.
//
// The Roll20/Foundry split on `r` vs `rr` is the one piece of dice
// notation the community has never converged on — document the choice
// once here so future maintainers don't relitigate it.
//
// Explode semantics:
//
//	!       explode on the maximum face of the die (sides).
//	!<N>    explode when the die shows exactly N.
//	!>N     explode when the die shows strictly greater than N.
//	!>=N    explode when the die shows N or greater.
//	!<N     explode when the die shows strictly less than N.
//	!<=N    explode when the die shows N or less.
//
// Exploding dice are validated at Parse time: if the explode predicate
// matches every face of the die, the expression is rejected. This
// catches "1d1!", "1d6!>0", "1d2!>=2", and similar traps loudly rather
// than running into the runtime depth cap. See ast.go for the check.
//
// Safety caps:
//
//	maxDieCount       10000 — rejected at Parse time.
//	maxDieSides       10000 — rejected at Parse time.
//	maxExplodeDepth     100 — surfaces as CapWarningExplode in Result.
//	maxRerollDepth      100 — surfaces as CapWarningReroll in Result.
//
// RNG contract:
//
// Execute takes an explicit *math/rand/v2 *rand.Rand. The package never
// seeds or owns an RNG of its own. Callers deciding between PCG and
// ChaCha8 should prefer PCG for speed and ChaCha8 for unpredictability
// (server-authoritative gameplay, anti-cheat). Determinism across Go
// versions is guaranteed by math/rand/v2 when the same seed is used.

const (
	maxDieCount     = 10000
	maxDieSides     = 10000
	maxExplodeDepth = 100
	maxRerollDepth  = 100
)

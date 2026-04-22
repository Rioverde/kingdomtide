// Package dice parses and evaluates RPG-style dice notation — strings
// such as "3d6+2", "4d6dl1", compound expressions, exploding dice, and
// reroll modifiers.
//
// The package splits cleanly into three pure concerns: parsing a
// notation string into an immutable Expression, executing an
// Expression against a caller-supplied *rand.Rand to produce a Result
// with full provenance, and computing closed-form or numerical Stats
// (mean, variance, min, max) without touching an RNG.
//
// Supported notation:
//
//   - Basic rolls:     NdS,  e.g. "3d6", "1d20", "d%", "d100".
//   - Flat modifiers:  NdS+M or NdS-M, e.g. "3d6+2".
//   - Fudge dice:      NdF — each die yields {-1, 0, +1} uniformly.
//   - Keep/drop:       NdSkhK, NdSklK, NdSdhK, NdSdlK — explicit counts
//     are required to avoid parser ambiguity with the "d" prefix.
//   - Compound:        multiple terms joined by '+' or '-', e.g. "1d20+1d4+5".
//   - Explode:         NdS! or NdS!>N — roll additional dice on trigger.
//   - Reroll:          NdSrN (once) or NdSrrN (recursive) — replace matching rolls.
//
// Expressions are immutable after Parse; an Expression value and its
// derived Result values may be shared across goroutines so long as the
// caller does not share a single *rand.Rand concurrently.
//
// Grammar:
//
//	expression        = term_or_constant (('+' | '-') term_or_constant)*
//	term_or_constant  = term | digit+
//	term              = [count] ('d' | 'D') sides [modifiers]
//	count             = digit+                   ; default 1 when omitted
//	sides             = digit+ | '%' | 'F' | 'f' ; % → 100, F/f → fudge
//	modifiers         = (keepMod | dropMod | explodeMod | rerollMod)*
//	keepMod           = ('kh' | 'kl') digit+
//	dropMod           = ('dh' | 'dl') digit+
//	explodeMod        = '!' [comparison] [digit+]
//	rerollMod         = ('r' | 'rr') [comparison] digit+
//	comparison        = '<' | '<=' | '>=' | '>' | '='
//
// Whitespace inside a term is rejected ("2 d 6" is not legal). In
// compound expressions whitespace is permitted around the join operators.
//
// Error strings begin with "dice: ", are lowercase, unpunctuated, and
// carry a 1-indexed column when the failure originates in the parser.
package dice

package dice

import (
	"math/rand/v2"
)

// A Salt selects an independent dice stream for a simulation subsystem.
// Two different Salts yield uncorrelated PCG sequences even when seeded
// from the same WorldSeed; two equal Salts produce identical sequences.
// Salts MUST be unique across subsystems — duplicate salts silently break
// the determinism guarantee.
type Salt uint64

// Subsystem salts. Every simulation subsystem reserves exactly one entry.
// Adding a new subsystem appends a fresh constant — never reuse an existing
// value. The concrete numbers are arbitrary; only distinctness matters.
const (
	SaltCityAnchor       Salt = 0xae5f2d8b9c4671e3
	SaltCityPopulation   Salt = 0x1122334455667788
	SaltVillageAnchor    Salt = 0x064174aaa0bfec2d
	SaltKingdomYear      Salt = 0x7a293cfa1b5d87e6
	SaltKingdomDominance Salt = 0x8b3a4d0b2c6e98f7
	SaltRevolutions      Salt = 0x7f8a9b0c1d2e3f40
	SaltMinerals         Salt = 0x5c3d1e2f7a8b4c9d
	SaltTech             Salt = 0x1a2b3c4d5e6f7081
	SaltGreatPeople      Salt = 0x9f8e7d6c5b4a3928
	SaltFactions         Salt = 0x34567890abcdef12
	SaltReligion         Salt = 0xfedcba0987654321
	SaltLifeEvents       Salt = 0x2468ace013579bdf
	SaltDisasters        Salt = 0xbeefcafe00112233
)

var (
	d4Expr   = MustParse("1d4")
	d6Expr   = MustParse("1d6")
	d20Expr  = MustParse("1d20")
	d100Expr = MustParse("1d100")
	statExpr = MustParse("4d6dl1")
)

// Simple-die fast-path documentation.
//
// D4/D6/D20/D100 and Check bypass the Expression pipeline entirely
// and call rng.IntN directly. For "1dN" with no reroll / explode /
// keep-drop / modifier the Expression path ultimately calls
// rng.IntN(N)+1 exactly once per die, so same-seed determinism is
// preserved byte-for-byte against the old path. See
// TestStream_SimpleRollSameAsExpression for the cross-check.
//
// Stat4D6DropLowest also runs on the fast path: it inlines four
// IntN(6)+1 draws and subtracts the minimum. The call order matches
// statExpr.Execute ("4d6dl1") draw-for-draw so the PCG state stays
// byte-identical. See TestStat4D6DropLowest_PreservesPCGSequence.

// Stream is a per-subsystem deterministic dice source. A Stream owns one
// *rand.Rand seeded from (worldSeed, salt). Callers MUST give each
// subsystem its own Stream — sharing a Stream between subsystems couples
// them and breaks the "add a roll here, nothing moves there" invariant.
// Not safe for concurrent use; caller owns the goroutine discipline.
type Stream struct {
	rng *rand.Rand
}

// New builds a Stream seeded from worldSeed and a subsystem salt. Both
// values feed rand.NewPCG directly as the two halves of its 128-bit
// state, so a distinct salt always produces an uncorrelated sequence
// regardless of worldSeed. The independence between subsystems is a
// call-site invariant: callers must pass a distinct Salt per subsystem —
// see the SaltXxx constants.
func New(worldSeed int64, salt Salt) *Stream {
	return &Stream{
		rng: rand.New(rand.NewPCG(uint64(worldSeed), uint64(salt))),
	}
}

// D4, D6, D20, and D100 each roll one die of the named size and return
// the total in [1, N]. Zero-alloc fast path — calls rng.IntN directly,
// bypassing the Expression pipeline. Determinism against the old path
// holds because "1dN" consumes exactly one rng.IntN(N) call in both
// paths. Stat4D6DropLowest rolls four d6, drops the lowest, and
// returns a value in [3, 18] — the classic D&D ability-score
// distribution. Also on the zero-alloc fast path; the PCG sequence is
// pinned byte-for-byte against statExpr.Execute by
// TestStat4D6DropLowest_PreservesPCGSequence.
func (s *Stream) D4() int   { return s.rng.IntN(4) + 1 }
func (s *Stream) D6() int   { return s.rng.IntN(6) + 1 }
func (s *Stream) D20() int  { return s.rng.IntN(20) + 1 }
func (s *Stream) D100() int { return s.rng.IntN(100) + 1 }

// Stat4D6DropLowest mirrors statExpr ("4d6dl1"): four sequential rolls
// of IntN(6)+1, then drop the minimum. Sequential roll order is the
// load-bearing invariant — statExpr.Execute consumes exactly four
// IntN(6) draws in order before applying keep/drop, and any other
// order here would diverge the PCG state against every determinism
// test. See TestStat4D6DropLowest_PreservesPCGSequence.
func (s *Stream) Stat4D6DropLowest() int {
	d1 := s.rng.IntN(6) + 1
	d2 := s.rng.IntN(6) + 1
	d3 := s.rng.IntN(6) + 1
	d4 := s.rng.IntN(6) + 1
	minDie := min(min(d1, d2), min(d3, d4))
	return d1 + d2 + d3 + d4 - minDie
}

// Int63 returns a non-negative pseudo-random 63-bit integer.
// Exposes the Stream's underlying RNG for callers that hold a custom
// dice.Expression and need to call Execute directly.
func (s *Stream) Int63() int64 {
	return int64(s.rng.Uint64() >> 1)
}

// Check rolls a D20, adds bonus, and returns true when the total meets or
// exceeds dc. No natural-20 autosuccess or natural-1 autofail — the caller
// applies those rules if needed. Matches the decree-execution semantics
// used by the ruler-action subsystem. Zero-alloc fast path — shares the
// rng.IntN(20) call shape with D20 so determinism holds.
func (s *Stream) Check(dc int, bonus int) bool {
	return s.rng.IntN(20)+1+bonus >= dc
}

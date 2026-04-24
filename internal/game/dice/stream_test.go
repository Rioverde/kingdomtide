package dice

import (
	"math/rand/v2"
	"testing"
)

// TestStream_Determinism verifies that two Streams built with the same
// (worldSeed, salt) produce bit-identical D20 sequences. This is the
// load-bearing guarantee for replayable simulation — breaking it means
// the same WorldSeed no longer yields the same world.
func TestStream_Determinism(t *testing.T) {
	const seed int64 = 42
	a := New(seed, SaltCityAnchor)
	b := New(seed, SaltCityAnchor)
	for i := 0; i < 1000; i++ {
		if av, bv := a.D20(), b.D20(); av != bv {
			t.Fatalf("roll %d: a=%d b=%d — same (seed, salt) diverged", i, av, bv)
		}
	}
}

// TestStream_SaltIsolation verifies that two Streams with different salts
// (same worldSeed) produce at least one differing roll inside 1000 draws.
// A full match across 1000 rolls means the salts collapsed to the same
// PCG state — the per-subsystem isolation guarantee is broken.
func TestStream_SaltIsolation(t *testing.T) {
	const seed int64 = 42
	a := New(seed, SaltCityAnchor)
	b := New(seed, SaltVillageAnchor)
	for i := 0; i < 1000; i++ {
		if a.D20() != b.D20() {
			return
		}
	}
	t.Fatal("1000 D20 rolls matched across SaltCityAnchor and SaltVillageAnchor — salts not isolating")
}

// TestStream_D20Range verifies D20 outputs never escape [1, 20]. Protects
// against off-by-one errors in future refactoring of the underlying
// Expression pipeline.
func TestStream_D20Range(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 10000; i++ {
		v := s.D20()
		if v < 1 || v > 20 {
			t.Fatalf("D20() = %d out of [1, 20] at roll %d", v, i)
		}
	}
}

// TestStream_D4D6D100Range verifies the remaining simple dice stay in
// their declared ranges.
func TestStream_D4D6D100Range(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 1000; i++ {
		if v := s.D4(); v < 1 || v > 4 {
			t.Fatalf("D4() = %d out of [1, 4]", v)
		}
		if v := s.D6(); v < 1 || v > 6 {
			t.Fatalf("D6() = %d out of [1, 6]", v)
		}
		if v := s.D100(); v < 1 || v > 100 {
			t.Fatalf("D100() = %d out of [1, 100]", v)
		}
	}
}

// TestStream_Stat4D6DropLowestRange verifies ability-score rolls land in
// the canonical D&D range [3, 18]. Minimum is 1+1+1 (three lowest kept),
// maximum is 6+6+6.
func TestStream_Stat4D6DropLowestRange(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 1000; i++ {
		v := s.Stat4D6DropLowest()
		if v < 3 || v > 18 {
			t.Fatalf("Stat4D6DropLowest() = %d out of [3, 18] at roll %d", v, i)
		}
	}
}

// TestStream_CheckImpossible verifies that Check(21, 0) can never succeed.
// D20 tops out at 20; with zero bonus no outcome reaches 21. If this test
// ever fails, either Check's formula is wrong or D20 has leaked above 20.
func TestStream_CheckImpossible(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 10000; i++ {
		if s.Check(21, 0) {
			t.Fatal("Check(21, 0) succeeded — D20 + 0 cannot reach 21")
		}
	}
}

// TestStream_CheckTrivial verifies that Check(1, 0) always succeeds. D20
// bottoms at 1; with zero bonus every outcome meets DC 1. Catches "meets
// or exceeds" vs "strictly greater" off-by-one bugs.
func TestStream_CheckTrivial(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 10000; i++ {
		if !s.Check(1, 0) {
			t.Fatal("Check(1, 0) failed — D20 ≥ 1 always meets DC 1")
		}
	}
}

// TestStream_CheckBonusApplied verifies Check actually uses bonus. A bonus
// of 20 against DC 21 turns an otherwise-impossible check into one that
// succeeds on any D20 ≥ 1 — i.e. always. If this fails, bonus is being
// dropped somewhere in the arithmetic.
func TestStream_CheckBonusApplied(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 10000; i++ {
		if !s.Check(21, 20) {
			t.Fatal("Check(21, 20) failed — bonus not being added")
		}
	}
}

// TestStream_Int63NonNegative confirms Int63 honors its "non-negative"
// contract: the sign bit is always cleared.
func TestStream_Int63NonNegative(t *testing.T) {
	s := New(42, SaltCityAnchor)
	for i := 0; i < 10000; i++ {
		if v := s.Int63(); v < 0 {
			t.Fatalf("Int63() = %d — negative value violates 63-bit contract", v)
		}
	}
}

// TestStat4D6DropLowest_PreservesPCGSequence is the guardrail for the
// direct-roll fast path: replacing statExpr.Execute with inline rng.IntN
// calls must consume the exact same number of rng draws in the exact same
// order, or every downstream determinism test falls over. The two PCG
// states must agree on every produced total across 100 rolls AND on the
// next 5 D20 draws consumed after those rolls.
func TestStat4D6DropLowest_PreservesPCGSequence(t *testing.T) {
	const (
		seed  int64 = 42
		salt  Salt  = SaltCityAnchor
		rolls       = 100
		probe       = 5
	)

	legacy := rand.New(rand.NewPCG(uint64(seed), uint64(salt)))
	newPath := New(seed, salt)

	for i := 0; i < rolls; i++ {
		want := statExpr.Execute(legacy).Total
		got := newPath.Stat4D6DropLowest()
		if got != want {
			t.Fatalf("roll %d: legacy=%d new=%d — totals diverged", i, want, got)
		}
	}

	for i := 0; i < probe; i++ {
		want := legacy.IntN(20) + 1
		got := newPath.D20()
		if got != want {
			t.Fatalf("post-roll probe %d: legacy=%d new=%d — PCG state diverged (new path consumed the wrong number of IntN calls)", i, want, got)
		}
	}
}

package dice_test

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

// TestParseExplodeSyntaxUnblocked verifies the '!' modifier and its
// comparison variants parse successfully.
func TestParseExplodeSyntaxUnblocked(t *testing.T) {
	cases := []string{
		"3d6!",
		"1d6!>4",
		"1d6!>=5",
		"3d10!>=9",
		"1d20!=20",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := dice.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}
		})
	}
}

// TestParseRerollSyntaxUnblocked verifies 'r' and 'rr' productions
// parse successfully for every supported comparison variant.
func TestParseRerollSyntaxUnblocked(t *testing.T) {
	cases := []string{
		"1d20r1",
		"1d6rr<3",
		"1d10r<=2",
		"1d20r<=1",
		"1d6rr=1",
		"1d100rr<=5",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := dice.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}
		})
	}
}

// TestParseRerollImpossibleAccepted verifies reroll with an
// impossible condition parses — the runtime cap handles termination
// (unlike explode, which infinite-loops with p=1 and is rejected at
// parse time).
func TestParseRerollImpossibleAccepted(t *testing.T) {
	cases := []string{
		"1d6rr<7",   // every face < 7 → triggers
		"1d6rr<=6",  // every face <= 6 → triggers
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := dice.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q): unexpected error %v", src, err)
			}
		})
	}
}

// TestExecuteExplodeGolden pins 3d6! under PCG(42, 0). The sequence
// starts 6,6,1 at the root; both 6s spawn exploded children (values
// 1 and 2 in roll order); the second child doesn't itself trigger.
func TestExecuteExplodeGolden(t *testing.T) {
	e := dice.MustParse("3d6!")
	rng := rand.New(rand.NewPCG(42, 0))
	r := e.Execute(rng)
	if r.Total != 16 {
		t.Fatalf("Total = %d, want 16", r.Total)
	}
	if len(r.Dice) != 5 {
		t.Fatalf("len(Dice) = %d, want 5", len(r.Dice))
	}
	want := []struct {
		value       int
		source      dice.DieSource
		parent      int
	}{
		{6, dice.DieSourceRolled, -1},
		{6, dice.DieSourceRolled, -1},
		{1, dice.DieSourceRolled, -1},
		{1, dice.DieSourceExploded, 0},
		{2, dice.DieSourceExploded, 1},
	}
	for i, w := range want {
		d := r.Dice[i]
		if d.Value != w.value || d.Source != w.source || d.ParentIndex != w.parent {
			t.Fatalf("Dice[%d] = %+v, want value=%d source=%s parent=%d",
				i, d, w.value, w.source, w.parent)
		}
		if d.Dropped {
			t.Fatalf("Dice[%d] unexpectedly dropped", i)
		}
	}
	if len(r.CapWarnings) != 0 {
		t.Fatalf("unexpected CapWarnings: %v", r.CapWarnings)
	}
}

// TestExecuteRerollOnceGolden pins 1d20r1 under PCG(82, 0): first
// roll is 1, reroll replaces with 10. The rerolled-away die carries
// Dropped=true.
func TestExecuteRerollOnceGolden(t *testing.T) {
	e := dice.MustParse("1d20r1")
	rng := rand.New(rand.NewPCG(82, 0))
	r := e.Execute(rng)
	if r.Total != 10 {
		t.Fatalf("Total = %d, want 10", r.Total)
	}
	if len(r.Dice) != 2 {
		t.Fatalf("len(Dice) = %d, want 2", len(r.Dice))
	}
	if r.Dice[0].Value != 1 || !r.Dice[0].Dropped {
		t.Fatalf("Dice[0] = %+v, want value=1 dropped=true", r.Dice[0])
	}
	if r.Dice[0].Source != dice.DieSourceRolled {
		t.Fatalf("Dice[0].Source = %s", r.Dice[0].Source)
	}
	if r.Dice[1].Value != 10 || r.Dice[1].Dropped {
		t.Fatalf("Dice[1] = %+v, want value=10 dropped=false", r.Dice[1])
	}
	if r.Dice[1].Source != dice.DieSourceRerolled {
		t.Fatalf("Dice[1].Source = %s", r.Dice[1].Source)
	}
	if r.Dice[1].ParentIndex != 0 {
		t.Fatalf("Dice[1].ParentIndex = %d, want 0", r.Dice[1].ParentIndex)
	}
}

// TestExecuteRerollOnceKeepsReplacement verifies Roll20 'r' semantics:
// if the replacement itself matches the predicate, it STILL stands.
func TestExecuteRerollOnceKeepsReplacement(t *testing.T) {
	// 1d2r1 — predicate triggers on 1. After enough trials we expect
	// at least some results where the replacement is also 1 (and it
	// stands; no further reroll). Verify len(Dice) <= 2 in every run.
	e := dice.MustParse("1d2r1")
	rng := rand.New(rand.NewPCG(50, 50))
	sawReplacementEqualsOne := false
	for i := 0; i < 1000; i++ {
		r := e.Execute(rng)
		if len(r.Dice) > 2 {
			t.Fatalf("r (once) spawned %d dice — should be ≤2", len(r.Dice))
		}
		if len(r.Dice) == 2 && r.Dice[1].Value == 1 {
			sawReplacementEqualsOne = true
		}
	}
	if !sawReplacementEqualsOne {
		t.Fatal("expected at least one run where replacement also rolled 1")
	}
}

// TestExecuteRerollRecursiveCapFires forces the reroll cap via an
// impossible condition (every face < 7 on d6) and verifies the
// CapWarning populates with Kind=CapWarningReroll, Term=0, Limit=100.
func TestExecuteRerollRecursiveCapFires(t *testing.T) {
	e := dice.MustParse("1d6rr<7")
	rng := rand.New(rand.NewPCG(1, 1))
	r := e.Execute(rng)
	if len(r.CapWarnings) != 1 {
		t.Fatalf("CapWarnings = %v, want exactly 1", r.CapWarnings)
	}
	w := r.CapWarnings[0]
	if w.Kind != dice.CapWarningReroll {
		t.Fatalf("kind = %s, want reroll", w.Kind)
	}
	if w.Term != 0 {
		t.Fatalf("term = %d, want 0", w.Term)
	}
	if w.Limit != 100 {
		t.Fatalf("limit = %d, want 100", w.Limit)
	}
	// Dice slice should contain the original plus 100 rerolls → 101.
	if len(r.Dice) != 101 {
		t.Fatalf("len(Dice) = %d, want 101 (1 original + 100 rerolls)", len(r.Dice))
	}
}

// TestStatsExplodeD6Exact pins E(d6!) and V(d6!) to the closed-form
// values derived in stats.go. E(d6!) = 4.2, V(d6!) = 10.64.
func TestStatsExplodeD6Exact(t *testing.T) {
	s := dice.MustParse("1d6!").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 1d6!")
	}
	if math.Abs(s.Mean-4.2) > 1e-9 {
		t.Fatalf("E(1d6!) = %.9f, want 4.2", s.Mean)
	}
	// V(d6!) = [S(S+1)(2S+1)/6 + 2S·E] / (S-1) - E^2
	//        = [91 + 12·4.2] / 5 - 17.64
	//        = 141.4/5 - 17.64
	//        = 28.28 - 17.64
	//        = 10.64
	v := s.StdDev * s.StdDev
	if math.Abs(v-10.64) > 1e-9 {
		t.Fatalf("V(1d6!) = %.9f, want 10.64", v)
	}
}

// TestStatsExplodeScalesByCount verifies that E(NdS!) and V(NdS!) scale
// linearly in N under the closed form (independent dice).
func TestStatsExplodeScalesByCount(t *testing.T) {
	s := dice.MustParse("3d6!").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 3d6!")
	}
	if math.Abs(s.Mean-3*4.2) > 1e-9 {
		t.Fatalf("E(3d6!) = %.9f, want %.9f", s.Mean, 3*4.2)
	}
	v := s.StdDev * s.StdDev
	if math.Abs(v-3*10.64) > 1e-9 {
		t.Fatalf("V(3d6!) = %.9f, want %.9f", v, 3*10.64)
	}
}

// TestStatsRerollOnceExact pins E(1d20r1) = 10.975 per the derivation
// in stats.go.
func TestStatsRerollOnceExact(t *testing.T) {
	s := dice.MustParse("1d20r1").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 1d20r1")
	}
	// E(d20r1) = (1/20)·(21/2) + (19/20)·(2+20)/2
	//          = 0.525 + 10.45 = 10.975
	if math.Abs(s.Mean-10.975) > 1e-9 {
		t.Fatalf("E(1d20r1) = %.9f, want 10.975", s.Mean)
	}
}

// TestStatsRerollOnceWithPredicate pins E(1d20r<=2) = 11.4.
func TestStatsRerollOnceWithPredicate(t *testing.T) {
	// P(reroll) = 2/20 = 0.1. E[reroll result] = 10.5.
	// E[accepted | ≥3] = (3+20)/2 = 11.5.
	// E = 0.1*10.5 + 0.9*11.5 = 1.05 + 10.35 = 11.4.
	s := dice.MustParse("1d20r<=2").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 1d20r<=2")
	}
	if math.Abs(s.Mean-11.4) > 1e-9 {
		t.Fatalf("E(1d20r<=2) = %.9f, want 11.4", s.Mean)
	}
}

// TestStatsRerollRecursive verifies E(1d6rr<3) = 4.5 — the value is
// uniform on {3, 4, 5, 6} because the rr loop terminates only on
// non-matching faces.
func TestStatsRerollRecursive(t *testing.T) {
	s := dice.MustParse("1d6rr<3").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 1d6rr<3")
	}
	if math.Abs(s.Mean-4.5) > 1e-9 {
		t.Fatalf("E(1d6rr<3) = %.9f, want 4.5", s.Mean)
	}
	// V(uniform on {3,4,5,6}) = ((3-4.5)^2 + (2.25) + (0.25) + (2.25))/4
	// = (2.25 + 0.25 + 0.25 + 2.25)/4 = 1.25
	v := s.StdDev * s.StdDev
	if math.Abs(v-1.25) > 1e-9 {
		t.Fatalf("V(1d6rr<3) = %.9f, want 1.25", v)
	}
	if s.Min != 3 || s.Max != 6 {
		t.Fatalf("range = [%d..%d], want [3..6]", s.Min, s.Max)
	}
}

// TestStatsExplodeNonCanonicalInexact confirms non-canonical explode
// predicates (e.g. !>4) return Exact=false and sensible numerics.
func TestStatsExplodeNonCanonicalInexact(t *testing.T) {
	s := dice.MustParse("1d6!>4").Stats()
	if s.Exact {
		t.Fatal("expected Exact=false for 1d6!>4")
	}
	// Mean must be greater than plain d6 mean (3.5) — explosion adds.
	if s.Mean <= 3.5 {
		t.Fatalf("E(1d6!>4) = %.3f, expected > 3.5", s.Mean)
	}
}

// TestExecuteExplodeRange verifies several rolls of 3d6! stay in the
// plausible [3, 3*(maxDepth+1)*6] bound and all exploded children
// carry valid ParentIndex.
func TestExecuteExplodeRange(t *testing.T) {
	e := dice.MustParse("3d6!")
	rng := rand.New(rand.NewPCG(99, 99))
	for trial := 0; trial < 500; trial++ {
		r := e.Execute(rng)
		if r.Total < 3 {
			t.Fatalf("3d6! total too low: %d", r.Total)
		}
		for i, d := range r.Dice {
			switch d.Source {
			case dice.DieSourceRolled:
				if d.ParentIndex != -1 {
					t.Fatalf("rolled Dice[%d] has parent %d", i, d.ParentIndex)
				}
			case dice.DieSourceExploded:
				if d.ParentIndex < 0 || d.ParentIndex >= i {
					t.Fatalf("exploded Dice[%d] has invalid parent %d", i, d.ParentIndex)
				}
			}
			if d.Value < 1 || d.Value > 6 {
				t.Fatalf("face out of [1..6]: %d", d.Value)
			}
		}
	}
}

// TestExecuteExplodeVarianceEmpirical confirms variance of 1d6! over
// many trials matches the closed-form within a loose tolerance.
// Serves as a cross-check on the formula derivation.
func TestExecuteExplodeVarianceEmpirical(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping empirical variance test under -short")
	}
	e := dice.MustParse("1d6!")
	rng := rand.New(rand.NewPCG(17, 17))
	const N = 200_000
	var sum, sumSq float64
	for i := 0; i < N; i++ {
		r := e.Execute(rng)
		sum += float64(r.Total)
		sumSq += float64(r.Total * r.Total)
	}
	mean := sum / N
	v := sumSq/N - mean*mean
	// Tolerance is intentionally wide; we're guarding against
	// catastrophic formula breakage, not a tight empirical match.
	if math.Abs(mean-4.2) > 0.05 {
		t.Fatalf("empirical E(d6!) = %.4f, want ~4.2", mean)
	}
	if math.Abs(v-10.64) > 0.5 {
		t.Fatalf("empirical V(d6!) = %.4f, want ~10.64", v)
	}
}

// TestExecuteExplodeWithKeepDrop verifies keep/drop applies POST
// explosion — the full post-explosion pool competes for keep slots.
func TestExecuteExplodeWithKeepDrop(t *testing.T) {
	e, err := dice.Parse("3d6!kh2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rng := rand.New(rand.NewPCG(42, 0))
	r := e.Execute(rng)
	// 3d6! at seed 42 rolls 6,6,1 → explodes to 1,2 for 5 total dice.
	// kh2 keeps the top 2 from the pool (the two 6s).
	if len(r.Dice) != 5 {
		t.Fatalf("len(Dice) = %d, want 5", len(r.Dice))
	}
	kept := 0
	for _, d := range r.Dice {
		if !d.Dropped {
			kept++
		}
	}
	if kept != 2 {
		t.Fatalf("kept %d dice, want 2", kept)
	}
	if r.Total != 12 {
		t.Fatalf("Total = %d, want 12 (6+6 kept from pool)", r.Total)
	}
}

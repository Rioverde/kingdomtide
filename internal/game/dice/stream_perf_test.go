package dice

import "testing"

// TestStream_SimpleRollSameAsExpression verifies the fast-path
// D-rolls produce the same values as going through an Expression
// with identical RNG state. Proves the zero-alloc path preserves
// byte-for-byte determinism against the pre-refactor Execute path.
func TestStream_SimpleRollSameAsExpression(t *testing.T) {
	const iter = 1000
	fast := New(42, SaltKingdomYear)
	slow := New(42, SaltKingdomYear)

	for i := 0; i < iter; i++ {
		f := fast.D20()
		s := d20Expr.Execute(slow.rng).Total
		if f != s {
			t.Fatalf("iter %d: fast=%d slow=%d", i, f, s)
		}
	}
}

// TestStream_D4D6D100SameAsExpression extends the cross-check to the
// remaining simple-die fast paths. Each subtest rolls 500 values
// against an identically-seeded Stream driving the Expression path
// and fails on the first divergence.
func TestStream_D4D6D100SameAsExpression(t *testing.T) {
	cases := []struct {
		name string
		fast func(*Stream) int
		expr Expression
	}{
		{"D4", func(s *Stream) int { return s.D4() }, d4Expr},
		{"D6", func(s *Stream) int { return s.D6() }, d6Expr},
		{"D100", func(s *Stream) int { return s.D100() }, d100Expr},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fast := New(42, SaltKingdomYear)
			slow := New(42, SaltKingdomYear)
			for i := 0; i < 500; i++ {
				f := c.fast(fast)
				s := c.expr.Execute(slow.rng).Total
				if f != s {
					t.Fatalf("%s iter %d: fast=%d slow=%d", c.name, i, f, s)
				}
			}
		})
	}
}

// TestStream_CheckSameAsExpression verifies Check's D20+bonus>=dc
// decision matches an equivalent Expression-driven evaluation for
// identical rng state.
func TestStream_CheckSameAsExpression(t *testing.T) {
	const iter = 1000
	const dc, bonus = 15, 2
	fast := New(42, SaltKingdomYear)
	slow := New(42, SaltKingdomYear)
	for i := 0; i < iter; i++ {
		got := fast.Check(dc, bonus)
		want := d20Expr.Execute(slow.rng).Total+bonus >= dc
		if got != want {
			t.Fatalf("iter %d: got=%v want=%v", i, got, want)
		}
	}
}

// TestExpression_TotalMatchesExecute covers the Expression.Total
// fast path across both the inline "simple term" shape and the
// Execute fallback shape (keep-low / explode). Same rng state must
// produce the same total in both paths.
func TestExpression_TotalMatchesExecute(t *testing.T) {
	cases := []string{
		"1d20",
		"2d6+3",
		"3d8-2",
		"1d20+1d4+5",
		"4dF",
		"4d6dl1", // fallback: keep/drop triggers Execute path.
		"2d6!",   // fallback: explode triggers Execute path.
		"3d10r1", // fallback: reroll triggers Execute path.
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			expr := MustParse(src)
			fast := New(7, SaltKingdomYear)
			slow := New(7, SaltKingdomYear)
			for i := 0; i < 500; i++ {
				got := expr.Total(fast.rng)
				want := expr.Execute(slow.rng).Total
				if got != want {
					t.Fatalf("iter %d: got=%d want=%d", i, got, want)
				}
			}
		})
	}
}

// BenchmarkStreamD20_FastPath measures the zero-alloc target.
func BenchmarkStreamD20_FastPath(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.D20()
	}
}

// BenchmarkStreamD6_FastPath measures the zero-alloc target for 1d6.
func BenchmarkStreamD6_FastPath(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.D6()
	}
}

// BenchmarkStreamCheck_FastPath measures the zero-alloc target for
// the D20 + bonus ≥ dc check used by the ruler-action subsystem.
func BenchmarkStreamCheck_FastPath(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Check(15, 2)
	}
}

// BenchmarkExpressionTotal_Simple measures Expression.Total on a
// single-term no-modifier expression — the common hot-path case
// where the fast path skips slice allocation entirely.
func BenchmarkExpressionTotal_Simple(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = d20Expr.Total(s.rng)
	}
}

// BenchmarkExpressionExecute_Simple is the "before" counterpart to
// BenchmarkExpressionTotal_Simple — quantifies the savings.
func BenchmarkExpressionExecute_Simple(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = d20Expr.Execute(s.rng).Total
	}
}

// BenchmarkStat4D6DropLowest_FastPath measures the direct-roll fast path
// after replacing statExpr.Execute with inline IntN(6) calls. The target
// is zero allocs/op (previously ~8 allocs/op from Expression.Execute's
// DieRoll/TermResult/CapWarning slice allocations).
func BenchmarkStat4D6DropLowest_FastPath(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = s.Stat4D6DropLowest()
	}
}

// BenchmarkStat4D6DropLowest_Legacy is the "before" counterpart — routes
// through statExpr.Execute so the alloc delta is visible side-by-side.
func BenchmarkStat4D6DropLowest_Legacy(b *testing.B) {
	s := New(42, SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = statExpr.Execute(s.rng).Total
	}
}

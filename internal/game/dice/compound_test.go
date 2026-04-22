package dice_test

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

// TestParseCompoundRoundTrip covers every compound-expression shape —
// round-trip through Parse+String preserves the source text.
func TestParseCompoundRoundTrip(t *testing.T) {
	cases := []string{
		"1d20+5",
		"1d20+1d4",
		"2d6-1d4",
		"1d20+1d4+5",
		"5+2d8",
		"5-2d8",
		"1d6+1d8-1d10+2",
		"1d20 + 1d4",      // whitespace around '+'
		" 1d20 + 1d4 + 5 ", // surrounding whitespace
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			e, err := dice.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q): %v", src, err)
			}
			if e.String() != src {
				t.Fatalf("String() = %q, want %q", e.String(), src)
			}
		})
	}
}

// TestParseCompoundRejects covers the edge cases spelled out in the
// plan: empty, trailing operator, double operator, leading sign.
func TestParseCompoundRejects(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", "empty expression"},
		{"trailing plus", "1d20+", "expected term or constant after sign"},
		{"double plus", "1d20++5", "unexpected sign character '+'"},
		{"double minus", "1d20-+5", "unexpected sign character '+'"},
		{"leading plus", "+1d20", "unexpected leading sign"},
		{"leading minus", "-1d20", "unexpected leading sign"},
		{"intra-term whitespace", "1 d 20", "whitespace not allowed inside term"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dice.Parse(tc.input)
			if err == nil {
				t.Fatalf("Parse(%q) succeeded, wanted error", tc.input)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Parse(%q) err = %q, want substring %q", tc.input, err.Error(), tc.want)
			}
		})
	}
}

// TestExecuteCompoundGolden pins concrete results under PCG(42, 0).
// These fixtures document the byte-exact sequence the current Go
// math/rand/v2 PCG generator produces and protect against silent
// parser drift.
func TestExecuteCompoundGolden(t *testing.T) {
	cases := []struct {
		expr   string
		total  int
		values []int
	}{
		// 1d20 rolls 18, 1d4 rolls 1, +5 → 24.
		{expr: "1d20+1d4+5", total: 24, values: []int{18, 1}},
		// 1d20=18, 1d4=1 → 19.
		{expr: "1d20+1d4", total: 19, values: []int{18, 1}},
		// 2d6=6,6 then 1d4=4 → 12 - 4 = 8.
		{expr: "2d6-1d4", total: 8, values: []int{6, 6, 4}},
		// 5 + 2d8 where 2d8 = 4, 6 → 5 + 10 = 15.
		// Calibrate the values-check against actual draws.
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			e := dice.MustParse(tc.expr)
			rng := rand.New(rand.NewPCG(42, 0))
			r := e.Execute(rng)
			if r.Total != tc.total {
				t.Fatalf("Total = %d, want %d (dice=%v)", r.Total, tc.total,
					diceValues(r))
			}
			if len(r.Dice) != len(tc.values) {
				t.Fatalf("len(Dice) = %d, want %d", len(r.Dice), len(tc.values))
			}
			for i, d := range r.Dice {
				if d.Value != tc.values[i] {
					t.Fatalf("Dice[%d].Value = %d, want %d", i, d.Value, tc.values[i])
				}
			}
		})
	}
}

// TestExecuteCompoundTermCounts asserts the TermResult slice carries
// one entry per term with the correct Sign and that dice are appended
// in left-to-right term order.
func TestExecuteCompoundTermCounts(t *testing.T) {
	e := dice.MustParse("2d6-1d4")
	rng := rand.New(rand.NewPCG(42, 0))
	r := e.Execute(rng)
	if len(r.Terms) != 2 {
		t.Fatalf("len(Terms) = %d, want 2", len(r.Terms))
	}
	if r.Terms[0].Sign != 1 || r.Terms[1].Sign != -1 {
		t.Fatalf("term signs = [%d, %d], want [+1, -1]",
			r.Terms[0].Sign, r.Terms[1].Sign)
	}
	if r.Terms[0].Count != 2 || r.Terms[0].Sides != 6 {
		t.Fatalf("term[0] shape = %+v", r.Terms[0])
	}
	if r.Terms[1].Count != 1 || r.Terms[1].Sides != 4 {
		t.Fatalf("term[1] shape = %+v", r.Terms[1])
	}
	// Dice slice: first 2 dice belong to 2d6, then 1 die for 1d4.
	if r.Terms[0].DiceStart != 0 || r.Terms[0].DiceEnd != 2 {
		t.Fatalf("term[0] range = [%d..%d]", r.Terms[0].DiceStart, r.Terms[0].DiceEnd)
	}
	if r.Terms[1].DiceStart != 2 || r.Terms[1].DiceEnd != 3 {
		t.Fatalf("term[1] range = [%d..%d]", r.Terms[1].DiceStart, r.Terms[1].DiceEnd)
	}
}

// TestStatsCompound pins Mean, Variance, Min, Max for compound
// expressions. E(1d20+1d4) = 10.5 + 2.5 = 13.0; V = 33.25 + 1.25.
func TestStatsCompound(t *testing.T) {
	cases := []struct {
		expr       string
		mean       float64
		variance   float64
		min, max   int
		exact      bool
	}{
		{"1d20+1d4", 13.0, 33.25 + 1.25, 2, 24, true},
		{"2d6-1d4", 7.0 - 2.5, 2.0*35.0/12.0 + 15.0/12.0, 2 - 4, 12 - 1, true},
		{"1d20+5", 15.5, 33.25, 6, 25, true},
		{"5-2d8", 5 - 9.0, 2.0 * (64 - 1) / 12.0, 5 - 16, 5 - 2, true},
		{"1d6+1d8-1d10+2", 3.5 + 4.5 - 5.5 + 2.0,
			35.0/12.0 + 63.0/12.0 + 99.0/12.0,
			1 + 1 - 10 + 2, 6 + 8 - 1 + 2, true},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			s := dice.MustParse(tc.expr).Stats()
			if s.Exact != tc.exact {
				t.Fatalf("Exact = %t, want %t", s.Exact, tc.exact)
			}
			if math.Abs(s.Mean-tc.mean) > 1e-9 {
				t.Fatalf("Mean = %f, want %f", s.Mean, tc.mean)
			}
			if math.Abs(s.StdDev*s.StdDev-tc.variance) > 1e-9 {
				t.Fatalf("Variance = %f, want %f", s.StdDev*s.StdDev, tc.variance)
			}
			if s.Min != tc.min || s.Max != tc.max {
				t.Fatalf("range = [%d..%d], want [%d..%d]", s.Min, s.Max, tc.min, tc.max)
			}
		})
	}
}

// TestExecuteCompoundNegativeAllowed confirms negative totals are not
// clamped — per plan policy.
func TestExecuteCompoundNegativeAllowed(t *testing.T) {
	// "1d4-1d20" frequently goes negative. Run many iterations and
	// assert at least one < 0 result appears.
	e := dice.MustParse("1d4-1d20")
	rng := rand.New(rand.NewPCG(7, 7))
	sawNeg := false
	for i := 0; i < 1000; i++ {
		r := e.Execute(rng)
		if r.Total < 0 {
			sawNeg = true
		}
	}
	if !sawNeg {
		t.Fatal("expected at least one negative total in 1000 trials")
	}
}

// TestExecuteCompoundModifierField asserts Result.Modifier correctly
// sums the bare constant with every term's signed modifier.
func TestExecuteCompoundModifierField(t *testing.T) {
	e := dice.MustParse("1d20+1d4+5")
	rng := rand.New(rand.NewPCG(42, 0))
	r := e.Execute(rng)
	if r.Modifier != 5 {
		t.Fatalf("Modifier = %d, want 5", r.Modifier)
	}
}

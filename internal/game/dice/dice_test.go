package dice_test

import (
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

func TestParseRoundTripBasic(t *testing.T) {
	cases := []string{
		"d6",
		"1d6",
		"3d6",
		"3d6+2",
		"3d6-1",
		"1d20",
		"1d%",
		"1d100",
		"4dF",
		"2d10+5",
		"4d6dl1",
		"4d6dh1",
		"2d20kh1",
		"2d20kl1",
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			e, err := dice.Parse(src)
			if err != nil {
				t.Fatalf("Parse(%q) returned error: %v", src, err)
			}
			if got := e.String(); got != src {
				t.Fatalf("Expression.String() = %q, want %q", got, src)
			}
		})
	}
}

func TestMustParsePanicsOnBadInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustParse did not panic on invalid input")
		}
	}()
	_ = dice.MustParse("not-dice")
}

func TestMustParseAcceptsGoodInput(t *testing.T) {
	e := dice.MustParse("1d6")
	if e.String() != "1d6" {
		t.Fatalf("unexpected expression: %s", e.String())
	}
}

func TestZeroExpressionExecutePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Execute on zero Expression did not panic")
		}
	}()
	var zero dice.Expression
	_ = zero.Execute(rand.New(rand.NewPCG(1, 2)))
}

func TestExecuteNilRngPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Execute with nil *rand.Rand did not panic")
		}
	}()
	e := dice.MustParse("1d6")
	_ = e.Execute(nil)
}

func TestExecuteRangeBasic(t *testing.T) {
	e, err := dice.Parse("3d6+2")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rng := rand.New(rand.NewPCG(1, 1))
	for i := 0; i < 1000; i++ {
		r := e.Execute(rng)
		if r.Total < 5 || r.Total > 20 {
			t.Fatalf("Total out of [5..20]: %d", r.Total)
		}
		if len(r.Dice) != 3 {
			t.Fatalf("Dice count = %d, want 3", len(r.Dice))
		}
		for _, d := range r.Dice {
			if d.Value < 1 || d.Value > 6 {
				t.Fatalf("die value out of [1..6]: %d", d.Value)
			}
			if d.Source != dice.DieSourceRolled {
				t.Fatalf("source = %v, want rolled", d.Source)
			}
			if d.ParentIndex != -1 {
				t.Fatalf("parent index = %d, want -1", d.ParentIndex)
			}
			if d.Dropped {
				t.Fatalf("no dice should be dropped for 3d6+2")
			}
		}
		if r.Modifier != 2 {
			t.Fatalf("Modifier = %d, want 2", r.Modifier)
		}
		if len(r.Terms) != 1 {
			t.Fatalf("Terms count = %d, want 1", len(r.Terms))
		}
		if r.Terms[0].DiceStart != 0 || r.Terms[0].DiceEnd != 3 {
			t.Fatalf("unexpected term range: %+v", r.Terms[0])
		}
		if len(r.CapWarnings) != 0 {
			t.Fatalf("expected empty CapWarnings for a non-explode non-reroll expression, got %d", len(r.CapWarnings))
		}
	}
}

func TestExecuteKeepDropFiltering(t *testing.T) {
	e := dice.MustParse("4d6dl1")
	rng := rand.New(rand.NewPCG(7, 42))
	for i := 0; i < 500; i++ {
		r := e.Execute(rng)
		dropped := 0
		for _, d := range r.Dice {
			if d.Dropped {
				dropped++
			}
		}
		if dropped != 1 {
			t.Fatalf("expected exactly 1 dropped die, got %d", dropped)
		}
		if r.Total < 3 || r.Total > 18 {
			t.Fatalf("Total out of [3..18]: %d", r.Total)
		}
		// Dropped die must be the lowest.
		lowest := 7
		for _, d := range r.Dice {
			if d.Value < lowest {
				lowest = d.Value
			}
		}
		for _, d := range r.Dice {
			if d.Dropped && d.Value != lowest {
				t.Fatalf("dropped die %d is not the lowest %d", d.Value, lowest)
			}
		}
	}
}

func TestExecuteFudgeRange(t *testing.T) {
	e := dice.MustParse("4dF")
	rng := rand.New(rand.NewPCG(3, 3))
	for i := 0; i < 500; i++ {
		r := e.Execute(rng)
		if r.Total < -4 || r.Total > 4 {
			t.Fatalf("fudge total out of [-4..4]: %d", r.Total)
		}
		for _, d := range r.Dice {
			if d.Value < -1 || d.Value > 1 {
				t.Fatalf("fudge face out of {-1,0,1}: %d", d.Value)
			}
		}
	}
}

func TestGoldenSeedDeterminism(t *testing.T) {
	// Pin concrete values under rand.NewPCG(42, 0) so the expression
	// engine never silently drifts. These are the exact sequences the
	// current Go (1.25) math/rand/v2 PCG implementation produces.
	cases := []struct {
		expr     string
		total    int
		modifier int
		values   []int
		dropped  []bool
	}{
		{
			expr:     "3d6+2",
			total:    15,
			modifier: 2,
			values:   []int{6, 6, 1},
			dropped:  []bool{false, false, false},
		},
		{
			expr:     "4d6dl1",
			total:    13,
			modifier: 0,
			values:   []int{6, 6, 1, 1},
			// Two dice tie at value=1; the stable sort keeps the first
			// tied die as the drop target, leaving the later one.
			dropped: []bool{false, false, true, false},
		},
		{
			expr:     "2d20kh1",
			total:    20,
			modifier: 0,
			values:   []int{18, 20},
			dropped:  []bool{true, false},
		},
		{
			expr:     "2d20kl1",
			total:    18,
			modifier: 0,
			values:   []int{18, 20},
			dropped:  []bool{false, true},
		},
	}
	for _, tc := range cases {
		t.Run(tc.expr, func(t *testing.T) {
			e := dice.MustParse(tc.expr)
			rng := rand.New(rand.NewPCG(42, 0))
			r := e.Execute(rng)
			if r.Total != tc.total {
				t.Fatalf("Total = %d, want %d (dice: %v)", r.Total, tc.total, diceValues(r))
			}
			if r.Modifier != tc.modifier {
				t.Fatalf("Modifier = %d, want %d", r.Modifier, tc.modifier)
			}
			if len(r.Dice) != len(tc.values) {
				t.Fatalf("len(Dice) = %d, want %d", len(r.Dice), len(tc.values))
			}
			for i, d := range r.Dice {
				if d.Value != tc.values[i] {
					t.Fatalf("Dice[%d].Value = %d, want %d", i, d.Value, tc.values[i])
				}
				if d.Dropped != tc.dropped[i] {
					t.Fatalf("Dice[%d].Dropped = %t, want %t", i, d.Dropped, tc.dropped[i])
				}
			}
		})
	}
}

func diceValues(r dice.Result) []int {
	out := make([]int, len(r.Dice))
	for i, d := range r.Dice {
		out[i] = d.Value
	}
	return out
}

func TestParseErrorIsParseError(t *testing.T) {
	_, err := dice.Parse("1d0")
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*dice.ParseError)
	if !ok {
		t.Fatalf("error is not *ParseError: %T", err)
	}
	if pe.Column <= 0 {
		t.Fatalf("expected positive column, got %d", pe.Column)
	}
}

func TestExpressionSafeAcrossGoroutines(t *testing.T) {
	// The Expression must be safe to share across goroutines as long
	// as each caller brings its own *rand.Rand. This doesn't deeply
	// test concurrency but it exercises that executing in parallel
	// doesn't mutate the shared tree.
	e := dice.MustParse("3d6+2")
	done := make(chan struct{}, 4)
	for g := 0; g < 4; g++ {
		g := g
		go func() {
			rng := rand.New(rand.NewPCG(uint64(g), 100))
			for i := 0; i < 200; i++ {
				r := e.Execute(rng)
				if r.Total < 5 || r.Total > 20 {
					t.Errorf("goroutine %d iter %d: total %d out of range", g, i, r.Total)
					return
				}
			}
			done <- struct{}{}
		}()
	}
	for g := 0; g < 4; g++ {
		<-done
	}
}

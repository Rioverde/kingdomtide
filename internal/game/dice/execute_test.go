package dice_test

import (
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

func TestExecuteDiceProvenance(t *testing.T) {
	e := dice.MustParse("2d6")
	rng := rand.New(rand.NewPCG(11, 22))
	r := e.Execute(rng)
	if len(r.Dice) != 2 {
		t.Fatalf("len(Dice) = %d", len(r.Dice))
	}
	for i, d := range r.Dice {
		if d.Source != dice.DieSourceRolled {
			t.Fatalf("Dice[%d].Source = %v", i, d.Source)
		}
		if d.ParentIndex != -1 {
			t.Fatalf("Dice[%d].ParentIndex = %d", i, d.ParentIndex)
		}
		if d.Value < 1 || d.Value > 6 {
			t.Fatalf("Dice[%d].Value out of [1..6]: %d", i, d.Value)
		}
	}
}

func TestExecuteTermRange(t *testing.T) {
	e := dice.MustParse("4d6dl1+1")
	rng := rand.New(rand.NewPCG(2, 2))
	r := e.Execute(rng)
	if len(r.Terms) != 1 {
		t.Fatalf("len(Terms) = %d", len(r.Terms))
	}
	term := r.Terms[0]
	// The trailing "+1" in "4d6dl1+1" parses as a compound bare
	// constant, not a per-term modifier. So term.Modifier stays 0 and
	// the +1 lives on the expression root; the composite Result.Modifier
	// reflects it.
	if term.Count != 4 || term.Sides != 6 || term.Modifier != 0 {
		t.Fatalf("term = %+v", term)
	}
	if r.Modifier != 1 {
		t.Fatalf("Result.Modifier = %d, want 1", r.Modifier)
	}
	if term.Sign != 1 {
		t.Fatalf("term.Sign = %d, want +1", term.Sign)
	}
	if term.DiceStart != 0 || term.DiceEnd != 4 {
		t.Fatalf("term dice range = [%d..%d]", term.DiceStart, term.DiceEnd)
	}
}

func TestExecuteKeepHighStats(t *testing.T) {
	// Sanity check that applying keep highest yields values in range
	// and that many trials converge roughly to expected mean.
	e := dice.MustParse("2d20kh1")
	rng := rand.New(rand.NewPCG(101, 202))
	const trials = 5000
	sum := 0
	for i := 0; i < trials; i++ {
		r := e.Execute(rng)
		if r.Total < 1 || r.Total > 20 {
			t.Fatalf("kh1 total out of [1..20]: %d", r.Total)
		}
		dropped := 0
		for _, d := range r.Dice {
			if d.Dropped {
				dropped++
			}
		}
		if dropped != 1 {
			t.Fatalf("expected 1 dropped die, got %d", dropped)
		}
		sum += r.Total
	}
	mean := float64(sum) / float64(trials)
	if mean < 13.0 || mean > 14.6 {
		t.Fatalf("empirical mean drift: %.3f", mean)
	}
}

func TestExecuteMinMax(t *testing.T) {
	// Verify that very many trials hit the documented range
	// extremes for d20 (both 1 and 20).
	e := dice.MustParse("1d20")
	rng := rand.New(rand.NewPCG(999, 999))
	sawMin, sawMax := false, false
	for i := 0; i < 10000 && !(sawMin && sawMax); i++ {
		r := e.Execute(rng)
		if r.Total == 1 {
			sawMin = true
		}
		if r.Total == 20 {
			sawMax = true
		}
	}
	if !sawMin || !sawMax {
		t.Fatalf("did not observe both extremes (min=%t max=%t)", sawMin, sawMax)
	}
}

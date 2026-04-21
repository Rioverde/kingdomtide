package worldgen

import (
	"math"
	"testing"
)

func TestOctaveNoiseDeterministic(t *testing.T) {
	a := NewOctaveNoise(42, DefaultOctaveOpts)
	b := NewOctaveNoise(42, DefaultOctaveOpts)

	for _, p := range []struct{ x, y float64 }{
		{0, 0}, {1, 2}, {-10, 50}, {1234, -5678},
	} {
		if got, want := a.Eval2(p.x, p.y), b.Eval2(p.x, p.y); got != want {
			t.Errorf("Eval2(%v, %v) not deterministic: %v vs %v", p.x, p.y, got, want)
		}
	}
}

func TestOctaveNoiseRange(t *testing.T) {
	n := NewOctaveNoise(7, DefaultOctaveOpts)
	for x := -500.0; x <= 500.0; x += 31 {
		for y := -500.0; y <= 500.0; y += 31 {
			v := n.Eval2(x, y)
			if v < -1 || v > 1 {
				t.Fatalf("Eval2(%v, %v) = %v, outside [-1, 1]", x, y, v)
			}
			vn := n.Eval2Normalized(x, y)
			if vn < 0 || vn > 1 {
				t.Fatalf("Eval2Normalized(%v, %v) = %v, outside [0, 1]", x, y, vn)
			}
		}
	}
}

// TestEval2RidgeRange verifies the ridge transform stays in [0, 1] over a dense sweep
// and matches the manual formula `(1 - |raw|)²` at each sample. The square ensures
// high values only where raw ≈ 0.
func TestEval2RidgeRange(t *testing.T) {
	n := NewOctaveNoise(11, DefaultOctaveOpts)
	for x := -400.0; x <= 400.0; x += 17 {
		for y := -400.0; y <= 400.0; y += 17 {
			ridge := n.Eval2Ridge(x, y)
			if ridge < 0.0 || ridge > 1.0 {
				t.Fatalf("Eval2Ridge(%v, %v) = %v, outside [0, 1]", x, y, ridge)
			}
			// Formula check.
			raw := n.Eval2(x, y)
			want := (1.0 - math.Abs(raw)) * (1.0 - math.Abs(raw))
			if math.Abs(ridge-want) > 1e-12 {
				t.Fatalf("Eval2Ridge(%v, %v) = %v, want %v (raw=%v)", x, y, ridge, want, raw)
			}
		}
	}
}

// TestEval2RidgePeakAndValley asserts the transform's shape at two ends: where signed
// noise crosses zero the ridge value approaches 1.0; where |raw| reaches the empirical
// extremum of the fBm output the ridge value approaches 0.0.
//
// Multi-octave OpenSimplex rarely saturates at |raw| = 1.0 after normalisation — the
// empirical max magnitude sits closer to 0.75. The valley check uses 0.5 as a "large
// |raw|" cut-off and asserts ridge is at most (1-0.5)² = 0.25 there, which is the
// formula's guarantee.
func TestEval2RidgePeakAndValley(t *testing.T) {
	n := NewOctaveNoise(23, DefaultOctaveOpts)

	peakBest := 0.0   // largest ridge value observed near raw≈0 (want → 1)
	valleyBest := 1.0 // smallest ridge value observed at large |raw| (want → 0)
	sawLargeRaw := false
	for x := -300.0; x <= 300.0; x += 7 {
		for y := -300.0; y <= 300.0; y += 7 {
			raw := n.Eval2(x, y)
			ridge := n.Eval2Ridge(x, y)
			if math.Abs(raw) < 0.02 && ridge > peakBest {
				peakBest = ridge
			}
			if math.Abs(raw) > 0.5 {
				sawLargeRaw = true
				if ridge < valleyBest {
					valleyBest = ridge
				}
			}
		}
	}
	if peakBest < 0.95 {
		t.Errorf("no ridge peak ≈ 1.0 found across scan; best = %v", peakBest)
	}
	if !sawLargeRaw {
		t.Fatal("scan never observed |raw| > 0.5 — cannot test valley branch")
	}
	// At |raw|=0.5 the formula caps ridge at 0.25. Any sample with larger |raw| must
	// drop below that — the strictness of the bound grows with |raw|.
	if valleyBest > 0.25 {
		t.Errorf("valley ridge %v exceeds analytical cap 0.25 for |raw|>0.5", valleyBest)
	}
}

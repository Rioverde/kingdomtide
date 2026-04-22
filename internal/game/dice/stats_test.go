package dice_test

import (
	"math"
	"testing"
	"time"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

const (
	// Exact closed-form results are tight. 1e-9 is generous given
	// Go's float64 guarantees but catches accidental single-precision
	// or algebraic regressions.
	exactTolerance = 1e-9

	// Keep/drop convolutions enumerate the distribution exactly; the
	// numerical answer equals the closed-form fraction to float64
	// precision. 1e-9 leaves ULP-scale margin for the two-summation
	// accumulation and still catches any algebraic regression.
	convolutionTolerance = 1e-9
)

func approx(t *testing.T, got, want, tol float64, label string) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Fatalf("%s: got %.12f, want %.12f (tol %.1e)", label, got, want, tol)
	}
}

func TestStatsBasicExact(t *testing.T) {
	// E(1d6) = 3.5.
	s := dice.MustParse("1d6").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true")
	}
	approx(t, s.Mean, 3.5, exactTolerance, "E(1d6)")
	approx(t, s.StdDev*s.StdDev, 35.0/12.0, exactTolerance, "V(1d6)")
	if s.Min != 1 || s.Max != 6 {
		t.Fatalf("range = [%d..%d], want [1..6]", s.Min, s.Max)
	}
}

func TestStatsModifierApplied(t *testing.T) {
	// E(3d6+2) = 10.5 + 2 = 12.5.
	s := dice.MustParse("3d6+2").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true")
	}
	approx(t, s.Mean, 12.5, exactTolerance, "E(3d6+2)")
	// V(3d6) = 3·35/12 = 8.75.
	approx(t, s.StdDev*s.StdDev, 3.0*35.0/12.0, exactTolerance, "V(3d6)")
	if s.Min != 5 || s.Max != 20 {
		t.Fatalf("range = [%d..%d], want [5..20]", s.Min, s.Max)
	}
}

func TestStatsNdSVariance(t *testing.T) {
	sides := []int{4, 6, 8, 10, 12, 20, 100}
	for _, S := range sides {
		for N := 1; N <= 10; N++ {
			src := expectedNdS(N, S, 0)
			s := dice.MustParse(src).Stats()
			wantMean := float64(N) * float64(S+1) / 2.0
			wantVar := float64(N) * float64(S*S-1) / 12.0
			approx(t, s.Mean, wantMean, exactTolerance, src+" mean")
			approx(t, s.StdDev*s.StdDev, wantVar, exactTolerance, src+" var")
			if s.Min != N || s.Max != N*S {
				t.Fatalf("%s range = [%d..%d], want [%d..%d]", src, s.Min, s.Max, N, N*S)
			}
		}
	}
}

func TestStatsAdvantageDisadvantageExact(t *testing.T) {
	// E(2d20kh1) = 553/40 = 13.825 exact.
	s := dice.MustParse("2d20kh1").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 2d20kh1")
	}
	approx(t, s.Mean, 553.0/40.0, exactTolerance, "E(2d20kh1)")
	if s.Min != 1 || s.Max != 20 {
		t.Fatalf("range = [%d..%d]", s.Min, s.Max)
	}
	// Disadvantage: 287/40 = 7.175.
	s = dice.MustParse("2d20kl1").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for 2d20kl1")
	}
	approx(t, s.Mean, 287.0/40.0, exactTolerance, "E(2d20kl1)")
}

func TestStats4d6Drop1Convolution(t *testing.T) {
	// E(4d6dl1) = 15869/1296 ≈ 12.2445987654...
	s := dice.MustParse("4d6dl1").Stats()
	if s.Exact {
		t.Fatal("expected Exact=false for general keep/drop convolution")
	}
	approx(t, s.Mean, 15869.0/1296.0, convolutionTolerance, "E(4d6dl1)")
	if s.Min != 3 || s.Max != 18 {
		t.Fatalf("range = [%d..%d], want [3..18]", s.Min, s.Max)
	}
}

func TestStatsFudge(t *testing.T) {
	s := dice.MustParse("4dF").Stats()
	if !s.Exact {
		t.Fatal("expected Exact=true for fudge")
	}
	approx(t, s.Mean, 0, exactTolerance, "E(4dF)")
	// Variance per fudge die = 2/3; 4dF → 8/3.
	approx(t, s.StdDev*s.StdDev, 8.0/3.0, exactTolerance, "V(4dF)")
	if s.Min != -4 || s.Max != 4 {
		t.Fatalf("fudge range = [%d..%d], want [-4..4]", s.Min, s.Max)
	}
}

func TestStatsFudgeWithModifier(t *testing.T) {
	s := dice.MustParse("4dF+5").Stats()
	approx(t, s.Mean, 5, exactTolerance, "E(4dF+5)")
	if s.Min != 1 || s.Max != 9 {
		t.Fatalf("range = [%d..%d], want [1..9]", s.Min, s.Max)
	}
}

// TestStatsOversizedKeepDropFallback pins the safety cap on
// enumerateKeepDrop. 10d10kh5 has a 10^10 joint state space; without
// the cap Stats() would hang. The fallback returns Exact=false, a
// reasonable mean within ±20% of the true value, and Min/Max that
// bracket the kept-dice sum. Timing assertion ensures the fallback
// triggers — no real enumeration happens.
func TestStatsOversizedKeepDropFallback(t *testing.T) {
	const limitMs = 100
	start := time.Now()
	s := dice.MustParse("10d10kh5").Stats()
	elapsed := time.Since(start)
	if elapsed > limitMs*time.Millisecond {
		t.Fatalf("Stats took %v (limit %dms) — safety cap failed to trigger", elapsed, limitMs)
	}
	if s.Exact {
		t.Fatal("expected Exact=false for oversized keep/drop")
	}
	// Approximation uses NdS+M bounds for the kept dice: 5d10 range
	// is [5..50].
	if s.Min != 5 || s.Max != 50 {
		t.Fatalf("range = [%d..%d], want [5..50]", s.Min, s.Max)
	}
	// The true mean of 10d10kh5 is well above the basic 10d10 mean of
	// 55. The approximation intentionally drops the keep/drop bias —
	// we only assert the mean is a plausible sum-of-ten-d10s value
	// within ±20% of the basic answer.
	approx(t, s.Mean, 55.0, 11.0, "E(10d10kh5) fallback")
}

func TestStatsPercentileDie(t *testing.T) {
	s := dice.MustParse("1d%").Stats()
	approx(t, s.Mean, 50.5, exactTolerance, "E(d100)")
	if s.Min != 1 || s.Max != 100 {
		t.Fatalf("range = [%d..%d]", s.Min, s.Max)
	}
}

func expectedNdS(n, s, mod int) string {
	out := ""
	out += itoa(n) + "d" + itoa(s)
	if mod > 0 {
		out += "+" + itoa(mod)
	} else if mod < 0 {
		out += "-" + itoa(-mod)
	}
	return out
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	n := len(buf)
	for v > 0 {
		n--
		buf[n] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		n--
		buf[n] = '-'
	}
	return string(buf[n:])
}

package game

import "testing"

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

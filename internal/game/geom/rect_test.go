package geom

import "testing"

func TestRect_Contains(t *testing.T) {
	r := Rect{MinX: -2, MinY: 0, MaxX: 2, MaxY: 3}
	cases := []struct {
		p    Position
		want bool
	}{
		{Position{X: -2, Y: 0}, true},   // top-left inclusive
		{Position{X: 1, Y: 2}, true},    // interior
		{Position{X: 2, Y: 2}, false},   // MaxX exclusive
		{Position{X: 1, Y: 3}, false},   // MaxY exclusive
		{Position{X: -3, Y: 0}, false},  // left of MinX
		{Position{X: 0, Y: -1}, false},  // above MinY
	}
	for _, c := range cases {
		if got := r.Contains(c.p); got != c.want {
			t.Errorf("Rect%+v.Contains(%+v) = %v, want %v", r, c.p, got, c.want)
		}
	}
}

func TestRect_Empty(t *testing.T) {
	cases := []struct {
		r    Rect
		want bool
	}{
		{Rect{}, true},
		{Rect{MinX: 0, MinY: 0, MaxX: 1, MaxY: 1}, false},
		{Rect{MinX: 5, MinY: 5, MaxX: 5, MaxY: 10}, true}, // zero width
		{Rect{MinX: 5, MinY: 5, MaxX: 10, MaxY: 5}, true}, // zero height
		{Rect{MinX: 5, MinY: 5, MaxX: 3, MaxY: 3}, true},  // negative
	}
	for _, c := range cases {
		if got := c.r.Empty(); got != c.want {
			t.Errorf("Rect%+v.Empty() = %v, want %v", c.r, got, c.want)
		}
	}
}

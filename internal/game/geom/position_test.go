package game

import "testing"

func TestPositionAdd(t *testing.T) {
	tests := []struct {
		name   string
		start  Position
		dx, dy int
		want   Position
	}{
		{"origin right", Position{0, 0}, 1, 0, Position{1, 0}},
		{"origin down", Position{0, 0}, 0, 1, Position{0, 1}},
		{"left", Position{3, 4}, -1, 0, Position{2, 4}},
		{"up", Position{3, 4}, 0, -1, Position{3, 3}},
		{"zero", Position{7, 2}, 0, 0, Position{7, 2}},
		{"multi", Position{1, 1}, 4, -3, Position{5, -2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.start.Add(tc.dx, tc.dy)
			if !got.Equal(tc.want) {
				t.Fatalf("Add(%d,%d) = %+v, want %+v", tc.dx, tc.dy, got, tc.want)
			}
		})
	}
}

func TestPositionAddDoesNotMutate(t *testing.T) {
	p := Position{X: 2, Y: 5}
	_ = p.Add(10, 10)
	if p.X != 2 || p.Y != 5 {
		t.Fatalf("Add mutated receiver: %+v", p)
	}
}

func TestPositionEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b Position
		want bool
	}{
		{"same", Position{1, 2}, Position{1, 2}, true},
		{"diff x", Position{1, 2}, Position{0, 2}, false},
		{"diff y", Position{1, 2}, Position{1, 9}, false},
		{"zero equal", Position{}, Position{}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Fatalf("Equal(%+v,%+v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

func TestChunkedSourceDeterministic(t *testing.T) {
	s1 := NewChunkedSource(42)
	s2 := NewChunkedSource(42)
	for _, p := range []geom.Position{
		{X: 0, Y: 0},
		{X: 10, Y: 10},
		{X: -5, Y: 7},
		{X: 100, Y: -100},
	} {
		a := s1.TileAt(p.X, p.Y)
		b := s2.TileAt(p.X, p.Y)
		if a.Terrain != b.Terrain {
			t.Fatalf("seed-determinism broken at %+v: %q vs %q", p, a.Terrain, b.Terrain)
		}
	}
}

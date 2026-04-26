package worldgen

import (
	"math/rand/v2"
	"reflect"
	"testing"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
)

func TestBridsonDeterminism(t *testing.T) {
	bounds := geom.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	rng1 := rand.New(rand.NewPCG(42, 42))
	rng2 := rand.New(rand.NewPCG(42, 42))
	got1 := bridsonSample(rng1, bounds, 10, 30)
	got2 := bridsonSample(rng2, bounds, 10, 30)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatalf("bridsonSample is not deterministic: got %d vs %d points, first diff at equal=%v",
			len(got1), len(got2), reflect.DeepEqual(got1, got2))
	}
	if len(got1) == 0 {
		t.Fatal("bridsonSample returned no points for a 100×100 bounds with minSpacing=10")
	}
}

func TestBridsonSpacing(t *testing.T) {
	bounds := geom.Rect{MinX: 0, MinY: 0, MaxX: 100, MaxY: 100}
	rng := rand.New(rand.NewPCG(7, 13))
	pts := bridsonSample(rng, bounds, 10, 30)
	for i := 0; i < len(pts); i++ {
		for j := i + 1; j < len(pts); j++ {
			d := geom.ChebyshevDist(pts[i], pts[j])
			if d < 10 {
				t.Fatalf("points[%d]=%v and points[%d]=%v have Chebyshev distance %d < 10",
					i, pts[i], j, pts[j], d)
			}
		}
	}
}

func TestBridsonInBounds(t *testing.T) {
	bounds := geom.Rect{MinX: 5, MinY: 10, MaxX: 75, MaxY: 85}
	rng := rand.New(rand.NewPCG(99, 1))
	pts := bridsonSample(rng, bounds, 8, 30)
	for i, p := range pts {
		if !bounds.Contains(p) {
			t.Fatalf("point[%d]=%v is outside bounds %v", i, p, bounds)
		}
	}
}

func TestBridsonCountInRange(t *testing.T) {
	bounds := geom.Rect{MinX: 0, MinY: 0, MaxX: 64, MaxY: 64}
	rng := rand.New(rand.NewPCG(42, 42))
	pts := bridsonSample(rng, bounds, 8, 30)
	const lo, hi = 20, 60
	if len(pts) < lo || len(pts) > hi {
		t.Fatalf("bridsonSample returned %d points for 64×64/spacing=8; want [%d, %d]", len(pts), lo, hi)
	}
	t.Logf("bridsonSample 64×64 minSpacing=8 k=30 → %d points", len(pts))
}

func TestBridsonEmptyBounds(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	validBounds := geom.Rect{MinX: 0, MinY: 0, MaxX: 32, MaxY: 32}

	// Empty rect (MaxX <= MinX).
	emptyRect := geom.Rect{MinX: 5, MinY: 5, MaxX: 5, MaxY: 20}
	if got := bridsonSample(rng, emptyRect, 8, 30); got != nil {
		t.Fatalf("expected nil for empty rect, got %v", got)
	}

	// minSpacing == 0.
	if got := bridsonSample(rng, validBounds, 0, 30); got != nil {
		t.Fatalf("expected nil for minSpacing=0, got %v", got)
	}

	// minSpacing < 0.
	if got := bridsonSample(rng, validBounds, -5, 30); got != nil {
		t.Fatalf("expected nil for minSpacing=-5, got %v", got)
	}
}

func TestBridsonMultipleSeeds(t *testing.T) {
	bounds := geom.Rect{MinX: 0, MinY: 0, MaxX: 80, MaxY: 80}

	rngA := rand.New(rand.NewPCG(1, 1))
	rngB := rand.New(rand.NewPCG(2, 2))
	ptsA := bridsonSample(rngA, bounds, 10, 30)
	ptsB := bridsonSample(rngB, bounds, 10, 30)

	// Each seed produces deterministic output individually.
	rngA2 := rand.New(rand.NewPCG(1, 1))
	rngB2 := rand.New(rand.NewPCG(2, 2))
	ptsA2 := bridsonSample(rngA2, bounds, 10, 30)
	ptsB2 := bridsonSample(rngB2, bounds, 10, 30)

	if !reflect.DeepEqual(ptsA, ptsA2) {
		t.Fatal("seed A is not self-consistent across two calls")
	}
	if !reflect.DeepEqual(ptsB, ptsB2) {
		t.Fatal("seed B is not self-consistent across two calls")
	}

	// Different seeds should produce different results.
	if reflect.DeepEqual(ptsA, ptsB) {
		t.Fatal("seed A and seed B produced identical output — seeds are not independent")
	}
}

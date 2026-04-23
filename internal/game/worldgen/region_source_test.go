package worldgen

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// TestNoiseRegionSourceDeterminism guarantees the core interface contract:
// same seed + same SuperChunkCoord ⇒ byte-identical Region, name included.
func TestNoiseRegionSourceDeterminism(t *testing.T) {
	const seed int64 = 0x1234_5678

	srcA := NewNoiseRegionSource(seed)
	srcB := NewNoiseRegionSource(seed)

	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0},
		{X: 1, Y: -3},
		{X: -7, Y: 42},
		{X: 128, Y: 256},
	}
	for _, sc := range coords {
		a := srcA.RegionAt(sc)
		b := srcB.RegionAt(sc)
		if a != b {
			t.Errorf("RegionAt(%v) differs between identically-seeded sources:\n  A=%+v\n  B=%+v",
				sc, a, b)
		}
	}

	// Additionally: two calls on the same source must match each other.
	for _, sc := range coords {
		a := srcA.RegionAt(sc)
		b := srcA.RegionAt(sc)
		if a != b {
			t.Errorf("RegionAt(%v) is non-idempotent on a single source", sc)
		}
	}
}

// TestInfluenceAtFloor samples 1000 random coordinates and asserts every
// component of the influence vector lives in [0, 1] — the floor-and-
// rescale pipeline is the only thing between raw [0, 1] noise and the
// caller, so any drift here means the math is off.
func TestInfluenceAtFloor(t *testing.T) {
	src := NewNoiseRegionSource(99)
	rng := rand.New(rand.NewPCG(1, 1))

	for i := range 1000 {
		x := rng.IntN(100_000) - 50_000
		y := rng.IntN(100_000) - 50_000
		infl := src.InfluenceAt(x, y)

		components := []struct {
			name string
			v    float32
		}{
			{"Blight", infl.Blight},
			{"Fae", infl.Fae},
			{"Ancient", infl.Ancient},
			{"Savage", infl.Savage},
			{"Holy", infl.Holy},
			{"Wild", infl.Wild},
		}
		for _, c := range components {
			if c.v < 0 || c.v > 1 {
				t.Fatalf("iteration %d at (%d,%d): %s = %f outside [0, 1]",
					i, x, y, c.name, c.v)
			}
		}
	}
}

// TestInfluenceAtContinuity spot-checks the smoothness of the noise
// fields. Adjacent tiles must differ by less than 0.05 on the Blight
// component — fBm is locally Lipschitz and any jump larger than that
// signals a bug in the sampling pipeline (e.g. forgetting to use
// floating-point coords).
//
// The test uses non-zero anchor coords to dodge the unlikely event that
// (0, 0) sits exactly on a zero-crossing that amplifies rounding noise.
func TestInfluenceAtContinuity(t *testing.T) {
	src := NewNoiseRegionSource(7)

	const delta = 0.05
	base := 10_000
	for i := range 200 {
		x := base + i
		y := base - i
		a := src.InfluenceAt(x, y).Blight
		b := src.InfluenceAt(x+1, y).Blight
		if diff := math.Abs(float64(a - b)); diff >= delta {
			t.Fatalf("Blight jumped %f between (%d,%d) and (%d,%d)", diff, x, y, x+1, y)
		}
	}
}

// TestInfluenceAtVsRegionAt locks the invariant that a region's recorded
// influence equals the one you would sample at its anchor. Breaking this
// would desync server-side RegionAt results from client-side tile-level
// influence rendering.
func TestInfluenceAtVsRegionAt(t *testing.T) {
	src := NewNoiseRegionSource(0xcafe_babe)

	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0},
		{X: 3, Y: -2},
		{X: -1, Y: 1},
		{X: 100, Y: 100},
	}
	for _, sc := range coords {
		region := src.RegionAt(sc)
		want := src.InfluenceAt(region.Anchor.X, region.Anchor.Y)
		if region.Influence != want {
			t.Errorf("sc=%v: region.Influence=%+v, InfluenceAt(anchor)=%+v",
				sc, region.Influence, want)
		}
	}
}

// TestInterfaceCompliance is a readable restatement of the compile-time
// assertion in region_source.go. Keeping it in the test file too gives a
// second point of discoverability when the interface changes.
func TestInterfaceCompliance(t *testing.T) {
	var _ world.RegionSource = (*NoiseRegionSource)(nil)
	var _ world.RegionSource = NewNoiseRegionSource(1)
}

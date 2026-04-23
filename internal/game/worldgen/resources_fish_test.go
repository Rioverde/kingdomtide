package worldgen

import (
	"math"
	"testing"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// sweepFish walks a side×side window and returns the fish deposits it
// finds, plus the count of eligible beach-facing-ocean tiles observed
// so tests can compute the selection fraction without re-walking.
func sweepFish(seed int64, side int) (fish []world.Deposit, eligible int) {
	wg := NewWorldGenerator(seed)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			t := geom.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			if tile.Terrain == world.TerrainBeach && beachFacesOpenOcean(t, wg) {
				eligible++
			}
			if dep, ok := fishDepositAt(seed, t, wg); ok {
				fish = append(fish, dep)
			}
		}
	}
	return fish, eligible
}

// TestFishDepositAt_OnlyBeachFacingOcean asserts every fish deposit
// sits on a beach tile that has at least one ocean or deep-ocean
// neighbour. Lake / river adjacency alone does not qualify.
func TestFishDepositAt_OnlyBeachFacingOcean(t *testing.T) {
	if testing.Short() {
		t.Skip("400x400 beach-facing-ocean fish placement sweep")
	}
	const seed int64 = 42
	const side = 400
	wg := NewWorldGenerator(seed)
	fish, _ := sweepFish(seed, side)
	if len(fish) == 0 {
		t.Fatalf("seed %d yielded no fish deposits in %dx%d window", seed, side, side)
	}
	for _, dep := range fish {
		tile := wg.TileAt(dep.Position.X, dep.Position.Y)
		if tile.Terrain != world.TerrainBeach {
			t.Errorf("fish on non-beach terrain %q at %+v", tile.Terrain, dep.Position)
		}
		if !beachFacesOpenOcean(dep.Position, wg) {
			t.Errorf("fish at %+v does not face open ocean", dep.Position)
		}
	}
}

// TestFishDepositAt_NoCrashOnLandmarks is a light smoke test — fish
// placement does not consult the landmark source in M2, so a landmark
// sitting on a beach-facing-ocean tile can co-exist with a fish
// deposit. The test just asserts fishDepositAt does not crash across
// a broad sweep.
func TestFishDepositAt_NoCrashOnLandmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("100x100 fish landmark crash smoke test")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	for y := -50; y < 50; y++ {
		for x := -50; x < 50; x++ {
			_, _ = fishDepositAt(seed, geom.Position{X: x, Y: y}, wg)
		}
	}
}

// TestFishDepositAt_DensityFraction asserts the observed fish fraction
// among eligible beach-facing-ocean tiles stays within ±10% of
// fishDensityFraction. The absolute tolerance absorbs the hash
// stream's small-sample noise over a single window; sweeping multiple
// seeds shrinks the variance further.
func TestFishDepositAt_DensityFraction(t *testing.T) {
	if testing.Short() {
		t.Skip("4-seed 500x500 fish density fraction sweep")
	}
	seeds := []int64{1, 2, 3, 42}
	const side = 500
	var totalFish, totalEligible int
	for _, seed := range seeds {
		fish, eligible := sweepFish(seed, side)
		totalFish += len(fish)
		totalEligible += eligible
	}
	if totalEligible == 0 {
		t.Fatalf("no eligible coast tiles across %d seeds x %d^2 tiles", len(seeds), side)
	}
	got := float64(totalFish) / float64(totalEligible)
	t.Logf("eligible=%d fish=%d fraction=%.3f want=%.3f",
		totalEligible, totalFish, got, fishDensityFraction)
	if math.Abs(got-fishDensityFraction) > 0.10 {
		t.Errorf("fish density %.3f outside want=%.3f ± 0.10", got, fishDensityFraction)
	}
}

// TestFishDepositAt_Determinism asserts same seed + same coords yield
// the same outcome (present / absent and if present, same Deposit).
func TestFishDepositAt_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("200x200 fish determinism dual-sweep")
	}
	const seed int64 = 42
	wg1 := NewWorldGenerator(seed)
	wg2 := NewWorldGenerator(seed)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			p := geom.Position{X: x, Y: y}
			a, okA := fishDepositAt(seed, p, wg1)
			b, okB := fishDepositAt(seed, p, wg2)
			if okA != okB {
				t.Fatalf("pos %+v: okA=%v okB=%v", p, okA, okB)
			}
			if okA && a != b {
				t.Errorf("pos %+v: a=%+v b=%+v", p, a, b)
			}
		}
	}
}

// TestFishDepositAt_RejectsNonBeach asserts a call on a clearly
// non-beach tile (e.g. a plains tile far from any water) returns
// (Deposit{}, false). Sample several tiles to stay robust against
// seed-specific terrain layouts.
func TestFishDepositAt_RejectsNonBeach(t *testing.T) {
	if testing.Short() {
		t.Skip("1000x1000 non-beach rejection scan")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	checked := 0
	for y := -500; y < 500 && checked < 10; y++ {
		for x := -500; x < 500 && checked < 10; x++ {
			p := geom.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			if tile.Terrain == world.TerrainBeach {
				continue
			}
			_, ok := fishDepositAt(seed, p, wg)
			if ok {
				t.Errorf("fish on non-beach terrain %q at %+v", tile.Terrain, p)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatalf("no non-beach tiles sampled")
	}
}

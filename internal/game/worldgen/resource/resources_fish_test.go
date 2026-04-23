package resource_test

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/game/worldgen/resource"
)

// sweepFish walks a side×side window and returns the fish deposits it
// finds, plus the count of eligible beach-facing-ocean tiles observed
// so tests can compute the selection fraction without re-walking.
func sweepFish(seed int64, side int) (fish []world.Deposit, eligible int) {
	wg := worldgen.NewWorldGenerator(seed)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			t := geom.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			if tile.Terrain == world.TerrainBeach && resource.BeachFacesOpenOceanForTest(t, wg) {
				eligible++
			}
			if dep, ok := resource.FishDepositAtForTest(seed, t, wg); ok {
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
	wg := worldgen.NewWorldGenerator(seed)
	fish, _ := sweepFish(seed, side)
	if len(fish) == 0 {
		t.Fatalf("seed %d yielded no fish deposits in %dx%d window", seed, side, side)
	}
	for _, dep := range fish {
		tile := wg.TileAt(dep.Position.X, dep.Position.Y)
		if tile.Terrain != world.TerrainBeach {
			t.Errorf("fish on non-beach terrain %q at %+v", tile.Terrain, dep.Position)
		}
		if !resource.BeachFacesOpenOceanForTest(dep.Position, wg) {
			t.Errorf("fish at %+v does not face open ocean", dep.Position)
		}
	}
}

// TestFishDepositAt_NoCrashOnLandmarks is a light smoke test — fish
// placement does not consult the landmark source, so a landmark
// sitting on a beach-facing-ocean tile can co-exist with a fish
// deposit. The test just asserts fishDepositAt does not crash across
// a broad sweep.
func TestFishDepositAt_NoCrashOnLandmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("100x100 fish landmark crash smoke test")
	}
	const seed int64 = 42
	wg := worldgen.NewWorldGenerator(seed)
	for y := -50; y < 50; y++ {
		for x := -50; x < 50; x++ {
			_, _ = resource.FishDepositAtForTest(seed, geom.Position{X: x, Y: y}, wg)
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
		totalEligible, totalFish, got, resource.FishDensityFractionForTest)
	if math.Abs(got-resource.FishDensityFractionForTest) > 0.10 {
		t.Errorf("fish density %.3f outside want=%.3f ± 0.10", got, resource.FishDensityFractionForTest)
	}
}

// TestFishDepositAt_Determinism asserts same seed + same coords yield
// the same outcome (present / absent and if present, same Deposit).
func TestFishDepositAt_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("200x200 fish determinism dual-sweep")
	}
	const seed int64 = 42
	wg1 := worldgen.NewWorldGenerator(seed)
	wg2 := worldgen.NewWorldGenerator(seed)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			p := geom.Position{X: x, Y: y}
			a, okA := resource.FishDepositAtForTest(seed, p, wg1)
			b, okB := resource.FishDepositAtForTest(seed, p, wg2)
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
	wg := worldgen.NewWorldGenerator(seed)
	checked := 0
	for y := -500; y < 500 && checked < 10; y++ {
		for x := -500; x < 500 && checked < 10; x++ {
			p := geom.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			if tile.Terrain == world.TerrainBeach {
				continue
			}
			_, ok := resource.FishDepositAtForTest(seed, p, wg)
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

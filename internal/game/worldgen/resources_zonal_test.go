package worldgen

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// newZonalNoiseMap builds the per-kind noise map the same way
// NewNoiseDepositSource does. Used by focused tests that exercise
// zonalDepositAt without paying for a full source construction.
func newZonalNoiseMap(seed int64) map[game.DepositKind]OctaveNoise {
	noises := make(map[game.DepositKind]OctaveNoise, len(zonalKinds))
	for _, k := range zonalKinds {
		noises[k] = NewOctaveNoise(seed^zonalSubSalts[k], zonalNoiseOpts)
	}
	return noises
}

// sweepZonal iterates a square window, calls zonalDepositAt on every
// tile, and returns every (position, deposit) that passed. Used by
// determinism and frequency tests. Window origin defaults to (0, 0).
func sweepZonal(seed int64, side int) map[game.Position]game.Deposit {
	wg := NewWorldGenerator(seed)
	noises := newZonalNoiseMap(seed)
	out := make(map[game.Position]game.Deposit, side*side/4)
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			t := game.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			if dep, ok := zonalDepositAt(t, tile.Terrain, noises); ok {
				out[t] = dep
			}
		}
	}
	return out
}

// TestZonalDepositAt_Determinism asserts two independently seeded
// runs with the same seed produce identical deposit maps over a known
// 200x200 window.
func TestZonalDepositAt_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("200x200 dual-sweep zonal determinism check")
	}
	const seed int64 = 42
	a := sweepZonal(seed, 200)
	b := sweepZonal(seed, 200)
	if len(a) != len(b) {
		t.Fatalf("len mismatch: a=%d b=%d", len(a), len(b))
	}
	for p, depA := range a {
		depB, ok := b[p]
		if !ok {
			t.Fatalf("pos %+v in a missing from b", p)
		}
		if depA != depB {
			t.Errorf("pos %+v: a=%+v b=%+v", p, depA, depB)
		}
	}
}

// TestZonalDepositAt_BiomeGate asserts Fertile, Timber, and Game only
// appear on their declared biome sets. Walks a large window and
// verifies every deposit's tile carries a biome-gate-accepting terrain
// for its kind.
func TestZonalDepositAt_BiomeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("300x300 biome gate verification sweep")
	}
	const seed int64 = 42
	const side = 300
	wg := NewWorldGenerator(seed)
	noises := newZonalNoiseMap(seed)

	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			p := game.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			dep, ok := zonalDepositAt(p, tile.Terrain, noises)
			if !ok {
				continue
			}
			if !zonalBiomeAccepts(dep.Kind, tile.Terrain) {
				t.Errorf("kind=%s on terrain=%q at %+v — biome gate should have rejected",
					dep.Kind, tile.Terrain, p)
			}
			// Extra belt-and-braces: Fertile must never appear on
			// mountain, ocean, desert, or forest; Timber must never on
			// plains, ocean, desert, or mountain; Game must never on
			// desert, ocean, or mountain.
			switch dep.Kind {
			case game.DepositFertile:
				switch tile.Terrain {
				case game.TerrainMountain, game.TerrainSnowyPeak,
					game.TerrainOcean, game.TerrainDeepOcean,
					game.TerrainDesert,
					game.TerrainForest, game.TerrainTaiga, game.TerrainJungle:
					t.Errorf("fertile on invalid terrain %q at %+v", tile.Terrain, p)
				}
			case game.DepositTimber:
				switch tile.Terrain {
				case game.TerrainPlains, game.TerrainGrassland,
					game.TerrainMeadow, game.TerrainSavanna,
					game.TerrainDesert,
					game.TerrainMountain, game.TerrainSnowyPeak,
					game.TerrainOcean, game.TerrainDeepOcean:
					t.Errorf("timber on invalid terrain %q at %+v", tile.Terrain, p)
				}
			case game.DepositGame:
				switch tile.Terrain {
				case game.TerrainDesert,
					game.TerrainMountain, game.TerrainSnowyPeak,
					game.TerrainOcean, game.TerrainDeepOcean,
					game.TerrainPlains:
					t.Errorf("game on invalid terrain %q at %+v", tile.Terrain, p)
				}
			}
		}
	}
}

// TestZonalDepositAt_Frequency samples valid-biome tiles for each
// zonal kind and asserts the observed in-zone fraction lands within
// ±10% absolute of the tuned target. The targets come from the
// thresholds tuned in resources_zonal.go against the OpenSimplex
// distribution (see comment on zonalThresholds). Target fractions:
//
//	Fertile 0.35, Timber 0.40, Game 0.38
func TestZonalDepositAt_Frequency(t *testing.T) {
	if testing.Short() {
		t.Skip("4-seed 400x400 zonal frequency sweep")
	}
	// Sweep multiple seeds and a large window so the fractions have
	// enough samples to stabilise. Noise is strongly position-correlated
	// at the 50-tile zone size, so small windows produce swingy
	// fractions. Across 4 seeds x 400^2 tiles the counts settle.
	seeds := []int64{1, 2, 3, 42}
	const side = 400

	wantFraction := map[game.DepositKind]float64{
		game.DepositFertile: 0.35,
		game.DepositTimber:  0.40,
		game.DepositGame:    0.38,
	}

	type counters struct {
		validBiome int
		inZone     int
	}
	byKind := map[game.DepositKind]*counters{
		game.DepositFertile: {},
		game.DepositTimber:  {},
		game.DepositGame:    {},
	}

	// Because zonalDepositAt picks the first passing kind in enum order
	// (Fertile, Timber, Game), Timber and Game can over-gate a forest
	// tile: if a forest tile passes Timber's threshold it never gets
	// asked about Game. To measure per-kind frequency in isolation we
	// sample each kind independently, bypassing the iteration order.
	for _, seed := range seeds {
		wg := NewWorldGenerator(seed)
		noises := newZonalNoiseMap(seed)
		for y := 0; y < side; y++ {
			for x := 0; x < side; x++ {
				tile := wg.TileAt(x, y)
				for kind, ctr := range byKind {
					if !zonalBiomeAccepts(kind, tile.Terrain) {
						continue
					}
					ctr.validBiome++
					fx := float64(x) * zonalPerlinScale
					fy := float64(y) * zonalPerlinScale
					v := noises[kind].Eval2Normalized(fx, fy)
					if v > zonalThresholds[kind] {
						ctr.inZone++
					}
				}
			}
		}
	}

	const tolerance = 0.10
	for kind, ctr := range byKind {
		if ctr.validBiome == 0 {
			t.Fatalf("kind=%s: zero valid biome tiles across seeds; widen the window", kind)
		}
		got := float64(ctr.inZone) / float64(ctr.validBiome)
		want := wantFraction[kind]
		t.Logf("kind=%s valid=%d inZone=%d got=%.3f want=%.3f", kind, ctr.validBiome, ctr.inZone, got, want)
		if math.Abs(got-want) > tolerance {
			t.Errorf("kind=%s frequency %.3f outside want=%.3f ± %.3f", kind, got, want, tolerance)
		}
	}
}

// TestZonalDepositAt_AtMostOneKind asserts zonalDepositAt never returns
// a deposit whose Kind is DepositNone or that appears to carry more
// than one role. Since zonalDepositAt returns a single Deposit value,
// "at most one" is satisfied by construction — the test makes the
// invariant explicit by sampling 1000 tiles and checking every
// returned Kind is one of the three enumerated zonal kinds.
func TestZonalDepositAt_AtMostOneKind(t *testing.T) {
	if testing.Short() {
		t.Skip("1000-tile zonal kind uniqueness sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	noises := newZonalNoiseMap(seed)
	valid := map[game.DepositKind]bool{
		game.DepositFertile: true,
		game.DepositTimber:  true,
		game.DepositGame:    true,
	}
	checked := 0
	for y := 0; y < 100 && checked < 1000; y++ {
		for x := 0; x < 100 && checked < 1000; x++ {
			t.Helper()
			p := game.Position{X: x, Y: y}
			tile := wg.TileAt(x, y)
			dep, ok := zonalDepositAt(p, tile.Terrain, noises)
			checked++
			if !ok {
				continue
			}
			if !valid[dep.Kind] {
				t.Errorf("unexpected kind %s at %+v", dep.Kind, p)
			}
			if dep.MaxAmount <= 0 {
				t.Errorf("deposit %+v has zero or negative MaxAmount", dep)
			}
			if dep.CurrentAmount != dep.MaxAmount {
				t.Errorf("deposit %+v: CurrentAmount %d != MaxAmount %d",
					dep, dep.CurrentAmount, dep.MaxAmount)
			}
		}
	}
}

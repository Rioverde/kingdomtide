package worldgen

import (
	"math"
	"reflect"
	"sync"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// newVolcanoTestSource wires a fresh NoiseVolcanoSource for seed. The
// world-gen stack mirrors the production plumbing so biome gates and
// landmark collisions exercise real data.
func newVolcanoTestSource(tb testing.TB, seed int64) *NoiseVolcanoSource {
	tb.Helper()
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	return NewNoiseVolcanoSource(seed, wg, lm)
}

// collectVolcanoes returns every volcano whose anchor sits inside the
// SC block [minSCX, maxSCX) x [minSCY, maxSCY). Caller-controlled scope
// keeps individual tests from paying for a globally large sweep.
func collectVolcanoes(src *NoiseVolcanoSource, minSCX, minSCY, maxSCX, maxSCY int) []world.Volcano {
	var out []world.Volcano
	for x := minSCX; x < maxSCX; x++ {
		for y := minSCY; y < maxSCY; y++ {
			out = append(out, src.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y})...)
		}
	}
	return out
}

// volcanoesAcrossSeeds sweeps a few seeds and returns every volcano
// observed inside a modest SC window per seed. Used by distribution
// tests that need enough samples to stabilise empirical fractions.
func volcanoesAcrossSeeds(t testing.TB, seeds []int64, scSide int) []world.Volcano {
	t.Helper()
	var out []world.Volcano
	for _, s := range seeds {
		src := newVolcanoTestSource(t, s)
		out = append(out, collectVolcanoes(src, -scSide, -scSide, scSide, scSide)...)
	}
	return out
}

func TestNoiseVolcanoSource_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("20x20 SC sweep; run without -short for full coverage")
	}
	const seed int64 = 1337
	srcA := newVolcanoTestSource(t, seed)
	srcB := newVolcanoTestSource(t, seed)

	for x := -10; x < 10; x++ {
		for y := -10; y < 10; y++ {
			sc := geom.SuperChunkCoord{X: x, Y: y}
			a := srcA.VolcanoAt(sc)
			b := srcB.VolcanoAt(sc)
			if !reflect.DeepEqual(a, b) {
				t.Fatalf("seed %d sc=(%d,%d) mismatch:\n  a=%+v\n  b=%+v", seed, x, y, a, b)
			}
		}
	}
}

func TestNoiseVolcanoSource_PoissonDiskMinSpacing(t *testing.T) {
	if testing.Short() {
		t.Skip("40x40 SC sweep")
	}
	const seed int64 = 9001
	src := newVolcanoTestSource(t, seed)
	// 5x5 super-region block = 40x40 super-chunks.
	all := collectVolcanoes(src, -20, -20, 20, 20)
	if len(all) < 2 {
		t.Skipf("seed %d yielded %d volcanoes, not enough to test spacing", seed, len(all))
	}
	minSq := volcanoMinSpacingTiles * volcanoMinSpacingTiles
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			// Cross-super-region pairs can sit closer than the Poisson-
			// disk spacing (each super-region is sampled independently),
			// so only assert the invariant for same-super-region pairs.
			srI := superRegionOf(geom.WorldToSuperChunk(all[i].Anchor.X, all[i].Anchor.Y))
			srJ := superRegionOf(geom.WorldToSuperChunk(all[j].Anchor.X, all[j].Anchor.Y))
			if srI != srJ {
				continue
			}
			dx := all[i].Anchor.X - all[j].Anchor.X
			dy := all[i].Anchor.Y - all[j].Anchor.Y
			if dx*dx+dy*dy < minSq {
				t.Errorf("anchors %+v and %+v within super-region %+v closer than %d tiles (d^2=%d)",
					all[i].Anchor, all[j].Anchor, srI, volcanoMinSpacingTiles, dx*dx+dy*dy)
			}
		}
	}
}

func TestNoiseVolcanoSource_StateDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("10 seeds x 96x96 SC for 500+ volcanoes")
	}
	seeds := []int64{1, 2, 3, 5, 7, 11, 13, 17, 19, 23}
	all := volcanoesAcrossSeeds(t, seeds, 48)
	if len(all) < 500 {
		t.Fatalf("need >=500 volcanoes for distribution, got %d", len(all))
	}
	counts := map[world.VolcanoState]int{}
	for _, v := range all {
		counts[v.State]++
	}
	total := float64(len(all))
	fracActive := float64(counts[world.VolcanoActive]) / total
	fracDormant := float64(counts[world.VolcanoDormant]) / total
	fracExtinct := float64(counts[world.VolcanoExtinct]) / total
	t.Logf("n=%d active=%.3f dormant=%.3f extinct=%.3f", len(all), fracActive, fracDormant, fracExtinct)
	checkFrac := func(name string, got, want, tol float64) {
		if math.Abs(got-want) > tol {
			t.Errorf("fraction %s = %.3f, want %.3f ± %.3f", name, got, want, tol)
		}
	}
	checkFrac("active", fracActive, 0.20, 0.05)
	checkFrac("dormant", fracDormant, 0.30, 0.05)
	checkFrac("extinct", fracExtinct, 0.50, 0.05)
}

func TestNoiseVolcanoSource_BiomeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("32x32 SC sweep")
	}
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)
	wg := NewWorldGenerator(seed)
	all := collectVolcanoes(src, -16, -16, 16, 16)
	if len(all) == 0 {
		t.Fatalf("seed %d yielded no volcanoes inside SC window", seed)
	}
	for _, v := range all {
		tile := wg.TileAt(v.Anchor.X, v.Anchor.Y)
		if isWaterOrRiverTile(tile) {
			t.Errorf("anchor %+v is on water tile terrain=%q overlays=%v", v.Anchor, tile.Terrain, tile.Overlays)
		}
		if tile.Terrain == world.TerrainBeach {
			t.Errorf("anchor %+v is on beach", v.Anchor)
		}
	}
}

func TestNoiseVolcanoSource_NoLandmarkCollision(t *testing.T) {
	if testing.Short() {
		t.Skip("32x32 SC sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	src := NewNoiseVolcanoSource(seed, wg, lm)

	all := collectVolcanoes(src, -16, -16, 16, 16)
	if len(all) == 0 {
		t.Fatalf("seed %d yielded no volcanoes", seed)
	}

	for _, v := range all {
		home := geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y)
		landmarkSet := make(map[geom.Position]struct{})
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
				for _, l := range lm.LandmarksIn(sc) {
					landmarkSet[l.Coord] = struct{}{}
				}
			}
		}
		check := func(label string, ps []geom.Position) {
			for _, p := range ps {
				if _, hit := landmarkSet[p]; hit {
					t.Errorf("volcano %+v %s tile %+v collides with landmark", v.Anchor, label, p)
				}
			}
		}
		check("core", v.CoreTiles)
		check("slope", v.SlopeTiles)
		check("ashland", v.AshlandTiles)
	}
}

func TestNoiseVolcanoSource_TerrainOverrideAt_KnownAnchor(t *testing.T) {
	if testing.Short() {
		t.Skip("40x40 SC sweep to find one volcano per state")
	}
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)

	// Scan a 40x40 SC block — with the post-fix density this surfaces
	// several dozen volcanoes, plenty to cover every state at least once.
	// Previously the test hard-coded (anchor, state) triples from an
	// empirical probe; tuning placement parameters would silently break
	// those co-ordinates. The parametric form instead samples the world
	// at runtime and asserts the invariant per state — resilient to
	// future density tweaks.
	all := collectVolcanoes(src, -20, -20, 20, 20)
	if len(all) < 10 {
		t.Fatalf("seed %d yielded only %d volcanoes in a 40x40 SC window; density too low to exercise the test", seed, len(all))
	}

	wantByState := map[world.VolcanoState]world.Terrain{
		world.VolcanoActive:  world.TerrainVolcanoCore,
		world.VolcanoDormant: world.TerrainVolcanoCoreDormant,
		world.VolcanoExtinct: world.TerrainCraterLake,
	}
	seen := map[world.VolcanoState]bool{}
	for _, v := range all {
		if seen[v.State] {
			continue
		}
		seen[v.State] = true
		ter, ok := src.TerrainOverrideAt(v.Anchor)
		if !ok {
			t.Errorf("state=%s anchor=%+v: TerrainOverrideAt returned no override", v.State, v.Anchor)
			continue
		}
		want := wantByState[v.State]
		if ter != want {
			t.Errorf("state=%s anchor=%+v: got terrain %q want %q", v.State, v.Anchor, ter, want)
		}
	}
	for state := range wantByState {
		if !seen[state] {
			t.Logf("state %s not observed in window — widen the scan if this fires repeatedly", state)
		}
	}
}

func TestNoiseVolcanoSource_TerrainOverrideAt_OutsideFootprint(t *testing.T) {
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)
	// A point far from every volcano observed in the probe sweep.
	ter, ok := src.TerrainOverrideAt(geom.Position{X: 9999999, Y: 9999999})
	if ok {
		t.Errorf("unexpected override far from origin: %q", ter)
	}
}

func TestNoiseVolcanoSource_CrossSR_Spillover(t *testing.T) {
	if testing.Short() {
		t.Skip("48x48 SC sweep")
	}
	// Search a small SC window looking for an anchor whose footprint
	// spills across a super-region boundary. When found, pick a spill
	// tile and assert the override resolves.
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)

	found := false
	for x := -24; x < 24 && !found; x++ {
		for y := -24; y < 24 && !found; y++ {
			sc := geom.SuperChunkCoord{X: x, Y: y}
			home := superRegionOf(sc)
			for _, v := range src.VolcanoAt(sc) {
				all := append([]geom.Position{}, v.CoreTiles...)
				all = append(all, v.SlopeTiles...)
				all = append(all, v.AshlandTiles...)
				for _, p := range all {
					tileSR := superRegionOf(geom.WorldToSuperChunk(p.X, p.Y))
					if tileSR == home {
						continue
					}
					ter, ok := src.TerrainOverrideAt(p)
					if !ok {
						t.Fatalf("spill tile %+v (from anchor %+v, home sr %+v, tile sr %+v) not overridden",
							p, v.Anchor, home, tileSR)
					}
					if ter == "" {
						t.Fatalf("spill tile %+v returned empty terrain", p)
					}
					found = true
					break
				}
				if found {
					break
				}
			}
		}
	}
	// A spillover case is likely but not guaranteed within the 48x48 SC
	// window; if none present we skip rather than fail so the test isn't
	// brittle against small tuning changes.
	if !found {
		t.Skip("no cross-super-region spill observed in the scan window for seed 42")
	}
}

func TestNoiseVolcanoSource_ConcurrentRead(t *testing.T) {
	if testing.Short() {
		t.Skip("8 goroutines x 500 queries")
	}
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)

	const goroutines = 8
	const perGoroutine = 500

	// Pre-collect the reference across the SC window the goroutines below
	// actually query.
	reference := collectVolcanoes(src, -16, -16, 16, 16)
	want := make(map[geom.SuperChunkCoord][]world.Volcano)
	for _, v := range reference {
		sc := geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y)
		want[sc] = append(want[sc], v)
	}

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := range goroutines {
		go func(g int) {
			defer wg.Done()
			for i := range perGoroutine {
				x := (g*31 + i*7) % 16
				y := (g*17 + i*13) % 16
				if (i & 1) == 0 {
					x = -x
				}
				if (i & 2) == 0 {
					y = -y
				}
				sc := geom.SuperChunkCoord{X: x, Y: y}
				got := src.VolcanoAt(sc)
				expected := want[sc]
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("goroutine %d iter %d sc=%+v mismatch\n  got=%+v\n  want=%+v",
						g, i, sc, got, expected)
					return
				}
				_, _ = src.TerrainOverrideAt(geom.Position{X: x*64 + i, Y: y*64 + i})
			}
		}(g)
	}
	wg.Wait()
}

func TestNoiseVolcanoSource_AnchorSCRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("16x16 SC sweep")
	}
	const seed int64 = 42
	src := newVolcanoTestSource(t, seed)
	all := collectVolcanoes(src, -8, -8, 8, 8)
	for _, v := range all {
		sc := geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y)
		seen := false
		for _, w := range src.VolcanoAt(sc) {
			if w.Anchor.Equal(v.Anchor) {
				seen = true
				break
			}
		}
		if !seen {
			t.Errorf("volcano anchor %+v not returned by VolcanoAt(%+v)", v.Anchor, sc)
		}
	}
}

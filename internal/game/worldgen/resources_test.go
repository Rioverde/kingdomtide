package worldgen

import (
	"reflect"
	"sort"
	"sync"
	"testing"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// newDepositTestSource wires a fresh NoiseDepositSource for seed.
// landmarks and volcanoes are nil for M2 — neither is consulted by the
// zonal or fish placement paths. When M3/M4 land, swap in real sources
// here so the integration tests exercise the full pipeline.
func newDepositTestSource(tb testing.TB, seed int64) *NoiseDepositSource {
	tb.Helper()
	wg := NewWorldGenerator(seed)
	return NewNoiseDepositSource(seed, wg, nil, nil)
}

// collectDeposits yields every deposit whose position lies inside the
// SC block [minSCX, maxSCX) x [minSCY, maxSCY). Mirrors the volcano
// test helper pattern so the two layers stay easy to compare.
func collectDeposits(src *NoiseDepositSource, minSCX, minSCY, maxSCX, maxSCY int) []world.Deposit {
	rect := geom.Rect{
		MinX: minSCX * geom.SuperChunkSize,
		MinY: minSCY * geom.SuperChunkSize,
		MaxX: maxSCX * geom.SuperChunkSize,
		MaxY: maxSCY * geom.SuperChunkSize,
	}
	return src.DepositsIn(rect)
}

// TestNoiseDepositSource_DepositAt_ZonalHit finds the first zonal
// deposit inside a modest window and verifies DepositAt returns it by
// exact position. Used as the canonical round-trip: sweep -> lookup.
func TestNoiseDepositSource_DepositAt_ZonalHit(t *testing.T) {
	if testing.Short() {
		t.Skip("8x8 SC deposit round-trip sweep")
	}
	const seed int64 = 42
	src := newDepositTestSource(t, seed)

	all := collectDeposits(src, -4, -4, 4, 4)
	if len(all) == 0 {
		t.Fatalf("seed %d yielded no deposits in the 8x8 SC window", seed)
	}
	// Pick the first zonal hit so the assertion is not contingent on
	// fish appearing in the window.
	var target world.Deposit
	found := false
	for _, d := range all {
		if d.Kind == world.DepositFertile || d.Kind == world.DepositTimber || d.Kind == world.DepositGame {
			target = d
			found = true
			break
		}
	}
	if !found {
		t.Skipf("no zonal deposits in window for seed %d", seed)
	}
	got, ok := src.DepositAt(target.Position)
	if !ok {
		t.Fatalf("DepositAt(%+v) returned not-found", target.Position)
	}
	if got != target {
		t.Errorf("DepositAt mismatch: got=%+v want=%+v", got, target)
	}
}

// TestNoiseDepositSource_DepositAt_Miss asserts a tile inside a deep
// ocean returns (Deposit{}, false). Walk until we find a deep-ocean
// tile in the window; otherwise skip.
func TestNoiseDepositSource_DepositAt_Miss(t *testing.T) {
	const seed int64 = 42
	src := newDepositTestSource(t, seed)
	wg := NewWorldGenerator(seed)

	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			tile := wg.TileAt(x, y)
			if tile.Terrain != world.TerrainDeepOcean {
				continue
			}
			p := geom.Position{X: x, Y: y}
			dep, ok := src.DepositAt(p)
			if ok {
				t.Fatalf("deep-ocean tile %+v unexpectedly has deposit %+v", p, dep)
			}
			return
		}
	}
	t.Skipf("no deep-ocean tile found in 200x200 window for seed %d", seed)
}

// TestNoiseDepositSource_DepositsIn_RectFilter asserts DepositsIn
// returns only deposits whose position lies strictly inside the
// half-open rect.
func TestNoiseDepositSource_DepositsIn_RectFilter(t *testing.T) {
	if testing.Short() {
		t.Skip("rect filter correctness sweep over deposit source")
	}
	const seed int64 = 42
	src := newDepositTestSource(t, seed)

	rect := geom.Rect{MinX: 10, MinY: 10, MaxX: 120, MaxY: 120}
	in := src.DepositsIn(rect)
	if len(in) == 0 {
		t.Fatalf("expected at least one deposit in rect %+v, got 0", rect)
	}
	for _, d := range in {
		if !rect.Contains(d.Position) {
			t.Errorf("deposit %+v outside rect %+v", d, rect)
		}
	}
}

// TestNoiseDepositSource_DepositsNear_Sorted asserts results are
// sorted by Chebyshev distance ascending with (X, Y) tiebreak.
func TestNoiseDepositSource_DepositsNear_Sorted(t *testing.T) {
	if testing.Short() {
		t.Skip("Chebyshev sort order verification with radius-40 query")
	}
	const seed int64 = 42
	src := newDepositTestSource(t, seed)

	// Pick a center tile that usually sits in-land for seed 42 so the
	// query radius hits several deposits.
	center := geom.Position{X: 64, Y: 64}
	near := src.DepositsNear(center, 40)
	if len(near) < 2 {
		t.Skipf("only %d deposits near %+v at seed %d", len(near), center, seed)
	}
	prevDist := -1
	for i, d := range near {
		dist := chebyshev(d.Position, center)
		if dist > 40 {
			t.Errorf("deposit %+v Chebyshev=%d exceeds radius 40", d.Position, dist)
		}
		if dist < prevDist {
			t.Errorf("at index %d: distance %d < prev %d (not sorted)", i, dist, prevDist)
		}
		prevDist = dist
	}
	// Verify (X, Y) tiebreak: for each distinct distance band, positions
	// inside the band should be sorted by X then Y.
	byDist := make(map[int][]geom.Position)
	for _, d := range near {
		dist := chebyshev(d.Position, center)
		byDist[dist] = append(byDist[dist], d.Position)
	}
	for dist, ps := range byDist {
		if !sort.SliceIsSorted(ps, func(i, j int) bool {
			if ps[i].X != ps[j].X {
				return ps[i].X < ps[j].X
			}
			return ps[i].Y < ps[j].Y
		}) {
			t.Errorf("positions at distance %d not (X,Y) sorted: %+v", dist, ps)
		}
	}
}

// TestNoiseDepositSource_DepositsNear_EmptyOnZeroRadius asserts
// radius 0 returns at most the single deposit on the centre tile.
func TestNoiseDepositSource_DepositsNear_EmptyOnZeroRadius(t *testing.T) {
	const seed int64 = 42
	src := newDepositTestSource(t, seed)

	// Scan for a tile that actually has a deposit.
	var center geom.Position
	found := false
	rect := geom.Rect{MinX: 0, MinY: 0, MaxX: 200, MaxY: 200}
	for _, d := range src.DepositsIn(rect) {
		center = d.Position
		found = true
		break
	}
	if !found {
		t.Skipf("no deposits found in 200x200 tile window for seed %d", seed)
	}

	near := src.DepositsNear(center, 0)
	if len(near) != 1 {
		t.Fatalf("radius 0 centred on %+v returned %d deposits, want 1", center, len(near))
	}
	if near[0].Position != center {
		t.Errorf("radius 0 deposit %+v != center %+v", near[0].Position, center)
	}
}

// TestNoiseDepositSource_DepositsNear_EmptyAway asserts a query far
// from any deposit returns an empty slice — ocean tiles at very large
// coords should have nothing within a few tiles.
func TestNoiseDepositSource_DepositsNear_EmptyAway(t *testing.T) {
	const seed int64 = 42
	src := newDepositTestSource(t, seed)
	wg := NewWorldGenerator(seed)

	// Find an ocean tile well away from any coast by scanning a
	// mid-size window.
	var center geom.Position
	found := false
	for y := 0; y < 300 && !found; y++ {
		for x := 0; x < 300 && !found; x++ {
			if wg.TileAt(x, y).Terrain != world.TerrainDeepOcean {
				continue
			}
			// Require a 5-tile buffer of deep ocean so the 3-tile
			// radius cannot touch beach.
			clear := true
			for dy := -5; dy <= 5 && clear; dy++ {
				for dx := -5; dx <= 5; dx++ {
					if wg.TileAt(x+dx, y+dy).Terrain != world.TerrainDeepOcean {
						clear = false
						break
					}
				}
			}
			if clear {
				center = geom.Position{X: x, Y: y}
				found = true
			}
		}
	}
	if !found {
		t.Skipf("no deep-ocean buffer found in 300x300 window for seed %d", seed)
	}
	near := src.DepositsNear(center, 3)
	if len(near) != 0 {
		t.Errorf("DepositsNear(%+v, 3) returned %d deposits, want 0 (deep ocean)", center, len(near))
	}
}

// TestNoiseDepositSource_ConcurrentRead hammers DepositsIn and
// DepositAt across many goroutines and asserts results match a
// reference computed up front. Run with -race to prove the sync.Map +
// sync.Once cache discipline holds.
func TestNoiseDepositSource_ConcurrentRead(t *testing.T) {
	if testing.Short() {
		t.Skip("8-goroutine concurrent read stress test")
	}
	const seed int64 = 42
	src := newDepositTestSource(t, seed)

	reference := collectDeposits(src, -4, -4, 4, 4)
	refByTile := make(map[geom.Position]world.Deposit, len(reference))
	for _, d := range reference {
		refByTile[d.Position] = d
	}

	const goroutines = 8
	const perGoroutine = 300

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				// Rect query
				x0 := (g*19 + i*7) % 200
				y0 := (g*13 + i*11) % 200
				rect := geom.Rect{MinX: x0, MinY: y0, MaxX: x0 + 32, MaxY: y0 + 32}
				in := src.DepositsIn(rect)
				for _, d := range in {
					if !rect.Contains(d.Position) {
						t.Errorf("g=%d i=%d: deposit %+v outside rect %+v", g, i, d, rect)
					}
				}
				// Tile lookup cross-check
				for p, want := range refByTile {
					if p.X < -256 || p.X >= 256 || p.Y < -256 || p.Y >= 256 {
						continue
					}
					got, ok := src.DepositAt(p)
					if !ok {
						t.Errorf("g=%d i=%d: DepositAt(%+v) miss", g, i, p)
						continue
					}
					if got != want {
						t.Errorf("g=%d i=%d: DepositAt(%+v) got=%+v want=%+v", g, i, p, got, want)
					}
					break // one lookup per iteration is enough
				}
			}
		}(g)
	}
	wg.Wait()
}

// TestNoiseDepositSource_DeterminismAcrossInstances asserts two
// independent sources with the same seed produce bit-identical
// deposit slices across a 4x4 SC window.
func TestNoiseDepositSource_DeterminismAcrossInstances(t *testing.T) {
	if testing.Short() {
		t.Skip("4x4 SC cross-instance determinism sweep")
	}
	const seed int64 = 42
	a := newDepositTestSource(t, seed)
	b := newDepositTestSource(t, seed)

	depA := collectDeposits(a, -4, -4, 4, 4)
	depB := collectDeposits(b, -4, -4, 4, 4)

	sortDeposits := func(in []world.Deposit) {
		sort.Slice(in, func(i, j int) bool {
			if in[i].Position.X != in[j].Position.X {
				return in[i].Position.X < in[j].Position.X
			}
			if in[i].Position.Y != in[j].Position.Y {
				return in[i].Position.Y < in[j].Position.Y
			}
			return in[i].Kind < in[j].Kind
		})
	}
	sortDeposits(depA)
	sortDeposits(depB)

	if !reflect.DeepEqual(depA, depB) {
		t.Fatalf("determinism broken: len a=%d b=%d", len(depA), len(depB))
	}
}

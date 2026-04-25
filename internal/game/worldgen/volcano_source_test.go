package worldgen

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)


// TestVolcanoSource_PlacesAtMountains exercises the full pipeline on a
// real Standard world: at least a handful of volcanoes get placed, and
// every anchor sits on a high-elevation mountain/peak/hills cell.
// Gated by -short because Generate(WorldSizeStandard) takes ~25s and
// dominates the CI fast loop.
func TestVolcanoSource_PlacesAtMountains(t *testing.T) {
	if testing.Short() {
		t.Skip("short: standard worldgen ~25s, gated for fast loop")
	}
	w := Generate(testSeed, WorldSizeStandard)
	src := NewVolcanoSource(w, testSeed)

	all := src.All()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 volcanoes on Standard, got %d", len(all))
	}

	for i, v := range all {
		cellID := w.Voronoi.CellIDAt(v.Anchor.X, v.Anchor.Y)
		if w.IsOcean(cellID) {
			t.Errorf("volcano %d: anchor (%d,%d) is in ocean cell %d",
				i, v.Anchor.X, v.Anchor.Y, cellID)
		}
		if got := w.Elevation[cellID]; got <= volcanoMinElevation {
			t.Errorf("volcano %d: anchor (%d,%d) elevation %.3f ≤ %.3f",
				i, v.Anchor.X, v.Anchor.Y, got, volcanoMinElevation)
		}
		switch w.Terrain[cellID] {
		case world.TerrainMountain, world.TerrainSnowyPeak, world.TerrainHills:
			// ok
		default:
			t.Errorf("volcano %d: anchor (%d,%d) on terrain %q, want mountain/peak/hills",
				i, v.Anchor.X, v.Anchor.Y, w.Terrain[cellID])
		}
		if len(v.CoreTiles) == 0 {
			t.Errorf("volcano %d: empty CoreTiles", i)
		}
	}
}

// TestVolcanoSource_TerrainOverrides verifies the zone-to-terrain
// mapping at a few sample offsets per volcano.
// Standard-world gen is heavy so this test is also -short gated.
func TestVolcanoSource_TerrainOverrides(t *testing.T) {
	if testing.Short() {
		t.Skip("short: standard worldgen ~25s, gated for fast loop")
	}
	w := Generate(testSeed, WorldSizeStandard)
	src := NewVolcanoSource(w, testSeed)

	all := src.All()
	if len(all) == 0 {
		t.Fatalf("no volcanoes placed; cannot exercise overrides")
	}

	for i, v := range all {
		// Anchor must always resolve to the state-appropriate core.
		got, ok := src.TerrainOverrideAt(v.Anchor)
		if !ok {
			t.Errorf("volcano %d: anchor not in override map", i)
			continue
		}
		want := coreTerrain(v.State)
		if got != want {
			t.Errorf("volcano %d: anchor terrain %q, want %q (state=%s)",
				i, got, want, v.State)
		}

		// Probe a tile inside the slope ring (offset 3 east). May be
		// dropped by the ocean filter on coastal volcanoes — assert
		// only when the tile actually exists in the override map.
		slopeProbe := v.Anchor.Add(3, 0)
		if terr, ok := src.TerrainOverrideAt(slopeProbe); ok {
			// Could be slope (within radius 5) or ashland (if a
			// neighbouring volcano's ring overlaps). Both are valid
			// volcanic terrains; assert that we're not getting a non-
			// volcanic terrain back through the override.
			if !isVolcanicTerrain(terr) {
				t.Errorf("volcano %d: slope probe returned %q (non-volcanic)", i, terr)
			}
		}

		// Far outside any ring — must miss.
		far := v.Anchor.Add(100, 100)
		if terr, ok := src.TerrainOverrideAt(far); ok {
			t.Errorf("volcano %d: distant tile %v unexpectedly returned %q",
				i, far, terr)
		}
	}
}

// TestVolcanoSource_Determinism verifies two builds against the same
// seed produce the same volcano list (anchors, states, tile counts).
// Heavy because we run Generate twice; gated by -short.
func TestVolcanoSource_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("short: two standard worldgens ~50s, gated for fast loop")
	}
	w1 := Generate(testSeed, WorldSizeStandard)
	w2 := Generate(testSeed, WorldSizeStandard)
	a := NewVolcanoSource(w1, testSeed).All()
	b := NewVolcanoSource(w2, testSeed).All()
	if len(a) != len(b) {
		t.Fatalf("non-deterministic count: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if !a[i].Anchor.Equal(b[i].Anchor) {
			t.Errorf("anchor[%d] mismatch: %v vs %v", i, a[i].Anchor, b[i].Anchor)
		}
		if a[i].State != b[i].State {
			t.Errorf("state[%d] mismatch: %s vs %s", i, a[i].State, b[i].State)
		}
		if !reflect.DeepEqual(a[i].CoreTiles, b[i].CoreTiles) {
			t.Errorf("core[%d] mismatch", i)
		}
		if !reflect.DeepEqual(a[i].SlopeTiles, b[i].SlopeTiles) {
			t.Errorf("slope[%d] mismatch", i)
		}
		if !reflect.DeepEqual(a[i].AshlandTiles, b[i].AshlandTiles) {
			t.Errorf("ashland[%d] mismatch", i)
		}
	}
}

// TestVolcanoSource_TinyWorld is the cheap sanity test — runs in a few
// hundred ms even without -short, exercises the wiring end-to-end on
// a small world without depending on the full Standard pipeline.
func TestVolcanoSource_TinyWorld(t *testing.T) {
	w := Generate(testSeed, WorldSizeTiny)
	src := NewVolcanoSource(w, testSeed)

	// Tiny is small; volcanoes may not be placed if the world has no
	// high-elevation cells. Just verify the source is well-formed.
	all := src.All()
	for i, v := range all {
		if len(v.CoreTiles) == 0 {
			t.Errorf("volcano %d: empty core", i)
		}
		if v.State == world.VolcanoStateUnknown {
			t.Errorf("volcano %d: unknown state", i)
		}
		// Anchor must be inside the world bounds.
		if v.Anchor.X < 0 || v.Anchor.Y < 0 ||
			v.Anchor.X >= w.Width || v.Anchor.Y >= w.Height {
			t.Errorf("volcano %d: anchor %v out of bounds", i, v.Anchor)
		}
	}

	// A tile far from any anchor must miss the override.
	far := geom.Position{X: -100, Y: -100}
	if terr, ok := src.TerrainOverrideAt(far); ok {
		t.Errorf("far tile %v unexpectedly returned %q", far, terr)
	}
}

// TestVolcanoSource_NilWorldDefensive verifies the constructor returns
// an empty (but valid) source when handed nothing useful — no panics on
// VolcanoAt or TerrainOverrideAt against the empty result.
func TestVolcanoSource_NilWorldDefensive(t *testing.T) {
	src := NewVolcanoSource(nil, 1)
	if got := src.VolcanoAt(geom.SuperChunkCoord{X: 0, Y: 0}); got != nil {
		t.Errorf("nil world: got %d volcanoes, want 0", len(got))
	}
	if terr, ok := src.TerrainOverrideAt(geom.Position{X: 5, Y: 5}); ok {
		t.Errorf("nil world: got terrain %q, want miss", terr)
	}
}

// TestVolcanoSource_VolcanoAtIndex verifies the per-super-chunk index
// matches the anchor list — every placed volcano must be retrievable
// through VolcanoAt with its anchor's super-chunk coord.
func TestVolcanoSource_VolcanoAtIndex(t *testing.T) {
	w := Generate(testSeed, WorldSizeSmall)
	src := NewVolcanoSource(w, testSeed)

	for i, v := range src.All() {
		sc := geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y)
		got := src.VolcanoAt(sc)
		found := false
		for _, c := range got {
			if c.Anchor.Equal(v.Anchor) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("volcano %d (anchor %v) not in VolcanoAt(%v)",
				i, v.Anchor, sc)
		}
	}
}

// TestRollState_StateDistribution sanity-checks rollState's weighted
// roll over a synthetic anchor sweep. Cheap, no worldgen needed.
func TestRollState_StateDistribution(t *testing.T) {
	const samples = 1000
	counts := map[world.VolcanoState]int{}
	for i := 0; i < samples; i++ {
		s := rollState(testSeed, geom.Position{X: i, Y: i * 7})
		counts[s]++
	}
	// Loose bounds — 30/50/20 split with ±10pp tolerance for n=1000.
	checks := []struct {
		state    world.VolcanoState
		minPct   int
		maxPct   int
		stateKey string
	}{
		{world.VolcanoActive, 20, 40, "active"},
		{world.VolcanoDormant, 40, 60, "dormant"},
		{world.VolcanoExtinct, 10, 30, "extinct"},
	}
	for _, c := range checks {
		pct := counts[c.state] * 100 / samples
		if pct < c.minPct || pct > c.maxPct {
			t.Errorf("state %s: %d%% (%d/%d), want [%d,%d]",
				c.stateKey, pct, counts[c.state], samples, c.minPct, c.maxPct)
		}
	}
	if counts[world.VolcanoStateUnknown] != 0 {
		t.Errorf("unknown state slipped through: %d", counts[world.VolcanoStateUnknown])
	}
}

// TestPackPos verifies the (x,y) → uint64 packing is collision-free
// for a sweep of small coords including negatives. Trivial but cheap.
func TestPackPos(t *testing.T) {
	seen := map[uint64]geom.Position{}
	for y := -8; y <= 8; y++ {
		for x := -8; x <= 8; x++ {
			p := geom.Position{X: x, Y: y}
			k := geom.PackPos(p)
			if other, dup := seen[k]; dup {
				t.Fatalf("collision: %v and %v both pack to %#x", p, other, k)
			}
			seen[k] = p
		}
	}
}

// TestPickWithSpacing covers the greedy-spacing helper: respects min
// distance, stops at budget, returns empty for budget ≤ 0.
func TestPickWithSpacing(t *testing.T) {
	cands := []geom.Position{
		{X: 0, Y: 0},
		{X: 5, Y: 0},   // too close to (0,0) under minDist=10
		{X: 20, Y: 0},  // ok
		{X: 22, Y: 0},  // too close to (20,0)
		{X: 50, Y: 50}, // ok
	}
	got := pickWithSpacing(cands, 10, 10)
	want := []geom.Position{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 50, Y: 50}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	if got := pickWithSpacing(cands, 0, 10); got != nil {
		t.Errorf("budget=0: got %v, want nil", got)
	}

	got = pickWithSpacing(cands, 1, 10)
	if len(got) != 1 || !got[0].Equal(geom.Position{X: 0, Y: 0}) {
		t.Errorf("budget=1: got %v, want [(0,0)]", got)
	}
}

// isVolcanicTerrain returns true for any terrain produced by the
// volcano source's override path. Helper for the override probe test.
func isVolcanicTerrain(t world.Terrain) bool {
	switch t {
	case world.TerrainVolcanoCore,
		world.TerrainVolcanoCoreDormant,
		world.TerrainCraterLake,
		world.TerrainVolcanoSlope,
		world.TerrainAshland:
		return true
	default:
		return false
	}
}

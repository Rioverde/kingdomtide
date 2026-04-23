package worldgen

import (
	"math"
	"testing"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// newVolcanicDepositTestSource wires a fresh NoiseDepositSource with a
// real volcano source so obsidian / sulfur placement exercises the
// production data flow. Mirrors newDepositTestSource but with the
// volcano arm connected — used by every test in this file.
func newVolcanicDepositTestSource(tb testing.TB, seed int64) (*NoiseDepositSource, *NoiseVolcanoSource) {
	tb.Helper()
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	return NewNoiseDepositSource(seed, wg, lm, vs), vs
}

// collectSlopeTiles returns every slope tile in the volcano window
// alongside the owning volcano. Used by invariant tests that need a
// one-to-many lookup from slope to its containing volcano.
func collectSlopeTiles(vs *NoiseVolcanoSource, minSCX, minSCY, maxSCX, maxSCY int) map[geom.Position]world.Volcano {
	out := make(map[geom.Position]world.Volcano)
	for x := minSCX; x < maxSCX; x++ {
		for y := minSCY; y < maxSCY; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					out[p] = v
				}
			}
		}
	}
	return out
}

// volcanoByState collects the first volcano observed per lifecycle
// state inside the window. Returns a map keyed by VolcanoState so
// individual tests can pick "give me an active one" without walking
// the full set again.
func volcanoByState(vs *NoiseVolcanoSource, minSCX, minSCY, maxSCX, maxSCY int) map[world.VolcanoState]world.Volcano {
	out := make(map[world.VolcanoState]world.Volcano)
	for x := minSCX; x < maxSCX; x++ {
		for y := minSCY; y < maxSCY; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				if _, seen := out[v.State]; !seen {
					out[v.State] = v
				}
			}
		}
	}
	return out
}

// volcanoScanWindow defines the super-chunk scan window used by every
// test in this file. 12x12 SC = 9 super-regions at the 4x4 SC
// super-region granularity — large enough to catch volcanoes of every
// lifecycle state at seed 42, small enough that DepositsIn returns in
// a few seconds even under -race.
const (
	volcanoScanMinSC = -6
	volcanoScanMaxSC = 6
)

// TestObsidianDepositAt_OnlyOnSlope asserts every obsidian produced
// via the direct helper sits on a volcano slope tile. The direct
// helper (not DepositsIn) keeps the test off the expensive full-SR
// generation path so -race fits inside the default test timeout.
func TestObsidianDepositAt_OnlyOnSlope(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC obsidian slope-only placement sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	slopes := collectSlopeTiles(vs, volcanoScanMinSC, volcanoScanMinSC, volcanoScanMaxSC, volcanoScanMaxSC)
	if len(slopes) == 0 {
		t.Fatalf("seed %d produced no slope tiles in the scan window", seed)
	}

	found := 0
	for p := range slopes {
		dep, ok := obsidianDepositAt(seed, p, vs)
		if !ok {
			continue
		}
		found++
		if dep.Kind != world.DepositObsidian {
			t.Errorf("obsidian helper returned non-obsidian kind %q at %+v", dep.Kind, p)
		}
		if dep.Position != p {
			t.Errorf("obsidian at %+v returned with Position %+v", p, dep.Position)
		}
	}
	if found == 0 {
		t.Fatalf("seed %d produced zero obsidian deposits across %d slope tiles", seed, len(slopes))
	}
	t.Logf("seed %d obsidian count: %d / slope tiles: %d", seed, found, len(slopes))
}

// TestObsidianDepositAt_NoCoreOrAshland asserts the obsidian helper
// rejects core and ashland tiles of every volcano in the window.
// Exercises the zone gate in obsidianDepositAt directly.
func TestObsidianDepositAt_NoCoreOrAshland(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC obsidian core/ashland rejection sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	for x := volcanoScanMinSC; x < volcanoScanMaxSC; x++ {
		for y := volcanoScanMinSC; y < volcanoScanMaxSC; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.CoreTiles {
					if _, ok := obsidianDepositAt(seed, p, vs); ok {
						t.Errorf("obsidian on core tile %+v (anchor %+v)", p, v.Anchor)
					}
				}
				for _, p := range v.AshlandTiles {
					if _, ok := obsidianDepositAt(seed, p, vs); ok {
						t.Errorf("obsidian on ashland tile %+v (anchor %+v)", p, v.Anchor)
					}
				}
			}
		}
	}
}

// TestObsidianDepositAt_Density samples slope tiles across the scan
// window and asserts the observed obsidian fraction sits within ±10%
// of obsidianDensityFraction. A wide tolerance keeps the test robust
// against small sample sizes — the invariant is "the hash gate is
// applied correctly," not "the fraction is exactly 0.7."
func TestObsidianDepositAt_Density(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC obsidian density fraction sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	var total, hits int
	for x := volcanoScanMinSC; x < volcanoScanMaxSC; x++ {
		for y := volcanoScanMinSC; y < volcanoScanMaxSC; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					total++
					if _, ok := obsidianDepositAt(seed, p, vs); ok {
						hits++
					}
				}
			}
		}
	}
	if total < 100 {
		t.Skipf("seed %d produced only %d slope tiles, not enough for density probe", seed, total)
	}
	frac := float64(hits) / float64(total)
	tol := 0.10
	if math.Abs(frac-obsidianDensityFraction) > tol {
		t.Errorf("obsidian density = %.3f over %d slope tiles, want %.3f ± %.2f",
			frac, total, obsidianDensityFraction, tol)
	}
	t.Logf("seed %d slope=%d obsidian_hits=%d frac=%.3f target=%.3f", seed, total, hits, frac, obsidianDensityFraction)
}

// TestObsidianDepositAt_StateIndependent asserts obsidian density on
// active-volcano slopes is comparable to density on extinct-volcano
// slopes. State independence is a documented behaviour — an extinct
// volcano's slope carries obsidian from historical flows even though
// its core healed to a crater lake. Uses a widened scan window so a
// density comparison is meaningful; volcano enumeration is cheap, the
// cost is per-volcano map allocation in slopeAdjacentToCore which
// obsidianDepositAt does not call.
func TestObsidianDepositAt_StateIndependent(t *testing.T) {
	if testing.Short() {
		t.Skip("24x24 SC obsidian state-independence sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	const (
		wideMin = -12
		wideMax = 12
	)
	var activeTotal, activeHits, extinctTotal, extinctHits int
	for x := wideMin; x < wideMax; x++ {
		for y := wideMin; y < wideMax; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					switch v.State {
					case world.VolcanoActive:
						activeTotal++
						if _, ok := obsidianDepositAt(seed, p, vs); ok {
							activeHits++
						}
					case world.VolcanoExtinct:
						extinctTotal++
						if _, ok := obsidianDepositAt(seed, p, vs); ok {
							extinctHits++
						}
					}
				}
			}
		}
	}
	if activeTotal < 40 || extinctTotal < 40 {
		t.Skipf("not enough slope samples per state (active=%d extinct=%d)", activeTotal, extinctTotal)
	}
	fracActive := float64(activeHits) / float64(activeTotal)
	fracExtinct := float64(extinctHits) / float64(extinctTotal)
	if math.Abs(fracActive-fracExtinct) > 0.10 {
		t.Errorf("obsidian density differs by state: active=%.3f extinct=%.3f (delta > 0.10)",
			fracActive, fracExtinct)
	}
	t.Logf("seed %d active_frac=%.3f (n=%d) extinct_frac=%.3f (n=%d)",
		seed, fracActive, activeTotal, fracExtinct, extinctTotal)
}

// TestSulfurDepositAt_OnlyCoreAdjacent asserts every sulfur deposit
// the direct helper produces sits on a slope tile with at least one
// 4-neighbour in the containing volcano's core. Uses the direct
// helper on enumerated slope tiles to avoid the expensive DepositsIn
// generation path under -race.
func TestSulfurDepositAt_OnlyCoreAdjacent(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC sulfur core-adjacency invariant sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	found := 0
	for x := volcanoScanMinSC; x < volcanoScanMaxSC; x++ {
		for y := volcanoScanMinSC; y < volcanoScanMaxSC; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					dep, ok := sulfurDepositAt(seed, p, vs)
					if !ok {
						continue
					}
					found++
					if dep.Kind != world.DepositSulfur {
						t.Errorf("sulfur helper returned non-sulfur kind %q at %+v", dep.Kind, p)
					}
					if !slopeAdjacentToCore(p, v) {
						t.Errorf("sulfur at %+v is on slope but not 4-adjacent to any core tile of volcano anchored at %+v",
							p, v.Anchor)
					}
				}
			}
		}
	}
	if found == 0 {
		t.Logf("seed %d produced zero sulfur deposits in the scan window (acceptable — depends on active/dormant volcano distribution)", seed)
	}
}

// TestSulfurDepositAt_StateDependentDensity probes the density gate
// per lifecycle state: Active should accept every core-adjacent slope
// tile, Dormant should accept roughly sulfurDormantFraction, Extinct
// should reject wholesale.
func TestSulfurDepositAt_StateDependentDensity(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC sulfur per-state density sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	type bucket struct{ total, hits int }
	counts := map[world.VolcanoState]*bucket{
		world.VolcanoActive:  {},
		world.VolcanoDormant: {},
		world.VolcanoExtinct: {},
	}
	for x := volcanoScanMinSC; x < volcanoScanMaxSC; x++ {
		for y := volcanoScanMinSC; y < volcanoScanMaxSC; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					if !slopeAdjacentToCore(p, v) {
						continue
					}
					b, ok := counts[v.State]
					if !ok {
						continue
					}
					b.total++
					if _, hit := sulfurDepositAt(seed, p, vs); hit {
						b.hits++
					}
				}
			}
		}
	}
	check := func(state world.VolcanoState, want, tol float64) {
		b := counts[state]
		if b.total < 10 {
			t.Logf("state=%s: only %d core-adjacent slope samples, skipping density check", state, b.total)
			return
		}
		frac := float64(b.hits) / float64(b.total)
		t.Logf("state=%s core-adjacent slope n=%d hits=%d frac=%.3f", state, b.total, b.hits, frac)
		if math.Abs(frac-want) > tol {
			t.Errorf("state=%s sulfur density = %.3f, want %.3f ± %.2f (n=%d)", state, frac, want, tol, b.total)
		}
	}
	check(world.VolcanoActive, 1.0, 0.01)
	check(world.VolcanoDormant, sulfurDormantFraction, 0.15)
	check(world.VolcanoExtinct, 0.0, 0.01)
}

// TestSulfurDepositAt_NilVolcanoSource_ReturnsFalse asserts sulfur
// placement degrades cleanly when no volcano source is wired — the
// caller gets a not-found rather than a panic or a stale deposit.
// The obsidian helper is checked in the same test since it shares
// the identical nil-guard.
func TestSulfurDepositAt_NilVolcanoSource_ReturnsFalse(t *testing.T) {
	const seed int64 = 42
	p := geom.Position{X: 1, Y: 2}
	if dep, ok := sulfurDepositAt(seed, p, nil); ok {
		t.Errorf("sulfurDepositAt(nil vs) returned deposit %+v, want not-found", dep)
	}
	if dep, ok := obsidianDepositAt(seed, p, nil); ok {
		t.Errorf("obsidianDepositAt(nil vs) returned deposit %+v, want not-found", dep)
	}
}

// TestSulfurDepositAt_NotOnNonAdjacentSlope picks a slope tile that
// is not 4-adjacent to any core tile of its volcano, then asserts
// sulfur is rejected regardless of state.
func TestSulfurDepositAt_NotOnNonAdjacentSlope(t *testing.T) {
	if testing.Short() {
		t.Skip("12x12 SC non-adjacent slope rejection sweep")
	}
	const seed int64 = 42
	_, vs := newVolcanicDepositTestSource(t, seed)

	found := false
	for x := volcanoScanMinSC; x < volcanoScanMaxSC && !found; x++ {
		for y := volcanoScanMinSC; y < volcanoScanMaxSC && !found; y++ {
			for _, v := range vs.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					if slopeAdjacentToCore(p, v) {
						continue
					}
					if _, ok := sulfurDepositAt(seed, p, vs); ok {
						t.Errorf("sulfur produced on non-core-adjacent slope tile %+v (volcano anchor %+v, state=%s)",
							p, v.Anchor, v.State)
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
	if !found {
		t.Skipf("seed %d: no non-core-adjacent slope tile found in window — every slope tile on every volcano borders a core tile at this density", seed)
	}
}

// TestNoiseDepositSource_VolcanicIntegration exercises the full
// pipeline end-to-end through DepositAt on known slope tiles:
// generate deposits for the window, tally obsidian and sulfur per
// probed volcano state, and verify no sulfur is attached to an
// extinct volcano's slopes. Scans one representative volcano per
// state to keep the -race runtime bounded.
func TestNoiseDepositSource_VolcanicIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("end-to-end volcanic deposit pipeline integration sweep")
	}
	const seed int64 = 42
	ds, vs := newVolcanicDepositTestSource(t, seed)

	byState := volcanoByState(vs, volcanoScanMinSC, volcanoScanMinSC, volcanoScanMaxSC, volcanoScanMaxSC)
	if len(byState) == 0 {
		t.Fatalf("seed %d produced no volcanoes in the scan window", seed)
	}

	// Tally deposits by kind via DepositAt so the cached path is the
	// one under test. Probing a single tile per volcano forces the
	// owning super-region through the generation pipeline once; the
	// remaining slope-tile lookups hit the cache.
	var obsidianHits, sulfurActiveHits, sulfurDormantHits, sulfurExtinctHits int
	for state, v := range byState {
		if len(v.SlopeTiles) == 0 {
			continue
		}
		for _, p := range v.SlopeTiles {
			d, ok := ds.DepositAt(p)
			if !ok {
				continue
			}
			switch d.Kind {
			case world.DepositObsidian:
				obsidianHits++
			case world.DepositSulfur:
				switch state {
				case world.VolcanoActive:
					sulfurActiveHits++
				case world.VolcanoDormant:
					sulfurDormantHits++
				case world.VolcanoExtinct:
					sulfurExtinctHits++
				}
			}
		}
	}
	if obsidianHits == 0 {
		t.Errorf("no obsidian deposits observed on any probed volcano's slope")
	}
	if sulfurExtinctHits != 0 {
		t.Errorf("sulfur observed on extinct volcano slope: got %d, want 0", sulfurExtinctHits)
	}
	// Active / dormant sulfur is conditional on the window actually
	// containing such a volcano with a core-adjacent slope; log the
	// counts so density regressions surface without flaking the test.
	t.Logf("seed %d integration: obsidian=%d sulfur_active=%d sulfur_dormant=%d sulfur_extinct=%d",
		seed, obsidianHits, sulfurActiveHits, sulfurDormantHits, sulfurExtinctHits)
}

// TestNoiseDepositSource_VolcanicDeterminism asserts two independent
// sources built from the same seed produce the same volcanic deposit
// set on a narrow probe window. Catches regressions where placement
// starts depending on iteration order of a map or another non-
// deterministic source. Uses a tiny 4x4 SC window so the test
// completes in a few seconds under -race.
func TestNoiseDepositSource_VolcanicDeterminism(t *testing.T) {
	const seed int64 = 42
	dsA, vsA := newVolcanicDepositTestSource(t, seed)
	dsB, _ := newVolcanicDepositTestSource(t, seed)

	// Probe volcano slope tiles directly — cheap and sufficient to
	// verify the volcanic layer is deterministic. Avoids DepositsIn's
	// per-tile zonal + point-like pass on millions of tiles.
	count := 0
	for x := -2; x < 2; x++ {
		for y := -2; y < 2; y++ {
			for _, v := range vsA.VolcanoAt(geom.SuperChunkCoord{X: x, Y: y}) {
				for _, p := range v.SlopeTiles {
					a, aok := dsA.DepositAt(p)
					b, bok := dsB.DepositAt(p)
					if aok != bok || a != b {
						t.Errorf("deterministic mismatch at %+v: a=(%+v,%v) b=(%+v,%v)",
							p, a, aok, b, bok)
					}
					count++
				}
			}
		}
	}
	if count == 0 {
		t.Skipf("seed %d: no volcano slope tiles in probe window", seed)
	}
}

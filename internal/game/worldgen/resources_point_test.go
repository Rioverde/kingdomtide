package worldgen

import (
	"reflect"
	"sort"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// newPointTestSource wires a deposit source with real landmark and
// volcano sources so the point-like pass exercises the full collision
// gate. Reused across point-specific tests to keep wiring identical.
func newPointTestSource(tb testing.TB, seed int64) *NoiseDepositSource {
	tb.Helper()
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	return NewNoiseDepositSource(seed, wg, lm, vs)
}

// collectPointDeposits walks a 10x10 SC block and returns only the
// point-like deposits. Filter keeps tests insensitive to zonal and fish
// counts so a fix to a zonal threshold does not cascade into point
// assertions.
func collectPointDeposits(src *NoiseDepositSource, minSCX, minSCY, maxSCX, maxSCY int) []game.Deposit {
	all := collectDeposits(src, minSCX, minSCY, maxSCX, maxSCY)
	out := make([]game.Deposit, 0, len(all))
	for _, d := range all {
		if _, ok := pointMinDistance[d.Kind]; ok {
			out = append(out, d)
		}
	}
	return out
}

// TestPointDeposits_Determinism builds two independent sources with the
// same seed and asserts the point-like deposits across a 4x4 SC window
// are bit-identical. The sort inside pointDepositsInRegion is the
// canonical iteration order so DeepEqual on the collected slice is a
// sound determinism check.
func TestPointDeposits_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("4x4 SC point deposit determinism sweep")
	}
	const seed int64 = 42
	a := newPointTestSource(t, seed)
	b := newPointTestSource(t, seed)

	depA := collectPointDeposits(a, -4, -4, 4, 4)
	depB := collectPointDeposits(b, -4, -4, 4, 4)

	// DepositsIn yields per-super-region, within each SR in generation
	// order. Two sources pass through the same generation pass so the
	// slices align without re-sorting.
	if !reflect.DeepEqual(depA, depB) {
		t.Fatalf("determinism broken: len a=%d b=%d", len(depA), len(depB))
	}
}

// TestPointDeposits_BiomeGate asserts every placed point deposit sits
// on a tile whose terrain passes pointBiomeAccepts for that kind. A
// placement on the wrong biome signals a broken gate or a regression
// in tileBlocked.
func TestPointDeposits_BiomeGate(t *testing.T) {
	if testing.Short() {
		t.Skip("4x4 SC point biome gate verification")
	}
	const seed int64 = 42
	src := newPointTestSource(t, seed)
	wg := NewWorldGenerator(seed)

	deposits := collectPointDeposits(src, -4, -4, 4, 4)
	if len(deposits) == 0 {
		t.Fatalf("seed %d produced no point deposits in the window", seed)
	}
	for _, d := range deposits {
		tile := wg.TileAt(d.Position.X, d.Position.Y)
		if !pointBiomeAccepts(d.Kind, tile.Terrain) {
			t.Errorf(
				"deposit %s at %+v on terrain %q rejected by biome gate",
				d.Kind, d.Position, tile.Terrain,
			)
		}
	}
}

// TestPointDeposits_MinSpacingPerKind asserts that same-kind deposits
// within a single super-region are at least pointMinDistance[kind]
// apart. Cross-super-region pairs use independent Poisson streams so
// the check is per-SR.
func TestPointDeposits_MinSpacingPerKind(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-SR Poisson min-distance spacing sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)

	for _, sr := range []superRegion{{0, 0}, {1, 0}, {0, 1}, {1, 1}, {-1, 0}} {
		deposits := pointDepositsInRegion(seed, sr, wg, lm, vs)
		byKind := make(map[game.DepositKind][]game.Position)
		for _, d := range deposits {
			byKind[d.Kind] = append(byKind[d.Kind], d.Position)
		}
		for kind, positions := range byKind {
			minDist := pointMinDistance[kind]
			for i := 0; i < len(positions); i++ {
				for j := i + 1; j < len(positions); j++ {
					dx := positions[i].X - positions[j].X
					dy := positions[i].Y - positions[j].Y
					distSq := dx*dx + dy*dy
					if distSq < minDist*minDist {
						t.Errorf(
							"sr=%+v kind=%s: pair %+v and %+v are closer than min %d (dist^2=%d)",
							sr, kind, positions[i], positions[j], minDist, distSq,
						)
					}
				}
			}
		}
	}
}

// TestPointDeposits_LandmarkRejection asserts no point deposit lands on
// a landmark tile. Walks the landmarks in a 4x4 SC block, asserts each
// landmark coord is absent from the point-deposit byTile index.
func TestPointDeposits_LandmarkRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("4x4 SC landmark collision sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	src := NewNoiseDepositSource(seed, wg, lm, vs)

	points := make(map[game.Position]game.Deposit)
	for _, d := range collectPointDeposits(src, -4, -4, 4, 4) {
		points[d.Position] = d
	}

	landmarkCount := 0
	for scY := -4; scY < 4; scY++ {
		for scX := -4; scX < 4; scX++ {
			for _, l := range lm.LandmarksIn(game.SuperChunkCoord{X: scX, Y: scY}) {
				landmarkCount++
				if dep, collide := points[l.Coord]; collide {
					t.Errorf(
						"point deposit %s collided with landmark %s at %+v",
						dep.Kind, l.Kind, l.Coord,
					)
				}
			}
		}
	}
	if landmarkCount == 0 {
		t.Skipf("no landmarks in 8x8 SC window for seed %d", seed)
	}
}

// TestPointDeposits_VolcanoRejection asserts no point deposit lands
// on a volcano footprint tile — core, slope, or ashland. Iterates every
// volcano in the window and checks each zone tile against the point-
// deposit byTile index.
func TestPointDeposits_VolcanoRejection(t *testing.T) {
	if testing.Short() {
		t.Skip("4x4 SC volcano footprint collision sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)
	src := NewNoiseDepositSource(seed, wg, lm, vs)

	points := make(map[game.Position]game.Deposit)
	for _, d := range collectPointDeposits(src, -4, -4, 4, 4) {
		points[d.Position] = d
	}

	volcanoCount := 0
	for scY := -4; scY < 4; scY++ {
		for scX := -4; scX < 4; scX++ {
			for _, v := range vs.VolcanoAt(game.SuperChunkCoord{X: scX, Y: scY}) {
				volcanoCount++
				check := func(zone string, tiles []game.Position) {
					for _, p := range tiles {
						if dep, collide := points[p]; collide {
							t.Errorf(
								"point deposit %s at %+v sits on volcano %s tile",
								dep.Kind, p, zone,
							)
						}
					}
				}
				check("core", v.CoreTiles)
				check("slope", v.SlopeTiles)
				check("ashland", v.AshlandTiles)
			}
		}
	}
	if volcanoCount == 0 {
		t.Skipf("no volcanoes in 8x8 SC window for seed %d", seed)
	}
}

// TestPointDeposits_KindRaritySortOrder asserts that
// pointDepositsInRegion output is ordered kind-ordinal ascending with
// (X, Y) lex tiebreak — the contract sortPointDeposits encodes. The
// test runs against one SR so the assertion is local: collectDeposits
// runs per-SR and concatenates, so cross-SR ordering is a different
// contract.
func TestPointDeposits_KindRaritySortOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("single-SR point deposit sort order check")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	regions := NewNoiseRegionSource(seed)
	lm := NewNoiseLandmarkSource(seed, regions, wg)
	vs := NewNoiseVolcanoSource(seed, wg, lm)

	deposits := pointDepositsInRegion(seed, superRegion{X: 0, Y: 0}, wg, lm, vs)
	if len(deposits) < 2 {
		t.Skipf("only %d point deposits in sr{0,0} — need at least 2 to test order", len(deposits))
	}
	ok := sort.SliceIsSorted(deposits, func(i, j int) bool {
		if deposits[i].Kind != deposits[j].Kind {
			return deposits[i].Kind < deposits[j].Kind
		}
		if deposits[i].Position.X != deposits[j].Position.X {
			return deposits[i].Position.X < deposits[j].Position.X
		}
		return deposits[i].Position.Y < deposits[j].Position.Y
	})
	if !ok {
		t.Errorf("pointDepositsInRegion output not sorted: %+v", deposits)
	}
}

// TestPointDeposits_RareSparseAbsence samples many seed / SR pairs and
// asserts Gems counts stay well below Stone counts (common kinds
// dominate). Some pairs are allowed to contain zero Gems — rare is
// rare. The exact threshold comes from the plan's "rare-and-sometimes-
// absent" semantics.
func TestPointDeposits_RareSparseAbsence(t *testing.T) {
	if testing.Short() {
		t.Skip("6-seed 4x4 SR rarity sparseness sweep")
	}
	const (
		seedCount = 6
		srCount   = 4
	)
	stoneTotal, gemsTotal := 0, 0
	gemsZeroCount := 0
	pairCount := 0

	for seedOffset := int64(0); seedOffset < seedCount; seedOffset++ {
		seed := int64(100) + seedOffset
		wg := NewWorldGenerator(seed)
		regions := NewNoiseRegionSource(seed)
		lm := NewNoiseLandmarkSource(seed, regions, wg)
		vs := NewNoiseVolcanoSource(seed, wg, lm)
		for sy := 0; sy < srCount; sy++ {
			for sx := 0; sx < srCount; sx++ {
				deposits := pointDepositsInRegion(seed, superRegion{X: sx, Y: sy}, wg, lm, vs)
				stone := 0
				gems := 0
				for _, d := range deposits {
					switch d.Kind {
					case game.DepositStone:
						stone++
					case game.DepositGems:
						gems++
					}
				}
				stoneTotal += stone
				gemsTotal += gems
				if gems == 0 {
					gemsZeroCount++
				}
				pairCount++
			}
		}
	}
	if pairCount == 0 {
		t.Fatalf("no seed / sr pairs sampled")
	}
	if stoneTotal <= gemsTotal {
		t.Errorf("expected stone dominance: stone=%d gems=%d", stoneTotal, gemsTotal)
	}
	// At least one pair should produce zero Gems — the "sometimes
	// absent" semantic. Without this, tuning has drifted too dense.
	if gemsZeroCount == 0 {
		t.Errorf("expected at least one (seed, sr) pair with zero Gems, got 0 across %d pairs", pairCount)
	}
	// And Gems must not be suspiciously common — keep at most 6x Stone
	// density parity (generous headroom, plan wants order of magnitude
	// sparser).
	if gemsTotal*6 > stoneTotal && stoneTotal > 0 {
		t.Errorf("Gems too dense: stone=%d gems=%d", stoneTotal, gemsTotal)
	}
}

// TestTileBlocked_WaterRejects walks a window looking for an ocean
// tile and asserts tileBlocked returns true. Runs against a real
// WorldGenerator so the check covers the live terrain mix rather than
// a stub.
func TestTileBlocked_WaterRejects(t *testing.T) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)
	for y := 0; y < 200; y++ {
		for x := 0; x < 200; x++ {
			tile := wg.TileAt(x, y)
			if tile.Terrain != game.TerrainOcean && tile.Terrain != game.TerrainDeepOcean {
				continue
			}
			p := game.Position{X: x, Y: y}
			if !tileBlocked(p, wg, nil, nil) {
				t.Fatalf("ocean tile %+v terrain=%q was not blocked", p, tile.Terrain)
			}
			return
		}
	}
	t.Skipf("no ocean tile in 200x200 window for seed %d", seed)
}

// TestTileBlocked_NilLandmarkAndVolcano asserts that tileBlocked with
// nil lm and vs only gates on water / rivers. A known land tile must
// pass; a known ocean tile must fail. Guards against a future refactor
// that forgets the nil check.
func TestTileBlocked_NilLandmarkAndVolcano(t *testing.T) {
	const seed int64 = 42
	wg := NewWorldGenerator(seed)

	// Find a mountain / hills / plains / desert tile — any non-water
	// land tile — and a water tile, then probe both.
	var land, water game.Position
	landFound, waterFound := false, false
	for y := 0; y < 400 && (!landFound || !waterFound); y++ {
		for x := 0; x < 400; x++ {
			tile := wg.TileAt(x, y)
			p := game.Position{X: x, Y: y}
			switch tile.Terrain {
			case game.TerrainMountain, game.TerrainHills, game.TerrainPlains, game.TerrainDesert:
				if !landFound {
					land = p
					landFound = true
				}
			case game.TerrainOcean, game.TerrainDeepOcean:
				if !waterFound {
					water = p
					waterFound = true
				}
			}
			if landFound && waterFound {
				break
			}
		}
	}
	if !landFound || !waterFound {
		t.Skipf("missing fixture tiles in 400x400 window (land=%v water=%v)", landFound, waterFound)
	}
	if tileBlocked(land, wg, nil, nil) {
		t.Errorf("land tile %+v unexpectedly blocked with nil lm/vs", land)
	}
	if !tileBlocked(water, wg, nil, nil) {
		t.Errorf("water tile %+v unexpectedly accepted with nil lm/vs", water)
	}
}

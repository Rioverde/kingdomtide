package worldgen

import (
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
)

// zoneSet returns a position set for fast contains checks.
func zoneSet(ps []game.Position) map[game.Position]struct{} {
	m := make(map[game.Position]struct{}, len(ps))
	for _, p := range ps {
		m[p] = struct{}{}
	}
	return m
}

// fourConnected reports whether every tile in ps is reachable from ps[0]
// via 4-connected orthogonal steps that stay inside ps. A zone with zero
// or one tile is trivially connected.
func fourConnected(ps []game.Position) bool {
	if len(ps) < 2 {
		return true
	}
	set := zoneSet(ps)
	visited := make(map[game.Position]struct{}, len(ps))
	stack := []game.Position{ps[0]}
	visited[ps[0]] = struct{}{}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, off := range footprintNeighbourOffsets {
			n := game.Position{X: cur.X + off[0], Y: cur.Y + off[1]}
			if _, inZone := set[n]; !inZone {
				continue
			}
			if _, seen := visited[n]; seen {
				continue
			}
			visited[n] = struct{}{}
			stack = append(stack, n)
		}
	}
	return len(visited) == len(ps)
}

func TestGrowFootprint_ZoneSizes(t *testing.T) {
	if testing.Short() {
		t.Skip("100-anchor zone size bounds sweep")
	}
	const seed int64 = 2024
	wg := NewWorldGenerator(seed)
	states := []game.VolcanoState{game.VolcanoActive, game.VolcanoDormant, game.VolcanoExtinct}

	// 100 anchor × state trials, with anchors drawn from a PRNG keyed on
	// the test so reruns exercise the same distribution.
	rng := rand.New(rand.NewPCG(uint64(seed), 0x1234))
	for i := 0; i < 100; i++ {
		state := states[rng.IntN(len(states))]
		anchor := game.Position{X: rng.IntN(8192) - 4096, Y: rng.IntN(8192) - 4096}
		core, slope, ashland := growFootprint(anchor, state, seed, wg, nil)

		sizes := tierSizes[state]
		if len(core) < sizes.core[0] || len(core) > sizes.core[1] {
			t.Errorf("state=%s anchor=%+v core size %d not in [%d, %d]",
				state, anchor, len(core), sizes.core[0], sizes.core[1])
		}
		// Slope and ashland walks can fall short when they hit landmark or
		// water barriers; assert the upper bound strictly and accept
		// shorter slices.
		if len(slope) > sizes.slope[1] {
			t.Errorf("state=%s anchor=%+v slope size %d exceeds max %d",
				state, anchor, len(slope), sizes.slope[1])
		}
		if len(ashland) > sizes.ashland[1] {
			t.Errorf("state=%s anchor=%+v ashland size %d exceeds max %d",
				state, anchor, len(ashland), sizes.ashland[1])
		}
	}
}

func TestGrowFootprint_4Connectivity(t *testing.T) {
	if testing.Short() {
		t.Skip("80-anchor 4-connectivity flood-fill sweep")
	}
	const seed int64 = 31337
	wg := NewWorldGenerator(seed)
	states := []game.VolcanoState{game.VolcanoActive, game.VolcanoDormant, game.VolcanoExtinct}

	// Ring-shape refactor: slope and ashland radiate outward from EVERY
	// tile of the prior zone in parallel, not from one seed. That means
	// the slope's own tiles need not be 4-connected among themselves —
	// they're each anchored on a different core boundary tile. The
	// correct invariant is that (prior zone ∪ new zone) is 4-connected,
	// i.e. the footprint stays a single contiguous blob as it grows.
	// Core, which has no prior zone, is still self-connected.
	rng := rand.New(rand.NewPCG(uint64(seed), 0xbeef))
	for i := 0; i < 80; i++ {
		state := states[rng.IntN(len(states))]
		anchor := game.Position{X: rng.IntN(4096) - 2048, Y: rng.IntN(4096) - 2048}
		core, slope, ashland := growFootprint(anchor, state, seed, wg, nil)

		if !fourConnected(core) {
			t.Errorf("state=%s anchor=%+v core not 4-connected: %+v", state, anchor, core)
		}
		coreSlope := append(append([]game.Position{}, core...), slope...)
		if !fourConnected(coreSlope) {
			t.Errorf("state=%s anchor=%+v core∪slope not 4-connected: core=%+v slope=%+v",
				state, anchor, core, slope)
		}
		if len(ashland) > 0 {
			all := append(append(append([]game.Position{}, core...), slope...), ashland...)
			if !fourConnected(all) {
				t.Errorf("state=%s anchor=%+v core∪slope∪ashland not 4-connected: core=%+v slope=%+v ashland=%+v",
					state, anchor, core, slope, ashland)
			}
		}
	}
}

func TestGrowFootprint_ExtinctNoAshland(t *testing.T) {
	const seed int64 = 777
	wg := NewWorldGenerator(seed)
	for i := 0; i < 20; i++ {
		anchor := game.Position{X: i * 500, Y: i * 700}
		_, _, ashland := growFootprint(anchor, game.VolcanoExtinct, seed, wg, nil)
		if len(ashland) != 0 {
			t.Errorf("Extinct anchor=%+v yielded %d ashland tiles", anchor, len(ashland))
		}
	}
}

func TestGrowFootprint_AshlandNotOnWater(t *testing.T) {
	if testing.Short() {
		t.Skip("32x32 SC ashland water-exception sweep")
	}
	const seed int64 = 42
	wg := NewWorldGenerator(seed)

	// Force the volcano near an ocean candidate by scanning for an
	// anchor inside a super-region that contains water. The plan
	// guarantees the water-exception: ashland skips water tiles.
	//
	// Walk a handful of anchors and assert none of the ashland tiles
	// are water. Use many trials so at least some sit near coasts.
	trials := 0
	for x := -16; x < 16; x++ {
		for y := -16; y < 16; y++ {
			sc := game.SuperChunkCoord{X: x, Y: y}
			regions := NewNoiseRegionSource(seed)
			lm := NewNoiseLandmarkSource(seed, regions, wg)
			src := NewNoiseVolcanoSource(seed, wg, lm)
			for _, v := range src.VolcanoAt(sc) {
				for _, p := range v.AshlandTiles {
					tile := wg.TileAt(p.X, p.Y)
					if isWaterOrRiverTile(tile) {
						t.Errorf("volcano %+v ashland tile %+v is water (terrain=%q overlays=%v)",
							v.Anchor, p, tile.Terrain, tile.Overlays)
					}
					trials++
				}
			}
		}
	}
	t.Logf("water-exception trials: %d ashland tiles checked", trials)
}

func TestGrowFootprint_NoLandmarkOverwrite(t *testing.T) {
	const seed int64 = 8080
	wg := NewWorldGenerator(seed)
	anchor := game.Position{X: 100, Y: 100}
	// Place a synthetic landmark next to the anchor so the growth
	// walk must route around it.
	landmark := game.Landmark{Coord: game.Position{X: 101, Y: 100}}
	landmarks := []game.Landmark{landmark}

	core, slope, ashland := growFootprint(anchor, game.VolcanoActive, seed, wg, landmarks)
	check := func(zone []game.Position, label string) {
		for _, p := range zone {
			if p.Equal(landmark.Coord) {
				t.Errorf("%s tile collides with landmark at %+v", label, landmark.Coord)
			}
		}
	}
	check(core, "core")
	check(slope, "slope")
	check(ashland, "ashland")
}

func TestGrowFootprint_CoreContainsAnchor(t *testing.T) {
	const seed int64 = 99
	wg := NewWorldGenerator(seed)
	states := []game.VolcanoState{game.VolcanoActive, game.VolcanoDormant, game.VolcanoExtinct}
	for i := 0; i < 30; i++ {
		for _, st := range states {
			anchor := game.Position{X: i * 123, Y: i * 456}
			core, _, _ := growFootprint(anchor, st, seed, wg, nil)
			found := false
			for _, p := range core {
				if p.Equal(anchor) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("state=%s anchor=%+v missing from core %+v", st, anchor, core)
			}
		}
	}
}

func TestGrowFootprint_SortedStable(t *testing.T) {
	if testing.Short() {
		t.Skip("footprint sort stability check across all zones")
	}
	const seed int64 = 54321
	wg := NewWorldGenerator(seed)
	anchor := game.Position{X: 77, Y: 88}
	core, slope, ashland := growFootprint(anchor, game.VolcanoActive, seed, wg, nil)

	isSorted := func(ps []game.Position) bool {
		for i := 1; i < len(ps); i++ {
			if ps[i-1].X > ps[i].X {
				return false
			}
			if ps[i-1].X == ps[i].X && ps[i-1].Y > ps[i].Y {
				return false
			}
		}
		return true
	}
	if !isSorted(core) {
		t.Errorf("core not sorted: %+v", core)
	}
	if !isSorted(slope) {
		t.Errorf("slope not sorted: %+v", slope)
	}
	if !isSorted(ashland) {
		t.Errorf("ashland not sorted: %+v", ashland)
	}
}

func TestGrowFootprint_ZonesDisjoint(t *testing.T) {
	if testing.Short() {
		t.Skip("40-anchor x 3-state zone disjointness sweep")
	}
	const seed int64 = 4242
	wg := NewWorldGenerator(seed)
	states := []game.VolcanoState{game.VolcanoActive, game.VolcanoDormant, game.VolcanoExtinct}

	for i := 0; i < 40; i++ {
		for _, st := range states {
			anchor := game.Position{X: i * 71, Y: i * 97}
			core, slope, ashland := growFootprint(anchor, st, seed, wg, nil)
			all := make(map[game.Position]string)
			reg := func(ps []game.Position, label string) {
				for _, p := range ps {
					if prev, dup := all[p]; dup {
						t.Errorf("state=%s anchor=%+v tile %+v appears in both %s and %s",
							st, anchor, p, prev, label)
					}
					all[p] = label
				}
			}
			reg(core, "core")
			reg(slope, "slope")
			reg(ashland, "ashland")
		}
	}
}

func TestTerrainForZone(t *testing.T) {
	cases := []struct {
		zone  game.VolcanoZone
		state game.VolcanoState
		want  game.Terrain
	}{
		{game.VolcanoZoneCore, game.VolcanoActive, game.TerrainVolcanoCore},
		{game.VolcanoZoneCore, game.VolcanoDormant, game.TerrainVolcanoCoreDormant},
		{game.VolcanoZoneCore, game.VolcanoExtinct, game.TerrainCraterLake},
		{game.VolcanoZoneSlope, game.VolcanoActive, game.TerrainVolcanoSlope},
		{game.VolcanoZoneSlope, game.VolcanoDormant, game.TerrainVolcanoSlope},
		{game.VolcanoZoneSlope, game.VolcanoExtinct, game.TerrainVolcanoSlope},
		{game.VolcanoZoneAshland, game.VolcanoActive, game.TerrainAshland},
		{game.VolcanoZoneAshland, game.VolcanoDormant, game.TerrainAshland},
		{game.VolcanoZoneNone, game.VolcanoActive, ""},
	}
	for _, c := range cases {
		got := terrainForZone(c.zone, c.state)
		if got != c.want {
			t.Errorf("terrainForZone(%s, %s) = %q want %q", c.zone, c.state, got, c.want)
		}
	}
}

func TestBridsonSample_RespectsMinDistance(t *testing.T) {
	if testing.Short() {
		t.Skip("Bridson 200x200 min-distance pairwise sweep")
	}
	rng := rand.New(rand.NewPCG(1, 2))
	const min = 40
	samples := bridsonSample(rng, 0, 0, 200, 200, min, 30)
	if len(samples) < 5 {
		t.Fatalf("too few samples from bridsonSample: %d", len(samples))
	}
	minSq := min * min
	for i := range samples {
		for j := i + 1; j < len(samples); j++ {
			dx := samples[i].X - samples[j].X
			dy := samples[i].Y - samples[j].Y
			if dx*dx+dy*dy < minSq {
				t.Errorf("pair %+v %+v closer than %d (d^2=%d)",
					samples[i], samples[j], min, dx*dx+dy*dy)
			}
		}
	}
}

func TestBridsonSample_WithinBounds(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	samples := bridsonSample(rng, 100, 200, 300, 400, 30, 30)
	for _, p := range samples {
		if p.X < 100 || p.X >= 400 || p.Y < 200 || p.Y >= 600 {
			t.Errorf("sample %+v outside rect [100,400) x [200,600)", p)
		}
	}
}

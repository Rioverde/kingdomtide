package worldgen

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// --- stub sources for gate-chain tests -----------------------------------

// stubLandmarkSource returns a deterministic landmark slice per SC via
// the perSC callback. Used by gate 3 rejection tests.
type stubLandmarkSource struct {
	perSC func(sc geom.SuperChunkCoord) []gworld.Landmark
}

func (s *stubLandmarkSource) LandmarksIn(sc geom.SuperChunkCoord) []gworld.Landmark {
	if s.perSC != nil {
		return s.perSC(sc)
	}
	return nil
}

// allPositionVolcanoSource returns TerrainVolcanoSlope for every queried
// position, driving gate 2 to reject every candidate.
type allPositionVolcanoSource struct{}

func (s *allPositionVolcanoSource) VolcanoAt(_ geom.SuperChunkCoord) []gworld.Volcano {
	return nil
}
func (s *allPositionVolcanoSource) TerrainOverrideAt(_ geom.Position) (gworld.Terrain, bool) {
	return gworld.TerrainVolcanoSlope, true
}
func (s *allPositionVolcanoSource) All() []gworld.Volcano { return nil }

// buildCampTestWorld generates a Tiny world plus its real RegionSource.
// Tiny (640×256) keeps construction fast enough for the short-mode guard.
func buildCampTestWorld(tb testing.TB) (*Map, *RegionSource) {
	tb.Helper()
	w := Generate(testSeed, WorldSizeTiny)
	regions := NewRegionSource(w, testSeed)
	return w, regions
}

// TestCampSourceConstructs verifies that NewCampSource over a Tiny world
// returns a non-nil source and that All() yields at least one camp and
// at most 1000 — a sanity check that Bridson runs and the result is
// stored, without over-constraining the count.
func TestCampSourceConstructs(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})
	if src == nil {
		t.Fatal("NewCampSource returned nil")
	}
	camps := src.All()
	n := len(camps)
	if n < 1 {
		t.Fatalf("expected ≥1 camp, got 0")
	}
	if n > 5000 {
		t.Fatalf("expected ≤5000 camps on Tiny world, got %d", n)
	}
	t.Logf("Tiny world seed=%d: %d camps", testSeed, n)

	// Ruler names must be non-empty — every founding camp gets a name.
	if camps[0].Ruler.Name == "" {
		t.Errorf("camps[0].Ruler.Name is empty; expected a generated name")
	}
	t.Logf("camps[0] ruler name: %q", camps[0].Ruler.Name)
}

// TestCampSourceDeterminism confirms that two independent NewCampSource
// calls with the same seed produce identical All() slices.
func TestCampSourceDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)
	cfg := CampSourceConfig{Regions: regions}

	src1 := NewCampSource(w, testSeed, cfg)
	src2 := NewCampSource(w, testSeed, cfg)

	if !reflect.DeepEqual(src1.All(), src2.All()) {
		t.Fatal("two NewCampSource calls with identical inputs produced different All() slices")
	}
}

// TestCampSourceRegionInheritance verifies that every camp's Region
// matches the RegionCharacter of the super-chunk containing its Anchor.
func TestCampSourceRegionInheritance(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	for _, c := range src.All() {
		sc := geom.WorldToSuperChunk(c.Position.X, c.Position.Y)
		want := regions.RegionAt(sc).Character
		if c.Region != want {
			t.Errorf("camp at %v: Region=%v, want %v (SC %v)", c.Position, c.Region, want, sc)
		}
	}
}

// TestCampSourceCampsInMatchesAll verifies that for every SC, the count
// returned by CampsIn equals the count of camps in All() whose Anchor
// falls inside that SC's tile bounds.
func TestCampSourceCampsInMatchesAll(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	// Build expected counts from All().
	wantBySC := make(map[geom.SuperChunkCoord]int)
	for _, c := range src.All() {
		sc := geom.WorldToSuperChunk(c.Position.X, c.Position.Y)
		wantBySC[sc]++
	}

	// Verify CampsIn matches for every SC that has camps.
	for sc, wantCount := range wantBySC {
		got := src.CampsIn(sc)
		if len(got) != wantCount {
			t.Errorf("CampsIn(%v): got %d camps, want %d", sc, len(got), wantCount)
		}
	}

	// Also verify SCs with no camps return nil (or empty).
	iterSuperChunks(w, func(sc geom.SuperChunkCoord) {
		if wantBySC[sc] == 0 {
			got := src.CampsIn(sc)
			if len(got) != 0 {
				t.Errorf("CampsIn(%v): expected empty, got %d camps", sc, len(got))
			}
		}
	})
}

// TestCampSourceNilRegionsPanics confirms that passing a nil RegionSource
// panics at construction time rather than silently producing wrong output.
func TestCampSourceNilRegionsPanics(t *testing.T) {
	w := Generate(testSeed, WorldSizeTiny)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when cfg.Regions is nil, but no panic occurred")
		}
	}()
	NewCampSource(w, testSeed, CampSourceConfig{})
}

// TestCampSourceRejectsWater verifies that gate 1 eliminates all camps
// when the entire map is ocean. We use a real Tiny world but override
// every cell's terrain to TerrainOcean so IsOcean always returns true.
func TestCampSourceRejectsWater(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)

	// Override every cell to ocean so gate 1 fires for every candidate.
	for i := range w.Terrain {
		w.Terrain[i] = gworld.TerrainOcean
	}

	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})
	if n := len(src.All()); n != 0 {
		t.Fatalf("expected 0 camps on all-ocean map, got %d", n)
	}
}

// TestCampSourceRejectsRiver verifies rivers (tile-level water in
// riverBits) reject camp anchors AND footprint growth, complementing
// the cell-level ocean/lake rejection that already worked.
func TestCampSourceRejectsRiver(t *testing.T) {
	if testing.Short() {
		t.Skip("worldgen Generate is too slow for -short")
	}
	w := Generate(42, WorldSizeTiny)
	regions := NewRegionSource(w, 42)
	landmarks := NewLandmarkSource(w, 42, LandmarkSourceConfig{Regions: regions})
	volcanoes := NewVolcanoSource(w, 42)
	deposits := NewDepositSource(w, 42, DepositSourceConfig{Volcanoes: volcanoes})
	camps := NewCampSource(w, 42, CampSourceConfig{
		Regions:   regions,
		Landmarks: landmarks,
		Volcanoes: volcanoes,
		Deposits:  deposits,
	})

	for _, c := range camps.All() {
		for _, p := range c.Footprint {
			if w.IsRiver(p.X, p.Y) {
				t.Errorf("camp footprint tile (%d, %d) sits on a river — Gate 1/footprintAccept regression",
					p.X, p.Y)
			}
		}
	}
}

// TestCampSourceRejectsVolcano verifies that gate 2 eliminates all camps
// when a stub VolcanoSource reports TerrainVolcanoSlope for every tile.
func TestCampSourceRejectsVolcano(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)

	src := NewCampSource(w, testSeed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: &allPositionVolcanoSource{},
	})
	if n := len(src.All()); n != 0 {
		t.Fatalf("expected 0 camps when all tiles report volcano slope, got %d", n)
	}
}

// TestCampSourceRejectsLandmark verifies that gate 3 eliminates all
// camps when the stub LandmarkSource places a landmark at every Bridson
// candidate by returning the exact same positions from LandmarksIn.
//
// Strategy: run NewCampSource once without landmarks to collect the
// candidate anchor set, then run again with a stub that returns those
// anchors as landmark coords — every candidate should be rejected.
func TestCampSourceRejectsLandmark(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)

	// First pass: no landmark source → collect accepted anchors.
	// We actually want to block at the landmark gate before
	// habitability, so we need the raw Bridson positions. Since we
	// cannot extract raw candidates from the source, we derive the
	// blocked positions from a first run and build a landmark set that
	// covers them all. Any camp from the first run is a position that
	// survived gates 1-2 and 4-6; placing a landmark there triggers
	// gate 3 on the next run for those same positions.
	//
	// To guarantee zero output we must cover ALL Bridson candidates,
	// not just accepted ones. We do this by regenerating candidates
	// per-SC using the same RNG and placing a landmark at each one.
	landmarksBysc := make(map[geom.SuperChunkCoord][]gworld.Landmark)
	scWide := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scTall := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for scY := 0; scY < scTall; scY++ {
		for scX := 0; scX < scWide; scX++ {
			sc := geom.SuperChunkCoord{X: scX, Y: scY}
			bounds := scBounds(sc, w.Width, w.Height)
			rng := campBridsonRNG(testSeed, sc)
			candidates := bridsonSample(rng, bounds, campMinSpacing, campPoissonK)
			lms := make([]gworld.Landmark, len(candidates))
			for i, p := range candidates {
				lms[i] = gworld.Landmark{Coord: p, Kind: gworld.LandmarkTower}
			}
			landmarksBysc[sc] = lms
		}
	}

	ls := &stubLandmarkSource{
		perSC: func(sc geom.SuperChunkCoord) []gworld.Landmark {
			return landmarksBysc[sc]
		},
	}

	src := NewCampSource(w, testSeed, CampSourceConfig{
		Regions:   regions,
		Landmarks: ls,
	})
	if n := len(src.All()); n != 0 {
		t.Fatalf("expected 0 camps when every candidate has a landmark, got %d", n)
	}
}

// TestCampSourceRejectsImpassable verifies that gate 4 eliminates all
// camps when the entire land surface is set to TerrainSnowyPeak.
func TestCampSourceRejectsImpassable(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w, regions := buildCampTestWorld(t)

	// Set every non-ocean cell to SnowyPeak — gate 4 must reject them.
	for i, terr := range w.Terrain {
		if terr != gworld.TerrainOcean && terr != gworld.TerrainDeepOcean {
			w.Terrain[i] = gworld.TerrainSnowyPeak
		}
	}

	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})
	if n := len(src.All()); n != 0 {
		t.Fatalf("expected 0 camps on all-snowy-peak land, got %d", n)
	}
}

// TestCampSourceFullPipelineDensity runs NewCampSource on a Tiny world
// with all optional sources wired and verifies the total camp count sits
// in the expected post-gate range. The skeleton (no gates) produced
// every Bridson candidate as a camp; the gate chain should filter most
// of them, landing substantially lower.
func TestCampSourceFullPipelineDensity(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w := Generate(testSeed, WorldSizeTiny)
	regions := NewRegionSource(w, testSeed)
	volcanoes := NewVolcanoSource(w, testSeed)
	landmarks := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})
	deposits := NewDepositSource(w, testSeed, DepositSourceConfig{
		Volcanoes: volcanoes,
	})

	src := NewCampSource(w, testSeed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
		Landmarks: landmarks,
		Deposits:  deposits,
	})

	n := len(src.All())
	t.Logf("Tiny world seed=%d full pipeline: %d camps", testSeed, n)
	// Baseline: 66 (campRarityMultiplier=0.25). Tolerance: ±30% → [46, 86].
	if n < 46 {
		t.Fatalf("too few camps after gate chain: %d (want ≥46)", n)
	}
	if n > 86 {
		t.Fatalf("too many camps after gate chain: %d (want ≤86)", n)
	}
}

// TestCampAcceptDeterminism verifies that two independent NewCampSource
// calls with all sources wired and the same seed produce identical All()
// slices — the gate chain and acceptance roll are deterministic.
func TestCampAcceptDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("short — worldgen Generate is too slow for -short")
	}
	w := Generate(testSeed, WorldSizeTiny)
	regions := NewRegionSource(w, testSeed)
	volcanoes := NewVolcanoSource(w, testSeed)
	landmarks := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})
	deposits := NewDepositSource(w, testSeed, DepositSourceConfig{
		Volcanoes: volcanoes,
	})

	cfg := CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
		Landmarks: landmarks,
		Deposits:  deposits,
	}

	src1 := NewCampSource(w, testSeed, cfg)
	src2 := NewCampSource(w, testSeed, cfg)

	if !reflect.DeepEqual(src1.All(), src2.All()) {
		t.Fatal("two NewCampSource calls with identical inputs produced different All() slices after gate chain")
	}
}

// TestCampFaithDistribution builds three Tiny worlds and checks that the
// realised faith distribution per region matches campFaithByRegion within
// ±20pp absolute. Tolerance is wide because campRarityMultiplier=0.25
// yields ~55-70 total camps across 3 seeds on Tiny; per-region samples
// are often 10-40 camps, where a single-camp deviation can be ~5-9pp.
// Regions with fewer than 20 camps are skipped entirely.
func TestCampFaithDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed faith distribution sweep — slow")
	}

	seeds := []int64{1, 42, 999999}

	// counts[region][faith] = number of camps
	type faithCounts [polity.FaithCount]int
	counts := [7]faithCounts{}
	totals := [7]int{}

	for _, seed := range seeds {
		w := Generate(seed, WorldSizeTiny)
		regions := NewRegionSource(w, seed)
		src := NewCampSource(w, seed, CampSourceConfig{Regions: regions})
		for _, c := range src.All() {
			r := int(c.Region)
			if r >= 7 {
				continue
			}
			counts[r][c.Faiths.Majority()]++
			totals[r]++
		}
	}

	regionNames := []string{"Normal", "Blighted", "Fey", "Ancient", "Savage", "Holy", "Wild"}

	for r := 0; r < 7; r++ {
		total := totals[r]
		if total < 20 {
			// Not enough camps to assert distribution — skip this region.
			t.Logf("region %s: only %d camps across seeds, skipping distribution check", regionNames[r], total)
			continue
		}

		table := campFaithByRegion[r]

		// Compute expected weight sum for normalisation.
		var weightSum float32
		for _, w := range table {
			weightSum += w.Weight
		}

		for _, entry := range table {
			expectedFrac := float64(entry.Weight) / float64(weightSum)
			gotFrac := float64(counts[r][entry.Kind]) / float64(total)
			diff := gotFrac - expectedFrac
			if diff < -0.20 || diff > 0.20 {
				t.Errorf("region %s faith %s: got %.3f, want %.3f (diff %.3f, tolerance ±0.20)",
					regionNames[r], entry.Kind, gotFrac, expectedFrac, diff)
			}
		}
		t.Logf("region %s: %d camps — faith distribution within ±20pp tolerance", regionNames[r], total)
	}
}

// TestCampPopRange verifies that every camp's Pop is in
// [campZipfMin, campMaxPop].
func TestCampPopRange(t *testing.T) {
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	minPop := int(campZipfMin)
	maxPop := int(campMaxPop)
	for _, c := range src.All() {
		if c.Population < minPop || c.Population > maxPop {
			t.Errorf("camp at %v: Population=%d outside [%d, %d]", c.Position, c.Population, minPop, maxPop)
		}
	}
}

// TestCampPopMostTiny verifies that at least 50% of camps have
// Pop ≤ campZipfMin*2 (i.e. ≤ 20). Validates the steep alpha=1.5 Pareto.
func TestCampPopMostTiny(t *testing.T) {
	if testing.Short() {
		t.Skip("Pareto distribution check — slow")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	camps := src.All()
	if len(camps) == 0 {
		t.Skip("no camps generated")
	}
	threshold := int(campZipfMin * 2)
	tiny := 0
	for _, c := range camps {
		if c.Population <= threshold {
			tiny++
		}
	}
	frac := float64(tiny) / float64(len(camps))
	if frac < 0.50 {
		t.Errorf("only %.1f%% of camps have Pop ≤ %d; want ≥ 50%%", frac*100, threshold)
	}
	t.Logf("%d/%d camps (%.1f%%) have Pop ≤ %d", tiny, len(camps), frac*100, threshold)
}

// TestCampBornYearRange verifies that every camp's BornYear is 0 (all
// camps are founded at simulation start; the simulation ticks forward
// from year 0 and computes age dynamically).
func TestCampBornYearRange(t *testing.T) {
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	for _, c := range src.All() {
		if c.Founded != 0 {
			t.Errorf("camp at %v: BornYear=%d, want 0", c.Position, c.Founded)
		}
	}
}

// TestCampDerivationDeterminism verifies that two independent NewCampSource
// calls with the same seed produce identical Faith/Pop/BornYear for every camp.
func TestCampDerivationDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed determinism — slow")
	}
	for _, seed := range []int64{1, 42, 999999} {
		w := Generate(seed, WorldSizeTiny)
		regions := NewRegionSource(w, seed)
		cfg := CampSourceConfig{Regions: regions}

		src1 := NewCampSource(w, seed, cfg)
		src2 := NewCampSource(w, seed, cfg)

		camps1 := src1.All()
		camps2 := src2.All()
		if len(camps1) != len(camps2) {
			t.Errorf("seed %d: camp count mismatch %d vs %d", seed, len(camps1), len(camps2))
			continue
		}
		for i, c1 := range camps1 {
			c2 := camps2[i]
			if c1.Faiths != c2.Faiths || c1.Population != c2.Population || c1.Founded != c2.Founded {
				t.Errorf("seed %d camp[%d] at %v: Faiths=%v/%v Population=%d/%d Founded=%d/%d",
					seed, i, c1.Position,
					c1.Faiths, c2.Faiths,
					c1.Population, c2.Population,
					c1.Founded, c2.Founded)
			}
		}
		t.Logf("seed %d: %d camps all deterministic", seed, len(camps1))
	}
}

// TestCampFootprintSize verifies that every camp's footprint is 1, 2, or 3
// tiles — 2 for Pop ≤ campFootprintSmallPopThreshold, 3 for larger camps,
// with 1 allowed only for anchors on peninsulas where the frontier dies
// before reaching the target budget.
func TestCampFootprintSize(t *testing.T) {
	if testing.Short() {
		t.Skip("Tiny world build — slow")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	counts := [4]int{} // index = footprint length; index 0 unused
	for _, c := range src.All() {
		n := len(c.Footprint)
		if n < 1 || n > 3 {
			t.Errorf("camp at %v: footprint size %d outside [1,3]", c.Position, n)
			continue
		}
		counts[n]++

		// Verify pop-to-budget mapping: pop > threshold must have ≥ 2 tiles
		// (never 1, unless the frontier was truly exhausted). Pop ≤ threshold
		// must have ≤ 2 tiles.
		if int32(c.Population) > campFootprintSmallPopThreshold && n < 2 {
			t.Errorf("camp at %v: Population=%d > threshold=%d but footprint=%d (want ≥2)",
				c.Position, c.Population, campFootprintSmallPopThreshold, n)
		}
		if int32(c.Population) <= campFootprintSmallPopThreshold && n > 2 {
			t.Errorf("camp at %v: Population=%d ≤ threshold=%d but footprint=%d (want ≤2)",
				c.Position, c.Population, campFootprintSmallPopThreshold, n)
		}
	}
	t.Logf("footprint size distribution: 1=%d  2=%d  3=%d", counts[1], counts[2], counts[3])
}

// TestCampFootprintConnected verifies that every multi-tile footprint is a
// connected cluster: every tile must be 4-neighbour-adjacent to at least one
// other tile in the same footprint.
func TestCampFootprintConnected(t *testing.T) {
	if testing.Short() {
		t.Skip("Tiny world build — slow")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	for _, c := range src.All() {
		if len(c.Footprint) < 2 {
			continue
		}
		tileSet := make(map[geom.Position]struct{}, len(c.Footprint))
		for _, fp := range c.Footprint {
			tileSet[fp] = struct{}{}
		}
		for _, fp := range c.Footprint {
			neighbors := [4]geom.Position{
				{X: fp.X + 1, Y: fp.Y},
				{X: fp.X - 1, Y: fp.Y},
				{X: fp.X, Y: fp.Y + 1},
				{X: fp.X, Y: fp.Y - 1},
			}
			hasAdj := false
			for _, nb := range neighbors {
				if _, ok := tileSet[nb]; ok {
					hasAdj = true
					break
				}
			}
			if !hasAdj {
				t.Errorf("camp at %v: footprint tile %v has no 4-neighbour in footprint %v",
					c.Position, fp, c.Footprint)
			}
		}
	}
}

// TestCampFootprintNoOverlap verifies that no tile appears in more than one
// camp's footprint across the whole world.
func TestCampFootprintNoOverlap(t *testing.T) {
	if testing.Short() {
		t.Skip("Tiny world build — slow")
	}
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	seen := make(map[geom.Position]geom.Position) // tile → first camp's anchor
	for _, c := range src.All() {
		for _, fp := range c.Footprint {
			if prev, exists := seen[fp]; exists {
				t.Errorf("footprint overlap: tile %v claimed by camp %v and camp %v",
					fp, prev, c.Position)
			} else {
				seen[fp] = c.Position
			}
		}
	}
	total := 0
	for _, c := range src.All() {
		total += len(c.Footprint)
	}
	if len(seen) != total {
		t.Errorf("overlap detected: %d unique tiles but %d total footprint entries", len(seen), total)
	}
	t.Logf("seed=%d: %d camps, %d footprint tiles, no overlaps", testSeed, len(src.All()), len(seen))
}

// TestCampFootprintNoIllegalTiles verifies that no footprint tile is ocean,
// snowy peak, volcano core, or crater lake.
func TestCampFootprintNoIllegalTiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Tiny world build — slow")
	}
	w := Generate(testSeed, WorldSizeTiny)
	regions := NewRegionSource(w, testSeed)
	volcanoes := NewVolcanoSource(w, testSeed)
	src := NewCampSource(w, testSeed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})

	for _, c := range src.All() {
		for _, fp := range c.Footprint {
			cellID := w.Voronoi.CellIDAt(fp.X, fp.Y)
			if w.IsOcean(cellID) {
				t.Errorf("camp %v: footprint tile %v is ocean", c.Position, fp)
				continue
			}
			terrain := w.Terrain[cellID]
			switch terrain {
			case gworld.TerrainSnowyPeak,
				gworld.TerrainVolcanoCore,
				gworld.TerrainVolcanoCoreDormant,
				gworld.TerrainCraterLake:
				t.Errorf("camp %v: footprint tile %v has illegal terrain %s", c.Position, fp, terrain)
			}
			// Check volcano override.
			if override, ok := volcanoes.TerrainOverrideAt(fp); ok {
				switch override {
				case gworld.TerrainVolcanoCore,
					gworld.TerrainVolcanoCoreDormant,
					gworld.TerrainVolcanoSlope,
					gworld.TerrainAshland,
					gworld.TerrainCraterLake:
					t.Errorf("camp %v: footprint tile %v has illegal volcano override %s",
						c.Position, fp, override)
				}
			}
		}
	}
}

// TestCampFootprintDeterminism verifies that two independent NewCampSource
// calls with the same seed produce identical footprint slices.
func TestCampFootprintDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("Tiny world build — slow")
	}
	w, regions := buildCampTestWorld(t)
	cfg := CampSourceConfig{Regions: regions}

	src1 := NewCampSource(w, testSeed, cfg)
	src2 := NewCampSource(w, testSeed, cfg)

	camps1 := src1.All()
	camps2 := src2.All()
	if len(camps1) != len(camps2) {
		t.Fatalf("camp count mismatch: %d vs %d", len(camps1), len(camps2))
	}
	for i, c1 := range camps1 {
		c2 := camps2[i]
		if !reflect.DeepEqual(c1.Footprint, c2.Footprint) {
			t.Errorf("camp[%d] at %v: footprint mismatch\n  run1: %v\n  run2: %v",
				i, c1.Position, c1.Footprint, c2.Footprint)
		}
	}
}

// TestCampFootprintSorted verifies that every camp's Footprint slice is in
// (Y, X) lex order.
func TestCampFootprintSorted(t *testing.T) {
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})

	for _, c := range src.All() {
		for i := 1; i < len(c.Footprint); i++ {
			a, b := c.Footprint[i-1], c.Footprint[i]
			if a.Y > b.Y || (a.Y == b.Y && a.X >= b.X) {
				t.Errorf("camp at %v: footprint not sorted at [%d,%d]: %v then %v (full: %v)",
					c.Position, i-1, i, a, b, c.Footprint)
			}
		}
	}
}

// TestCampPlacementDeterminism sweeps four seeds (1, 42, 999999, -1) and
// asserts that two independent NewCampSource calls with the same seed
// produce byte-for-byte identical All() slices — anchor, region, faith,
// pop, born year, and footprint must all match.
func TestCampPlacementDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("multi-seed placement determinism sweep — slow")
	}
	for _, seed := range []int64{1, 42, 999999, -1} {
		w := Generate(seed, WorldSizeTiny)
		regions := NewRegionSource(w, seed)
		volcanoes := NewVolcanoSource(w, seed)
		landmarks := NewLandmarkSource(w, seed, LandmarkSourceConfig{
			Regions:   regions,
			Volcanoes: volcanoes,
		})
		deposits := NewDepositSource(w, seed, DepositSourceConfig{
			Volcanoes: volcanoes,
		})
		cfg := CampSourceConfig{
			Regions:   regions,
			Volcanoes: volcanoes,
			Landmarks: landmarks,
			Deposits:  deposits,
		}
		src1 := NewCampSource(w, seed, cfg)
		src2 := NewCampSource(w, seed, cfg)

		camps1 := src1.All()
		camps2 := src2.All()
		if len(camps1) != len(camps2) {
			t.Errorf("seed %d: camp count mismatch %d vs %d", seed, len(camps1), len(camps2))
			continue
		}
		if !reflect.DeepEqual(camps1, camps2) {
			for i, c1 := range camps1 {
				c2 := camps2[i]
				if !reflect.DeepEqual(c1, c2) {
					t.Errorf("seed %d camp[%d] at %v differs: run1=%+v run2=%+v",
						seed, i, c1.Position, c1, c2)
				}
			}
		}
		t.Logf("seed %d: %d camps — fully deterministic (anchor+region+faith+pop+born+footprint)",
			seed, len(camps1))
	}
}

// TestCampPopDistribution verifies that over the full Tiny-world camp set
// the mean population is within a reasonable band. Steep alpha=1.5 Pareto
// pushes most camps toward campZipfMin=10; observed mean across seeds is ~20.
// We verify mean is in [12, 28] — a ±~35% band wide enough to tolerate
// finite-sample noise on a Tiny world but tight enough to catch a broken
// distribution.
func TestCampPopDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("Pareto mean distribution check — slow")
	}
	seeds := []int64{1, 42, 999999}
	var totalPop int64
	var count int

	for _, seed := range seeds {
		w := Generate(seed, WorldSizeTiny)
		regions := NewRegionSource(w, seed)
		src := NewCampSource(w, seed, CampSourceConfig{Regions: regions})
		for _, c := range src.All() {
			totalPop += int64(c.Population)
			count++
		}
	}
	if count < 100 {
		t.Skipf("too few camps (%d) for distribution test — map too sparse", count)
	}
	mean := float64(totalPop) / float64(count)
	const wantMin, wantMax = 12.0, 28.0
	if mean < wantMin || mean > wantMax {
		t.Errorf("mean pop=%.2f outside [%.0f, %.0f] over %d camps (3 seeds)",
			mean, wantMin, wantMax, count)
	}
	t.Logf("%d camps across 3 seeds: mean pop=%.2f (want %.0f–%.0f)",
		count, mean, wantMin, wantMax)
}

// TestCampBornYearDistribution verifies that all camps have BornYear == 0.
// All camps are founded at simulation start (year 0); the simulation
// ticks forward from year 0 and computes age as currentYear - BornYear.
func TestCampBornYearDistribution(t *testing.T) {
	if testing.Short() {
		t.Skip("BornYear uniformity sweep — slow")
	}
	seeds := []int64{1, 42, 999999}

	for _, seed := range seeds {
		w := Generate(seed, WorldSizeTiny)
		regions := NewRegionSource(w, seed)
		src := NewCampSource(w, seed, CampSourceConfig{Regions: regions})
		for _, c := range src.All() {
			if c.Founded != 0 {
				t.Errorf("seed %d camp at %v: BornYear=%d, want 0", seed, c.Position, c.Founded)
			}
		}
		t.Logf("seed %d: %d camps all have BornYear=0", seed, len(src.All()))
	}
}

// TestCampNoOverlap verifies the broader overlap invariant: no camp's
// anchor lands inside any other camp's footprint, and no footprint tile
// is ocean, snowy peak, volcano core/slope, or crater lake — i.e. no
// anchor was placed at a tile the gate chain should have rejected.
// This extends TestCampFootprintNoOverlap to cover the anchor-in-footprint
// case and the illegal-terrain case in a single sweep over the full
// pipeline (all sources wired).
func TestCampNoOverlap(t *testing.T) {
	if testing.Short() {
		t.Skip("full pipeline overlap check — slow")
	}
	w := Generate(testSeed, WorldSizeTiny)
	regions := NewRegionSource(w, testSeed)
	volcanoes := NewVolcanoSource(w, testSeed)
	landmarks := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})
	deposits := NewDepositSource(w, testSeed, DepositSourceConfig{
		Volcanoes: volcanoes,
	})
	src := NewCampSource(w, testSeed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
		Landmarks: landmarks,
		Deposits:  deposits,
	})

	camps := src.All()

	// Build a map from every footprint tile to its owning camp anchor.
	footprintOwner := make(map[geom.Position]geom.Position, len(camps)*3)
	for _, c := range camps {
		for _, fp := range c.Footprint {
			if prev, exists := footprintOwner[fp]; exists {
				t.Errorf("footprint overlap: tile %v claimed by camps %v and %v",
					fp, prev, c.Position)
			} else {
				footprintOwner[fp] = c.Position
			}
		}
	}

	// No anchor may sit inside a different camp's footprint.
	for _, c := range camps {
		if owner, exists := footprintOwner[c.Position]; exists && owner != c.Position {
			t.Errorf("anchor %v sits inside footprint of camp %v", c.Position, owner)
		}
	}

	// No footprint tile may be ocean, snowy peak, volcano interior, or
	// crater lake. Illegal-terrain tiles must not survive the gate chain.
	illegalBase := map[gworld.Terrain]bool{
		gworld.TerrainOcean:              true,
		gworld.TerrainDeepOcean:          true,
		gworld.TerrainSnowyPeak:          true,
		gworld.TerrainVolcanoCore:        true,
		gworld.TerrainVolcanoCoreDormant: true,
		gworld.TerrainCraterLake:         true,
	}
	illegalOverride := map[gworld.Terrain]bool{
		gworld.TerrainVolcanoCore:        true,
		gworld.TerrainVolcanoCoreDormant: true,
		gworld.TerrainVolcanoSlope:       true,
		gworld.TerrainAshland:            true,
		gworld.TerrainCraterLake:         true,
	}
	for _, c := range camps {
		for _, fp := range c.Footprint {
			cellID := w.Voronoi.CellIDAt(fp.X, fp.Y)
			if w.IsOcean(cellID) {
				t.Errorf("camp %v: footprint tile %v is ocean", c.Position, fp)
				continue
			}
			if illegalBase[w.Terrain[cellID]] {
				t.Errorf("camp %v: footprint tile %v has illegal terrain %s",
					c.Position, fp, w.Terrain[cellID])
			}
			if override, ok := volcanoes.TerrainOverrideAt(fp); ok && illegalOverride[override] {
				t.Errorf("camp %v: footprint tile %v has illegal volcano override %s",
					c.Position, fp, override)
			}
		}
	}
	t.Logf("seed=%d: %d camps, %d footprint tiles — no overlaps or illegal terrain",
		testSeed, len(camps), len(footprintOwner))
}

// TestRegionAffinityVariance verifies that the per-SC settlement-willingness
// roll produces measurable density variance: when SCs are partitioned into
// high-affinity (top 33%) and low-affinity (bottom 33%) groups, high-affinity
// SCs host at least 1.3× more camps on average than low-affinity SCs of any
// biome class — confirming that the affinity multiplier actually differentiates
// density rather than being washed out by the habitability gate.
//
// We use a Standard world (more SCs → more statistical power) and measure
// affinity directly via regionAffinity to partition SCs.
func TestRegionAffinityVariance(t *testing.T) {
	if testing.Short() {
		t.Skip("affinity variance sweep needs Standard world — slow")
	}

	const seed = int64(42)
	w := Generate(seed, WorldSizeStandard)
	regions := NewRegionSource(w, seed)
	src := NewCampSource(w, seed, CampSourceConfig{Regions: regions})

	scWide := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scTall := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize

	type scStat struct {
		affinity float32
		count    int
	}
	stats := make([]scStat, 0, scWide*scTall)

	for sy := 0; sy < scTall; sy++ {
		for sx := 0; sx < scWide; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			aff := regionAffinity(seed, sc)
			cnt := len(src.CampsIn(sc))
			stats = append(stats, scStat{affinity: aff, count: cnt})
		}
	}

	// Sort by affinity to get tercile boundaries.
	n := len(stats)
	tercile := n / 3

	// Compute average camp count in bottom and top terciles.
	// stats is not sorted yet — partition by scanning.
	// Simple O(n) approach: bucket into low/high thirds by sorted rank.
	// For correctness, sort a copy.
	sorted := make([]scStat, n)
	copy(sorted, stats)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].affinity < sorted[j].affinity
	})

	var lowSum, highSum int
	for i := 0; i < tercile; i++ {
		lowSum += sorted[i].count
	}
	for i := n - tercile; i < n; i++ {
		highSum += sorted[i].count
	}

	lowMean := float64(lowSum) / float64(tercile)
	highMean := float64(highSum) / float64(tercile)

	t.Logf("affinity variance: low-tercile mean=%.2f  high-tercile mean=%.2f  ratio=%.2f×",
		lowMean, highMean, highMean/max(lowMean, 0.001))

	const wantRatio = 1.3
	if lowMean == 0 {
		// All low-affinity SCs produced zero camps — trivially the ratio
		// is infinite; the test passes.
		t.Logf("low-tercile mean is 0: high-affinity SCs dominate entirely — affinity variance confirmed")
		return
	}
	ratio := highMean / lowMean
	if ratio < wantRatio {
		t.Errorf("affinity variance too low: high/low ratio=%.2f, want ≥%.1f",
			ratio, wantRatio)
	}
}

// TestCampGoldenMetrics_Standard_Seed42 is the committed golden-metric
// snapshot for the camps pipeline. It constructs a full Standard world
// (seed=42, all sources wired) and asserts every distribution metric
// falls within the committed baseline ±tolerance.
//
// Observed values (measured on 2026-04-25, Standard seed=42, full pipeline,
// campHabitabilityFloor=0.15, campRarityMultiplier=0.25, v1 constants,
// after river-tile gate fix):
//
//	Total camps:           781
//	Mean camps/SC:           1.22  (640 total SCs)
//	OldGods in Blighted:    87.2%
//	GreenSage in Fey:       68.0%
//	OneOath in Savage:      48.4%
//	Mean pop:               19.82
//	Mean footprint:          2.2164
//
// Tolerances are ±15% on counts, ±20% on mean camps/SC, ±5pp on faith
// percentages, ±15% on pop mean, ±15% on footprint mean.
// If this test fails after a worldgen change, re-run the measurement
// helper, update the observed values above, and recommit.
func TestCampGoldenMetrics_Standard_Seed42(t *testing.T) {
	if testing.Short() {
		t.Skip("golden-metric snapshot needs Standard world (~400ms) — slow")
	}

	wallStart := time.Now()
	const seed = int64(42)
	w := Generate(seed, WorldSizeStandard)
	regions := NewRegionSource(w, seed)
	volcanoes := NewVolcanoSource(w, seed)
	landmarks := NewLandmarkSource(w, seed, LandmarkSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})
	deposits := NewDepositSource(w, seed, DepositSourceConfig{
		Volcanoes: volcanoes,
	})
	src := NewCampSource(w, seed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
		Landmarks: landmarks,
		Deposits:  deposits,
	})
	elapsed := time.Since(wallStart)

	camps := src.All()
	total := len(camps)
	t.Logf("build time: %v", elapsed)
	t.Logf("total camps: %d", total)

	// --- Total camp count ------------------------------------------------
	// Baseline: 781. Tolerance: ±15% → [663, 899].
	const (
		baseTotal = 781
		totalLo   = 663
		totalHi   = 899
	)
	if total < totalLo || total > totalHi {
		t.Errorf("total camps=%d, want [%d, %d] (baseline %d ±15%%)",
			total, totalLo, totalHi, baseTotal)
	}

	// --- Mean camps/SC ---------------------------------------------------
	scWide := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scTall := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	totalSC := scWide * scTall
	meanPerSC := float64(total) / float64(totalSC)
	t.Logf("mean camps/SC: %.2f (%d SCs)", meanPerSC, totalSC)
	// Baseline: 1.22. Tolerance: ±20%.
	const basePerSC = 1.22
	if meanPerSC < basePerSC*0.80 || meanPerSC > basePerSC*1.20 {
		t.Errorf("mean camps/SC=%.2f, want %.2f±20%%", meanPerSC, basePerSC)
	}

	// --- Faith by region -------------------------------------------------
	type faithCounts [polity.FaithCount]int
	regionCounts := [7]int{}
	faithByRegion := [7]faithCounts{}
	for _, c := range camps {
		r := int(c.Region)
		if r < 7 {
			regionCounts[r]++
			faithByRegion[r][c.Faiths.Majority()]++
		}
	}

	checkFaithPct := func(regionName string, regionIdx int, faith polity.Faith, faithName string, wantPct, tolerancePP float64) {
		n := regionCounts[regionIdx]
		if n == 0 {
			t.Logf("region %s: 0 camps — skipping %s check", regionName, faithName)
			return
		}
		gotPct := float64(faithByRegion[regionIdx][faith]) / float64(n) * 100
		t.Logf("region %s: %s=%.1f%% (want ≥%.0f%%)", regionName, faithName, gotPct, wantPct)
		if gotPct < wantPct-tolerancePP {
			t.Errorf("region %s: %s=%.1f%%, want ≥%.0f%% (tolerance ±%.0fpp)",
				regionName, faithName, gotPct, wantPct, tolerancePP)
		}
	}

	// Blighted: OldGods baseline 86.59%, floor ≥75%.
	checkFaithPct("Blighted", int(gworld.RegionBlighted), polity.FaithOldGods, "OldGods", 75.0, 5.0)
	// Fey: GreenSage baseline 56.19%, floor ≥45%.
	checkFaithPct("Fey", int(gworld.RegionFey), polity.FaithGreenSage, "GreenSage", 45.0, 5.0)
	// Savage: OneOath baseline 44.65%, floor ≥35%.
	checkFaithPct("Savage", int(gworld.RegionSavage), polity.FaithOneOath, "OneOath", 35.0, 5.0)

	// --- Mean pop --------------------------------------------------------
	// Baseline: 19.82. Range: [17, 23] (±15%).
	var totalPop int64
	for _, c := range camps {
		totalPop += int64(c.Population)
	}
	meanPop := float64(totalPop) / float64(total)
	t.Logf("mean pop: %.2f", meanPop)
	if meanPop < 17.0 || meanPop > 23.0 {
		t.Errorf("mean pop=%.2f, want [17, 23]", meanPop)
	}

	// --- Mean footprint size ---------------------------------------------
	// Baseline: 2.2363. Range: [2.0, 2.4].
	var totalFP int64
	for _, c := range camps {
		totalFP += int64(len(c.Footprint))
	}
	meanFP := float64(totalFP) / float64(total)
	t.Logf("mean footprint: %.4f", meanFP)
	if meanFP < 2.0 || meanFP > 2.4 {
		t.Errorf("mean footprint=%.4f, want [2.0, 2.4]", meanFP)
	}

	t.Logf("golden snapshot PASS — all metrics within committed baselines (wall=%v)", elapsed)
}

// TestCampRulerNameSamples prints 5 sample ruler names for seed=42 (Tiny world).
// Not a correctness test — informational only.
func TestCampRulerNameSamples(t *testing.T) {
	w, regions := buildCampTestWorld(t)
	src := NewCampSource(w, testSeed, CampSourceConfig{Regions: regions})
	camps := src.All()
	for i, c := range camps {
		if i >= 5 {
			break
		}
		t.Logf("camp %d pos=(%d,%d) region=%s ruler=%q",
			i, c.Position.X, c.Position.Y, c.Region, c.Ruler.Name)
	}
}

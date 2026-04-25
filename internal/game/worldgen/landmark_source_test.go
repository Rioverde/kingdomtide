package worldgen

import (
	"math"
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)


// buildLandmarkTestWorld generates a Standard world plus its real
// region source. Centralised so each test pays the gen cost once when
// invoked individually but never duplicates the world build.
func buildLandmarkTestWorld(tb testing.TB) (*Map, *RegionSource) {
	tb.Helper()
	w := Generate(testSeed, WorldSizeStandard)
	regions := NewRegionSource(w, testSeed)
	return w, regions
}

// sweepLandmarkSuperChunks walks every super-chunk grid cell in w and
// invokes visit with the placed landmark slice. Visit is called in
// row-major order so callers can deterministically aggregate.
func sweepLandmarkSuperChunks(
	w *Map,
	src *LandmarkSource,
	visit func(geom.SuperChunkCoord, []gworld.Landmark),
) {
	maxX := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	maxY := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < maxY; sy++ {
		for sx := 0; sx < maxX; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			visit(sc, src.LandmarksIn(sc))
		}
	}
}

// TestLandmarkSource_PlacesLandmarks confirms the placement pipeline
// produces a non-trivial number of landmarks across a Standard world.
// The 50-landmark floor is a sanity check — the (60/30/8/2) count
// distribution averages ~0.5 landmarks per super-chunk, and Standard
// has ~640 super-chunks, so a healthy run lands several hundred.
func TestLandmarkSource_PlacesLandmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	total := 0
	sweepLandmarkSuperChunks(w, src, func(_ geom.SuperChunkCoord, lms []gworld.Landmark) {
		total += len(lms)
	})
	if total < 50 {
		t.Fatalf("expected ≥50 landmarks across Standard world, got %d", total)
	}
}

// TestLandmarkSource_RegionAffinity confirms the per-character kind
// weights actually shape the produced distribution. Standard maps are
// ~80% ocean and the (60/30/8/2) count distribution averages ~0.5
// landmarks per super-chunk, so individual character buckets are
// sparse. We use a Huge world to grow the sample pool and check that
// the observed share of each kind lands within ±25 percentage points
// of the configured weight. The wide tolerance absorbs sample variance
// from the still-modest bucket sizes.
func TestLandmarkSource_RegionAffinity(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Huge world generation costs ~10s")
	}
	w := Generate(testSeed, WorldSizeHuge)
	regions := NewRegionSource(w, testSeed)
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	// counts[character][kind] = number of landmarks of that kind in
	// regions of that character.
	counts := map[gworld.RegionCharacter]map[gworld.LandmarkKind]int{}
	totals := map[gworld.RegionCharacter]int{}

	sweepLandmarkSuperChunks(w, src, func(sc geom.SuperChunkCoord, lms []gworld.Landmark) {
		ch := regions.RegionAt(sc).Character
		if _, ok := counts[ch]; !ok {
			counts[ch] = map[gworld.LandmarkKind]int{}
		}
		for _, lm := range lms {
			counts[ch][lm.Kind]++
			totals[ch]++
		}
	})

	const tolerance = 0.25
	const minSample = 20 // skip buckets too sparse for a reliable share

	checked := 0
	for ch, weights := range map[gworld.RegionCharacter][]weighted[gworld.LandmarkKind]{
		gworld.RegionNormal:   landmarkKindsNormal,
		gworld.RegionBlighted: landmarkKindsBlighted,
		gworld.RegionFey:      landmarkKindsFey,
		gworld.RegionAncient:  landmarkKindsAncient,
		gworld.RegionSavage:   landmarkKindsSavage,
		gworld.RegionHoly:     landmarkKindsHoly,
		gworld.RegionWild:     landmarkKindsWild,
	} {
		total := totals[ch]
		if total < minSample {
			t.Logf("character %s under sampled (%d landmarks) — skipping affinity check",
				ch, total)
			continue
		}
		checked++
		for _, w := range weights {
			share := float64(counts[ch][w.Kind]) / float64(total)
			diff := math.Abs(share - float64(w.Weight))
			if diff > tolerance {
				t.Errorf("character=%s kind=%s share=%.3f want=%.3f diff=%.3f > tol=%.2f",
					ch, w.Kind, share, w.Weight, diff, tolerance)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("no character bucket reached minSample=%d — placement may be misconfigured",
			minSample)
	}
}

// TestLandmarkSource_AvoidsOcean asserts every placed landmark sits on
// a non-ocean tile. A regression here would surface as players seeing
// shrines floating mid-sea or landmark approach screens loading on
// drowned coords.
func TestLandmarkSource_AvoidsOcean(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	violations := 0
	sweepLandmarkSuperChunks(w, src, func(_ geom.SuperChunkCoord, lms []gworld.Landmark) {
		for _, lm := range lms {
			if src.tileIsOcean(lm.Coord) {
				violations++
				if violations <= 5 {
					t.Errorf("landmark on ocean tile: kind=%s coord=%v",
						lm.Kind, lm.Coord)
				}
			}
		}
	})
	if violations > 0 {
		t.Fatalf("%d landmarks placed on ocean tiles", violations)
	}
}

// TestLandmarkSource_AvoidsVolcanoes wires a real VolcanoSource and
// verifies every placed landmark sits outside every volcano's core +
// slope union. Ashland tiles are not part of the reject zone (see
// NewLandmarkSource) so we explicitly check core and slope only.
func TestLandmarkSource_AvoidsVolcanoes(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	volcanoes := NewVolcanoSource(w, testSeed)
	if len(volcanoes.All()) == 0 {
		t.Skip("no volcanoes placed for this seed — check is vacuously true")
	}
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions, Volcanoes: volcanoes})

	// Build a flat lookup once, mirroring the source's own index, so
	// the test does not depend on private map state.
	reject := map[uint64]struct{}{}
	for _, v := range volcanoes.All() {
		for _, t := range v.CoreTiles {
			reject[geom.PackPos(t)] = struct{}{}
		}
		for _, t := range v.SlopeTiles {
			reject[geom.PackPos(t)] = struct{}{}
		}
	}

	violations := 0
	sweepLandmarkSuperChunks(w, src, func(_ geom.SuperChunkCoord, lms []gworld.Landmark) {
		for _, lm := range lms {
			if _, bad := reject[geom.PackPos(lm.Coord)]; bad {
				violations++
				if violations <= 5 {
					t.Errorf("landmark in volcano zone: kind=%s coord=%v",
						lm.Kind, lm.Coord)
				}
			}
		}
	})
	if violations > 0 {
		t.Fatalf("%d landmarks placed inside volcano core/slope", violations)
	}
}

// TestLandmarkSource_Determinism asserts two LandmarkSource instances
// constructed from the same (world, seed, regions) produce byte-
// identical results across every super-chunk. Cache state must not
// leak across instances.
func TestLandmarkSource_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	a := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})
	b := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	maxX := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	maxY := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < maxY; sy++ {
		for sx := 0; sx < maxX; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			la := a.LandmarksIn(sc)
			lb := b.LandmarksIn(sc)
			if !reflect.DeepEqual(la, lb) {
				t.Fatalf("non-determinism at %v: a=%v b=%v", sc, la, lb)
			}
		}
	}
}

// TestLandmarkSource_NamesGenerated asserts every produced Landmark
// carries a non-zero BodySeed. A zero seed is technically valid for
// the 1-in-2⁶⁴ tail draw, so we tolerate at most one zero across the
// full world.
func TestLandmarkSource_NamesGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	zeros := 0
	total := 0
	sweepLandmarkSuperChunks(w, src, func(_ geom.SuperChunkCoord, lms []gworld.Landmark) {
		for _, lm := range lms {
			total++
			if lm.Name.BodySeed == 0 {
				zeros++
			}
		}
	})
	if total == 0 {
		t.Fatalf("no landmarks generated — preconditions broken")
	}
	if zeros > 1 {
		t.Fatalf("got %d landmarks with zero BodySeed (allow ≤1), total=%d",
			zeros, total)
	}
}

// TestLandmarkSource_Spacing asserts no two landmarks inside the same
// super-chunk sit within landmarkMinSpacing tiles of each other under
// the Chebyshev metric used in the reject filter.
func TestLandmarkSource_Spacing(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	src := NewLandmarkSource(w, testSeed, LandmarkSourceConfig{Regions: regions})

	violations := 0
	sweepLandmarkSuperChunks(w, src, func(sc geom.SuperChunkCoord, lms []gworld.Landmark) {
		for i := 0; i < len(lms); i++ {
			for j := i + 1; j < len(lms); j++ {
				dx := lms[i].Coord.X - lms[j].Coord.X
				if dx < 0 {
					dx = -dx
				}
				dy := lms[i].Coord.Y - lms[j].Coord.Y
				if dy < 0 {
					dy = -dy
				}
				if dx < landmarkMinSpacing && dy < landmarkMinSpacing {
					violations++
					if violations <= 5 {
						t.Errorf("spacing violation in sc=%v: %v vs %v",
							sc, lms[i].Coord, lms[j].Coord)
					}
				}
			}
		}
	})
	if violations > 0 {
		t.Fatalf("%d landmark pairs violated min spacing", violations)
	}
}

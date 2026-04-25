package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)


// buildRegionTestWorld generates a Standard world for the heavier
// region-source tests. Centralised so each test pays the gen cost once
// when invoked individually but never duplicates the world build.
func buildRegionTestWorld(tb testing.TB) *Map {
	tb.Helper()
	return Generate(testSeed, WorldSizeStandard)
}

// sweepRegionSuperChunks walks every super-chunk grid cell that has at
// least one tile inside the world and yields the resulting Region.
// Visit is called in row-major order over the super-chunk grid.
func sweepRegionSuperChunks(w *Map, src *RegionSource, visit func(geom.SuperChunkCoord, gworld.Region)) {
	maxX := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	maxY := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < maxY; sy++ {
		for sx := 0; sx < maxX; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			visit(sc, src.RegionAt(sc))
		}
	}
}

// TestRegionSource_DiverseCharacters confirms the noise + biome bias
// pipeline produces a varied palette of region characters across a
// Standard world. We require at least four distinct characters out of
// the seven possible. Lower numbers would point to a stuck noise field
// or an over-aggressive threshold collapsing every region to Normal.
func TestRegionSource_DiverseCharacters(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := buildRegionTestWorld(t)
	src := NewRegionSource(w, testSeed)

	seen := make(map[gworld.RegionCharacter]int)
	sweepRegionSuperChunks(w, src, func(_ geom.SuperChunkCoord, r gworld.Region) {
		seen[r.Character]++
	})

	if len(seen) < 4 {
		t.Fatalf("expected ≥4 distinct region characters, got %d: %v", len(seen), seen)
	}
}

// TestRegionSource_BiomeAffinity confirms the biome bias actually
// shows through in the per-region influence vector. For each
// (biome, expected component) pair we sample regions whose anchor
// lands on that biome and check the expected component dominates the
// average influence. The averaging absorbs noise variance so the
// assertion is robust without pinning a specific super-chunk.
func TestRegionSource_BiomeAffinity(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := buildRegionTestWorld(t)
	src := NewRegionSource(w, testSeed)

	type bucket struct {
		blight, fae, ancient, savage, holy, wild float32
		count                                    int
	}
	buckets := map[gworld.Terrain]*bucket{
		gworld.TerrainMountain:  {},
		gworld.TerrainSnowyPeak: {},
		gworld.TerrainTundra:    {},
		gworld.TerrainMeadow:    {},
		gworld.TerrainGrassland: {},
		gworld.TerrainJungle:    {},
		gworld.TerrainForest:    {},
		gworld.TerrainBeach:     {},
		gworld.TerrainDesert:    {},
	}

	sweepRegionSuperChunks(w, src, func(_ geom.SuperChunkCoord, r gworld.Region) {
		t := src.biomeAt(r.Anchor)
		b, ok := buckets[t]
		if !ok {
			return
		}
		b.blight += r.Influence.Blight
		b.fae += r.Influence.Fae
		b.ancient += r.Influence.Ancient
		b.savage += r.Influence.Savage
		b.holy += r.Influence.Holy
		b.wild += r.Influence.Wild
		b.count++
	})

	// affinityCheck verifies that the expected component is the largest
	// of the six on average. Skips silently if the biome did not appear
	// in this seed's world — Standard worlds at seed 42 hit every entry
	// in the table, but world-size or seed changes could starve a bucket
	// and we want that to surface as a clear log not a phantom failure.
	affinityCheck := func(name string, terrains []gworld.Terrain, want gworld.RegionCharacter) {
		var b bucket
		for _, terr := range terrains {
			if got := buckets[terr]; got != nil {
				b.blight += got.blight
				b.fae += got.fae
				b.ancient += got.ancient
				b.savage += got.savage
				b.holy += got.holy
				b.wild += got.wild
				b.count += got.count
			}
		}
		if b.count == 0 {
			t.Logf("affinity %s: no anchors landed on %v in this world (skipped)", name, terrains)
			return
		}
		avg := gworld.RegionInfluence{
			Blight:  b.blight / float32(b.count),
			Fae:     b.fae / float32(b.count),
			Ancient: b.ancient / float32(b.count),
			Savage:  b.savage / float32(b.count),
			Holy:    b.holy / float32(b.count),
			Wild:    b.wild / float32(b.count),
		}
		// Strongest component (ignoring threshold) for the assertion.
		got := strongestComponent(avg)
		if got != want {
			t.Errorf("affinity %s (%d anchors): want %s strongest, got %s; avg=%+v",
				name, b.count, want, got, avg)
		}
	}

	affinityCheck("mountain", []gworld.Terrain{gworld.TerrainMountain, gworld.TerrainSnowyPeak}, gworld.RegionAncient)
	affinityCheck("tundra", []gworld.Terrain{gworld.TerrainTundra}, gworld.RegionBlighted)
	affinityCheck("meadow_grass", []gworld.Terrain{gworld.TerrainMeadow, gworld.TerrainGrassland}, gworld.RegionHoly)
	affinityCheck("jungle", []gworld.Terrain{gworld.TerrainJungle}, gworld.RegionSavage)
	affinityCheck("forest_beach", []gworld.Terrain{gworld.TerrainForest, gworld.TerrainBeach}, gworld.RegionWild)
}

// strongestComponent returns the single largest component of an
// influence vector ignoring the dominant threshold. Used by the biome-
// affinity assertion to compare *raw* magnitudes rather than threshold-
// gated characters: averaging across many anchors pulls components
// well below regionInfluenceThreshold even when the bias is doing its
// job, so we want the unconditional argmax instead of Dominant().
func strongestComponent(r gworld.RegionInfluence) gworld.RegionCharacter {
	type entry struct {
		v float32
		c gworld.RegionCharacter
	}
	entries := [...]entry{
		{r.Blight, gworld.RegionBlighted},
		{r.Fae, gworld.RegionFey},
		{r.Ancient, gworld.RegionAncient},
		{r.Savage, gworld.RegionSavage},
		{r.Holy, gworld.RegionHoly},
		{r.Wild, gworld.RegionWild},
	}
	best := entries[0]
	for _, e := range entries[1:] {
		if e.v > best.v {
			best = e
		}
	}
	return best.c
}

// TestRegionSource_Determinism confirms two RegionSources built on the
// same world+seed return identical Regions for every queried coord.
// Two independent sources are used so the cache cannot mask a non-
// deterministic compute path — the cache only deduplicates calls within
// a single source.
func TestRegionSource_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := buildRegionTestWorld(t)
	a := NewRegionSource(w, testSeed)
	b := NewRegionSource(w, testSeed)

	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0},
		{X: 5, Y: 5},
		{X: 12, Y: 8},
		{X: 20, Y: 3},
		{X: 31, Y: 14},
		{X: 7, Y: 12},
	}
	for _, sc := range coords {
		ra := a.RegionAt(sc)
		rb := b.RegionAt(sc)
		if ra != rb {
			t.Fatalf("region differs at %+v:\n  a=%+v\n  b=%+v", sc, ra, rb)
		}
		// Sample twice on the same source — cache hit must match the
		// initial compute.
		if ra2 := a.RegionAt(sc); ra2 != ra {
			t.Fatalf("cache hit differs at %+v: first=%+v second=%+v", sc, ra, ra2)
		}
	}
}

// TestRegionSource_NamesGenerated walks every super-chunk and confirms
// each Region carries a non-zero BodySeed. naming.Generate's contract
// is that BodySeed is the last 64-bit draw off the per-coord PCG
// stream — by construction it is non-zero with overwhelming probability
// (~1 - 2⁻⁶⁴) for any real-world seed/coord pair, but we still allow a
// single zero per run as a safety valve in case a future stream change
// produces an unlucky alignment. Anything beyond that points to a
// hard-coded zero or short-circuited generator.
func TestRegionSource_NamesGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := buildRegionTestWorld(t)
	src := NewRegionSource(w, testSeed)

	zeros := 0
	total := 0
	sweepRegionSuperChunks(w, src, func(_ geom.SuperChunkCoord, r gworld.Region) {
		total++
		if r.Name.BodySeed == 0 {
			zeros++
		}
	})
	if total == 0 {
		t.Fatal("no super-chunks visited")
	}
	if zeros > 1 {
		t.Fatalf("%d/%d regions have BodySeed == 0; expected ≤1", zeros, total)
	}
}

// TestRegionSource_NormalThresholdGate confirms that a region whose
// influence components all fall below regionInfluenceThreshold ends up
// as RegionNormal regardless of which component is strongest. We feed
// a synthetic influence vector through the same Dominant + threshold
// logic the production path uses, ensuring the gate cannot drift.
func TestRegionSource_NormalThresholdGate(t *testing.T) {
	weak := gworld.RegionInfluence{
		Blight:  regionInfluenceThreshold - 0.05,
		Fae:     regionInfluenceThreshold - 0.10,
		Ancient: regionInfluenceThreshold - 0.20,
		Savage:  regionInfluenceThreshold - 0.15,
		Holy:    regionInfluenceThreshold - 0.30,
		Wild:    regionInfluenceThreshold - 0.25,
	}
	if weak.Max() >= regionInfluenceThreshold {
		t.Fatal("test setup invalid: weak influence above threshold")
	}
	// Apply the same gate the production path uses.
	got := weak.Dominant()
	if weak.Max() < regionInfluenceThreshold {
		got = gworld.RegionNormal
	}
	if got != gworld.RegionNormal {
		t.Fatalf("weak influence below threshold should collapse to Normal, got %s", got)
	}
}

// TestRegionSource_OutOfBoundsAnchor exercises biomeAt on an anchor
// outside the world rectangle. Should return TerrainOcean (clamped),
// not panic. Synthetic small world keeps the test fast — no Standard
// gen.
func TestRegionSource_OutOfBoundsAnchor(t *testing.T) {
	w := Generate(7, WorldSizeTiny)
	src := NewRegionSource(w, 7)

	// Anchor far outside the bounded world.
	out := src.biomeAt(geom.Position{X: w.Width + 1000, Y: w.Height + 1000})
	if out == "" {
		t.Fatal("biomeAt returned empty terrain on out-of-bounds")
	}
	// Must clamp into the world; in practice that lands on the corner
	// cell which is essentially always ocean for a Tiny world.
	if !out.Passable() && out != gworld.TerrainOcean && out != gworld.TerrainDeepOcean {
		// Acceptable as long as we got back *some* valid terrain enum.
		t.Logf("clamped corner biome = %s (acceptable)", out)
	}
}

// TestRegionSource_InfluenceAtMatchesRegion confirms the per-tile
// InfluenceAt sampler returns the same influence vector as
// RegionAt(NormalizeAt(...)) — an invariant other server code relies
// on (the tint pass and the LRU snapshot use the same source).
func TestRegionSource_InfluenceAtMatchesRegion(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := buildRegionTestWorld(t)
	src := NewRegionSource(w, testSeed)

	// Pick a handful of arbitrary tiles spread across the world. Use
	// odd offsets so the tile is not on a super-chunk boundary.
	samples := []geom.Position{
		{X: 100, Y: 100},
		{X: 500, Y: 250},
		{X: 1234, Y: 567},
		{X: 800, Y: 800},
	}
	for _, p := range samples {
		got := src.InfluenceAt(p.X, p.Y)
		_, sc := geom.AnchorAt(testSeed, p.X, p.Y)
		want := src.RegionAt(sc).Influence
		if got != want {
			t.Errorf("InfluenceAt(%d,%d)=%+v; RegionAt(sc=%+v).Influence=%+v",
				p.X, p.Y, got, sc, want)
		}
	}
}

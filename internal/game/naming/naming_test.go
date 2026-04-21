package naming

import (
	"math"
	"math/rand/v2"
	"testing"
)

// coord is a test-local two-int holder replacing any game.Position
// usage. The naming package must remain independent of the game domain
// to avoid an import cycle (game embeds naming.Parts in Region.Name).
type coord struct{ x, y int }

// testBounds mirrors the shape of the real region / landmark /
// settlement catalogs. The exact numbers are not load-bearing — tests
// only need non-zero counts so the Format downgrade path is not
// exercised in the happy-path cases.
func testBounds() Bounds {
	return Bounds{
		PatternCount: map[string]int{
			"region.forest":       5,
			"region.plain":        4,
			"region.mountain":     4,
			"region.water":        4,
			"region.desert":       4,
			"region.tundra":       3,
			"region.unknown":      1,
			"landmark.tower":      3,
			"landmark.giant_tree": 3,
			"settlement.village":  3,
			"settlement.town":     3,
			"settlement.city":     3,
		},
		PrefixCount: map[string]int{
			"normal":   5,
			"blighted": 5,
			"fey":      5,
			"ancient":  5,
			"savage":   5,
			"holy":     5,
			"wild":     5,
		},
	}
}

// sampleInput returns a spec-sized Input varying only in Coord. Callers
// override the fields they care about; the rest stays fixed so
// determinism tests exercise the common path.
func sampleInput(c coord) Input {
	return Input{
		Domain:    DomainRegion,
		Character: "blighted",
		SubKind:   "forest",
		Seed:      42,
		CoordX:    c.x,
		CoordY:    c.y,
	}
}

func TestGenerate_Determinism(t *testing.T) {
	bounds := testBounds()
	in := sampleInput(coord{x: 7, y: -3})

	first := Generate(in, bounds)
	for i := 0; i < 100; i++ {
		got := Generate(in, bounds)
		if got != first {
			t.Fatalf("Generate not deterministic at iter %d: want %+v got %+v", i, first, got)
		}
	}
}

// TestGenerate_BodySeedDeterminism pins the invariant that the
// language-agnostic BodySeed is stable across repeated calls with the
// same Input. Clients rely on this to reproduce the same body text on
// every render without re-contacting the server.
func TestGenerate_BodySeedDeterminism(t *testing.T) {
	bounds := testBounds()
	in := sampleInput(coord{x: -19, y: 41})

	want := Generate(in, bounds).BodySeed
	for i := 0; i < 100; i++ {
		got := Generate(in, bounds).BodySeed
		if got != want {
			t.Fatalf("BodySeed not deterministic at iter %d: want %x got %x", i, want, got)
		}
	}
}

// TestGenerate_BodySeedVariesWithCoord asserts that across a large
// sample of coords the BodySeed field is not constant. Any single
// BodySeed value appearing more than 10× in 5000 trials would signal
// the PCG stream collapsed.
func TestGenerate_BodySeedVariesWithCoord(t *testing.T) {
	bounds := testBounds()
	const trials = 5000

	seen := make(map[int64]int, trials)
	for i := 0; i < trials; i++ {
		in := sampleInput(coord{x: i, y: -i * 3})
		seen[Generate(in, bounds).BodySeed]++
	}
	for s, n := range seen {
		if n > 10 {
			t.Fatalf("BodySeed %x appeared %d times across %d trials — stream collapsed", s, n, trials)
		}
	}
	if len(seen) < trials/2 {
		t.Fatalf("only %d unique BodySeed values across %d trials", len(seen), trials)
	}
}

// TestGenerate_FormatDistribution samples DefaultWeights many times and
// asserts each format lands within ±5% of its target share. The
// guardrail is loose on purpose — sampling noise dominates at 10k
// trials.
func TestGenerate_FormatDistribution(t *testing.T) {
	bounds := testBounds()
	counts := map[Format]int{}
	const n = 10_000

	for i := 0; i < n; i++ {
		in := Input{
			Domain:    DomainRegion,
			Character: "blighted",
			SubKind:   "forest",
			Seed:      int64(i) * 101,
			CoordX:    i,
			CoordY:    -i,
		}
		counts[Generate(in, bounds).Format]++
	}

	check := func(f Format, want float64) {
		t.Helper()
		got := float64(counts[f]) / float64(n)
		if math.Abs(got-want) > 0.05 {
			t.Errorf("format %v: got share %.3f, want within 0.05 of %.3f (count %d)",
				f, got, want, counts[f])
		}
	}
	check(FormatBodyOnly, 0.40)
	check(FormatCharacterPrefix, 0.40)
	check(FormatKindPattern, 0.20)
}

// TestGenerate_BoundsRespected spot-checks 2000 random inputs to verify
// every emitted index stays below its bound. A zero-bound key must
// yield a zero index and never crash.
func TestGenerate_BoundsRespected(t *testing.T) {
	bounds := testBounds()
	seedRng := rand.New(rand.NewPCG(1, 2))

	for i := 0; i < 2000; i++ {
		in := Input{
			Domain:    DomainRegion,
			Character: "blighted",
			SubKind:   "forest",
			Seed:      int64(seedRng.Uint64()),
			CoordX:    seedRng.IntN(1000) - 500,
			CoordY:    seedRng.IntN(1000) - 500,
		}
		got := Generate(in, bounds)

		if int(got.PrefixIndex) >= bounds.PrefixCount[in.Character] {
			t.Fatalf("trial %d: PrefixIndex %d out of bounds %d",
				i, got.PrefixIndex, bounds.PrefixCount[in.Character])
		}
		patternKey := string(in.Domain) + "." + in.SubKind
		if int(got.PatternIndex) >= bounds.PatternCount[patternKey] {
			t.Fatalf("trial %d: PatternIndex %d out of bounds %d",
				i, got.PatternIndex, bounds.PatternCount[patternKey])
		}
	}
}

// TestGenerate_FormatDowngrade covers the architect's explicit rule: a
// FormatCharacterPrefix selection with PrefixCount=0 for the requested
// character must collapse to FormatBodyOnly. Forcing the format via
// 100/0/0 weights keeps the test tight.
func TestGenerate_FormatDowngrade(t *testing.T) {
	SetDomainWeights(DomainLandmark, Weights{
		BodyOnly:        0,
		CharacterPrefix: 100,
		KindPattern:     0,
	})
	t.Cleanup(func() { resetDomainWeights(DomainLandmark) })

	bounds := Bounds{
		PatternCount: map[string]int{"landmark.tower": 3},
		PrefixCount:  map[string]int{"blighted": 0}, // forces downgrade.
	}

	in := Input{
		Domain:    DomainLandmark,
		Character: "blighted",
		SubKind:   "tower",
		Seed:      99,
		CoordX:    1,
		CoordY:    1,
	}

	for i := 0; i < 50; i++ {
		in.CoordX = i
		in.CoordY = i * 3
		got := Generate(in, bounds)
		if got.Format != FormatBodyOnly {
			t.Fatalf("coord (%d, %d): expected downgrade to FormatBodyOnly, got %v",
				in.CoordX, in.CoordY, got.Format)
		}
		if got.PrefixIndex != 0 {
			t.Fatalf("coord (%d, %d): expected PrefixIndex 0 under zero bound, got %d",
				in.CoordX, in.CoordY, got.PrefixIndex)
		}
	}

	// KindPattern downgrade: empty pattern count, weights force
	// KindPattern.
	SetDomainWeights(DomainLandmark, Weights{
		BodyOnly:        0,
		CharacterPrefix: 0,
		KindPattern:     100,
	})
	bounds = Bounds{
		PatternCount: map[string]int{"landmark.tower": 0},
		PrefixCount:  map[string]int{"blighted": 4},
	}
	got := Generate(in, bounds)
	if got.Format != FormatBodyOnly {
		t.Fatalf("KindPattern with zero bound should downgrade, got %v", got.Format)
	}
}

// TestGenerate_SaltDistinct exercises the per-domain decorrelation
// rule: Parts produced for the same (Coord, Seed) under different
// Domains must not share BodySeed or the trio of structural values.
// Without per-domain salts the three streams collapse.
func TestGenerate_SaltDistinct(t *testing.T) {
	bounds := testBounds()

	// Pick a few coords that trigger non-zero draws across all three
	// domains. A single coord is too narrow because an occasional
	// collision on any one component is plausible; requiring at least
	// one coord to produce three distinct BodySeeds rules out the silent
	// "all domains share a stream" failure mode.
	coords := []coord{{1, 1}, {2, 7}, {-3, 4}, {100, -100}, {31, 29}}
	distinct := false
	for _, c := range coords {
		reg := Generate(Input{Domain: DomainRegion, Character: "blighted", SubKind: "forest",
			Seed: 99, CoordX: c.x, CoordY: c.y}, bounds)
		lm := Generate(Input{Domain: DomainLandmark, Character: "blighted", SubKind: "tower",
			Seed: 99, CoordX: c.x, CoordY: c.y}, bounds)
		st := Generate(Input{Domain: DomainSettlement, Character: "blighted", SubKind: "village",
			Seed: 99, CoordX: c.x, CoordY: c.y}, bounds)

		if reg.BodySeed != lm.BodySeed && lm.BodySeed != st.BodySeed && reg.BodySeed != st.BodySeed {
			distinct = true
			break
		}
	}
	if !distinct {
		t.Fatal("per-domain BodySeeds collapsed for every sampled coord — salts are not decorrelating")
	}
}

// TestGenerate_CoordHash rejects the (X, Y) vs (Y, X) collision. Two
// Parts produced at swapped coords must differ on at least one field —
// otherwise diagonal mirror-image tiles would share names.
func TestGenerate_CoordHash(t *testing.T) {
	bounds := testBounds()
	base := Input{
		Domain:    DomainRegion,
		Character: "blighted",
		SubKind:   "forest",
		Seed:      7,
	}

	for _, c := range []coord{{1, 2}, {3, 7}, {11, -5}, {-1, 1}, {100, 0}} {
		a := Generate(Input{Domain: base.Domain, Character: base.Character, SubKind: base.SubKind,
			Seed: base.Seed, CoordX: c.x, CoordY: c.y}, bounds)
		b := Generate(Input{Domain: base.Domain, Character: base.Character, SubKind: base.SubKind,
			Seed: base.Seed, CoordX: c.y, CoordY: c.x}, bounds)
		if a == b {
			t.Fatalf("swapped coords (%d,%d) vs (%d,%d) produced identical Parts %+v",
				c.x, c.y, c.y, c.x, a)
		}
	}
}

// resetDomainWeights removes a per-domain override so subsequent tests
// see DefaultWeights again.
func resetDomainWeights(d Domain) {
	weightsMu.Lock()
	delete(weights, d)
	weightsMu.Unlock()
}

// TestSaltDistinct guards the architect's must-fix: every naming salt
// must differ from every existing 64-bit constant harvested from the
// internal/game/ and internal/game/worldgen/ trees.
func TestSaltDistinct(t *testing.T) {
	namingSalts := []int64{saltRegion, saltLandmarkName, saltSettlement}

	existing := []int64{
		toSaltInt64(0x9e3779b97f4a7c15),
		toSaltInt64(0xbf58476d1ce4e5b9),
		toSaltInt64(0x94d049bb133111eb),
		toSaltInt64(0x9e3779b185ebca87),
		toSaltInt64(0xc2b2ae3d27d4eb4f),
		toSaltInt64(0x243f6a8885a308d3),
		toSaltInt64(0x13198a2e03707344),
		toSaltInt64(0x5a308d313198a2e0),
		toSaltInt64(0x452821e638d01377),
		toSaltInt64(0xbe5466cf34e90c6c),
		toSaltInt64(0x6c62272e07bb0142),
		toSaltInt64(0x3c6ef372fe94f82b),
		toSaltInt64(0xd1b54a32d192ed03),
		toSaltInt64(0x7f4a7c15be5466cf),
		toSaltInt64(0x34e90c6c85a308d3),
		toSaltInt64(0x82efa98ec4eec6a9),
		toSaltInt64(0xc0ac29b7c97c50dd),
		toSaltInt64(0x3f84d5b5b5470917),
		// Naming-local hashCoord primes.
		toSaltInt64(hashCoordPrimeX),
		toSaltInt64(hashCoordPrimeY),
	}

	for _, s := range namingSalts {
		for _, e := range existing {
			if s == e {
				t.Fatalf("naming salt 0x%016x collides with existing constant", uint64(s))
			}
		}
	}

	// Naming salts must also be pairwise distinct.
	for i := range namingSalts {
		for j := i + 1; j < len(namingSalts); j++ {
			if namingSalts[i] == namingSalts[j] {
				t.Fatalf("naming salts[%d] == salts[%d] = 0x%016x", i, j, uint64(namingSalts[i]))
			}
		}
	}
}

// TestDomainSaltCoverage verifies every declared Domain has a non-zero
// salt and distinct Domain values route to distinct salts. Catches a
// copy-paste bug in domainSalt at test time.
func TestDomainSaltCoverage(t *testing.T) {
	seen := map[int64]Domain{}
	for _, d := range []Domain{DomainRegion, DomainLandmark, DomainSettlement} {
		s := domainSalt(d)
		if s == 0 {
			t.Fatalf("domain %q has zero salt", d)
		}
		if prev, ok := seen[s]; ok {
			t.Fatalf("domains %q and %q share salt 0x%016x", prev, d, uint64(s))
		}
		seen[s] = d
	}
}

func BenchmarkGenerate(b *testing.B) {
	bounds := testBounds()
	in := Input{
		Domain:    DomainRegion,
		Character: "blighted",
		SubKind:   "forest",
		Seed:      1234,
		CoordX:    10,
		CoordY:    20,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		in.CoordX = i
		_ = Generate(in, bounds)
	}
}

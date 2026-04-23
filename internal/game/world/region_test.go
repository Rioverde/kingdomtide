package world

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

func TestRegionCharacterString(t *testing.T) {
	cases := []struct {
		c    RegionCharacter
		want string
	}{
		{RegionNormal, "normal"},
		{RegionBlighted, "blighted"},
		{RegionFey, "fey"},
		{RegionAncient, "ancient"},
		{RegionSavage, "savage"},
		{RegionHoly, "holy"},
		{RegionWild, "wild"},
	}
	for _, tc := range cases {
		if got := tc.c.String(); got != tc.want {
			t.Fatalf("RegionCharacter(%d).String() = %q, want %q", tc.c, got, tc.want)
		}
		if got := tc.c.Key(); got != tc.want {
			t.Fatalf("RegionCharacter(%d).Key() = %q, want %q", tc.c, got, tc.want)
		}
	}
}

func TestRegionInfluenceDominant(t *testing.T) {
	cases := []struct {
		name string
		in   RegionInfluence
		want RegionCharacter
	}{
		{
			name: "zero vector is Normal",
			in:   RegionInfluence{},
			want: RegionNormal,
		},
		{
			name: "just below threshold stays Normal",
			in:   RegionInfluence{Blight: regionDominantThreshold},
			want: RegionNormal,
		},
		{
			name: "single Blight above threshold",
			in:   RegionInfluence{Blight: 0.9},
			want: RegionBlighted,
		},
		{
			name: "single Wild above threshold",
			in:   RegionInfluence{Wild: 0.8},
			want: RegionWild,
		},
		{
			name: "Fey dominates Ancient by magnitude",
			in:   RegionInfluence{Fae: 0.9, Ancient: 0.6},
			want: RegionFey,
		},
		{
			name: "exact tie broken by declaration order (Blight > Fae)",
			in:   RegionInfluence{Blight: 0.8, Fae: 0.8},
			want: RegionBlighted,
		},
		{
			name: "exact tie broken by declaration order (Ancient > Savage)",
			in:   RegionInfluence{Ancient: 0.7, Savage: 0.7},
			want: RegionAncient,
		},
		{
			name: "three above, highest wins",
			in:   RegionInfluence{Blight: 0.5, Holy: 0.95, Wild: 0.6},
			want: RegionHoly,
		},
		{
			name: "all at threshold exactly",
			in: RegionInfluence{
				Blight: regionDominantThreshold, Fae: regionDominantThreshold,
				Ancient: regionDominantThreshold, Savage: regionDominantThreshold,
				Holy: regionDominantThreshold, Wild: regionDominantThreshold,
			},
			want: RegionNormal,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.in.Dominant(); got != tc.want {
				t.Fatalf("Dominant() = %s, want %s", got, tc.want)
			}
		})
	}
}

func TestRegionInfluenceSum(t *testing.T) {
	cases := []struct {
		name string
		in   RegionInfluence
		want float32
	}{
		{"zero", RegionInfluence{}, 0},
		{"single", RegionInfluence{Blight: 0.5}, 0.5},
		{
			name: "all one",
			in: RegionInfluence{
				Blight: 1, Fae: 1, Ancient: 1, Savage: 1, Holy: 1, Wild: 1,
			},
			want: 6,
		},
		{
			name: "mixed",
			in:   RegionInfluence{Blight: 0.1, Fae: 0.2, Ancient: 0.3, Savage: 0.05, Holy: 0.25, Wild: 0.1},
			want: 1.0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Sum()
			// Float epsilon is fine here — the test operates on clean decimals
			// whose exact binary representations still sum to within 1e-6.
			if diff := got - tc.want; diff < -1e-6 || diff > 1e-6 {
				t.Fatalf("Sum() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRegionInfluenceMax(t *testing.T) {
	cases := []struct {
		name string
		in   RegionInfluence
		want float32
	}{
		{"zero vector", RegionInfluence{}, 0},
		{"single Blight", RegionInfluence{Blight: 0.7}, 0.7},
		{"single Wild", RegionInfluence{Wild: 0.5}, 0.5},
		{"all one", RegionInfluence{Blight: 1, Fae: 1, Ancient: 1, Savage: 1, Holy: 1, Wild: 1}, 1},
		{"Fae dominates", RegionInfluence{Blight: 0.3, Fae: 0.9, Ancient: 0.5}, 0.9},
		{"Holy max", RegionInfluence{Blight: 0.1, Fae: 0.2, Ancient: 0.3, Savage: 0.4, Holy: 0.95, Wild: 0.6}, 0.95},
		{"all equal", RegionInfluence{Blight: 0.5, Fae: 0.5, Ancient: 0.5, Savage: 0.5, Holy: 0.5, Wild: 0.5}, 0.5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.Max()
			if diff := got - tc.want; diff < -1e-6 || diff > 1e-6 {
				t.Fatalf("Max() = %v, want %v", got, tc.want)
			}
			// Max must never exceed Sum, and must never exceed 1.
			if got > 1.0+1e-6 {
				t.Fatalf("Max() = %v exceeds 1.0", got)
			}
			if got > tc.in.Sum()+1e-6 {
				t.Fatalf("Max() = %v exceeds Sum() = %v", got, tc.in.Sum())
			}
		})
	}
}

func TestWorldToSuperChunkNegative(t *testing.T) {
	cases := []struct {
		x, y int
		want geom.SuperChunkCoord
	}{
		{0, 0, geom.SuperChunkCoord{X: 0, Y: 0}},
		{63, 63, geom.SuperChunkCoord{X: 0, Y: 0}},
		{64, 0, geom.SuperChunkCoord{X: 1, Y: 0}},
		{0, 64, geom.SuperChunkCoord{X: 0, Y: 1}},
		{-1, -1, geom.SuperChunkCoord{X: -1, Y: -1}},
		{-64, -64, geom.SuperChunkCoord{X: -1, Y: -1}},
		{-65, -65, geom.SuperChunkCoord{X: -2, Y: -2}},
		{-63, 63, geom.SuperChunkCoord{X: -1, Y: 0}},
	}
	for _, tc := range cases {
		got := geom.WorldToSuperChunk(tc.x, tc.y)
		if got != tc.want {
			t.Fatalf("WorldToSuperChunk(%d, %d) = %+v, want %+v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestAnchorOfDeterminism(t *testing.T) {
	const seed int64 = 42
	coords := []geom.SuperChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 2}, {X: -3, Y: 4}, {X: 100, Y: -100}, {X: -5000, Y: 5000},
	}
	for _, sc := range coords {
		a1 := geom.AnchorOf(seed, sc)
		a2 := geom.AnchorOf(seed, sc)
		if a1 != a2 {
			t.Fatalf("AnchorOf not deterministic for %+v: %+v vs %+v", sc, a1, a2)
		}
	}
}

func TestAnchorOfJitterBounds(t *testing.T) {
	const seed int64 = 1337
	// Sweep a 100×100 super-chunk grid (10 000 anchors) centred near the
	// origin. Every anchor's local offset must stay in [geom.AnchorJitterMin,
	// geom.AnchorJitterMax] on both axes.
	for y := -50; y < 50; y++ {
		for x := -50; x < 50; x++ {
			sc := geom.SuperChunkCoord{X: x, Y: y}
			a := geom.AnchorOf(seed, sc)
			localX := a.X - sc.X*geom.SuperChunkSize
			localY := a.Y - sc.Y*geom.SuperChunkSize
			if localX < geom.AnchorJitterMin || localX > geom.AnchorJitterMax {
				t.Fatalf("anchor X out of bounds at %+v: local=%d", sc, localX)
			}
			if localY < geom.AnchorJitterMin || localY > geom.AnchorJitterMax {
				t.Fatalf("anchor Y out of bounds at %+v: local=%d", sc, localY)
			}
		}
	}
}

func TestAnchorOfDifferentSeeds(t *testing.T) {
	// Across 1000 coords, the fraction of coords where two different seeds
	// produce the same anchor must be vanishingly small. Allow a tiny
	// tolerance for the occasional coincidence; 1% is orders of magnitude
	// above any realistic random overlap and still tight enough to fail on
	// a bug that collapses the seed entropy.
	const (
		seedA int64 = 1
		seedB int64 = 987654321
		size        = 1000
	)
	collisions := 0
	for i := range size {
		sc := geom.SuperChunkCoord{X: i, Y: -i}
		if geom.AnchorOf(seedA, sc) == geom.AnchorOf(seedB, sc) {
			collisions++
		}
	}
	if collisions > size/100 {
		t.Fatalf("too many seed collisions: %d / %d", collisions, size)
	}
}

func TestAnchorAtCorrectness(t *testing.T) {
	// Verify that AnchorAt returns the nearest anchor among the 9 candidates
	// for a tile at a known position. The test picks tiles close to the
	// centre of a super-chunk so the answer should be that super-chunk's
	// anchor itself; then picks tiles near the shared boundary where the
	// winner should be the neighbour.
	const seed int64 = 7

	type query struct {
		name string
		x, y int
	}
	queries := []query{
		{"near origin", 32, 32},
		{"near (1,0) centre", geom.SuperChunkSize + 32, 32},
		{"near (-1,-1)", -32, -32},
		{"near (3,3)", 3*geom.SuperChunkSize + 10, 3*geom.SuperChunkSize + 50},
	}

	for _, q := range queries {
		t.Run(q.name, func(t *testing.T) {
			gotAnchor, gotSC := geom.AnchorAt(seed, q.x, q.y)

			// Brute-force the 9 candidates and compute the expected winner
			// with the same tie-break rule.
			home := geom.WorldToSuperChunk(q.x, q.y)
			type cand struct {
				sc geom.SuperChunkCoord
				a  geom.Position
				d  int
			}
			cands := make([]cand, 0, 9)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
					a := geom.AnchorOf(seed, sc)
					cands = append(cands, cand{sc, a, geom.SqDist(a.X, a.Y, q.x, q.y)})
				}
			}
			best := cands[0]
			for _, c := range cands[1:] {
				if c.d < best.d || (c.d == best.d && geom.LessSC(c.sc, best.sc)) {
					best = c
				}
			}
			if gotSC != best.sc || gotAnchor != best.a {
				t.Fatalf("AnchorAt(%d, %d) = (%+v, %+v), want (%+v, %+v)",
					q.x, q.y, gotAnchor, gotSC, best.a, best.sc)
			}
		})
	}
}

func TestAnchorAtVoronoiProperty(t *testing.T) {
	// Walk a horizontal line at y=0 and record the sequence of returned
	// SuperChunkCoords. Because anchors are jittered inside [8, 56], the
	// boundary between two neighbouring cells almost never lands on a
	// multiple of SuperChunkSize. The test asserts that at least one
	// boundary transition happens at an x offset that is not a multiple of
	// SuperChunkSize — that is, the borders are not grid-aligned.
	const seed int64 = 42
	const n = 500

	prev := geom.SuperChunkCoord{X: -1 << 31} // sentinel that cannot match
	nonGridBoundary := false
	for x := range n {
		_, sc := geom.AnchorAt(seed, x, 0)
		if x > 0 && sc != prev {
			if x%geom.SuperChunkSize != 0 {
				nonGridBoundary = true
			}
		}
		prev = sc
	}
	if !nonGridBoundary {
		t.Fatalf("Voronoi boundaries all landed on multiples of %d — geometry is rectangular, not Voronoi", geom.SuperChunkSize)
	}
}

func TestNormalizeAtIsDeterministicAndTotal(t *testing.T) {
	// NormalizeAt is hard to drive into a specific peninsula case without
	// hand-placing anchors, which we cannot do without a mock. This test
	// guards the weaker properties the task asks for: it must never panic
	// on a broad sweep of random tiles, it must be deterministic, and its
	// result must always be one of the 9 candidate SuperChunkCoords (i.e.
	// the function is closed over the local neighbourhood).
	const seed int64 = 99
	for y := -50; y < 50; y++ {
		for x := -50; x < 50; x++ {
			a := geom.NormalizeAt(seed, x, y)
			b := geom.NormalizeAt(seed, x, y)
			if a != b {
				t.Fatalf("NormalizeAt not deterministic at (%d, %d): %+v vs %+v", x, y, a, b)
			}

			home := geom.WorldToSuperChunk(x, y)
			found := false
			for dy := -1; dy <= 1 && !found; dy++ {
				for dx := -1; dx <= 1 && !found; dx++ {
					if a == (geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}) {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("NormalizeAt(%d, %d) returned %+v outside the 3×3 neighbourhood of %+v", x, y, a, home)
			}
		}
	}
}

func TestIsInRegionConsistentWithAnchorAt(t *testing.T) {
	const seed int64 = 314159
	for y := -20; y < 20; y++ {
		for x := -20; x < 20; x++ {
			_, sc := geom.AnchorAt(seed, x, y)
			if !geom.IsInRegion(seed, sc, x, y) {
				t.Fatalf("IsInRegion disagrees with AnchorAt at (%d, %d)", x, y)
			}
			// Any other SuperChunkCoord in the neighbourhood that differs
			// from sc must report false for this tile.
			home := geom.WorldToSuperChunk(x, y)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					other := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
					if other == sc {
						continue
					}
					if geom.IsInRegion(seed, other, x, y) {
						t.Fatalf("IsInRegion(%+v) = true for tile (%d, %d) whose region is %+v",
							other, x, y, sc)
					}
				}
			}
		}
	}
}

func TestRegionTilesNearDeterministic(t *testing.T) {
	const seed int64 = 2024
	sc := geom.SuperChunkCoord{X: 3, Y: -2}

	a := geom.RegionTilesNear(seed, sc, 10, 16)
	b := geom.RegionTilesNear(seed, sc, 10, 16)
	if len(a) != len(b) {
		t.Fatalf("length mismatch across two calls: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("non-deterministic at index %d: %+v vs %+v", i, a[i], b[i])
		}
	}

	for _, p := range a {
		if !geom.IsInRegion(seed, sc, p.X, p.Y) {
			t.Fatalf("RegionTilesNear returned (%d, %d) which is not in region %+v", p.X, p.Y, sc)
		}
	}
	if len(a) > 10 {
		t.Fatalf("returned more than requested: %d > 10", len(a))
	}
}

func TestRegionTilesNearZero(t *testing.T) {
	got := geom.RegionTilesNear(1, geom.SuperChunkCoord{}, 0, 10)
	if got != nil {
		t.Fatalf("RegionTilesNear(..., n=0) = %v, want nil", got)
	}
}


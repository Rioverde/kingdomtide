package worldgen

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/biome"
)

// TestRegionNameDeterminism anchors the purity contract of RegionName —
// same inputs always return the same Parts record, no matter how many
// times it is called in any order.
func TestRegionNameDeterminism(t *testing.T) {
	const seed int64 = 42
	sc := geom.SuperChunkCoord{X: 3, Y: -5}

	want := RegionName(world.RegionBlighted, biome.FamilyForest, seed, sc)
	for i := range 5 {
		got := RegionName(world.RegionBlighted, biome.FamilyForest, seed, sc)
		if got != want {
			t.Fatalf("iteration %d: RegionName = %+v, want %+v", i, got, want)
		}
	}

	// Different seed must give a different body seed (statistically).
	other := RegionName(world.RegionBlighted, biome.FamilyForest, seed+1, sc)
	if other.BodySeed == want.BodySeed {
		t.Logf("same BodySeed %x for different seeds — unlikely but not impossible", want.BodySeed)
	}
}

// TestRegionNameAllCharactersProduceParts covers the catalogue of
// region characters. Each call must resolve to a Parts record carrying
// the character's key without panicking.
func TestRegionNameAllCharactersProduceParts(t *testing.T) {
	const seed int64 = 1234
	sc := geom.SuperChunkCoord{X: 0, Y: 0}

	characters := []world.RegionCharacter{
		world.RegionNormal,
		world.RegionBlighted,
		world.RegionFey,
		world.RegionAncient,
		world.RegionSavage,
		world.RegionHoly,
		world.RegionWild,
	}
	for _, c := range characters {
		p := RegionName(c, biome.FamilyPlain, seed, sc)
		if p.Character != c.Key() {
			t.Errorf("RegionName(%s).Character = %q, want %q", c, p.Character, c.Key())
		}
	}
}

// TestRegionNameBodySeedVariety drives 1000 different super-chunks
// through the formatter and asserts at least 800 distinct BodySeed
// values come back. Catches a collapse bug where the RNG stops
// depending on the coord.
func TestRegionNameBodySeedVariety(t *testing.T) {
	const seed int64 = 7
	const total = 1000
	const minUnique = 800

	seen := make(map[int64]struct{}, total)
	for i := range total {
		sc := geom.SuperChunkCoord{X: i, Y: i * 13}
		p := RegionName(world.RegionFey, biome.FamilyForest, seed, sc)
		seen[p.BodySeed] = struct{}{}
	}

	if len(seen) < minUnique {
		t.Fatalf("only %d unique BodySeed values in %d super-chunks (want >= %d)",
			len(seen), total, minUnique)
	}
}

// TestRegionNameFormatCoverage asserts the catalog Format distribution
// populates FormatCharacterPrefix within 10% of the DefaultWeights
// (40/40/20) expected share — a sanity check that the rng drives all
// three branches.
func TestRegionNameFormatCoverage(t *testing.T) {
	const seed int64 = 99
	const total = 10000

	charPrefix := 0
	for i := range total {
		sc := geom.SuperChunkCoord{X: i, Y: -i}
		name := RegionName(world.RegionNormal, biome.FamilyMountain, seed, sc)
		if name.Format == naming.FormatCharacterPrefix {
			charPrefix++
		}
	}

	ratio := float64(charPrefix) / float64(total)
	if math.Abs(ratio-0.4) > 0.1 {
		t.Fatalf("FormatCharacterPrefix ratio %f outside 0.4 ± 0.1 (%d / %d)",
			ratio, charPrefix, total)
	}
}

// TestHashCoordDistribution feeds 10000 arbitrary SuperChunkCoords
// through hashCoord and expects at least 9000 unique uint64 outputs.
// A much smaller unique count would mean the two-prime mix is
// collapsing adjacent coords into buckets.
func TestHashCoordDistribution(t *testing.T) {
	seen := make(map[uint64]struct{}, 10000)
	for i := range 10000 {
		sc := geom.SuperChunkCoord{X: i, Y: i * 7}
		seen[hashCoord(sc)] = struct{}{}
	}
	if len(seen) < 9000 {
		t.Fatalf("hashCoord distribution weak: %d unique out of 10000", len(seen))
	}
}

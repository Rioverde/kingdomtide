package cities_test

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/cities"
)

// allSettlementKinds lists every production SettlementKind. Kept in
// enum order so a new value added without a test-case update surfaces
// on the next catalog-coverage assertion.
var allSettlementKinds = []cities.SettlementKind{
	cities.SettlementVillage,
	cities.SettlementTown,
	cities.SettlementCity,
	cities.SettlementKeep,
	cities.SettlementRuin,
}

// allCultures lists every production Culture.
var allCultures = []cities.Culture{
	cities.CultureDrevan,
	cities.CultureWild,
	cities.CultureFallen,
	cities.CulturePlain,
}

// allCharacters mirrors the production RegionCharacter set. Declared
// locally rather than imported from game so this test stays readable.
var allCharacters = []world.RegionCharacter{
	world.RegionNormal,
	world.RegionBlighted,
	world.RegionFey,
	world.RegionAncient,
	world.RegionSavage,
	world.RegionHoly,
	world.RegionWild,
}

// TestSettlementName_Smoke drives the cartesian product of (culture,
// kind, character) through Name with a fixed seed and coord and
// verifies the returned Parts carries sensible structural fields. No
// index may exceed its catalog bound, and the sub_kind must match the
// "<culture>.<kind>" format the client dispatches on.
func TestSettlementName_Smoke(t *testing.T) {
	const seed int64 = 1
	coord := geom.Position{X: 0, Y: 0}
	bounds := cities.Bounds()

	for _, culture := range allCultures {
		for _, kind := range allSettlementKinds {
			for _, character := range allCharacters {
				p := cities.Name(culture, kind, character, seed, coord)

				wantSub := string(culture) + "." + kind.Key()
				if p.SubKind != wantSub {
					t.Errorf("Name(%s,%s,%s).SubKind = %q, want %q",
						culture, kind, character, p.SubKind, wantSub)
				}
				if p.Character != character.Key() {
					t.Errorf("Name(%s,%s,%s).Character = %q, want %q",
						culture, kind, character, p.Character, character.Key())
				}

				// Format is one of the three valid values — the underlying
				// type is a small enum, so any out-of-range value indicates
				// a Generate regression rather than a bounds issue.
				switch p.Format {
				case naming.FormatBodyOnly,
					naming.FormatCharacterPrefix,
					naming.FormatKindPattern:
				default:
					t.Errorf("Name(%s,%s,%s).Format = %d (invalid)",
						culture, kind, character, p.Format)
				}

				// Index bounds. Generate's downgrade logic means any format
				// with a non-zero index must have a positive catalog entry
				// for that key; the converse (zero index / zero bound) is
				// also legal.
				prefixMax := bounds.PrefixCount[character.Key()]
				if int(p.PrefixIndex) > prefixMax && prefixMax > 0 {
					t.Errorf("Name(%s,%s,%s).PrefixIndex = %d, max = %d",
						culture, kind, character, p.PrefixIndex, prefixMax)
				}
				patternMax := bounds.PatternCount["settlement."+wantSub]
				if int(p.PatternIndex) > patternMax && patternMax > 0 {
					t.Errorf("Name(%s,%s,%s).PatternIndex = %d, max = %d",
						culture, kind, character, p.PatternIndex, patternMax)
				}
			}
		}
	}
}

// TestSettlementName_Determinism asserts Name is pure — repeated calls
// with identical inputs return identical Parts. The underlying
// naming.Generate is pure by construction; this test guards against
// accidental globals creeping into the wrapper.
func TestSettlementName_Determinism(t *testing.T) {
	const seed int64 = 42
	coord := geom.Position{X: 7, Y: -13}

	want := cities.Name(cities.CultureDrevan, cities.SettlementVillage,
		world.RegionNormal, seed, coord)
	for i := range 5 {
		got := cities.Name(cities.CultureDrevan, cities.SettlementVillage,
			world.RegionNormal, seed, coord)
		if got != want {
			t.Fatalf("iteration %d: %+v, want %+v", i, got, want)
		}
	}
}

// TestSettlementName_SubKindCoverage drives every (culture, kind)
// combination and asserts the SubKind field is non-empty and matches
// the bounds catalog key. Catches a typo or drift between the bounds
// map and either enum.
func TestSettlementName_SubKindCoverage(t *testing.T) {
	const seed int64 = 99
	coord := geom.Position{X: 0, Y: 0}
	bounds := cities.Bounds()

	for _, culture := range allCultures {
		for _, kind := range allSettlementKinds {
			p := cities.Name(culture, kind, world.RegionNormal, seed, coord)
			if p.SubKind == "" {
				t.Errorf("Name(%s,%s): empty SubKind", culture, kind)
				continue
			}
			if _, ok := bounds.PatternCount["settlement."+p.SubKind]; !ok {
				t.Errorf("Name(%s,%s).SubKind = %q has no bounds entry",
					culture, kind, p.SubKind)
			}
		}
	}
}

// TestSettlementKindKeyRoundTrip pins the Key string for every
// production SettlementKind plus one out-of-range sentinel. Changes
// here require a catalog migration.
func TestSettlementKindKeyRoundTrip(t *testing.T) {
	cases := []struct {
		kind cities.SettlementKind
		want string
	}{
		{cities.SettlementVillage, "village"},
		{cities.SettlementTown, "town"},
		{cities.SettlementCity, "city"},
		{cities.SettlementKeep, "keep"},
		{cities.SettlementRuin, "ruin"},
		{cities.SettlementKind(99), ""}, // out-of-range → empty
	}
	for _, c := range cases {
		if got := c.kind.Key(); got != c.want {
			t.Errorf("SettlementKind(%d).Key() = %q, want %q",
				c.kind, got, c.want)
		}
		if got := c.kind.String(); got != c.want {
			t.Errorf("SettlementKind(%d).String() = %q, want %q",
				c.kind, got, c.want)
		}
	}
}

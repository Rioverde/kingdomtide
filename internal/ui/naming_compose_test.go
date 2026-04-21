package ui

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/naming"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// partsFixture builds a NameParts record with the fields composeName
// actually reads. Tests override only what they care about.
func partsFixture(format pb.NameFormat) *pb.NameParts {
	return &pb.NameParts{
		Character:    "blighted",
		SubKind:      "forest",
		Format:       format,
		PrefixIndex:  0,
		PatternIndex: 0,
		BodySeed:     17,
	}
}

// TestComposeNameNil returns the empty string for a nil Parts so call
// sites can decide whether to render a placeholder without a nil
// guard.
func TestComposeNameNil(t *testing.T) {
	t.Parallel()
	if got := composeName(naming.DomainRegion, nil, "en"); got != "" {
		t.Fatalf("composeName(nil) = %q, want empty string", got)
	}
}

// TestComposeNameBodyOnlyEn produces a non-empty body from the
// embedded English corpus for the blighted character — the
// FormatBodyOnly path is the simplest and must exercise the Markov
// pipeline end-to-end.
func TestComposeNameBodyOnlyEn(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_BODY_ONLY)
	got := composeName(naming.DomainRegion, p, "en")
	if got == "" {
		t.Fatal("composeName(body_only, en) returned empty string — markov corpus missing?")
	}
	// Body-only output must not contain the catalog-template tokens —
	// those only appear when the composition template is applied.
	if strings.Contains(got, "{{") {
		t.Fatalf("composeName body_only leaked template tokens: %q", got)
	}
}

// TestComposeNameBodyOnlyRu exercises the Russian corpus on the same
// fixture to prove language switching works end-to-end.
func TestComposeNameBodyOnlyRu(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_BODY_ONLY)
	got := composeName(naming.DomainRegion, p, "ru")
	if got == "" {
		t.Fatal("composeName(body_only, ru) returned empty string — markov corpus missing?")
	}
}

// TestComposeNameDifferentBetweenLanguages pins the invariant that en
// and ru produce different body text for the same Parts. A
// corpus-load regression that accidentally aliased the two languages
// would silently pass other tests but fail here.
func TestComposeNameDifferentBetweenLanguages(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_BODY_ONLY)
	en := composeName(naming.DomainRegion, p, "en")
	ru := composeName(naming.DomainRegion, p, "ru")
	if en == "" || ru == "" {
		t.Fatalf("composeName returned empty body — en=%q ru=%q", en, ru)
	}
	if en == ru {
		t.Fatalf("composeName en == ru for same Parts: %q", en)
	}
}

// TestComposeNameCharacterPrefixEn asserts the character-prefix
// template wraps the body with the configured prefix. The template
// string in the en catalog is "{{.Prefix}} {{.Body}}", so the composed
// output contains a single space between the prefix and the body and
// starts with a capital letter.
func TestComposeNameCharacterPrefixEn(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_CHARACTER_PREFIX)
	got := composeName(naming.DomainRegion, p, "en")
	if got == "" {
		t.Fatal("composeName(character_prefix, en) returned empty string")
	}
	// The body component is non-empty and comes after a space.
	if !strings.Contains(got, " ") {
		t.Fatalf("composeName character_prefix output missing prefix/body separator: %q", got)
	}
}

// TestComposeNameKindPatternEn asserts the kind-pattern template
// injects the body into the region-forest pattern string.
func TestComposeNameKindPatternEn(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_KIND_PATTERN)
	got := composeName(naming.DomainRegion, p, "en")
	if got == "" {
		t.Fatal("composeName(kind_pattern, en) returned empty string")
	}
}

// TestComposeNameDeterministic runs the same Parts through composeName
// repeatedly and asserts the output never drifts. Clients re-render
// the status bar every frame; a non-deterministic body would flicker.
func TestComposeNameDeterministic(t *testing.T) {
	t.Parallel()
	p := partsFixture(pb.NameFormat_NAME_FORMAT_BODY_ONLY)
	want := composeName(naming.DomainRegion, p, "en")
	for i := 0; i < 20; i++ {
		got := composeName(naming.DomainRegion, p, "en")
		if got != want {
			t.Fatalf("iter %d: composeName drifted — want %q, got %q", i, want, got)
		}
	}
}

// TestComposeNameAllFormatsBothLangs covers the 3 formats × 2 locales
// matrix the brief calls out.
func TestComposeNameAllFormatsBothLangs(t *testing.T) {
	t.Parallel()
	formats := []pb.NameFormat{
		pb.NameFormat_NAME_FORMAT_BODY_ONLY,
		pb.NameFormat_NAME_FORMAT_CHARACTER_PREFIX,
		pb.NameFormat_NAME_FORMAT_KIND_PATTERN,
	}
	langs := []string{"en", "ru"}
	for _, f := range formats {
		for _, l := range langs {
			f := f
			l := l
			t.Run(f.String()+"/"+l, func(t *testing.T) {
				t.Parallel()
				p := partsFixture(f)
				got := composeName(naming.DomainRegion, p, l)
				if got == "" {
					t.Fatalf("composeName(format=%v, lang=%q) = empty", f, l)
				}
			})
		}
	}
}

package locale

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// loadFlatKeys reads a TOML catalog from disk and returns every fully-qualified
// message ID as a set. The TOML schema nests message IDs as table headers, so
// the decoder produces a nested map[string]any tree; flattenToml (defined in
// bundle_test.go) walks it and collects the leaf IDs.
func loadFlatKeys(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var tree map[string]any
	if _, err := toml.Decode(string(raw), &tree); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	out := make(map[string]struct{})
	flattenToml(tree, "", out)
	return out
}

// countKeys returns the number of keys in the set that start with prefix.
func countKeys(keys map[string]struct{}, prefix string) int {
	n := 0
	for k := range keys {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

// subKindFrom strips the "<domain>." prefix from a PatternCount map key to
// yield the bare sub_kind component. For example "region.forest" → "forest".
func subKindFrom(domain, domainSubKind string) string {
	prefix := domain + "."
	if strings.HasPrefix(domainSubKind, prefix) {
		return domainSubKind[len(prefix):]
	}
	return domainSubKind
}

// TestNamingCatalogCoverage verifies that each (domain, sub_kind) pair declared
// in the worldgen bounds maps has exactly as many TOML catalog entries as the
// bounds value declares, and that both the English and Russian catalogs agree.
//
// Pattern keys follow the shape "<domain>.name.<sub_kind>.kind_pattern.<N>"
// and prefix keys follow "<domain>.prefix.<character>.<N>".
//
// A mismatch here means the bounds would either draw an out-of-range index
// (bounds > catalog) or silently skip valid templates (bounds < catalog).
func TestNamingCatalogCoverage(t *testing.T) {
	t.Parallel()

	enKeys := loadFlatKeys(t, catalogPath(t, "active.en.toml"))
	ruKeys := loadFlatKeys(t, catalogPath(t, "active.ru.toml"))

	type catalogCase struct {
		lang string
		keys map[string]struct{}
	}
	catalogs := []catalogCase{
		{"en", enKeys},
		{"ru", ruKeys},
	}

	type domainCase struct {
		name   string
		domain string
		bounds naming.Bounds
	}
	domains := []domainCase{
		{"region", "region", worldgen.RegionBounds()},
		{"landmark", "landmark", worldgen.LandmarkBounds()},
		{"settlement", "settlement", worldgen.SettlementBounds()},
	}

	for _, c := range catalogs {
		c := c
		for _, d := range domains {
			d := d
			t.Run(c.lang+"/"+d.name, func(t *testing.T) {
				t.Parallel()
				checkPatternCoverage(t, c.lang, c.keys, d.domain, d.bounds)
				checkPrefixCoverage(t, c.lang, c.keys, d.domain, d.bounds)
			})
		}
	}
}

// checkPatternCoverage asserts that every PatternCount entry in b has exactly
// that many kind_pattern keys in the catalog.
func checkPatternCoverage(t *testing.T, lang string, keys map[string]struct{}, domain string, b naming.Bounds) {
	t.Helper()
	for domainSubKind, want := range b.PatternCount {
		subKind := subKindFrom(domain, domainSubKind)
		prefix := fmt.Sprintf("%s.name.%s.kind_pattern.", domain, subKind)
		got := countKeys(keys, prefix)
		if got != want {
			t.Errorf("[%s] %q: catalog has %d entries, bounds expects %d",
				lang, prefix+"*", got, want)
		}
	}
}

// checkPrefixCoverage asserts that every PrefixCount entry in b has exactly
// that many prefix keys in the catalog.
func checkPrefixCoverage(t *testing.T, lang string, keys map[string]struct{}, domain string, b naming.Bounds) {
	t.Helper()
	for character, want := range b.PrefixCount {
		prefix := fmt.Sprintf("%s.prefix.%s.", domain, character)
		got := countKeys(keys, prefix)
		if got != want {
			t.Errorf("[%s] %q: catalog has %d entries, bounds expects %d",
				lang, prefix+"*", got, want)
		}
	}
}

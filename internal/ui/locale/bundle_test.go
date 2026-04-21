package locale

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// readCatalogKeys reads a catalog file from disk (not from the embed tree)
// and returns the set of fully-qualified message IDs it defines. Reading
// from disk exercises the on-disk source of truth and catches divergence
// from whatever the build embedded.
func readCatalogKeys(t *testing.T, path string) map[string]struct{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// TOML catalogs nest one table per message id: the top level maps an
	// id like "crossing.blighted" to a table containing "other", "one", ...
	// Unmarshal into a generic map and collect the top-level keys.
	var tree map[string]toml.Primitive
	if _, err := toml.Decode(string(raw), &tree); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	out := make(map[string]struct{}, len(tree))
	for k := range tree {
		out[k] = struct{}{}
	}
	return out
}

func catalogPath(t *testing.T, name string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, name)
}

// TestBundleKeyCoverage asserts that every message id defined in English is
// also defined in Russian. A diff in the failure message names the missing
// ids so translators can fix the gap without re-running the test.
// TestAllKeysNonEmpty ensures no Key* const is an empty string.
func TestAllKeysNonEmpty(t *testing.T) {
	t.Parallel()
	for _, k := range AllKeys() {
		if k == "" {
			t.Errorf("AllKeys() contains an empty string — a Key* constant is blank")
		}
	}
}

// flattenToml returns the set of fully-qualified dotted message IDs present in
// a TOML catalog. go-i18n message IDs map to TOML section headers like
// [crossing.blighted], which the TOML decoder parses as nested tables rather
// than top-level dotted keys. flattenToml walks the decoded tree recursively
// and stops at any node whose children are all non-map values (i.e. the
// go-i18n plural-form fields "other", "one", …). That node's dotted path is
// the message ID.
func flattenToml(tree map[string]any, prefix string, out map[string]struct{}) {
	for k, v := range tree {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		child, ok := v.(map[string]any)
		if !ok {
			// Leaf string value — the current prefix is the message ID.
			if prefix != "" {
				out[prefix] = struct{}{}
			}
			return
		}
		// Check whether any child is itself a nested table. If all children are
		// scalars this node is a go-i18n message table (contains "other", etc.).
		hasNestedTable := false
		for _, cv := range child {
			if _, isMap := cv.(map[string]any); isMap {
				hasNestedTable = true
				break
			}
		}
		if hasNestedTable {
			flattenToml(child, full, out)
		} else {
			// All children are plural-form fields — full is the message ID.
			out[full] = struct{}{}
		}
	}
}

// TestKeysPresentInCatalog iterates every Key* constant and asserts the key
// exists in active.en.toml. If a new constant is added to keys.go but the
// corresponding entry is missing from the catalog, this test fails with the
// offending key name so the gap is immediately obvious.
func TestKeysPresentInCatalog(t *testing.T) {
	t.Parallel()
	path := catalogPath(t, "active.en.toml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var tree map[string]any
	if _, err := toml.Decode(string(raw), &tree); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	keys := make(map[string]struct{})
	flattenToml(tree, "", keys)
	for _, key := range AllKeys() {
		if _, ok := keys[key]; !ok {
			t.Errorf("Key* constant %q is not present in active.en.toml", key)
		}
	}
}

func TestBundleKeyCoverage(t *testing.T) {
	t.Parallel()
	en := readCatalogKeys(t, catalogPath(t, "active.en.toml"))
	ru := readCatalogKeys(t, catalogPath(t, "active.ru.toml"))

	var missingInRu, missingInEn []string
	for k := range en {
		if _, ok := ru[k]; !ok {
			missingInRu = append(missingInRu, k)
		}
	}
	for k := range ru {
		if _, ok := en[k]; !ok {
			missingInEn = append(missingInEn, k)
		}
	}
	sort.Strings(missingInRu)
	sort.Strings(missingInEn)
	if len(missingInRu) > 0 {
		t.Errorf("keys present in en but missing in ru:\n  %s", strings.Join(missingInRu, "\n  "))
	}
	if len(missingInEn) > 0 {
		t.Errorf("keys present in ru but missing in en:\n  %s", strings.Join(missingInEn, "\n  "))
	}
}

func TestTrFallbackToEnglish(t *testing.T) {
	t.Parallel()
	got := Tr("xx-unknown", "status.you")
	want := Tr("en", "status.you")
	if got != want {
		t.Fatalf("Tr(unknown) = %q, want fallback to %q", got, want)
	}
	// Sanity: English returns "you", not the message ID.
	if want != "you" {
		t.Fatalf("Tr(\"en\", status.you) = %q, want \"you\"", want)
	}
}

func TestTrUnknownMessageID(t *testing.T) {
	t.Parallel()
	const id = "does.not.exist"
	got := Tr("en", id)
	if got != id {
		t.Fatalf("Tr(unknown id) = %q, want %q (messageID echoed back)", got, id)
	}
}

func TestTrTemplating(t *testing.T) {
	t.Parallel()
	got := Tr("en", "crossing.blighted", "Region", "The Elmwolds")
	want := "You feel the weight of The Elmwolds."
	if got != want {
		t.Fatalf("Tr templating = %q, want %q", got, want)
	}
}

func TestTrTemplatingRussian(t *testing.T) {
	t.Parallel()
	got := Tr("ru", "crossing.blighted", "Region", "Седолесье")
	want := "Ты чувствуешь тяжесть Седолесье."
	if got != want {
		t.Fatalf("Tr(ru) templating = %q, want %q", got, want)
	}
}

func TestDetect(t *testing.T) {
	// Not t.Parallel(): Setenv mutates process state.
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"ru_RU utf8", map[string]string{"LANG": "ru_RU.UTF-8"}, "ru"},
		{"en_US utf8", map[string]string{"LANG": "en_US.UTF-8"}, "en"},
		{"bare ru", map[string]string{"LANG": "ru"}, "ru"},
		{"unset", map[string]string{}, "en"},
		{"garbage", map[string]string{"LANG": "!!!"}, "en"},
		{"LC_ALL wins", map[string]string{"LC_ALL": "ru_RU.UTF-8", "LANG": "en"}, "ru"},
		{"LC_MESSAGES over LANG", map[string]string{"LC_MESSAGES": "ru", "LANG": "en"}, "ru"},
		{"C locale", map[string]string{"LANG": "C"}, "en"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Scrub every locale var so the test is self-contained.
			for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
				t.Setenv(k, "")
			}
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if got := Detect(); got != tc.want {
				t.Fatalf("Detect() = %q, want %q (env=%v)", got, tc.want, tc.env)
			}
		})
	}
}

func TestListReturnsCopy(t *testing.T) {
	t.Parallel()
	got := List()
	if len(got) == 0 {
		t.Fatal("List() returned empty slice")
	}
	got[0] = "mutated"
	again := List()
	if again[0] == "mutated" {
		t.Fatal("List() shares backing array — mutation leaked into package state")
	}
}

func TestPairsToMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []any
		want map[string]string
	}{
		{"empty", nil, nil},
		{"single pair", []any{"K", "V"}, map[string]string{"K": "V"}},
		{"odd length dropped", []any{"K"}, nil},
		{"non-string key dropped", []any{42, "V"}, nil},
		{"integer value stringified", []any{"N", 7}, map[string]string{"N": "7"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pairsToMap(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("pairsToMap(%v) = %v, want %v", tc.in, got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("pairsToMap(%v)[%q] = %q, want %q", tc.in, k, got[k], v)
				}
			}
		})
	}
}

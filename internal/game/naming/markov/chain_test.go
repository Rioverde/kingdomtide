package markov

import (
	"bufio"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// loadCorpusFile reads one of the bundled corpora off disk (not via
// embed so the test stays independent of the production embed wiring).
func loadCorpusFile(t *testing.T, rel string) []string {
	t.Helper()
	p := filepath.Join("data", "names", rel)
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("open %s: %v", p, err)
	}
	defer func() { _ = f.Close() }()

	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan %s: %v", p, err)
	}
	return out
}

// TestChainDeterminism builds two chains from the same corpus and
// drives them with identically-seeded PCGs — the 50 generated names
// must match byte-for-byte.
func TestChainDeterminism(t *testing.T) {
	corpus := loadCorpusFile(t, "en/blighted.txt")

	chainA, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}
	chainB, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	rngA := rand.New(rand.NewPCG(1, 1))
	rngB := rand.New(rand.NewPCG(1, 1))

	const n = 50
	for i := range n {
		a := chainA.Generate(rngA, 5, 11)
		b := chainB.Generate(rngB, 5, 11)
		if a != b {
			t.Fatalf("iteration %d: chainA.Generate = %q, chainB.Generate = %q", i, a, b)
		}
	}
}

// TestChainUniqueness asserts the generator produces a reasonable
// variety of names. The 500/10000 bound is deliberately loose — the
// purpose is flake resistance, not a distribution test.
func TestChainUniqueness(t *testing.T) {
	corpus := loadCorpusFile(t, "en/blighted.txt")
	chain, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	const total = 10000
	const minUnique = 500

	seen := make(map[string]struct{}, total)
	for i := range total {
		rng := rand.New(rand.NewPCG(uint64(i), uint64(i)^0xdeadbeef))
		name := chain.Generate(rng, 5, 11)
		seen[name] = struct{}{}
	}

	if len(seen) < minUnique {
		t.Fatalf("only %d unique names in %d samples (minimum %d) — chain "+
			"appears to have collapsed", len(seen), total, minUnique)
	}
}

// TestChainPronounceability smoke-checks that generated names avoid
// pathologically unreadable output: five-or-more consonant runs and
// three-or-more identical vowels in a row.
func TestChainPronounceability(t *testing.T) {
	corpus := loadCorpusFile(t, "en/fey.txt")
	chain, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	const total = 1000
	for i := range total {
		rng := rand.New(rand.NewPCG(uint64(i), 0xabcdef))
		name := chain.Generate(rng, 5, 11)

		if hasRunOfNonVowels(name, 5) {
			t.Fatalf("sample %d %q has 5+ consecutive consonants", i, name)
		}
		if hasIdenticalVowelRun(name, 3) {
			t.Fatalf("sample %d %q has 3+ identical vowels in a row", i, name)
		}
	}
}

// TestChainLength checks all generated names land inside a relaxed
// [minLen-2, maxLen+2] window.
func TestChainLength(t *testing.T) {
	corpus := loadCorpusFile(t, "en/holy.txt")
	chain, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	const minLen, maxLen = 5, 11
	const lo, hi = minLen - 2, maxLen + 2

	for i := range 1000 {
		rng := rand.New(rand.NewPCG(uint64(i), 0x1234))
		name := chain.Generate(rng, minLen, maxLen)
		if len([]rune(name)) < lo || len([]rune(name)) > hi {
			t.Fatalf("sample %d %q length %d outside [%d, %d]",
				i, name, len([]rune(name)), lo, hi)
		}
	}
}

// TestChainSmallCorpusErrors locks in the documented error contract:
// fewer than minCorpusSize usable entries returns a non-nil error.
func TestChainSmallCorpusErrors(t *testing.T) {
	cases := []struct {
		name   string
		corpus []string
	}{
		{"nil corpus", nil},
		{"two entries", []string{"a", "b"}},
		{"all too short", []string{"a", "b", "c", "d", "e"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chain, err := NewChain(tc.corpus)
			if err == nil {
				t.Fatalf("NewChain(%v) returned nil error, want non-nil", tc.corpus)
			}
			if chain != nil {
				t.Fatalf("NewChain(%v) returned non-nil chain with error", tc.corpus)
			}
		})
	}
}

// TestChainNonASCIICorpusFilter verifies that cleanWord preserves
// Unicode letters so the Russian corpus survives the cleaner, and that
// generated output never contains characters outside the union of
// input characters.
func TestChainNonASCIICorpusFilter(t *testing.T) {
	corpus := []string{
		"ashenfold", "mordhelm", "drakrel", "vornkar", "mourngrave",
		"sablereach", "пустошь", "скверна",
	}
	chain, err := NewChain(corpus)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	allowed := make(map[rune]struct{})
	for _, w := range corpus {
		for _, r := range strings.ToLower(w) {
			if unicode.IsLetter(r) || r == '\'' {
				allowed[r] = struct{}{}
			}
		}
	}

	for i := range 200 {
		rng := rand.New(rand.NewPCG(uint64(i), 0xfeedface))
		name := chain.Generate(rng, 5, 11)
		for _, r := range strings.ToLower(name) {
			if _, ok := allowed[r]; !ok {
				t.Fatalf("sample %d %q contains rune %q outside the corpus alphabet",
					i, name, r)
			}
		}
	}
}

// TestChainNilReceiver ensures a nil *Chain.Generate returns "" instead
// of panicking. Callers relying on ChainFor error paths may pass the
// nil result straight through; the generator must cope.
func TestChainNilReceiver(t *testing.T) {
	var c *Chain
	got := c.Generate(rand.New(rand.NewPCG(1, 1)), 5, 11)
	if got != "" {
		t.Fatalf("nil chain Generate = %q, want empty string", got)
	}
}

// hasRunOfNonVowels reports whether s contains n-or-more consecutive
// letters that are not English vowels. Non-letter runes reset the run.
func hasRunOfNonVowels(s string, n int) bool {
	run := 0
	for _, r := range strings.ToLower(s) {
		if !unicode.IsLetter(r) {
			run = 0
			continue
		}
		if isVowel(r) {
			run = 0
			continue
		}
		run++
		if run >= n {
			return true
		}
	}
	return false
}

// hasIdenticalVowelRun reports whether s contains n-or-more of the same
// vowel in a row.
func hasIdenticalVowelRun(s string, n int) bool {
	var prev rune
	run := 0
	for _, r := range strings.ToLower(s) {
		if isVowel(r) && r == prev {
			run++
			if run >= n {
				return true
			}
			continue
		}
		prev = r
		if isVowel(r) {
			run = 1
		} else {
			run = 0
		}
	}
	return false
}

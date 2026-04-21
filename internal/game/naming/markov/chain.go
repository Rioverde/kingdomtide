// Package markov owns the Markov-chain body generator used by the naming
// pipeline and, through it, by every language-aware display-name composer.
// The package is deliberately small: a pure Chain type built from a
// training corpus, plus a per-(lang, character) lookup for the embedded
// corpora in data/names/. No other package in the tree allocates a chain
// directly.
package markov

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"unicode"
)

// maxPrefixLen is the longest prefix the chain learns and uses at
// generation time. Three matches the Azgaar FMG names-generator default
// and produces noticeably more place-name-like output than length-2
// chains (which tend to look like random consonant clusters).
const maxPrefixLen = 3

// endMarker terminates a chain walk. A dollar sign is used because no
// valid letter in the corpus can collide with it — the pre-processor
// strips everything outside letters and apostrophes.
const endMarker = '$'

// minCorpusSize is the minimum number of usable entries required to
// build a chain. Below this the chain loses enough coverage that
// generation collapses into a handful of repeated words; the error
// surfaces the bad corpus at load time rather than at some later
// generation call.
const minCorpusSize = 5

// minWord and maxWord bound which corpus entries are accepted. Short
// entries produce low-information prefixes (a two-letter word offers at
// most one transition); very long entries skew the transition table
// toward their trailing syllables and rarely read as place names.
const (
	minWord = 3
	maxWord = 20
)

// maxGenerationAttempts caps how many times Generate will restart a walk
// that dead-ends before reaching minLen. The limit is intentionally
// small — a chain that cannot produce a minimum-length name in five
// tries usually indicates a pathological corpus, and the caller is
// better served by the best partial result than by an unbounded retry
// loop.
const maxGenerationAttempts = 5

// Chain is a precomputed transition table derived from a training
// corpus. transitions maps every observed 1..maxPrefixLen character
// prefix to the list of runes that may follow it; duplicates in the
// list act as weights so frequent transitions dominate generation
// naturally. starts holds every unique word-starting prefix
// (1..maxPrefixLen characters from the beginning of a training word) so
// Generate can pick a plausible opening syllable without biasing toward
// any specific length. prefixMax is stored rather than read from the
// constant so future chains with different prefix widths compose
// cleanly.
//
// A Chain is read-only after NewChain returns and therefore safe for
// concurrent use without additional synchronisation.
type Chain struct {
	transitions map[string][]rune
	starts      []string
	prefixMax   int
}

// NewChain builds a chain from corpus. Each entry is lowercased and
// stripped of characters that are not letters or apostrophes; entries
// that fall outside [minWord, maxWord] after cleaning are skipped.
// Returns an error when fewer than minCorpusSize entries survive
// cleaning — a chain that small cannot generate varied output and the
// caller should skip that (language, character) combination rather than
// serving degenerate names.
func NewChain(corpus []string) (*Chain, error) {
	cleaned := make([]string, 0, len(corpus))
	for _, raw := range corpus {
		w := cleanWord(raw)
		if len(w) < minWord || len(w) > maxWord {
			continue
		}
		cleaned = append(cleaned, w)
	}

	if len(cleaned) < minCorpusSize {
		return nil, fmt.Errorf("markov corpus: need at least %d usable entries, got %d",
			minCorpusSize, len(cleaned))
	}

	// Dedup starts while preserving insertion order so the resulting slice
	// is deterministic across runs with the same corpus order.
	transitions := make(map[string][]rune, len(cleaned)*4)
	startsSeen := make(map[string]struct{}, len(cleaned)*maxPrefixLen)
	starts := make([]string, 0, len(cleaned)*maxPrefixLen)

	for _, w := range cleaned {
		runes := []rune(w)
		addStarts(runes, startsSeen, &starts)
		recordTransitions(runes, transitions)
	}

	return &Chain{
		transitions: transitions,
		starts:      starts,
		prefixMax:   maxPrefixLen,
	}, nil
}

// cleanWord lowercases the input and drops every rune that is not a
// letter or apostrophe. Apostrophes are kept because they appear in a
// few real place names ("d'Evora") and removing them would fuse
// syllables.
func cleanWord(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		lr := unicode.ToLower(r)
		if unicode.IsLetter(lr) || lr == '\'' {
			b.WriteRune(lr)
		}
	}
	return b.String()
}

// addStarts records the 1..maxPrefixLen leading substrings of runes as
// valid word openings, deduped via seen.
func addStarts(runes []rune, seen map[string]struct{}, out *[]string) {
	maxLen := min(len(runes), maxPrefixLen)
	for l := 1; l <= maxLen; l++ {
		p := string(runes[:l])
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		*out = append(*out, p)
	}
}

// recordTransitions fills the transition table from a single word. For
// every position in runes and every prefix length 1..maxPrefixLen
// ending at that position, the next rune (or the end marker at the
// tail) is appended to the list for that prefix.
func recordTransitions(runes []rune, transitions map[string][]rune) {
	n := len(runes)
	for i := range n {
		maxLen := min(i+1, maxPrefixLen)
		for l := 1; l <= maxLen; l++ {
			prefix := string(runes[i+1-l : i+1])
			var next rune
			if i+1 < n {
				next = runes[i+1]
			} else {
				next = endMarker
			}
			transitions[prefix] = append(transitions[prefix], next)
		}
	}
}

// Generate walks the chain and returns a capitalised word whose length
// is usually within [minLen, maxLen]. The generator restarts from
// scratch up to maxGenerationAttempts times when a walk dead-ends
// before the minimum length; if all attempts fall short, the best
// (longest) partial result is returned so callers never see an empty
// string. Overlong walks are truncated at the nearest trailing vowel so
// the final syllable still reads as a plausible ending.
//
// The function is pure in rng — two calls with an identically-seeded
// *rand.Rand return the same string. A Chain itself is never mutated.
// A nil receiver returns the empty string so callers who gate on a
// missing (lang, character) pair do not need a nil check before each
// call.
func (c *Chain) Generate(rng *rand.Rand, minLen, maxLen int) string {
	if c == nil {
		return ""
	}
	if minLen < 1 {
		minLen = 1
	}
	if maxLen < minLen {
		maxLen = minLen
	}

	var best []rune
	for range maxGenerationAttempts {
		candidate := c.walk(rng, maxLen)
		if len(candidate) >= minLen {
			return capitalizeRunes(truncateAtVowel(candidate, maxLen))
		}
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	if len(best) == 0 {
		// Degenerate fallback: use the first available start so callers
		// never get back "". This branch is unreachable for any reasonably
		// sized corpus because at least one start always produces one
		// transition.
		best = []rune(c.starts[0])
	}
	return capitalizeRunes(truncateAtVowel(best, maxLen))
}

// walk performs a single Markov traversal up to maxLen runes. It picks
// a starting prefix uniformly from c.starts and then repeatedly extends
// by sampling from the transition list whose key is the last
// 1..prefixMax runes of the current word (trying the longest prefix
// first, falling back to shorter ones if the longer prefix has no
// entry).
func (c *Chain) walk(rng *rand.Rand, maxLen int) []rune {
	start := c.starts[rng.IntN(len(c.starts))]
	word := []rune(start)

	for len(word) < maxLen {
		next, ok := c.nextRune(word, rng)
		if !ok || next == endMarker {
			break
		}
		word = append(word, next)
	}
	return word
}

// nextRune looks up transitions for the longest suffix of word that
// exists in the table, falling back to shorter suffixes on miss.
// Returns ok=false when none of the suffixes have any recorded
// transition — the caller treats that as a natural end of the walk.
func (c *Chain) nextRune(word []rune, rng *rand.Rand) (rune, bool) {
	maxLen := min(len(word), c.prefixMax)
	for l := maxLen; l >= 1; l-- {
		key := string(word[len(word)-l:])
		list, ok := c.transitions[key]
		if !ok || len(list) == 0 {
			continue
		}
		return list[rng.IntN(len(list))], true
	}
	return 0, false
}

// truncateAtVowel clips word to at most maxLen runes, preferring to end
// at a vowel so the final syllable still sounds pronounceable. If there
// is no vowel in the acceptable tail window, the hard cap at maxLen
// applies.
func truncateAtVowel(word []rune, maxLen int) []rune {
	if len(word) <= maxLen {
		return word
	}
	// Scan back from maxLen for the first trailing vowel within a short
	// window. A window of 3 runes is enough to avoid chopping
	// mid-syllable on names like "Bramblemere" without creating multiple
	// valid endpoints.
	const lookback = 3
	lo := max(1, maxLen-lookback)
	for i := maxLen; i >= lo; i-- {
		if isVowel(word[i-1]) {
			return word[:i]
		}
	}
	return word[:maxLen]
}

// isVowel reports whether r is an English vowel. The Markov corpora are
// Latin-script after cleaning, so a fixed ASCII vowel set is adequate;
// the Russian corpus is latinised for the same reason.
func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

// capitalizeRunes returns the input with its leading rune upper-cased
// and the rest untouched. Works on empty input (returns "") so callers
// do not need a guard before invoking it.
func capitalizeRunes(w []rune) string {
	if len(w) == 0 {
		return ""
	}
	out := make([]rune, len(w))
	out[0] = unicode.ToUpper(w[0])
	copy(out[1:], w[1:])
	return string(out)
}

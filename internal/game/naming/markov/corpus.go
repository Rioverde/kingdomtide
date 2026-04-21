package markov

import (
	"bufio"
	"bytes"
	"embed"
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
)

// nameCorpora embeds every shipped naming-corpus text file. The "all:"
// prefix tells go:embed to include files starting with "_" or "." as
// well — not strictly needed today but cheap insurance against a future
// corpus file named e.g. "_common.txt".
//
//go:embed all:data/names
var nameCorpora embed.FS

// corporaRoot is the embed-tree subpath that contains per-language
// corpus directories. Extracted as a constant so tests and tooling can
// change it in one place if the layout ever moves.
const corporaRoot = "data/names"

// corpusInitialCap is the initial backing-array capacity for corpus
// lines when reading an embedded name corpus file. 64 entries covers
// every shipped corpus (each has 80-100 lines) without over-allocating
// for small files.
const corpusInitialCap = 64

// ErrChainNotFound is returned by ChainFor when the requested (lang,
// character) combination has no loaded chain. Exposing it as a sentinel
// lets callers distinguish "unknown locale / character" from a
// lower-level corpus-load failure without string matching.
var ErrChainNotFound = errors.New("markov chain not found")

// characters is the canonical per-(lang, character) set the package
// knows how to resolve. Each entry lists the corpus filename relative
// to data/names/<lang>/. The naming package keeps its own character
// enum; this slice is the source of truth for "which corpora ship with
// the binary" and is iterated at load time to populate the chain map.
//
// Kept as a plain slice rather than a map from an imported enum type so
// the markov package stays independent of the naming/game packages —
// callers resolve with a lowercase string key.
var characters = []string{
	"normal",
	"blighted",
	"fey",
	"ancient",
	"savage",
	"holy",
	"wild",
}

// chains stores the precomputed Markov chains keyed by language then by
// character. Populated lazily on first ChainFor call because project
// convention forbids I/O inside init(). sync.Once gives a race-free
// single-initialisation guarantee without a hand-rolled mutex.
var (
	chains     map[string]map[string]*Chain
	chainsOnce sync.Once
	chainsErr  error
)

// ChainFor resolves a (lang, character) pair to the precomputed Markov
// chain for that language's corpus. The first call triggers lazy
// loading of every embedded corpus via sync.Once; subsequent calls are
// map-lookups. Both arguments are case-sensitive lowercase keys —
// callers use "en"/"ru" for language and the RegionCharacter.Key()
// value for character (e.g. "blighted").
//
// Unknown languages or characters return (nil, ErrChainNotFound). The
// naming package tolerates a nil chain by emitting an empty Body; the
// client composer then resolves the locale fallback for the chosen
// format. A partially-loaded corpus (some (lang, character) pairs
// missing because their files failed to read) still returns a working
// chain for the pairs that did load.
func ChainFor(lang, character string) (*Chain, error) {
	chainsOnce.Do(loadChains)
	if chainsErr != nil {
		return nil, fmt.Errorf("markov chain load: %w", chainsErr)
	}
	byChar, ok := chains[lang]
	if !ok {
		return nil, fmt.Errorf("%w: lang %q", ErrChainNotFound, lang)
	}
	c, ok := byChar[character]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrChainNotFound, lang, character)
	}
	return c, nil
}

// loadChains reads every per-language/per-character corpus file out of
// the embedded FS and builds a Markov chain for each one. Called at
// most once per process via sync.Once. Per-language and per-character
// failures are logged and skipped so a partial corpus (e.g. a missing
// language directory) yields working chains for the languages that did
// load. chainsErr is set only when every language directory failed,
// i.e. nothing at all is usable.
func loadChains() {
	langs, err := nameCorpora.ReadDir(corporaRoot)
	if err != nil {
		chainsErr = fmt.Errorf("read embedded corpora root: %w", err)
		return
	}

	out := make(map[string]map[string]*Chain, len(langs))
	var loadErrors []string
	for _, entry := range langs {
		if !entry.IsDir() {
			continue
		}
		lang := entry.Name()
		byChar := make(map[string]*Chain, len(characters))
		for _, ch := range characters {
			file := ch + ".txt"
			corpus, err := readCorpus(path.Join(corporaRoot, lang, file))
			if err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("%s/%s: %v", lang, file, err))
				continue
			}
			chain, err := NewChain(corpus)
			if err != nil {
				loadErrors = append(loadErrors, fmt.Sprintf("%s/%s: %v", lang, file, err))
				continue
			}
			byChar[ch] = chain
		}
		if len(byChar) > 0 {
			out[lang] = byChar
		} else {
			loadErrors = append(loadErrors, fmt.Sprintf("%s: no characters loaded", lang))
		}
	}

	chains = out

	// Only signal a hard error when nothing at all loaded successfully.
	if len(out) == 0 {
		if len(loadErrors) > 0 {
			chainsErr = fmt.Errorf("all naming corpora failed to load: %s",
				strings.Join(loadErrors, "; "))
		} else {
			chainsErr = fmt.Errorf("no language directories found under %s", corporaRoot)
		}
	}
}

// readCorpus reads one corpus file and returns its non-empty trimmed
// lines. Keeps the parsing logic centralised so every chain is built
// from uniformly-preprocessed input.
func readCorpus(p string) ([]string, error) {
	raw, err := nameCorpora.ReadFile(p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, corpusInitialCap)
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

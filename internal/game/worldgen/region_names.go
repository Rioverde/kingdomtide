package worldgen

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"math/rand/v2"
	"path"
	"strings"
	"sync"

	"github.com/Rioverde/gongeons/internal/game"
)

// nameCorpora embeds every shipped naming-corpus text file. The "all:"
// prefix tells go:embed to include files starting with "_" or "." as well —
// not strictly needed today but cheap insurance against a future corpus
// file named e.g. "_common.txt".
//
//go:embed all:data/names
var nameCorpora embed.FS

// namingCorporaRoot is the embed-tree subpath that contains per-language
// corpus directories. Extracted as a constant so tests and tooling can
// change it in one place if the layout ever moves.
const namingCorporaRoot = "data/names"

// corpusInitialCap is the initial backing-array capacity for corpus lines
// when reading an embedded name corpus file. 64 entries covers every shipped
// corpus (each has 80-100 lines) without over-allocating for small files.
const corpusInitialCap = 64

// Regional geographical suffixes keyed by biome family. Each slice must be
// non-empty — the name formatter picks a suffix uniformly and any empty
// bucket would silently fall back to the body-only format. FamilyUnknown
// gets a single generic term so unmapped terrains still produce a
// plausible "The X Reach".
var geoTermsByBiomeFamily = map[BiomeFamily][]string{
	FamilyForest:   {"Thicket", "Weald", "Wood", "Glade", "Holt"},
	FamilyPlain:    {"Wolds", "Downs", "Reach", "Heath", "Moor"},
	FamilyMountain: {"Peaks", "Crags", "Spurs", "Heights"},
	FamilyWater:    {"Mere", "Fen", "Sound", "Marshes"},
	FamilyDesert:   {"Wastes", "Dunes", "Barrens", "Sands"},
	FamilyTundra:   {"Barrens", "Steppes", "Wastes"},
	FamilyUnknown:  {"Reach"},
}

// characterCorpusFile is the canonical filename used for each region
// character inside a per-language directory. Keeping this map beside
// loadNamingChains ties the corpus-on-disk layout to the RegionCharacter
// enum in one place.
var characterCorpusFile = map[game.RegionCharacter]string{
	game.RegionNormal:   "normal.txt",
	game.RegionBlighted: "blighted.txt",
	game.RegionFey:      "fey.txt",
	game.RegionAncient:  "ancient.txt",
	game.RegionSavage:   "savage.txt",
	game.RegionHoly:     "holy.txt",
	game.RegionWild:     "wild.txt",
}

// namingChains stores the precomputed Markov chains keyed by language then
// by character. Populated lazily on first RegionName call because project
// convention forbids I/O inside init(). sync.Once gives a race-free
// single-initialisation guarantee without a hand-rolled mutex.
var (
	namingChains     map[string]map[game.RegionCharacter]*markovChain
	namingChainsOnce sync.Once
	namingChainsErr  error
)

// regionNameMinLen and regionNameMaxLen bound the Markov walk's target
// length. Numbers match the Phase-1 plan; shorter than regionNameMinLen
// tends to produce one-syllable noise, longer than regionNameMaxLen runs
// past the UI allotment in the viewport status bar.
const (
	regionNameMinLen = 5
	regionNameMaxLen = 11
)

// defaultNamingLanguage is the hardcoded locale used by RegionName until
// Sub-phase 1d threads language through from ClientHello. Keeping it as a
// named constant makes the eventual swap a one-line change.
const defaultNamingLanguage = "en"

// hashCoordPrimeX and hashCoordPrimeY are large odd primes used to mix the
// two components of a SuperChunkCoord into a single 64-bit seed input. They
// are distinct so (a, b) and (b, a) don't collide; both values fit in
// uint64 without hitting the sign bit.
const (
	hashCoordPrimeX uint64 = 0x9e3779b185ebca87
	hashCoordPrimeY uint64 = 0xc2b2ae3d27d4eb4f
)

// RegionName produces a deterministic display name for a region. Same
// (character, biome, seed, sc) inputs always return the same string;
// different seeds or super-chunk coords produce different names without
// any persistence layer.
//
// The formatter picks one of two shapes with equal probability:
//
//	body only — "Gloomfold"
//	prefixed  — "The Gloomfold Moor"
//
// The biome-suffix option falls back to body-only if the family has no
// suffix list defined, so adding new BiomeFamily constants cannot break
// naming in the interim.
func RegionName(
	character game.RegionCharacter,
	biome BiomeFamily,
	seed int64,
	sc game.SuperChunkCoord,
) string {
	// TODO(Rioverde): thread language through from ClientHello (Sub-phase 1d).
	lang := defaultNamingLanguage

	namingChainsOnce.Do(loadNamingChains)
	if namingChainsErr != nil {
		// Fallback keeps the server alive even if the embed tree got
		// mangled at build time. The returned string still encodes enough
		// coord information to tell regions apart in logs.
		return fmt.Sprintf("Region (%d,%d)", sc.X, sc.Y)
	}

	byChar, ok := namingChains[lang]
	if !ok {
		return fmt.Sprintf("Region (%d,%d)", sc.X, sc.Y)
	}
	chain, ok := byChar[character]
	if !ok {
		// Unknown character also falls back — the caller is still served
		// a usable identifier.
		return fmt.Sprintf("Region (%d,%d)", sc.X, sc.Y)
	}

	rng := rand.New(rand.NewPCG(uint64(seed), hashCoord(sc)))

	body := chain.Generate(rng, regionNameMinLen, regionNameMaxLen)

	if rng.IntN(2) == 0 {
		return body
	}
	geos, ok := geoTermsByBiomeFamily[biome]
	if !ok || len(geos) == 0 {
		return body
	}

	var b strings.Builder
	b.Grow(len("The ") + len(body) + 1 + len(geos[0]))
	b.WriteString("The ")
	b.WriteString(body)
	b.WriteByte(' ')
	b.WriteString(geos[rng.IntN(len(geos))])
	return b.String()
}

// loadNamingChains reads every per-language/per-character corpus file out
// of the embedded FS and builds a Markov chain for each one. Called at
// most once per process via sync.Once. Per-language and per-character
// failures are logged and skipped so a partial corpus (e.g. a missing
// language directory) yields working chains for the languages that did
// load. namingChainsErr is set only when every language directory failed,
// i.e. nothing at all is usable.
func loadNamingChains() {
	langs, err := nameCorpora.ReadDir(namingCorporaRoot)
	if err != nil {
		namingChainsErr = fmt.Errorf("read embedded corpora root: %w", err)
		return
	}

	chains := make(map[string]map[game.RegionCharacter]*markovChain, len(langs))
	var langErrors []string
	for _, entry := range langs {
		if !entry.IsDir() {
			continue
		}
		lang := entry.Name()
		byChar := make(map[game.RegionCharacter]*markovChain, len(characterCorpusFile))
		for character, file := range characterCorpusFile {
			corpus, err := readEmbeddedCorpus(path.Join(namingCorporaRoot, lang, file))
			if err != nil {
				// Log and skip this (lang, character) pair; other chars in this
				// language still load normally.
				langErrors = append(langErrors, fmt.Sprintf("%s/%s: %v", lang, file, err))
				continue
			}
			chain, err := newMarkovChain(corpus)
			if err != nil {
				langErrors = append(langErrors, fmt.Sprintf("%s/%s: %v", lang, file, err))
				continue
			}
			byChar[character] = chain
		}
		if len(byChar) > 0 {
			chains[lang] = byChar
		} else {
			langErrors = append(langErrors, fmt.Sprintf("%s: no characters loaded", lang))
		}
	}

	// Assign whatever we managed to load. Partial coverage (some languages or
	// characters missing) is always better than zero coverage.
	namingChains = chains

	// Only signal a hard error when nothing at all loaded successfully.
	if len(chains) == 0 {
		if len(langErrors) > 0 {
			namingChainsErr = fmt.Errorf("all naming corpora failed to load: %s", strings.Join(langErrors, "; "))
		} else {
			namingChainsErr = fmt.Errorf("no language directories found under %s", namingCorporaRoot)
		}
	}
}

// readEmbeddedCorpus reads one corpus file and returns its non-empty
// trimmed lines. Keeps the parsing logic centralised so every chain is
// built from uniformly-preprocessed input.
func readEmbeddedCorpus(p string) ([]string, error) {
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

// hashCoord mixes the two components of a SuperChunkCoord into a single
// uint64 suitable for seeding rand.NewPCG's second input. Multiplying each
// component by a distinct large prime breaks the (a, b) vs (b, a)
// symmetry; XOR then folds the two streams together. Signed-to-unsigned
// conversion preserves the full bit pattern.
func hashCoord(sc game.SuperChunkCoord) uint64 {
	return uint64(int64(sc.X))*hashCoordPrimeX ^ uint64(int64(sc.Y))*hashCoordPrimeY
}

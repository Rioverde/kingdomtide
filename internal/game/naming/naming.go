// Package naming provides deterministic, language-agnostic structured
// names for world entities (regions, landmarks, settlements). Names are
// generated as a Parts record — a Format tag, two catalog indices, and
// a BodySeed — so the client can compose the final display string by
// feeding BodySeed into its embedded per-language Markov corpus and
// looking up locale keys under "<domain>.name.<sub_kind>.kind_pattern.<idx>"
// and "<domain>.prefix.<character>.<idx>".
//
// Same (Seed, Coord, Domain, Character, SubKind) always yields the same
// Parts across runs and across languages: Format, the two indices, and
// BodySeed are identical; only the Body letters the client renders
// differ by language. This invariant is what allows Russian and English
// clients to see structurally equivalent names drawn from the same
// world without the server ever thinking about language.
package naming

import (
	"math/rand/v2"
	"sync"

	"github.com/Rioverde/gongeons/internal/game/naming/parts"
)

// Domain is the namespace prefix for locale keys used by the client
// when composing names from Parts. Each Domain corresponds to a family
// of named entities (regions, landmarks, settlements) that share the
// same composition machinery but resolve different catalog keys.
type Domain string

// Known Domain values. Unknown domains are accepted by Generate but
// fall back to saltRegion for the PCG stream — tests forbid that path
// for any production call.
const (
	DomainRegion     Domain = "region"
	DomainLandmark   Domain = "landmark"
	DomainSettlement Domain = "settlement"
)

// String implements fmt.Stringer so Domain values print as their
// namespace prefix in logs and error messages.
func (d Domain) String() string { return string(d) }

// Format aliases the leaf type so callers continue to reference
// naming.Format regardless of where the type actually lives.
type Format = parts.Format

// Format constants re-exported from the parts leaf package. Kept as
// named constants in the naming namespace so existing callers continue
// to compile against naming.FormatX without chasing the leaf package.
const (
	FormatBodyOnly        = parts.FormatBodyOnly
	FormatCharacterPrefix = parts.FormatCharacterPrefix
	FormatKindPattern     = parts.FormatKindPattern
)

// Parts aliases the leaf type so game.Landmark.Name and
// naming.Generate return the same concrete type without an import
// cycle between naming and game.
type Parts = parts.Parts

// Input holds the caller-supplied parameters. (CoordX, CoordY) drive
// PCG seeding — same (Seed, CoordX, CoordY, Domain, Character, SubKind)
// always produces the same Parts regardless of locale. Coord is carried
// as two ints rather than a shared game.Position type so the naming
// package remains independent of the game domain (no import cycle).
// Lang is intentionally absent: language is a display concern handled
// client-side by composing BodySeed with a local corpus.
type Input struct {
	Domain    Domain
	Character string
	SubKind   string
	Seed      int64
	CoordX    int
	CoordY    int
}

// Bounds caps PrefixIndex/PatternIndex draws to the available catalog
// entries. A zero count for a key forces Format to downgrade to
// FormatBodyOnly when the selected format would otherwise require that
// zero-bound index.
type Bounds struct {
	// PatternCount is indexed by "<domain>.<sub_kind>" (e.g. "region.forest").
	PatternCount map[string]int
	// PrefixCount is indexed by the Character key (e.g. "blighted").
	PrefixCount map[string]int
}

// Weights is the per-domain format-selection weighting. The three
// fields are expected to sum to 100; non-100 sums are accepted and
// normalized at draw time by treating the observed sum as the
// denominator.
type Weights struct {
	BodyOnly        int
	CharacterPrefix int
	KindPattern     int
}

// DefaultWeights is the format-selection distribution applied when
// SetDomainWeights has not been called for the requested domain.
// 40/40/20 is calibrated so BodyOnly and CharacterPrefix each
// contribute roughly the same share while KindPattern stays less
// frequent — KindPattern templates carry the strongest domain flavour
// and should not dominate.
var DefaultWeights = Weights{BodyOnly: 40, CharacterPrefix: 40, KindPattern: 20}

// weights holds the per-domain overrides configured via
// SetDomainWeights. Reads and writes are guarded by weightsMu. A
// sync.Map would also work here but the access pattern (infrequent
// write at init, hot read in Generate) fits a plain mutex-protected
// map better.
var (
	weightsMu sync.RWMutex
	weights   = map[Domain]Weights{}
)

// SetDomainWeights overrides the format-selection distribution for a
// single domain. Callers typically invoke this once at process start.
// Unset domains use DefaultWeights. Zero-sum weight vectors are
// rejected (panic) because they would make the format selector
// non-deterministic.
func SetDomainWeights(d Domain, w Weights) {
	if w.BodyOnly+w.CharacterPrefix+w.KindPattern <= 0 {
		panic("naming: SetDomainWeights sum must be positive")
	}
	weightsMu.Lock()
	weights[d] = w
	weightsMu.Unlock()
}

// weightsFor returns the effective weights for d, falling back to
// DefaultWeights when no override has been registered.
func weightsFor(d Domain) Weights {
	weightsMu.RLock()
	w, ok := weights[d]
	weightsMu.RUnlock()
	if ok {
		return w
	}
	return DefaultWeights
}

// hashCoordPrimeX and hashCoordPrimeY are large odd primes used to fold
// Coord.X and Coord.Y into a single uint64 input for rand.NewPCG. They
// are distinct from each other so (a, b) and (b, a) cannot collide,
// and distinct from the worldgen hashCoord primes so the naming PCG
// stream is not correlated with the region-name or landmark-placement
// streams.
//
// Values come from the fractional hex of sqrt(2) and sqrt(3)
// respectively.
const (
	hashCoordPrimeX uint64 = 0x6a09e667f3bcc908
	hashCoordPrimeY uint64 = 0xbb67ae8584caa73b
)

// Generate produces a Parts record deterministic on (Seed, Coord,
// Domain, Character, SubKind). The PCG stream is consumed in a fixed
// order — Format, PrefixIndex, PatternIndex, then BodySeed — so every
// caller across languages sees identical structural indices and the
// same BodySeed.
//
// When the chosen format would require an index whose bound is zero
// (no catalog entries), the format degrades to FormatBodyOnly. That
// keeps Generate usable during catalog ramp-up without silently drawing
// from an empty list.
//
// Generate is pure: it performs no I/O, no body generation, and takes
// no language argument. The client produces a Body string at render
// time by seeding its embedded Markov chain with BodySeed.
func Generate(in Input, b Bounds) Parts {
	rng := newPCG(in)

	format := pickFormat(rng, weightsFor(in.Domain))

	prefixMax := b.PrefixCount[in.Character]
	prefixIdx := drawIndex(rng, prefixMax)

	patternKey := string(in.Domain) + "." + in.SubKind
	patternMax := b.PatternCount[patternKey]
	patternIdx := drawIndex(rng, patternMax)

	format = downgradeFormat(format, prefixMax, patternMax)

	// Drawing one more 64-bit value off the same stream gives the body
	// seed. Clients that build their own PCG from this seed see the same
	// starting state across languages, so Format + indices + BodySeed are
	// fully decorrelated by the three earlier draws yet reproducible
	// everywhere. The value is returned as int64 so Parts stays wire
	// compatible with the NameParts.body_seed proto field.
	bodySeed := int64(rng.Uint64())

	return Parts{
		Character:    in.Character,
		SubKind:      in.SubKind,
		Format:       format,
		PrefixIndex:  prefixIdx,
		PatternIndex: patternIdx,
		BodySeed:     bodySeed,
	}
}

// newPCG builds the deterministic RNG. The state word mixes Seed with
// the domain salt; the stream word mixes the two coord components with
// distinct primes so (X, Y) and (Y, X) cannot collide.
func newPCG(in Input) *rand.Rand {
	state := uint64(in.Seed) ^ uint64(domainSalt(in.Domain))
	stream := uint64(int64(in.CoordX))*hashCoordPrimeX ^
		uint64(int64(in.CoordY))*hashCoordPrimeY
	return rand.New(rand.NewPCG(state, stream))
}

// pickFormat draws a weighted format choice. The three cumulative
// thresholds are compared against a uniform draw in [0, sum).
// Zero-weight formats are unreachable.
func pickFormat(rng *rand.Rand, w Weights) Format {
	sum := w.BodyOnly + w.CharacterPrefix + w.KindPattern
	if sum <= 0 {
		return FormatBodyOnly
	}
	r := rng.IntN(sum)
	if r < w.BodyOnly {
		return FormatBodyOnly
	}
	if r < w.BodyOnly+w.CharacterPrefix {
		return FormatCharacterPrefix
	}
	return FormatKindPattern
}

// drawIndex consumes one PCG draw regardless of whether the resulting
// index is used. This is intentional — the PCG stream must advance by
// the same number of steps across languages so structural indices line
// up even when downstream code discards one of them. The uint8 cap is
// safe because every realistic catalog has fewer than 256 entries per
// bucket.
func drawIndex(rng *rand.Rand, bound int) uint8 {
	if bound <= 0 {
		// Advance the stream to keep the draw order fixed.
		_ = rng.IntN(1)
		return 0
	}
	return uint8(rng.IntN(bound))
}

// downgradeFormat collapses the chosen format to FormatBodyOnly when
// its dependency catalog is empty. Bounds with zero entries for the
// selected format cannot produce a valid index, and the client would
// fall back to Body anyway — doing the downgrade here makes the intent
// explicit.
func downgradeFormat(f Format, prefixMax, patternMax int) Format {
	switch f {
	case FormatCharacterPrefix:
		if prefixMax <= 0 {
			return FormatBodyOnly
		}
	case FormatKindPattern:
		if patternMax <= 0 {
			return FormatBodyOnly
		}
	}
	return f
}

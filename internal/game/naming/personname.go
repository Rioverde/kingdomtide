package naming

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming/markov"
)

// saltBodyRuler and saltBodySettlement are the per-role PCG salts applied
// to the Markov body-seed walk. Values are the fractional hex of sqrt(89)
// and sqrt(97) respectively — same primes used in worldgen/tuning.go so
// the streams are compatible with the legacy per-package constants they
// supersede.
const (
	saltBodyRuler      uint64 = 0x4f6b8c2e1a3d7059
	saltBodySettlement uint64 = 0x2c3e7a1f5b2d9064
)

// bodyPCG constructs a seeded PCG whose stream word is derived via
// Splitmix64 from state. This matches the worldgen.newPCG construction
// so callers that migrated from worldgen helpers produce identical
// output.
func bodyPCG(state uint64) *rand.Rand {
	stream := geom.Splitmix64(state ^ 0x94d049bb133111eb)
	return rand.New(rand.NewPCG(state, stream))
}

// GenerateRulerName returns a procedurally-generated personal name for a
// settlement's founding ruler or successor. Deterministic on (seed,
// position, region): same inputs always yield the same string. Walks the
// embedded "en" Markov corpus whose character matches region so e.g.
// Holy regions draw from Holy-character names. Returns a non-empty string
// even when the corpus is unavailable.
func GenerateRulerName(seed int64, position geom.Position, region string) string {
	parts := Generate(Input{
		Domain:    DomainRegion,
		Character: region,
		SubKind:   "ruler",
		Seed:      seed ^ int64(geom.PackPos(position)),
		CoordX:    position.X,
		CoordY:    position.Y,
	}, Bounds{})

	chain, err := markov.ChainFor("en", region)
	if err != nil {
		return fallbackName(parts.BodySeed)
	}
	rng := bodyPCG(uint64(parts.BodySeed) ^ saltBodyRuler)
	return chain.Generate(rng, 4, 9)
}

// GenerateSettlementName returns a procedurally-generated place name for
// a camp or hamlet. Uses the same Markov pipeline as GenerateRulerName
// but salted with saltBodySettlement so the place name differs from its
// founding ruler's name even when drawn from the same corpus.
// Deterministic on (seed, position, region).
func GenerateSettlementName(seed int64, position geom.Position, region string) string {
	parts := Generate(Input{
		Domain:    DomainSettlement,
		Character: region,
		SubKind:   "camp",
		Seed:      seed ^ int64(geom.PackPos(position)),
		CoordX:    position.X,
		CoordY:    position.Y,
	}, Bounds{})

	chain, err := markov.ChainFor("en", region)
	if err != nil {
		return fallbackName(parts.BodySeed ^ int64(saltBodySettlement))
	}
	rng := bodyPCG(uint64(parts.BodySeed) ^ saltBodySettlement)
	return chain.Generate(rng, 4, 9)
}

// fallbackName synthesises a short vowel-consonant name from seed when
// the Markov corpus is unavailable. Never returns an empty string.
func fallbackName(seed int64) string {
	const consonants = "bcdfghjklmnprstvwx"
	const vowels = "aeiou"
	n := uint64(seed)
	buf := [5]byte{
		consonants[n%uint64(len(consonants))],
		vowels[(n>>8)%uint64(len(vowels))],
		consonants[(n>>16)%uint64(len(consonants))],
		vowels[(n>>24)%uint64(len(vowels))],
		consonants[(n>>32)%uint64(len(consonants))],
	}
	if buf[0] >= 'a' && buf[0] <= 'z' {
		buf[0] -= 32
	}
	return string(buf[:])
}

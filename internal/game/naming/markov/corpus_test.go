package markov

import (
	"errors"
	"math/rand/v2"
	"testing"
)

// TestChainForKnownPairs covers the happy path: every shipped (lang,
// character) pair must resolve to a usable chain whose first generated
// name is non-empty.
func TestChainForKnownPairs(t *testing.T) {
	langs := []string{"en", "ru"}
	for _, lang := range langs {
		for _, ch := range characters {
			t.Run(lang+"/"+ch, func(t *testing.T) {
				chain, err := ChainFor(lang, ch)
				if err != nil {
					t.Fatalf("ChainFor(%q, %q): %v", lang, ch, err)
				}
				if chain == nil {
					t.Fatalf("ChainFor(%q, %q) chain is nil", lang, ch)
				}
				rng := rand.New(rand.NewPCG(1, 1))
				if got := chain.Generate(rng, 5, 11); got == "" {
					t.Fatalf("ChainFor(%q, %q).Generate = empty", lang, ch)
				}
			})
		}
	}
}

// TestChainForUnknownLang returns ErrChainNotFound wrapped with lang
// context. Callers rely on errors.Is to distinguish this from a
// corpus-load failure.
func TestChainForUnknownLang(t *testing.T) {
	_, err := ChainFor("zz", "blighted")
	if !errors.Is(err, ErrChainNotFound) {
		t.Fatalf("ChainFor unknown lang err = %v, want ErrChainNotFound", err)
	}
}

// TestChainForUnknownCharacter returns ErrChainNotFound wrapped with
// the missing character key.
func TestChainForUnknownCharacter(t *testing.T) {
	_, err := ChainFor("en", "imaginary")
	if !errors.Is(err, ErrChainNotFound) {
		t.Fatalf("ChainFor unknown character err = %v, want ErrChainNotFound", err)
	}
}

// TestChainForDeterministicOutput pins the invariant that two lookups
// return chains that produce identical sequences under identically-seeded
// rngs. This catches accidental per-call copy/rebuild regressions.
func TestChainForDeterministicOutput(t *testing.T) {
	a, err := ChainFor("en", "blighted")
	if err != nil {
		t.Fatalf("ChainFor: %v", err)
	}
	b, err := ChainFor("en", "blighted")
	if err != nil {
		t.Fatalf("ChainFor: %v", err)
	}

	rngA := rand.New(rand.NewPCG(7, 13))
	rngB := rand.New(rand.NewPCG(7, 13))
	for i := range 20 {
		ga := a.Generate(rngA, 5, 11)
		gb := b.Generate(rngB, 5, 11)
		if ga != gb {
			t.Fatalf("iter %d: %q vs %q — chains diverged across lookups", i, ga, gb)
		}
	}
}

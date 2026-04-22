package ui

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/naming/markov"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// Markov walk target length for display bodies. Mirrors the bounds
// used by the server-side naming callers in Phase 1/2 so retrofitted
// names keep their visual rhythm.
const (
	bodyMinLen = 5
	bodyMaxLen = 11
)

// bodySeedStream is a fixed second word for rand.NewPCG. Combined with
// the wire body_seed it produces a deterministic PCG stream that
// depends only on Parts.BodySeed. Any constant would work — this is
// the fractional hex of sqrt(17), a nothing-up-my-sleeve value kept
// stable so repeated renders of the same NameParts always produce
// identical body text.
const bodySeedStream uint64 = 0x4e5e1b6f0c0f2a40

// composeName renders the final display string for a NameParts record
// in the caller's language. domain is the catalog namespace (naming.DomainRegion,
// naming.DomainLandmark, etc.). A nil or zero-value parts record renders as ""
// so call sites can decide whether to show a placeholder.
func composeName(domain naming.Domain, p *pb.NameParts, lang string) string {
	if p == nil {
		return ""
	}
	body := localBody(p, lang)
	switch p.GetFormat() {
	case pb.NameFormat_NAME_FORMAT_BODY_ONLY:
		return body
	case pb.NameFormat_NAME_FORMAT_CHARACTER_PREFIX:
		prefixKey := prefixKeyFor(domain, p.GetCharacter(), uint8(p.GetPrefixIndex()))
		prefix := locale.Tr(lang, prefixKey)
		charPrefixKey := characterPrefixKeyFor(domain)
		return locale.Tr(lang, charPrefixKey, locale.ArgPrefix, prefix, locale.ArgBody, body)
	case pb.NameFormat_NAME_FORMAT_KIND_PATTERN:
		patternKey := patternKeyFor(domain, p.GetSubKind(), uint8(p.GetPatternIndex()))
		return locale.Tr(lang, patternKey, locale.ArgBody, body)
	}
	return body
}

// prefixKeyFor returns the typed catalog key for the character-prefix
// catalog entry, dispatching on domain. Returns an empty string for
// unknown domains — composeName falls back to echoing the key, which
// surfaces as a visible gap rather than a silent blank.
func prefixKeyFor(d naming.Domain, character string, idx uint8) string {
	switch d {
	case naming.DomainRegion:
		return locale.RegionPrefixKey(character, idx)
	case naming.DomainLandmark:
		return locale.LandmarkPrefixKey(character, idx)
	case naming.DomainSettlement:
		return locale.SettlementPrefixKey(character, idx)
	}
	return "" // unreachable for any production Domain
}

// patternKeyFor returns the typed catalog key for the kind-pattern
// template entry, dispatching on domain.
func patternKeyFor(d naming.Domain, subKind string, idx uint8) string {
	switch d {
	case naming.DomainRegion:
		return locale.RegionNamePatternKey(subKind, idx)
	case naming.DomainLandmark:
		return locale.LandmarkNamePatternKey(subKind, idx)
	case naming.DomainSettlement:
		return locale.SettlementNamePatternKey(subKind, idx)
	}
	return "" // unreachable for any production Domain
}

// characterPrefixKeyFor returns the catalog key for the
// FormatCharacterPrefix assembly template ("{{.Prefix}} {{.Body}}"),
// dispatching on domain.
func characterPrefixKeyFor(d naming.Domain) string {
	switch d {
	case naming.DomainRegion:
		return locale.KeyRegionNameCharacterPrefix
	case naming.DomainLandmark:
		return locale.KeyLandmarkNameCharacterPrefix
	case naming.DomainSettlement:
		return locale.KeySettlementNameCharacterPrefix
	}
	return "" // unreachable for any production Domain
}

// localBody reproduces the language-specific body text from a
// NameParts record by seeding the client's embedded Markov chain with
// BodySeed. Returns "" when the (lang, character) combination has no
// loaded corpus — the caller composes a usable display string via the
// catalog fallback without a body.
func localBody(p *pb.NameParts, lang string) string {
	chain, err := markov.ChainFor(lang, p.GetCharacter())
	if err != nil || chain == nil {
		return ""
	}
	rng := rand.New(rand.NewPCG(uint64(p.GetBodySeed()), bodySeedStream))
	return chain.Generate(rng, bodyMinLen, bodyMaxLen)
}

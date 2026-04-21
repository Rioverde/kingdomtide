package ui

import (
	"math/rand/v2"
	"strconv"

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
	d := string(domain)
	body := localBody(p, lang)
	switch p.GetFormat() {
	case pb.NameFormat_NAME_FORMAT_BODY_ONLY:
		return body
	case pb.NameFormat_NAME_FORMAT_CHARACTER_PREFIX:
		prefix := locale.Tr(lang,
			d+".prefix."+p.GetCharacter()+"."+strconv.Itoa(int(p.GetPrefixIndex())))
		return locale.Tr(lang, d+".name.character_prefix",
			locale.ArgPrefix, prefix, locale.ArgBody, body)
	case pb.NameFormat_NAME_FORMAT_KIND_PATTERN:
		key := d + ".name." + p.GetSubKind() + ".kind_pattern." +
			strconv.Itoa(int(p.GetPatternIndex()))
		return locale.Tr(lang, key, locale.ArgBody, body)
	}
	return body
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

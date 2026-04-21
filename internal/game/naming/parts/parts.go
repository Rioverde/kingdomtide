// Package parts carries the leaf types shared between the naming
// pipeline and the domain model. Splitting Parts and Format out of the
// main naming package lets internal/game embed a naming Parts record
// without pulling the naming package back through an import cycle —
// naming itself depends on nothing from the game domain, and game
// depends on this dependency-free leaf for its Region.Name and
// Landmark.Name fields.
package parts

// Format selects which composition template the client uses to render
// a Parts record. The numeric values are part of the wire contract (see
// the NameFormat proto enum) — do not renumber.
type Format uint8

// Format values, in the same order as the NameFormat proto enum.
const (
	FormatBodyOnly        Format = iota // "{{.Body}}"
	FormatCharacterPrefix               // "{{.Prefix}} {{.Body}}"
	FormatKindPattern                   // domain + sub-kind specific template
)

// String returns the short identifier used in catalog keys and logs.
func (f Format) String() string {
	switch f {
	case FormatBodyOnly:
		return "body_only"
	case FormatCharacterPrefix:
		return "character_prefix"
	case FormatKindPattern:
		return "kind_pattern"
	}
	return "unknown"
}

// Parts is the language-agnostic structured output of the naming
// pipeline. The server ships these structured values on the wire
// verbatim; the client composes the final display string locally using
// its embedded Markov corpora and locale catalogs. No Body string is
// ever produced or transmitted — BodySeed is the PCG seed the client
// feeds into its own Chain to regenerate a language-specific body.
type Parts struct {
	// Character is the thematic key that drives prefix lookup (e.g.
	// "blighted", "fey", "normal"). Empty when no prefix is needed.
	Character string

	// SubKind is the domain-specific sub-category that drives kind-pattern
	// lookup (e.g. "forest" for regions, "tower" for landmarks, "village"
	// for settlements).
	SubKind string

	// Format picks the composition template.
	Format Format

	// PrefixIndex selects one entry from the character prefix catalog.
	// Meaningful only when Format == FormatCharacterPrefix.
	PrefixIndex uint8

	// PatternIndex selects one entry from the sub-kind pattern catalog.
	// Meaningful only when Format == FormatKindPattern.
	PatternIndex uint8

	// BodySeed seeds the client-side Markov walk that produces the
	// language-specific Body text at render time. Deterministic on
	// (Seed, Coord, Domain, Character, SubKind): every language's client
	// derives the same BodySeed and therefore feeds its local corpus
	// from the same starting PCG state. Format + indices stay identical
	// across languages by the same invariant.
	BodySeed int64
}

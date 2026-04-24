package polity

// Culture is a civilization archetype per §7d. Used to drive the
// Mulk-cycle assimilation mechanic and will in future milestones
// influence succession law defaults and stat biases.
type Culture uint8

const (
	CultureFeudal Culture = iota
	CultureSteppe
	CultureCeltic
	CultureRepublican
	CultureImperial
	CultureNorthernFeudal
)

// String returns the dev-only English name.
func (c Culture) String() string {
	switch c {
	case CultureFeudal:
		return "Feudal"
	case CultureSteppe:
		return "Steppe"
	case CultureCeltic:
		return "Celtic"
	case CultureRepublican:
		return "Republican"
	case CultureImperial:
		return "Imperial"
	case CultureNorthernFeudal:
		return "NorthernFeudal"
	default:
		return "UnknownCulture"
	}
}

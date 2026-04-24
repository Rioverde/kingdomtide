package polity

// GreatPersonKind enumerates the archetypes of great people a city
// may host. Each kind grants a distinct civic bonus while alive and
// leaves a legacy effect after death.
type GreatPersonKind uint8

const (
	// GreatPersonScholar accelerates Innovation and tech discovery
	// while in residence.
	GreatPersonScholar GreatPersonKind = iota
	// GreatPersonGeneral boosts Army effectiveness and military
	// morale.
	GreatPersonGeneral
	// GreatPersonArchitect lowers construction cost and unlocks
	// wonder projects.
	GreatPersonArchitect
	// GreatPersonPriest raises Happiness and stabilizes the majority
	// faith against schism pressure.
	GreatPersonPriest
)

// String returns the English name of the great-person kind.
// Dev-only — player-visible text via client i18n catalog.
func (k GreatPersonKind) String() string {
	switch k {
	case GreatPersonScholar:
		return "Scholar"
	case GreatPersonGeneral:
		return "General"
	case GreatPersonArchitect:
		return "Architect"
	case GreatPersonPriest:
		return "Priest"
	default:
		return "UnknownGreatPersonKind"
	}
}

// GreatPerson is a single named notable hosted by a city. A city
// may host at most one great person at a time; the yearly
// great-people tick rolls arrival and death against the world clock.
// DeathYear == 0 indicates the great person is still alive.
type GreatPerson struct {
	// Kind is the archetype of this great person.
	Kind GreatPersonKind `json:"kind"`
	// BirthYear is the simulation year the great person arrived in
	// the city. Used for age calculations and legacy decay.
	BirthYear int `json:"birth_year"`
	// DeathYear is the simulation year of death, or 0 while the
	// great person is still alive.
	DeathYear int `json:"death_year"`
}

// Alive returns true while the great person has not yet been
// marked dead.
func (g GreatPerson) Alive() bool {
	return g.DeathYear == 0
}

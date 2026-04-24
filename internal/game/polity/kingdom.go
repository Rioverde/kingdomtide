package polity

// Kingdom is the political aggregate above City. Tracks the current
// reigning ruler, the historical sequence of past rulers (for
// dynasty display and succession rules), the cities under its
// political umbrella, the succession law, and the Asabiya score
// that drives dominance and collapse mechanics.
type Kingdom struct {
	ID   string `json:"id"`
	Name string `json:"name"`

	// CurrentRuler is the sovereign presently on the throne. Zero-
	// value Ruler means interregnum (no valid heir).
	CurrentRuler Ruler `json:"current_ruler"`

	// Rulers is the chronological history of every ruler this kingdom
	// has seated, oldest first. The sequence feeds UI dynasty panels
	// and succession-law resolution (e.g. Tanistry's kin-group pool).
	Rulers []Ruler `json:"rulers,omitempty"`

	// CityIDs is the set of city IDs this kingdom governs at the
	// current year. A city may appear in at most one kingdom's CityIDs
	// at any time; transfers (conquest, vassalage) must update both
	// kingdoms atomically.
	CityIDs []string `json:"city_ids,omitempty"`

	// SuccessionLaw governs how the heir is chosen on ruler death.
	SuccessionLaw SuccessionLaw `json:"succession_law"`

	// Culture is the ruling civilization archetype. When a kingdom
	// absorbs cities of a different culture, Mulk-cycle gravitation
	// may drift this value over time (§7d).
	Culture Culture `json:"culture"`

	// Asabiya is the group-cohesion score per Turchin's secular-cycle
	// model. Range [0, 1]. Rises at frontier cities, decays in the
	// interior; when it falls below turchinCollapseThreshold the
	// kingdom fragments.
	Asabiya float64 `json:"asabiya"`

	// Founded and Dissolved track the lifetime. Dissolved == 0 means
	// the kingdom is still active; non-zero is the year of fragmentation.
	Founded   int `json:"founded"`
	Dissolved int `json:"dissolved"`

	// InterPolityHistory lists completed cross-kingdom events where this
	// kingdom was the aggressor. Used by UI for timeline display and by
	// future alliance-stability logic.
	InterPolityHistory []InterPolityEvent `json:"inter_polity_history,omitempty"`
}

// Alive reports whether the kingdom still exists as a political unit.
func (k Kingdom) Alive() bool {
	return k.Dissolved == 0
}

// NewKingdom constructs a fresh kingdom with the given id, name,
// founding ruler, capital city, and succession law. Returns a
// pointer because the kingdom is mutated by the annual tick
// (asabiya update, tribute collection, city transfers).
func NewKingdom(id, name string, founder Ruler, capital string, law SuccessionLaw, founded int) *Kingdom {
	return &Kingdom{
		ID:            id,
		Name:          name,
		CurrentRuler:  founder,
		Rulers:        []Ruler{founder},
		CityIDs:       []string{capital},
		SuccessionLaw: law,
		Asabiya:       0.5, // neutral baseline
		Founded:       founded,
	}
}

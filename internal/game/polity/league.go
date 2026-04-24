package polity

// League is a horizontal alliance of cities per the Hansa pattern.
// No king, no dominance — just mutual trust and shared defense.
// League members pool trade bonuses and can leave voluntarily when
// trust drops.
type League struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Founded   int    `json:"founded"`
	Dissolved int    `json:"dissolved"` // 0 = active

	// MemberCityIDs lists the city IDs in the league. Max league size
	// per §7e is 6 members.
	MemberCityIDs []string `json:"member_city_ids,omitempty"`

	// Trust is a per-pair [0, 1] score. Keyed by "cityA|cityB" with
	// the ID ordering alphabetical so lookups are unambiguous.
	Trust map[string]float64 `json:"trust,omitempty"`
}

// Alive reports whether the league is still active.
func (l League) Alive() bool {
	return l.Dissolved == 0
}

// NewLeague builds a fresh league with a founding pair.
func NewLeague(id, name string, cityA, cityB string, founded int) *League {
	l := &League{
		ID:            id,
		Name:          name,
		Founded:       founded,
		MemberCityIDs: []string{cityA, cityB},
		Trust:         make(map[string]float64),
	}
	l.Trust[trustKey(cityA, cityB)] = 0.7 // initial good will
	return l
}

// trustKey returns the alphabetically-ordered pair key.
func trustKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

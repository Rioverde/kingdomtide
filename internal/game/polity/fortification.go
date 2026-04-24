package polity

// Fortification is a single defensive structure built in a city.
// MVP tracks only the count and a total defense value; later waves
// can split into walls, keeps, watchtowers with their own stats.
type Fortification struct {
	// Kind is the fortification type. Placeholder — the MVP only
	// cares about the count, not the specific kind.
	Kind string `json:"kind"`
	// Defense is the raw defense value this fortification adds.
	// Tech multipliers apply on top.
	Defense int `json:"defense"`
	// BuiltYear records when the structure was commissioned.
	BuiltYear int `json:"built_year"`
}

package polity

import "github.com/Rioverde/gongeons/internal/game/geom"

// Settlement is the shared identity + demographic base of any permanent
// human habitation — city, village, keep, ruin. Holds the four fields
// that every place has regardless of political status: a name, a world
// position, a founding year, and a headcount. City and Village embed it
// via composition so each gets these four fields (and the Age method)
// without duplication.
type Settlement struct {
	Name       string        `json:"name"`
	Position   geom.Position `json:"position"`
	Founded    int           `json:"founded"`    // in-game year of founding
	Population int           `json:"population"` // [0, 40 000]
}

// Age returns the settlement's age in years relative to the simulation's
// current year. Computed from Founded rather than stored so the value
// stays correct as the world clock advances without per-tick bookkeeping.
func (s Settlement) Age(currentYear int) int {
	return currentYear - s.Founded
}

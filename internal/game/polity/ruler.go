package polity

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// Ruler is a single named sovereign — king, lord, or village elder — who
// governs one political unit at a time. Carries the six D&D-style ability
// scores that feed decree checks, longevity, and succession eligibility.
// BirthYear and DeathYear track lifespan against the simulation's year
// counter; a Ruler with DeathYear == 0 is alive.
type Ruler struct {
	Name      string `json:"name"`
	Stats     stats.CoreStats
	BirthYear int
	// 0 if still alive
	DeathYear int

	// Faith is the ruler's personal religion. Used by the revolution
	// check's religion-mismatch bypass — when the ruler's faith
	// diverges from the city's majority faith by a high grievance score,
	// the happiness ceiling on revolt is lifted.
	Faith Faith `json:"faith"`
}

// NewRuler rolls all six ability scores via Stat4D6DropLowest on the
// provided stream and returns a freshly-crowned Ruler. All randomness
// flows through the Stream — same (seed, salt) yields an identical Ruler.
// The ruler defaults to FaithOldGods so legacy call-sites keep the
// previous zero-value alignment with the default city majority.
func NewRuler(s *dice.Stream, birthYear int, name string) Ruler {
	return Ruler{
		Name: name,
		Stats: stats.CoreStats{
			Strength:     s.Stat4D6DropLowest(),
			Dexterity:    s.Stat4D6DropLowest(),
			Constitution: s.Stat4D6DropLowest(),
			Intelligence: s.Stat4D6DropLowest(),
			Wisdom:       s.Stat4D6DropLowest(),
			Charisma:     s.Stat4D6DropLowest(),
		},
		BirthYear: birthYear,
		Faith:     FaithOldGods,
	}
}

// LifeExpectancy returns the Ruler's expected lifespan in years:
// 30 + 10 × Modifier(CON). The Constitution modifier is clamped to
// [-3, +5] before scaling, so a weak ruler (CON 3, mod -3) is
// expected to die at year of coronation and a strong ruler (CON
// 18+, mod +4) lives to 70+. Callers drive natural death by
// comparing currentYear - BirthYear against this value.
func (r Ruler) LifeExpectancy() int {
	mod := stats.Modifier(r.Stats.Constitution)
	mod = min(max(mod, -3), 5)
	return 30 + 10*mod
}

// Alive returns true while the Ruler has not been marked dead.
func (r Ruler) Alive() bool {
	return r.DeathYear == 0
}

package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// demesnePopMin is the viability floor for any demesne. A demesne
	// below this has effectively died — future milestones will drop it
	// from the parent City's demesne list.
	demesnePopMin = 20

	// demesnePopMaxCap is the hard ceiling — a demesne that crosses it
	// effectively becomes a Hamlet and should promote to a City in a
	// later milestone. Today we just clamp.
	demesnePopMaxCap = 400

	// demesneGrowthPermille is the baseline growth rate — slower than
	// cities because demesnes lack the urban density advantage.
	demesneGrowthPermille = 8

	// demesneFoodPerCapita is the per-demesne-resident yearly food surplus
	// that flows into the parent City's FoodBalance. A 100-person
	// demesne contributes +10 food per year to its city.
	demesneFoodPerCapita = 0.1
)

// ApplyDemesneYear advances one demesne by a simulated year. Demesnes
// are minor production nodes: population drifts slowly, and the
// demesne contributes a small food surplus to the parent city. Must
// be called BEFORE the parent city's TickCityYear so the food
// contribution feeds that city's harvest.
func ApplyDemesneYear(demesne *polity.Demesne, stream *dice.Stream) {
	// D20 centered at 10.5 → drift permille in [-5, +5] around
	// demesneGrowthPermille. Stochastic so two identical demesnes
	// don't grow at the exact same rate.
	drift := stream.D20() - 10 // [-9, +10]
	growth := demesneGrowthPermille + drift/2
	growth = max(-20, growth) // never more than -2 % shrink/yr

	delta := demesne.Population * growth / 1000
	demesne.Population += delta

	// Clamp to viability range.
	demesne.Population = min(demesnePopMaxCap, max(demesnePopMin, demesne.Population))
}

// ResolveDemesneToCity pushes every demesne's food contribution into
// its parent city's FoodBalance. Call once per tick AFTER all
// ApplyDemesneYear calls but BEFORE the city tick's food-dependent
// population step. The mapping table maps parentCityID to *City so
// the caller can pre-index the world's cities.
func ResolveDemesneToCity(demesnes []*polity.Demesne, cities map[string]*polity.City) {
	for _, d := range demesnes {
		parent, ok := cities[d.ParentCityID]
		if !ok {
			continue
		}
		contribution := int(float64(d.Population) * demesneFoodPerCapita)
		parent.FoodBalance += contribution
	}
}

package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// villagePopMin is the viability floor for any village. A village
	// below this has effectively died — future milestones will drop it
	// from the parent City's village list.
	villagePopMin = 20

	// villagePopMaxCap is the hard ceiling — a village that crosses it
	// effectively becomes a Hamlet and should promote to a City in a
	// later milestone. Today we just clamp.
	villagePopMaxCap = 400

	// villageGrowthPermille is the baseline growth rate — slower than
	// cities because villages lack the urban density advantage.
	villageGrowthPermille = 8

	// villageFoodPerCapita is the per-villager yearly food surplus
	// that flows into the parent City's FoodBalance. A 100-person
	// village contributes +10 food per year to its city.
	villageFoodPerCapita = 0.1
)

// ApplyVillageYear advances one village by a simulated year. Villages
// are minor production nodes: population drifts slowly, and the
// village contributes a small food surplus to the parent city. Must
// be called BEFORE the parent city's TickCityYear so the food
// contribution feeds that city's harvest.
func ApplyVillageYear(village *polity.Village, stream *dice.Stream) {
	// D20 centered at 10.5 → drift permille in [-5, +5] around
	// villageGrowthPermille. Stochastic so two identical villages
	// don't grow at the exact same rate.
	drift := stream.D20() - 10 // [-9, +10]
	growth := villageGrowthPermille + drift/2
	growth = max(-20, growth) // never more than -2 % shrink/yr

	delta := village.Population * growth / 1000
	village.Population += delta

	// Clamp to viability range.
	village.Population = min(villagePopMaxCap, max(villagePopMin, village.Population))
}

// ResolveVillageToCity pushes every village's food contribution into
// its parent city's FoodBalance. Call once per tick AFTER all
// ApplyVillageYear calls but BEFORE the city tick's food-dependent
// population step. The mapping table maps parentCityID to *City so
// the caller can pre-index the world's cities.
func ResolveVillageToCity(villages []*polity.Village, cities map[string]*polity.City) {
	for _, v := range villages {
		parent, ok := cities[v.ParentCityID]
		if !ok {
			continue
		}
		contribution := int(float64(v.Population) * villageFoodPerCapita)
		parent.FoodBalance += contribution
	}
}

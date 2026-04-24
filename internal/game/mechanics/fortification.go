package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

const (
	// masonryDefenseMultiplier applies when Masonry tech is unlocked
	// (MECHANICS.md §6c says +20 % fortification effectiveness).
	masonryDefenseMultiplier = 1.2

	// architectDefenseBonus is the flat per-fortification defense boost
	// an Architect great person provides while alive.
	architectDefenseBonus = 5

	// fortificationHappinessBonus is the pride-of-walls modifier — a
	// city with standing fortifications has a small civic boost on
	// top of the happiness baseline while the structure stands.
	fortificationHappinessBonus = 2
)

// TotalDefense sums every fortification's defense and applies tech /
// great-person multipliers. Returns the effective siege resistance.
func TotalDefense(city *polity.City) int {
	if len(city.Fortifications) == 0 {
		return 0
	}
	total := 0
	for _, f := range city.Fortifications {
		base := f.Defense
		if city.Techs.Has(polity.TechMasonry) {
			base = int(float64(base) * masonryDefenseMultiplier)
		}
		if greatPersonOf(city, polity.GreatPersonArchitect) {
			base += architectDefenseBonus
		}
		total += base
	}
	return total
}

// BuildFortification appends a new fortification to the city's list.
// Called by decree execution (BuildFortification) and future events
// ("Commission Walls"). Returns the newly-built fortification so the
// caller can log / UI-notify.
func BuildFortification(city *polity.City, currentYear int) polity.Fortification {
	f := polity.Fortification{
		Kind:      "Wall",
		Defense:   10,
		BuiltYear: currentYear,
	}
	city.Fortifications = append(city.Fortifications, f)
	return f
}

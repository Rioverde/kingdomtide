package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// Great-person effect constants. Each archetype grants a distinct
// civic bonus while alive in the city. Values are kept in one file
// so balance tuning has a single source.
const (
	// scholarInnovationBonus is the flat Innovation growth per year
	// added when a Scholar is alive in the city. MECHANICS.md §6d.
	scholarInnovationBonus = 3

	// generalArmyMultPermille lifts the army baseline by 25 % when a
	// General is alive in the city.
	generalArmyMultPermille = 1250

	// priestReligionMultPermille doubles the religion diffusion pulse
	// while a Priest is alive in the city.
	priestReligionMultPermille = 2000

	// priestSchismThresholdBump raises the schism innovation gate by
	// this amount while a Priest is alive — harder to fragment the
	// majority faith under a priest's stabilising authority.
	priestSchismThresholdBump = 5
)

// greatPersonOf reports whether the city hosts a great person of the
// given archetype that is still in the city's roster this tick.
// ApplyGreatPeopleYear prunes expired slots at the top of each year,
// so by the time effect sites run the pointer is nil when the great
// person has retired — a non-nil GreatPerson of matching Kind is
// enough to grant the bonus.
func greatPersonOf(city *polity.City, kind polity.GreatPersonKind) bool {
	if city.GreatPerson == nil {
		return false
	}
	return city.GreatPerson.Kind == kind
}

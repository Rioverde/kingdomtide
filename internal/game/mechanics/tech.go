package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

const (
	// innovationBaseMin / Max bound the per-year D3-equivalent growth
	// in Innovation — the city's scholars grow the score by 1–3
	// points a year before any stat / faction / faith bonuses.
	innovationBaseMin = 1
	innovationBaseMax = 3

	// innovationMagesFactionBonus converts the Mages faction's
	// influence into extra Innovation. At full 1.0 influence the
	// city gets +2 innovation/year on top of the base roll.
	innovationMagesFactionBonus = 2.0

	// innovationGreenSageBonus applies when Green Sage is the
	// majority faith. Flat +1/yr — keeps magnitude consistent with
	// the base roll while rewarding the nature-oriented tradition.
	innovationGreenSageBonus = 1.0
)

// ApplyTechnologyYear grows the city's Innovation score and unlocks
// techs whose thresholds it crosses this year. Growth = D3 base +
// INT modifier + Mages bonus + Green Sage faith bonus.
//
// Passes the stream in for the D3 (represented as D6 halved via
// table, since dice.Stream has no native D3). The INT modifier is
// taken from the ruler — acknowledges that a scholarly ruler
// accelerates progress.
func ApplyTechnologyYear(city *polity.City, stream *dice.Stream) {
	// D3-equivalent: (D6+1)/2 gives [1, 3] with slight bias; clean
	// approximation using Stream's existing D6. Alternative would
	// be a custom MustParse("1d3") expression in the stream
	// package, but that is out of scope here.
	base := (stream.D6() + 1) / 2
	base = max(innovationBaseMin, base)
	base = min(innovationBaseMax, base)

	growth := float64(base)
	growth += float64(stats.Modifier(city.Ruler.Stats.Intelligence))
	growth += city.Factions.Get(polity.FactionMages) * innovationMagesFactionBonus
	if city.Faiths.Majority() == polity.FaithGreenSage {
		growth += innovationGreenSageBonus
	}
	if greatPersonOf(city, polity.GreatPersonScholar) {
		growth += scholarInnovationBonus
	}
	// A dumb ruler can stall progress, not reverse it.
	growth = max(0, growth)
	city.Innovation += growth

	// Unlock every tech whose threshold the city has crossed.
	for _, t := range allTechsList {
		if !city.Techs.Has(t) && int(city.Innovation) >= t.InnovationThreshold() {
			city.Techs.Set(t)
		}
	}
}

// allTechsList enumerates every Tech in declaration order. Used by
// the year-end unlock check and by tests that iterate every tech.
// Package-level so the yearly tick reuses one slice header instead
// of allocating a fresh literal every call. Declared in the mechanics
// package rather than polity so polity has zero consumer-side
// knowledge of Innovation.
//
// Callers MUST NOT mutate the returned slice — it is shared across
// every tick.
var allTechsList = []polity.Tech{
	polity.TechIrrigation, polity.TechMasonry, polity.TechWriting,
	polity.TechMetallurgy, polity.TechNavigation, polity.TechCalendar,
	polity.TechPrinting, polity.TechBanking,
}

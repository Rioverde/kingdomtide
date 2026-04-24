package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// greatPersonBirthDC is the D100 roll target for a great person
	// to be born in a city this year — a 1 %/year rate. D100 returns
	// [1, 100]; a roll ≥ 100 fires — natural 100.
	greatPersonBirthDC = 100

	// Lifespan envelopes per archetype. Scholars live longest
	// (40-48), Generals shortest (33-40), Architects (36-44) and
	// Priests (38-46) in between.
	scholarLifespanMin   = 40
	scholarLifespanMax   = 48
	generalLifespanMin   = 33
	generalLifespanMax   = 40
	architectLifespanMin = 36
	architectLifespanMax = 44
	priestLifespanMin    = 38
	priestLifespanMax    = 46
)

// ApplyGreatPeopleYear rolls the birth of a great person (if the
// city has none) and expires the current one if they have reached
// their lifespan.
//
// A single city may host at most one great person at a time. We do
// not track lifetime contributions — when they die, the slot opens
// for a new birth the following year.
func ApplyGreatPeopleYear(city *polity.City, stream *dice.Stream, currentYear int) {
	if city.GreatPerson != nil {
		if expired(*city.GreatPerson, currentYear) {
			city.GreatPerson = nil
		}
		return
	}
	if stream.D100() < greatPersonBirthDC {
		return
	}
	kind := rollArchetype(stream)
	minLife, maxLife := lifespanEnvelope(kind)
	// Uniform over the closed interval [minLife, maxLife]. Using
	// D20 and a table-rescale gives determinism without adding a
	// new Stream API.
	span := maxLife - minLife + 1
	lifespan := minLife + (stream.D20()-1)*span/20
	lifespan = min(maxLife, lifespan)
	city.GreatPerson = &polity.GreatPerson{
		Kind:      kind,
		BirthYear: currentYear,
		DeathYear: currentYear + lifespan,
	}
}

// expired reports whether the great person's DeathYear has arrived.
func expired(gp polity.GreatPerson, year int) bool {
	return year >= gp.DeathYear
}

// rollArchetype selects one of four archetypes uniformly. Using
// D20 into 4 buckets of 5 to stay inside Stream's existing API.
func rollArchetype(stream *dice.Stream) polity.GreatPersonKind {
	switch (stream.D20() - 1) / 5 {
	case 0:
		return polity.GreatPersonScholar
	case 1:
		return polity.GreatPersonGeneral
	case 2:
		return polity.GreatPersonArchitect
	default:
		return polity.GreatPersonPriest
	}
}

// lifespanEnvelope returns the [min, max] lifespan range for kind.
func lifespanEnvelope(kind polity.GreatPersonKind) (int, int) {
	switch kind {
	case polity.GreatPersonScholar:
		return scholarLifespanMin, scholarLifespanMax
	case polity.GreatPersonGeneral:
		return generalLifespanMin, generalLifespanMax
	case polity.GreatPersonArchitect:
		return architectLifespanMin, architectLifespanMax
	case polity.GreatPersonPriest:
		return priestLifespanMin, priestLifespanMax
	}
	// Unknown kind — default to Scholar envelope.
	return scholarLifespanMin, scholarLifespanMax
}

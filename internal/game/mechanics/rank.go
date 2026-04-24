package mechanics

import "github.com/Rioverde/gongeons/internal/game/polity"

// BaseRank thresholds. A population below rankTownMin is a Hamlet;
// above rankMetropolisMin is a Metropolis. Age acts as a one-rank
// bump for cities old enough to have built the infrastructure their
// population alone would not merit.
const (
	rankTownMin       = 200
	rankCityMin       = 2000
	rankMetropolisMin = 20000

	// rankAgeBoostYears is the age threshold at which a city gets a
	// one-rank bump. Five centuries of growth promote a town to city
	// status even if population dipped during a famine — keeps
	// historically-old settlements from slipping in rank under a
	// single bad century. Tuneable.
	rankAgeBoostYears = 500
)

// DeriveBaseRank classifies a city's intrinsic tier from Population
// and Age. Pure function — deterministic, no randomness, no side
// effects.
func DeriveBaseRank(population, age int) polity.BaseRank {
	base := baseRankFromPopulation(population)

	// Age bonus: a city older than rankAgeBoostYears is promoted one
	// rank, capped at Metropolis.
	if age >= rankAgeBoostYears && base < polity.RankMetropolis {
		base++
	}
	return base
}

// baseRankFromPopulation returns the rank that matches population
// alone, without the age bonus.
func baseRankFromPopulation(pop int) polity.BaseRank {
	switch {
	case pop >= rankMetropolisMin:
		return polity.RankMetropolis
	case pop >= rankCityMin:
		return polity.RankCity
	case pop >= rankTownMin:
		return polity.RankTown
	default:
		return polity.RankHamlet
	}
}

// ApplyRankYear recomputes city.BaseRank from the current Population
// and Age. Must be called AFTER ApplyPopulationYear — it reads the
// freshly-grown population.
//
// EffectiveRank is NOT touched here; it is assigned by the kingdom
// dominance simulation, which lands with the kingdom layer.
func ApplyRankYear(city *polity.City, currentYear int) {
	city.BaseRank = DeriveBaseRank(city.Population, city.Age(currentYear))
}

package mechanics

import (
	"math"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// dominanceCadenceYears is how often the dominance compute block
	// runs — every 10 years rather than yearly, matching the spec
	// cadence for political recalibration.
	dominanceCadenceYears = 10

	// Asabiya constants per the Turchin secular-cycle model.
	// frontierGrowth is logistic growth at the political frontier;
	// interiorDecay is exponential decay deep inside a single polity.
	asabiyaFrontierGrowth = 0.2
	asabiyaInteriorDecay  = 0.1

	// collapseThreshold is the asabiya floor below which the kingdom
	// fragments — capital city becomes Independent, vassal cities free.
	collapseThreshold = 0.1

	// tributeVassalRate is the wealth share each vassal city transfers
	// upstream each year. §8b specifies 15 %.
	tributeVassalRate = 0.15

	// capitalReserveRate caps how much of the collected tribute stays
	// in the capital's treasury vs. flowing further up (where "up"
	// lands in a future imperial tier).
	capitalReserveRate = 1.0

	// spanCap is the number of non-capital cities a kingdom can govern
	// before administrative overload chips away at Asabiya each year.
	spanCap = 8

	// spanDrainPerOverflow is the per-extra-city Asabiya loss each
	// year when CityIDs exceeds spanCap+1.
	spanDrainPerOverflow = 0.03

	// corruptionBase controls how strongly chain size erodes the
	// fraction of tribute the capital actually keeps. A 10-city chain
	// keeps 96 % of a 1-city baseline; larger chains lose more.
	corruptionBase = 0.04

	// corruptionMinRetention caps corruption impact — the capital
	// always retains at least half of what vassals pay.
	corruptionMinRetention = 0.5

	// Rump-state thresholds. A collapsing kingdom that has had time
	// to build literacy and that still has a competent ruler keeps
	// its capital plus two cities instead of dissolving outright.
	rumpStateAgeMin        = 100
	rumpStateInnovationMin = 65
	rumpStateRulerStatMin  = 25
	rumpStateKeep          = 3
	rumpStateAsabiyaReset  = 0.3
)

// TickKingdomYear advances one kingdom by a simulated year. Not
// driven from TickCityYear — instead the orchestrator (a worldgen
// pass or per-year manager) calls it AFTER every city in its
// CityIDs has had TickCityYear applied. The kingdom tick reads
// post-tick city state and writes kingdom-level mutations.
//
// Does nothing on a dissolved kingdom.
func TickKingdomYear(
	k *polity.Kingdom,
	cities map[string]*polity.City,
	stream *dice.Stream,
	currentYear int,
) {
	if !k.Alive() {
		return
	}

	// Asabiya evolves every year — frontier growth if the kingdom
	// has any city, interior decay once it accumulates enough
	// cities to have "interior" depth. Simplified — the real model
	// would distinguish per-city positions; MVP just averages.
	applyAsabiyaYear(k)

	// Dominance block runs on the cadence year — compute power
	// projection between this and neighbor kingdoms, transfer
	// subjugated cities. Today there is no inter-kingdom layer yet,
	// so we only apply a self-consistency pass: the capital pulls
	// tribute from every vassal every year, and collapse fires when
	// the kingdom's asabiya drops below the threshold.
	if currentYear%dominanceCadenceYears == 0 {
		collectTribute(k, cities)
	}

	// Ruler rotation — if current ruler has died, trigger succession.
	if !k.CurrentRuler.Alive() ||
		currentYear-k.CurrentRuler.BirthYear >= k.CurrentRuler.LifeExpectancy() {
		applySuccession(k, stream, currentYear)
	}

	// Collapse check — asabiya fell through the floor, kingdom dissolves.
	if k.Asabiya <= collapseThreshold {
		dissolve(k, cities, currentYear)
	}
}

// applyAsabiyaYear evolves k.Asabiya by the Turchin logistic /
// exponential pair. With only one kingdom in scope we treat every
// city as "interior" once the kingdom has more than one city, a
// simplification that captures the qualitative behaviour without
// the full positional model. Over-wide kingdoms also suffer an
// administrative-span penalty on top of the decay.
func applyAsabiyaYear(k *polity.Kingdom) {
	if len(k.CityIDs) <= 1 {
		// Single-city kingdom — still frontier. Logistic growth.
		delta := asabiyaFrontierGrowth * k.Asabiya * (1 - k.Asabiya)
		k.Asabiya += delta
	} else {
		// Multi-city — treat as interior-dominant. Exponential decay.
		delta := asabiyaInteriorDecay * k.Asabiya
		k.Asabiya -= delta
	}

	// Span penalty — overflow counts vassal cities past spanCap.
	// CityIDs[0] is the capital, so overflow = len - 1 - spanCap.
	if overflow := len(k.CityIDs) - 1 - spanCap; overflow > 0 {
		k.Asabiya -= spanDrainPerOverflow * float64(overflow)
	}

	k.Asabiya = min(1.0, max(0.0, k.Asabiya))
}

// collectTribute moves a fraction of every vassal city's wealth up
// to the capital. The first entry of CityIDs is the capital by
// convention. Banking halves the extraction rate in the vassal; the
// capital retains only a log-scaled fraction of what it collects so
// longer tribute chains leak more to corruption.
func collectTribute(k *polity.Kingdom, cities map[string]*polity.City) {
	if len(k.CityIDs) <= 1 {
		return
	}
	capital, ok := cities[k.CityIDs[0]]
	if !ok {
		return
	}
	retention := retentionRate(len(k.CityIDs))
	for _, id := range k.CityIDs[1:] {
		vassal, ok := cities[id]
		if !ok {
			continue
		}
		if vassal.Wealth <= 0 {
			continue
		}
		rate := techTributeRate(vassal, tributeVassalRate)
		transfer := int(float64(vassal.Wealth) * rate)
		vassal.Wealth -= transfer
		capital.Wealth += int(float64(transfer) * retention)
	}
}

// retentionRate returns the fraction of collected tribute the capital
// keeps after corruption along a chain of this many cities. At size
// 1 returns 1.0; beyond that, drops logarithmically. Clamped to the
// [corruptionMinRetention, 1.0] envelope so corruption never wipes
// the tribute stream entirely.
func retentionRate(chainSize int) float64 {
	if chainSize <= 1 {
		return 1.0
	}
	rate := 1.0 - corruptionBase*math.Log10(float64(chainSize))
	return max(corruptionMinRetention, min(1.0, rate))
}

// applySuccession seats a new ruler via the kingdom's succession law,
// dispatching to newHeirFor so each law biases the heir's stat roll
// toward its cultural archetype.
func applySuccession(k *polity.Kingdom, stream *dice.Stream, currentYear int) {
	k.Rulers = append(k.Rulers, k.CurrentRuler)
	k.CurrentRuler = newHeirFor(k, stream, currentYear)
}

// dissolve marks the kingdom dissolved and sets every member city's
// EffectiveRank back to Independent. Does not remove the city from
// k.CityIDs — the list stays for audit / replay, but Alive() now
// returns false and downstream code ignores the kingdom.
//
// An old, literate kingdom with a competent ruler survives as a rump
// state instead — trimming to at most rumpStateKeep cities and
// resetting Asabiya to the salvaged-legitimacy baseline.
func dissolve(k *polity.Kingdom, cities map[string]*polity.City, currentYear int) {
	if isRumpEligible(k, cities, currentYear) {
		trimToRumpState(k, cities)
		return
	}
	k.Dissolved = currentYear
	for _, id := range k.CityIDs {
		if city, ok := cities[id]; ok {
			city.EffectiveRank = polity.RankIndependent
		}
	}
}

// isRumpEligible reports whether a collapsing kingdom has the age,
// capital literacy, and ruler competence to survive in reduced form.
func isRumpEligible(
	k *polity.Kingdom,
	cities map[string]*polity.City,
	currentYear int,
) bool {
	if currentYear-k.Founded < rumpStateAgeMin {
		return false
	}
	if len(k.CityIDs) == 0 {
		return false
	}
	capital, ok := cities[k.CityIDs[0]]
	if !ok {
		return false
	}
	if int(capital.Innovation) < rumpStateInnovationMin {
		return false
	}
	wisPlusCha := k.CurrentRuler.Stats.Wisdom + k.CurrentRuler.Stats.Charisma
	return wisPlusCha >= rumpStateRulerStatMin
}

// trimToRumpState keeps the capital plus up to (rumpStateKeep - 1)
// other cities inside the kingdom; the remainder are freed to
// Independent. Asabiya is reset to rumpStateAsabiyaReset so the
// kingdom has a runway of legitimacy to recover.
func trimToRumpState(k *polity.Kingdom, cities map[string]*polity.City) {
	keep := min(rumpStateKeep, len(k.CityIDs))
	for _, id := range k.CityIDs[keep:] {
		if c, ok := cities[id]; ok {
			c.EffectiveRank = polity.RankIndependent
		}
	}
	k.CityIDs = k.CityIDs[:keep]
	k.Asabiya = rumpStateAsabiyaReset
}

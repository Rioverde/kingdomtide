package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

const (
	// leagueFormationDC is the D20+CHA target for founding a new league
	// (§7e: "Formation DC: 22").
	leagueFormationDC = 22

	// leagueMaxMembers is the membership cap per §7e.
	leagueMaxMembers = 6

	// leagueTrustDriftMagnitude is the random walk step per pair per year.
	leagueTrustDriftMagnitude = 0.03

	// leagueTrustMinToStay is the average-trust floor; members below it leave.
	leagueTrustMinToStay = 0.3

	// leagueTradeBonus is the per-member-year flat bonus to TradeScore.
	leagueTradeBonus = 5

	// leagueHappinessBonus is the flat historical-mod magnitude for league membership.
	leagueHappinessBonus = 3
)

// TickLeagueYear advances one league by a simulated year. Trust scores
// drift stochastically; members below the minimum trust leave; a league
// with fewer than 2 members dissolves.
func TickLeagueYear(l *polity.League, cities map[string]*polity.City,
	stream *dice.Stream, currentYear int) {
	if !l.Alive() {
		return
	}

	driftTrust(l, stream)
	purgeLowTrustMembers(l, cities, currentYear)
	applyLeagueBenefits(l, cities, currentYear)
	dissolveIfTooSmall(l, currentYear)
}

// driftTrust walks every trust pair by a stochastic ±magnitude.
func driftTrust(l *polity.League, stream *dice.Stream) {
	for key, v := range l.Trust {
		// D6 in [1,6]: 1-3 = down, 4-6 = up.
		if stream.D6() <= 3 {
			v -= leagueTrustDriftMagnitude
		} else {
			v += leagueTrustDriftMagnitude
		}
		l.Trust[key] = max(0, min(1.0, v))
	}
}

// purgeLowTrustMembers drops members whose average trust across all pairs
// falls below the minimum threshold.
func purgeLowTrustMembers(l *polity.League, cities map[string]*polity.City, currentYear int) {
	if len(l.MemberCityIDs) < 2 {
		return
	}
	kept := l.MemberCityIDs[:0]
	for _, id := range l.MemberCityIDs {
		if averageTrustForMember(l, id) >= leagueTrustMinToStay {
			kept = append(kept, id)
		}
	}
	l.MemberCityIDs = kept
	pruneTrustKeys(l)
	_ = cities      // reserved: update city.LeagueID when it exists
	_ = currentYear // reserved: log departure year when event log exists
}

func averageTrustForMember(l *polity.League, id string) float64 {
	if len(l.MemberCityIDs) < 2 {
		return 0
	}
	total := 0.0
	count := 0
	for _, other := range l.MemberCityIDs {
		if other == id {
			continue
		}
		if v, ok := l.Trust[trustKeyWithOrder(id, other)]; ok {
			total += v
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

// trustKeyWithOrder mirrors polity.trustKey and produces the same alphabetical key.
func trustKeyWithOrder(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

func pruneTrustKeys(l *polity.League) {
	present := make(map[string]bool, len(l.MemberCityIDs))
	for _, id := range l.MemberCityIDs {
		present[id] = true
	}
	for k := range l.Trust {
		var a, b string
		for i, r := range k {
			if r == '|' {
				a = k[:i]
				b = k[i+1:]
				break
			}
		}
		if !present[a] || !present[b] {
			delete(l.Trust, k)
		}
	}
}

// applyLeagueBenefits grants per-year trade and happiness bonuses to every
// living member city.
func applyLeagueBenefits(l *polity.League, cities map[string]*polity.City, currentYear int) {
	for _, id := range l.MemberCityIDs {
		c, ok := cities[id]
		if !ok {
			continue
		}
		c.TradeScore = min(100, c.TradeScore+leagueTradeBonus)
		c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
			Kind:        polity.HistoricalModHappiness,
			Magnitude:   leagueHappinessBonus,
			YearApplied: currentYear,
			DecayYears:  2, // refreshed yearly so the effect feels persistent
		})
	}
}

func dissolveIfTooSmall(l *polity.League, currentYear int) {
	if len(l.MemberCityIDs) < 2 {
		l.Dissolved = currentYear
	}
}

// AttemptFormLeague tries to found a new league when two cities want to
// ally. Returns the new league on DC success, nil on failure. The initiator's
// ruler Charisma score provides the ability modifier added to the D20 roll.
func AttemptFormLeague(
	initiatorRulerCha int,
	stream *dice.Stream,
	id, name, cityA, cityB string,
	currentYear int,
) *polity.League {
	roll := stream.D20() + stats.Modifier(initiatorRulerCha)
	if roll < leagueFormationDC {
		return nil
	}
	return polity.NewLeague(id, name, cityA, cityB, currentYear)
}

// AddMember attempts to add a city to an existing league. Honors the
// max-member cap and rejects duplicates. Initializes trust pairs with all
// existing members at a neutral starting value. Returns true when added.
func AddMember(l *polity.League, newCity string) bool {
	if len(l.MemberCityIDs) >= leagueMaxMembers {
		return false
	}
	for _, id := range l.MemberCityIDs {
		if id == newCity {
			return false
		}
	}
	for _, existing := range l.MemberCityIDs {
		l.Trust[trustKeyWithOrder(existing, newCity)] = 0.5
	}
	l.MemberCityIDs = append(l.MemberCityIDs, newCity)
	return true
}

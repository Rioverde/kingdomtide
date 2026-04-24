package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// factionDriftMagnitude is the per-year swing magnitude per
	// faction (±0.05/yr). The actual direction is rolled each call.
	factionDriftMagnitude = 0.05

	// factionMilitaryPeacetimeDrift pushes Military faction DOWN by
	// 0.01 per year of peace. War state not tracked yet — we always
	// apply the peacetime drift, a documented approximation until
	// the war system lands. A gentle slope keeps Military from
	// pinning at zero across every multi-decade stretch of peace.
	factionMilitaryPeacetimeDrift = -0.01
)

// factionOrderForDrift is the deterministic faction traversal order
// used by ApplyFactionDriftYear. Declared at package level so every
// yearly tick reuses the same slice header instead of allocating a
// fresh literal on the heap. Order matches the Faction enum.
var factionOrderForDrift = []polity.Faction{
	polity.FactionMerchants, polity.FactionMilitary,
	polity.FactionMages, polity.FactionCriminals,
}

// ApplyFactionDriftYear drifts each faction's influence by a
// stochastic ±factionDriftMagnitude per year. Applied after events
// because future event systems will bias specific factions; today
// the drift is the only signal.
//
// Military also takes a small peacetime penalty until the war
// system grows a real war-state field on Kingdom; keeps the
// "peacetime drift" invariant truthful without extra coordination
// work.
func ApplyFactionDriftYear(city *polity.City, stream *dice.Stream) {
	for _, f := range factionOrderForDrift {
		// D6 splits evenly at 3.5 — values 1-3 push down, 4-6 push
		// up. Deterministic given the stream.
		var delta float64
		if stream.D6() <= 3 {
			delta = -factionDriftMagnitude
		} else {
			delta = +factionDriftMagnitude
		}
		if f == polity.FactionMilitary {
			delta += factionMilitaryPeacetimeDrift
		}
		city.Factions.Add(f, delta)
	}
}

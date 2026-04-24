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

// dieFaceToDrift maps D6 face (1..6, indexed 0..5) to signed faction drift delta.
// Replaces per-call RNG+branch with pure lookup: 1-3 → -0.05, 4-6 → +0.05.
var dieFaceToDrift = [6]float64{
	-factionDriftMagnitude, // face 1
	-factionDriftMagnitude, // face 2
	-factionDriftMagnitude, // face 3
	+factionDriftMagnitude, // face 4
	+factionDriftMagnitude, // face 5
	+factionDriftMagnitude, // face 6
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
		delta := dieFaceToDrift[stream.D6()-1]
		if f == polity.FactionMilitary {
			delta += factionMilitaryPeacetimeDrift
		}
		city.Factions.Add(f, delta)
	}
}

package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const (
	// mulkCycleYearsMin / Max — the assimilation window per §7d.
	mulkCycleYearsMin = 50
	mulkCycleYearsMax = 75

	// mulkAsabiyaCostPerFlip — the internal-identity-conflict cost
	// each time a cultural attribute actually flips.
	mulkAsabiyaCostPerFlip = 0.1

	// mulkFlipProbabilityPerYear — base chance per year that a city's
	// culture nudges toward the kingdom's when they differ.
	mulkFlipProbabilityPerYear = 0.015 // ≈ 1 flip per 65 years average
)

// ApplyMulkCycleYear drives cultural assimilation between a kingdom
// and its member cities. When a city's culture differs from the
// kingdom's, there is a small yearly chance it flips toward the
// kingdom's; each successful flip docks kingdom.Asabiya by the
// per-flip cost. This models the conquest-dynasty assimilation
// pattern without introducing a per-year drift on asabiya that
// fights against the Turchin secular-cycle term.
//
// Called from the orchestrator (future world manager) after
// TickKingdomYear — flip decisions use the stream D100 so replay
// determinism holds.
func ApplyMulkCycleYear(
	k *polity.Kingdom,
	cities map[string]*polity.City,
	stream interface{ D100() int },
) {
	if !k.Alive() {
		return
	}
	for _, id := range k.CityIDs {
		c, ok := cities[id]
		if !ok || c.Culture == k.Culture {
			continue
		}
		if float64(stream.D100()) >= mulkFlipProbabilityPerYear*100.0 {
			continue
		}
		// Flip — city's culture moves one step toward kingdom's
		// (MVP: snap directly, skipping transitional states).
		c.Culture = k.Culture
		k.Asabiya = max(0, k.Asabiya-mulkAsabiyaCostPerFlip)
	}
}

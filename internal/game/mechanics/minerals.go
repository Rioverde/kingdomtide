package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Mineral-depletion constants. Mining rate scales with army size as
// a labor proxy — more soldiers means more support infrastructure
// means faster extraction.
const (
	// mineralDrainMin is the per-year RemainingYield loss for any
	// active deposit, even with no standing army. The land is
	// still worked by civilians.
	mineralDrainMin = 0.01
	// mineralDrainPerThousandArmy adds per 1000 soldiers, capped
	// implicitly by mineralDrainMax.
	mineralDrainPerThousandArmy = 0.015
	// mineralDrainMax is the upper bound on the yearly drain
	// regardless of army size; keeps the total range in 0.01–0.04.
	mineralDrainMax = 0.04
	// mineralExhaustThreshold triggers deposit removal once
	// RemainingYield falls below this fraction.
	mineralExhaustThreshold = 0.1
)

// ApplyMineralDepletionYear drains every active deposit's
// RemainingYield in proportion to the city's mining labor and
// removes exhausted deposits. Must run after ApplyArmyYear so the
// labor proxy reflects current troop count. Purely deterministic —
// no dice draws.
func ApplyMineralDepletionYear(city *polity.City) {
	if len(city.Deposits) == 0 {
		return
	}
	drain := mineralDrainMin +
		float64(city.Army)*mineralDrainPerThousandArmy/1000.0
	drain = min(mineralDrainMax, drain)

	kept := city.Deposits[:0]
	for _, d := range city.Deposits {
		d.RemainingYield -= drain
		if d.RemainingYield > mineralExhaustThreshold {
			kept = append(kept, d)
		}
	}
	city.Deposits = kept
}

package stats

// Speed scale values applied to Player and Monster entities. NetHack-style
// units: 12 is the baseline, doubling is "very fast", halving is "very slow".
// The absolute numbers matter only relative to BaseActionCost.
const (
	SpeedVerySlow = 6
	SpeedSlow     = 9
	SpeedNormal   = 12
	SpeedFast     = 18
	SpeedVeryFast = 24
)

// BaseActionCost is the Energy consumed by a standard gameplay action
// (move, basic attack). Exotic actions override this via Intent.Cost.
// Exported so the server can populate the self_energy_cost Snapshot field
// that lets the client render the energy progress bar denominator.
const BaseActionCost = 12

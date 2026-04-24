package polity

// Faction enumerates the internal power blocs that compete for
// influence inside a city. Each bloc's share is tracked
// independently in FactionInfluence; the blocs do not form a
// partition of political power and their values do not sum to 1.
type Faction uint8

const (
	// FactionMerchants represents the trade and banking guilds. High
	// influence boosts trade volume and wealth accumulation.
	FactionMerchants Faction = iota
	// FactionMilitary represents the standing-army officer caste and
	// martial aristocracy. High influence eases conscription and
	// military funding at the cost of civilian happiness.
	FactionMilitary
	// FactionMages represents scholarly and arcane circles. High
	// influence accelerates Innovation and technology unlock pace.
	FactionMages
	// FactionCriminals represents black-market networks and the
	// shadow economy. High influence drains tax revenue and can
	// trigger coup paths.
	FactionCriminals
)

// String returns the English name of the faction.
// Dev-only — player-visible text via client i18n catalog.
func (f Faction) String() string {
	switch f {
	case FactionMerchants:
		return "Merchants"
	case FactionMilitary:
		return "Military"
	case FactionMages:
		return "Mages"
	case FactionCriminals:
		return "Criminals"
	default:
		return "UnknownFaction"
	}
}

// FactionInfluence holds the [0, 1] influence share of every Faction
// as a fixed-size array indexed by the Faction value. These values
// do NOT need to sum to 1 — each faction's hold on the city is
// independent. The fixed-size array avoids per-city map overhead and
// keeps the zero value (all zeros) immediately usable.
type FactionInfluence [4]float64

// Get returns the current influence share of faction f in [0, 1].
func (fi *FactionInfluence) Get(f Faction) float64 {
	return fi[f]
}

// Set writes the influence share for faction f, clamping the input
// to [0, 1] so callers cannot drive a faction outside the canonical
// range through rounding or compounding deltas.
func (fi *FactionInfluence) Set(f Faction, v float64) {
	fi[f] = min(max(v, 0), 1)
}

// Add shifts the influence share for faction f by delta, clamping
// the result to [0, 1]. Used by the yearly faction-drift step and
// by event handlers that nudge a single bloc.
func (fi *FactionInfluence) Add(f Faction, delta float64) {
	fi.Set(f, fi[f]+delta)
}

package polity

// Tech enumerates the technologies a city can unlock. Each tech has
// an Innovation-score threshold; crossing the threshold unlocks it
// in the owning city's TechMask. Techs are monotonic — once
// unlocked they are never lost.
type Tech uint8

const (
	// TechIrrigation boosts FoodBalance and reduces drought
	// severity. First entry on the tech ladder.
	TechIrrigation Tech = iota
	// TechMasonry unlocks stone construction and raises siege
	// resilience.
	TechMasonry
	// TechWriting enables record-keeping bonuses and slows the
	// decay rate of civic knowledge.
	TechWriting
	// TechMetallurgy boosts army quality and unlocks higher-tier
	// mineral deposits.
	TechMetallurgy
	// TechNavigation grants sea-trade bonuses and opens coastal
	// routes.
	TechNavigation
	// TechCalendar improves harvest timing, narrowing the food
	// variance.
	TechCalendar
	// TechPrinting sharply accelerates Innovation and raises faith
	// diffusion speed.
	TechPrinting
	// TechBanking unlocks lending, stabilizes Wealth, and boosts
	// trade volume.
	TechBanking
)

// String returns the English name of the technology.
// Dev-only — player-visible text via client i18n catalog.
func (t Tech) String() string {
	switch t {
	case TechIrrigation:
		return "Irrigation"
	case TechMasonry:
		return "Masonry"
	case TechWriting:
		return "Writing"
	case TechMetallurgy:
		return "Metallurgy"
	case TechNavigation:
		return "Navigation"
	case TechCalendar:
		return "Calendar"
	case TechPrinting:
		return "Printing"
	case TechBanking:
		return "Banking"
	default:
		return "UnknownTech"
	}
}

// InnovationThreshold returns the Innovation score a city must reach
// before this technology unlocks. Values grow roughly linearly so
// that later techs require proportionally more accumulated progress.
func (t Tech) InnovationThreshold() int {
	switch t {
	case TechIrrigation:
		return 20
	case TechMasonry:
		return 30
	case TechWriting:
		return 35
	case TechMetallurgy:
		return 45
	case TechNavigation:
		return 55
	case TechCalendar:
		return 60
	case TechPrinting:
		return 75
	case TechBanking:
		return 85
	default:
		return 0
	}
}

// TechMask is a bitmask of unlocked technologies. Bit i is set iff
// Tech(i) has been unlocked. Uint16 leaves headroom for up to 16
// techs; extending past that will require widening the type in lock
// step with new Tech constants.
type TechMask uint16

// Has reports whether technology t is unlocked in the mask.
func (m TechMask) Has(t Tech) bool {
	return m&(1<<t) != 0
}

// Set marks technology t as unlocked. Idempotent — calling Set on
// an already-unlocked tech is a no-op.
func (m *TechMask) Set(t Tech) {
	*m |= 1 << t
}

// Unlocked returns the slice of every technology currently set in
// the mask, in Tech-ordinal order. Allocates a fresh slice per call.
func (m TechMask) Unlocked() []Tech {
	out := make([]Tech, 0, 8)
	for _, t := range []Tech{
		TechIrrigation,
		TechMasonry,
		TechWriting,
		TechMetallurgy,
		TechNavigation,
		TechCalendar,
		TechPrinting,
		TechBanking,
	} {
		if m.Has(t) {
			out = append(out, t)
		}
	}
	return out
}

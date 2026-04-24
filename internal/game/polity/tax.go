package polity

// TaxRate is the ruler's chosen fiscal policy, mapping to a fixed
// income fraction of per-capita income. Each tier also carries a
// happiness delta applied in the per-year happiness recompute.
type TaxRate uint8

const (
	// TaxNormal is the baseline rate — 17% of per-capita income, no
	// happiness impact. Default when a city has not been decreed a
	// different policy.
	TaxNormal TaxRate = iota
	// TaxLow eases the burden — 10%, +5 happiness. Used to pacify
	// unrest or attract migration.
	TaxLow
	// TaxHigh extracts more — 28%, -8 happiness. Ruler chooses when
	// treasury urgent need outweighs mood cost.
	TaxHigh
	// TaxBrutal is confiscatory — 45%, -20 happiness. High revolution
	// risk; typically applied during crisis or by tyrannical rulers.
	TaxBrutal
)

// Fraction returns the share of per-capita income this rate collects
// per year, as a float in [0, 1]. Values: Low 10 %, Normal 17 %,
// High 28 %, Brutal 45 %.
func (r TaxRate) Fraction() float64 {
	switch r {
	case TaxLow:
		return 0.10
	case TaxNormal:
		return 0.17
	case TaxHigh:
		return 0.28
	case TaxBrutal:
		return 0.45
	default:
		return 0.17
	}
}

// HappinessDelta returns the happiness modifier this rate contributes
// each year. Positive for Low, zero for Normal, negative for High
// and Brutal.
func (r TaxRate) HappinessDelta() int {
	switch r {
	case TaxLow:
		return +5
	case TaxNormal:
		return 0
	case TaxHigh:
		return -8
	case TaxBrutal:
		return -20
	default:
		return 0
	}
}

// String returns the English name of the rate for logs and debugging.
// Player-visible localization is the client's responsibility.
func (r TaxRate) String() string {
	switch r {
	case TaxLow:
		return "Low"
	case TaxNormal:
		return "Normal"
	case TaxHigh:
		return "High"
	case TaxBrutal:
		return "Brutal"
	default:
		return "UnknownTaxRate"
	}
}

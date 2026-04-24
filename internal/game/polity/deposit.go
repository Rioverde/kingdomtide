package polity

// DepositKind enumerates the categories of extractable mineral
// deposits a city may host. Drives mining income, trade variety
// bonuses, and the depletion-driven economic decline that motivates
// expansion and conquest.
type DepositKind uint8

const (
	// DepositIron backs weapons, armor, and tools. Most common deposit
	// kind; feeds baseline military and craft output.
	DepositIron DepositKind = iota
	// DepositGold is the luxury-currency deposit. Small yield but
	// outsized contribution to Wealth and trade prestige.
	DepositGold
	// DepositCoal fuels smelting and later-era industry. Pairs with
	// Iron for military output bonuses.
	DepositCoal
	// DepositStone is the bulk construction material. Low per-unit
	// value, high volume; underwrites walls, roads, and wonders.
	DepositStone
	// DepositSilver is a secondary precious metal. Feeds Wealth and
	// ritual / ecclesiastical consumption.
	DepositSilver
	// DepositSalt is the preservation and seasoning commodity. Small
	// physical yield, outsized trade-value modifier.
	DepositSalt
)

// String returns the English name of the deposit kind.
// Dev-only — player-visible text via client i18n catalog.
func (k DepositKind) String() string {
	switch k {
	case DepositIron:
		return "Iron"
	case DepositGold:
		return "Gold"
	case DepositCoal:
		return "Coal"
	case DepositStone:
		return "Stone"
	case DepositSilver:
		return "Silver"
	case DepositSalt:
		return "Salt"
	default:
		return "UnknownDepositKind"
	}
}

// Deposit is a single active mineral deposit hosted by a city.
// RemainingYield is a [0, 1] fraction of the original deposit still
// extractable; a deposit is considered exhausted when RemainingYield
// falls at or below 0.1 and is removed from the owning city's
// Deposits slice by mechanics.ApplyMineralDepletionYear.
type Deposit struct {
	// Kind is the deposit category (iron, gold, stone, etc.).
	Kind DepositKind `json:"kind"`
	// RemainingYield is the fraction of original yield still in the
	// ground, in [0, 1]. Drains each year based on mining activity.
	// Deposits at or below 0.1 are treated as exhausted.
	RemainingYield float64 `json:"remaining_yield"`
}

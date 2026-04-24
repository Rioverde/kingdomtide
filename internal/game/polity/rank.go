package polity

// BaseRank is the intrinsic size tier of a settlement, derived from
// Population and Age. Drives local mechanics — army capacity, trade
// range, food demand — independently of political standing (for which
// see EffectiveRank).
type BaseRank uint8

const (
	// RankHamlet is the smallest populated place — fewer than ~200
	// people. Typical of frontier and early-founding settlements.
	RankHamlet BaseRank = iota
	// RankTown covers populations in the low thousands. Has local
	// market, small garrison, and a parish priest; no specialist
	// crafts.
	RankTown
	// RankCity covers mid-size settlements up to ~20 000 people. Walls,
	// cathedrals, guilds, specialist trades emerge at this tier.
	RankCity
	// RankMetropolis is a capital-scale urban centre (20 000+). Always
	// has a ruling court, standing army, and substantial trade network.
	RankMetropolis
)

// String returns the human-readable name of the rank.
func (r BaseRank) String() string {
	switch r {
	case RankHamlet:
		return "Hamlet"
	case RankTown:
		return "Town"
	case RankCity:
		return "City"
	case RankMetropolis:
		return "Metropolis"
	default:
		return "UnknownBaseRank"
	}
}

// EffectiveRank is the political tier of a settlement — assigned by the
// kingdom simulation, not by size. A small Capital outranks a large
// Vassal; a large Independent outranks a middling Autonomous. Drives
// tribute flow, succession eligibility, and diplomatic privilege.
type EffectiveRank uint8

const (
	// RankIndependent stands outside any kingdom's tribute chain. No
	// obligations, no protection.
	RankIndependent EffectiveRank = iota
	// RankAutonomous is nominally affiliated with a kingdom but keeps
	// its own ruler and taxation. Pays token tribute.
	RankAutonomous
	// RankVassal owes full tribute (15%) to a direct master and is
	// subject to martial levy.
	RankVassal
	// RankCapital is the seat of a kingdom — receives tribute upstream
	// from its vassals. Exactly one Capital per kingdom at any time.
	RankCapital
)

// String returns the human-readable name of the political rank.
func (r EffectiveRank) String() string {
	switch r {
	case RankIndependent:
		return "Independent"
	case RankAutonomous:
		return "Autonomous"
	case RankVassal:
		return "Vassal"
	case RankCapital:
		return "Capital"
	default:
		return "UnknownEffectiveRank"
	}
}

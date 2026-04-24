package mechanics

import (
	"math"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Trade constants. Weights sum to 100 so the resulting TradeScore
// naturally lands in [0, 100]. Each weight maps one raw signal to
// its capped contribution.
const (
	tradeWeightNeighbors    = 30 // Neighbor-city density
	tradeWeightDeposits     = 25 // Deposit-type variety
	tradeWeightWater        = 20 // Water-body adjacency
	tradeWeightPopulation   = 15 // City population log-scaled
	tradeWeightConnectivity = 10 // Road / river connectivity

	// tradePopLogCap is the population at which the pop contribution
	// saturates at its full weight. log10(40 000 + 1) ≈ 4.6 so this
	// divisor produces a number close to 1.0 at the city cap.
	tradePopLogCap = 40000
)

// Hoisted to avoid recomputing on every popLogScore call.
var tradePopLogDivisor = math.Log10(tradePopLogCap + 1)

// ApplyTradeYear recomputes city.TradeScore. MVP version uses only
// the population signal because the other four inputs (neighbors,
// deposits, water, connectivity) require world context that the
// per-city tick does not carry. Each of those five weights is
// honored — the missing-data contributions default to their
// per-component neutral value (0.5) so a population-only tick
// produces a plausible mid-range score rather than starving the
// output at zero. When the world-aware inputs land, the neutral
// fallbacks become real signals and this function's signature does
// not need to change.
func ApplyTradeYear(city *polity.City) {
	popScore := popLogScore(city.Population)

	// Neutral placeholders — replaced when the world-aware inputs
	// land in later milestones. Using 0.5 keeps the score centered
	// around 57 for a 1 000-person town with no known neighbors.
	neighborScore, depositScore, waterScore, connectivityScore :=
		0.5, 0.5, 0.5, 0.5

	raw := float64(tradeWeightPopulation)*popScore +
		float64(tradeWeightNeighbors)*neighborScore +
		float64(tradeWeightDeposits)*depositScore +
		float64(tradeWeightWater)*waterScore +
		float64(tradeWeightConnectivity)*connectivityScore

	score := min(100, max(0, int(raw)))
	score = int(float64(score) * techTradeMultiplier(city))
	score += techTradeFlatBonus(city)

	merchantBonus := int(city.Factions.Get(polity.FactionMerchants) *
		float64(city.Ruler.Stats.Charisma) * merchantTradeBonusScale)
	score += merchantBonus

	city.TradeScore = min(100, max(0, score))
}

// merchantTradeBonusScale is the coefficient applied to
// (Merchants influence × ruler Charisma) when converting the faction's
// grip on civic commerce into a TradeScore bonus. At max influence
// (1.0) and CHA 18 the merchants push trade up by 9 points — roughly
// the same magnitude as a Funded-Trade-Post decree.
const merchantTradeBonusScale = 0.5

// popLogScore maps population into a [0, 1] trade contribution.
// log-scaled so going from 1 000 to 2 000 matters more than 30 000
// to 40 000 — small market towns punch above their headcount.
func popLogScore(pop int) float64 {
	if pop <= 0 {
		return 0
	}
	return math.Min(1.0, math.Log10(float64(pop)+1)/tradePopLogDivisor)
}

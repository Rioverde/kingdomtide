package mechanics

import (
	"math"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

// Pass-1 seeding constants. These helpers produce the initial City
// attributes at worldgen time — unlike the per-year tick functions,
// they run once per city at founding and never again. All draw
// through dice.Stream to preserve the same-seed-same-history
// guarantee.
const (
	// Zipf / Pareto distribution parameters for city populations.
	// rank=0 produces the largest city ("capital scale"); rank grows as
	// the worldgen iterates Poisson-disk placement. The 3 000 multiplier
	// on log10(40 001) ≈ 4.6 lands the rank-0 city around 14 000 — a
	// mid-metropolis — before jitter, matching historical medieval
	// capital sizes.
	zipfLargestSeedFactor = 3000.0
	zipfDecayExponent     = 1.07 // Zipf power-law exponent, slightly > 1

	// Age seed bounds: uniform [10, 1500].
	ageSeedMin = 10
	ageSeedMax = 1500

	// Wealth seed — a base proportional to population's economic
	// output, jittered ±25 % to prevent deterministic clustering.
	wealthSeedPerCapita = 0.5
)

// SeedPopulationZipf returns an initial population for a city at the
// given rank (0 = largest). Follows a Zipf / Pareto distribution so
// a handful of cities dominate and many are small. Clamped to
// [80, 40 000].
func SeedPopulationZipf(stream *dice.Stream, rank int) int {
	rank = max(0, rank)
	largest := zipfLargestSeedFactor * math.Log10(40001)
	pop := largest / math.Pow(float64(rank+1), zipfDecayExponent)

	// Add a ±10 % jitter so two rank-1 cities in different worlds are
	// not identical. D20-based because the Stream already exposes it;
	// (stream.D20() - 10) / 100 gives a float in [-0.09, 0.10].
	jitter := float64(stream.D20()-10) / 100.0
	pop *= 1.0 + jitter

	return min(40000, max(80, int(pop)))
}

// SeedAge draws a founding offset — how long before the current world
// year the city is said to have existed. Uniform [10, 1500]. Returns
// the Age value (in years); the caller subtracts from the current
// year to compute Founded.
func SeedAge(stream *dice.Stream) int {
	// D100 returns [1, 100]. Scale onto [ageSeedMin, ageSeedMax].
	span := ageSeedMax - ageSeedMin + 1
	return ageSeedMin + (stream.D100()-1)*span/100
}

// SeedWealth draws an initial treasury for a city of the given
// population. Base wealth scales with population through a
// per-capita factor, then jittered ±25 % to break deterministic
// clustering. Log-linear scaling by resource-deposit richness lands
// when the deposit system grows a richness aggregate — for now the
// deposit multiplier defaults to 1.0.
func SeedWealth(stream *dice.Stream, population int) int {
	base := float64(population) * wealthSeedPerCapita
	// D20 - 10 gives [-9, +10]. Divide by 40 to land the jitter
	// fraction in [-0.225, +0.25] — a ~25 % envelope with 2.5 %
	// granularity. Close enough to the ±25 % target given the
	// discrete 20-step distribution; if we later need exact ±25 %
	// symmetry we switch to D100 for the extra resolution.
	jitter := float64(stream.D20()-10) / 40.0
	return int(base * (1.0 + jitter))
}

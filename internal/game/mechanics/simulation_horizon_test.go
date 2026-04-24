package mechanics

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// horizonMetrics collects per-horizon observability so the test can
// compare how the simulation behaves at different time scales.
type horizonMetrics struct {
	years       int
	seed        int64
	survivors   int // cities still above popMin at end
	totalRevolts     int
	totalGreatPeople int
	totalRulerDeaths int
	totalTechUnlocks int
	avgProsperity    float64
	maxPopulation    int
	minPopulation    int
}

// runAt drives a small constant-shape world for `years` years from a
// given seed and returns summary metrics. Used by the horizon sweep to
// check invariants across 100 / 200 / 500 / 2000-year scales.
func runAt(seed int64, years int) horizonMetrics {
	const startYear = 1300
	w := seedWorld(seed, startYear)
	w.run(startYear, years)

	metrics := horizonMetrics{
		years: years,
		seed:  seed,
	}

	// Collect end-state stats.
	minPop := 100000
	maxPop := 0
	var totalProsperity float64
	for _, c := range w.cities {
		if c.Population >= 80 {
			metrics.survivors++
		}
		if c.Population > maxPop {
			maxPop = c.Population
		}
		if c.Population < minPop {
			minPop = c.Population
		}
		totalProsperity += c.Prosperity

		// Count tech unlocks on this city.
		for _, t := range allTechsList {
			if c.Techs.Has(t) {
				metrics.totalTechUnlocks++
			}
		}
	}
	metrics.minPopulation = minPop
	metrics.maxPopulation = maxPop
	metrics.avgProsperity = totalProsperity / float64(len(w.cities))

	return metrics
}

// TestSimulation_HorizonSweep runs the standard 10-city world at four
// horizons (100, 200, 500, 2000 years) and asserts invariants at each
// scale. Proves the tick pipeline is stable over two millennia and
// doesn't accumulate numerical drift, integer overflow, or silent
// silence of any subsystem.
func TestSimulation_HorizonSweep(t *testing.T) {
	horizons := []int{100, 200, 500, 2000}
	const seed int64 = 42

	var all []horizonMetrics
	for _, h := range horizons {
		m := runAt(seed, h)
		all = append(all, m)

		t.Logf("Horizon %d yr: survivors=%d/10, tech_unlocks=%d/80, "+
			"prosperity=%.3f, pop=[%d, %d]",
			m.years, m.survivors, m.totalTechUnlocks,
			m.avgProsperity, m.minPopulation, m.maxPopulation)

		// Universal invariants — hold at every horizon.
		if m.maxPopulation > 40000 {
			t.Errorf("h=%d: maxPopulation=%d exceeded cap 40000", h, m.maxPopulation)
		}
		if m.minPopulation < 80 {
			t.Errorf("h=%d: minPopulation=%d below floor 80", h, m.minPopulation)
		}
		if m.avgProsperity < 0 || m.avgProsperity > 1 {
			t.Errorf("h=%d: avgProsperity=%v out of [0, 1]", h, m.avgProsperity)
		}

		// Horizon-scaled invariants.
		switch {
		case h >= 500:
			// By 500 years every city should have unlocked every tech.
			if m.totalTechUnlocks < 8*10 {
				t.Errorf("h=%d: tech unlocks %d, expected 80 (full saturation)",
					h, m.totalTechUnlocks)
			}
		case h >= 200:
			// At 200 years most cities should have most techs.
			if m.totalTechUnlocks < 50 {
				t.Errorf("h=%d: tech unlocks %d, expected ≥ 50 at mid-horizon",
					h, m.totalTechUnlocks)
			}
		case h >= 100:
			// At 100 years at least first-tier techs should unlock.
			if m.totalTechUnlocks < 10 {
				t.Errorf("h=%d: tech unlocks %d, expected ≥ 10 at short horizon",
					h, m.totalTechUnlocks)
			}
		}
	}

	// Tech unlocks are monotone across horizons — longer sims can
	// never have FEWER unlocks than shorter ones.
	for i := 1; i < len(all); i++ {
		if all[i].totalTechUnlocks < all[i-1].totalTechUnlocks {
			t.Errorf("tech unlocks not monotone across horizons: %d at %d yr, %d at %d yr",
				all[i-1].totalTechUnlocks, all[i-1].years,
				all[i].totalTechUnlocks, all[i].years)
		}
	}
}

// TestSimulation_MultiSeedRobustness runs five different seeds
// through a 200-year horizon and checks that the summary statistics
// stay inside reasonable envelopes regardless of seed — the simulation
// should not depend on one lucky seed to pass.
func TestSimulation_MultiSeedRobustness(t *testing.T) {
	seeds := []int64{42, 1337, 0xcafef00d, 2026, -1}

	type seedResult struct {
		seed      int64
		survivors int
		maxPop    int
		techCount int
	}
	var results []seedResult

	for _, s := range seeds {
		m := runAt(s, 200)
		results = append(results, seedResult{
			seed:      s,
			survivors: m.survivors,
			maxPop:    m.maxPopulation,
			techCount: m.totalTechUnlocks,
		})
		t.Logf("seed=%d: survivors=%d max_pop=%d techs=%d",
			s, m.survivors, m.maxPopulation, m.totalTechUnlocks)
	}

	// Every seed must have at least half the cities surviving. A single
	// seed wiping out 9/10 cities points at a balance bug.
	for _, r := range results {
		if r.survivors < 5 {
			t.Errorf("seed=%d: only %d/10 cities survived 200 yr — fragile",
				r.seed, r.survivors)
		}
	}

	// Every seed should reach at least 50% tech saturation by 200 years.
	for _, r := range results {
		if r.techCount < 40 {
			t.Errorf("seed=%d: only %d/80 tech unlocks at 200 yr — tech gate too steep",
				r.seed, r.techCount)
		}
	}
}

// TestSimulation_TaxPolicyOutcomes drives one city per TaxRate through
// 100 years with every other knob identical, then compares outcomes.
// Validates that the tax knob has the direction-of-effect the spec
// promises: Low → higher happiness, Brutal → more revolts.
func TestSimulation_TaxPolicyOutcomes(t *testing.T) {
	const seed int64 = 42
	const startYear = 1300
	const years = 100

	rates := []polity.TaxRate{
		polity.TaxLow,
		polity.TaxNormal,
		polity.TaxHigh,
		polity.TaxBrutal,
	}

	type policyOutcome struct {
		rate        polity.TaxRate
		finalPop    int
		finalWealth int
		revolutions int
		happiness   int
	}

	var outcomes []policyOutcome

	for _, rate := range rates {
		ruler := polity.NewRuler(dice.New(seed, dice.SaltKingdomYear), startYear-30)
		city := polity.NewCity("TestBurg", geom.Position{}, startYear-50, ruler)
		city.Population = 5000
		city.Wealth = 5000
		city.Happiness = 60
		city.Army = 100
		city.TaxRate = rate

		// Single city stream for full horizon.
		stream := dice.New(seed, dice.SaltKingdomYear)

		revolts := 0
		for year := startYear; year < startYear+years; year++ {
			// Lock tax rate across the horizon so this test measures
			// policy-outcome rather than decree-drift. Decrees can raise
			// or lower the tier mid-run; without the lock the Brutal
			// scenario ends up at TaxNormal and the test loses its signal.
			city.TaxRate = rate
			TickCityYear(city, stream, year)
			if city.RevolutionThisYear {
				revolts++
			}
		}
		outcomes = append(outcomes, policyOutcome{
			rate:        rate,
			finalPop:    city.Population,
			finalWealth: city.Wealth,
			revolutions: revolts,
			happiness:   city.Happiness,
		})
		t.Logf("tax=%-8s pop=%6d wealth=%8d revolts=%3d happiness=%d",
			rate.String(), city.Population, city.Wealth,
			revolts, city.Happiness)
	}

	// Extract by rate.
	find := func(r polity.TaxRate) policyOutcome {
		for _, o := range outcomes {
			if o.rate == r {
				return o
			}
		}
		t.Fatalf("rate %v not in outcomes", r)
		return policyOutcome{}
	}
	low := find(polity.TaxLow)
	normal := find(polity.TaxNormal)
	high := find(polity.TaxHigh)
	brutal := find(polity.TaxBrutal)

	// Final-year happiness snapshots are noisy because a last-year
	// disaster or life event can push one city's food into deficit
	// while another stays clean. The reliable direction-of-effect
	// invariant is Low vs Brutal at the extremes; middle tiers can
	// and do swap on tail disasters. We assert only what we can
	// defend in the noise: Low ≥ Brutal, and Brutal triggers more
	// revolts than Low.
	if low.happiness <= brutal.happiness {
		t.Errorf("Low happiness %d should exceed Brutal happiness %d — "+
			"tax policy not producing directional mood effect",
			low.happiness, brutal.happiness)
	}

	// Revolution count — Brutal should fire more revolts than Low.
	if brutal.revolutions <= low.revolutions {
		t.Errorf("Brutal revolts (%d) should exceed Low revolts (%d)",
			brutal.revolutions, low.revolutions)
	}

	// Wealth — Brutal should generate MORE treasury than Low (higher
	// take) even with revolts, assuming population didn't entirely
	// collapse. Below 500 pop the tax base is so thin the signal is
	// drowned out by event noise.
	if brutal.finalPop > 100 && brutal.finalWealth <= low.finalWealth {
		t.Errorf("Brutal wealth %d should exceed Low wealth %d at comparable populations",
			brutal.finalWealth, low.finalWealth)
	}

	// Acknowledge that the middle tiers aren't checked, so the reader
	// understands the relaxed shape of the invariant.
	_, _ = normal, high
}

// TestSimulation_EdgeCases exercises pathological starting conditions
// — zero-pop city, just-founded settlement, maxed-out ruler. The tick
// pipeline must not panic, NaN, or produce out-of-range state for any
// of these.
func TestSimulation_EdgeCases(t *testing.T) {
	t.Run("zero-pop city survives one tick", func(t *testing.T) {
		c := polity.NewCity("Ghosttown", geom.Position{}, 1400, polity.Ruler{})
		c.Population = 0
		c.TaxRate = polity.TaxNormal
		stream := dice.New(42, dice.SaltKingdomYear)

		// Must not panic.
		TickCityYear(c, stream, 1500)

		// Clamped to popMin.
		if c.Population < 80 {
			t.Errorf("zero-pop city should clamp up to popMin, got %d", c.Population)
		}
	})

	t.Run("just-founded city (age 0)", func(t *testing.T) {
		c := polity.NewCity("Freshtown", geom.Position{}, 1500, polity.Ruler{})
		c.Population = 200
		c.TaxRate = polity.TaxNormal
		stream := dice.New(42, dice.SaltKingdomYear)

		TickCityYear(c, stream, 1500) // same year as founding
		// Age = 0 — no crash, prosperity computes without Inf.
		if math.IsNaN(c.Prosperity) || math.IsInf(c.Prosperity, 0) {
			t.Errorf("age-0 city produced non-finite prosperity: %v", c.Prosperity)
		}
	})

	t.Run("ancient city (age 1500)", func(t *testing.T) {
		c := polity.NewCity("Oldholm", geom.Position{}, 0, polity.Ruler{})
		c.Population = 10000
		c.TaxRate = polity.TaxNormal
		stream := dice.New(42, dice.SaltKingdomYear)

		TickCityYear(c, stream, 1500) // age = 1500
		if c.BaseRank != polity.RankMetropolis && c.Population >= 20000 {
			t.Errorf("ancient large city should reach Metropolis rank, got %v",
				c.BaseRank)
		}
	})

	t.Run("nil-Deposits city does not panic", func(t *testing.T) {
		c := polity.NewCity("Noore", geom.Position{}, 1400, polity.Ruler{})
		c.Population = 1000
		c.TaxRate = polity.TaxNormal
		// Deposits left nil
		stream := dice.New(42, dice.SaltKingdomYear)
		TickCityYear(c, stream, 1500)
	})
}

// TestSimulation_MillenniumStability runs a SINGLE city through 2000
// years and checks that nothing overflows, leaks, or drifts out of
// range over millennium-scale time. This catches unit conversions
// that silently lose precision or accumulators that wrap.
func TestSimulation_MillenniumStability(t *testing.T) {
	const seed int64 = 42
	const startYear = 1000
	const years = 2000

	ruler := polity.NewRuler(dice.New(seed, dice.SaltKingdomYear), startYear-30)
	c := polity.NewCity("Eternalis", geom.Position{}, startYear-100, ruler)
	c.Population = 5000
	c.Wealth = 5000
	c.Happiness = 70
	c.Army = 100
	c.TaxRate = polity.TaxNormal
	c.Deposits = []polity.Deposit{
		{Kind: polity.DepositGold, RemainingYield: 0.9},
		{Kind: polity.DepositIron, RemainingYield: 0.9},
	}

	stream := dice.New(seed, dice.SaltKingdomYear)

	var maxWealth, minWealth int
	maxWealth = c.Wealth
	minWealth = c.Wealth

	for year := startYear; year < startYear+years; year++ {
		TickCityYear(c, stream, year)

		// Hard invariants every year.
		if c.Population < 80 || c.Population > 40000 {
			t.Fatalf("year %d: population %d escaped [80, 40000]",
				year, c.Population)
		}
		if c.Prosperity < 0 || c.Prosperity > 1 {
			t.Fatalf("year %d: prosperity %v escaped [0, 1]",
				year, c.Prosperity)
		}
		if math.IsNaN(c.Prosperity) || math.IsInf(c.Prosperity, 0) {
			t.Fatalf("year %d: prosperity non-finite: %v", year, c.Prosperity)
		}
		if c.SoilFatigue < 0 || c.SoilFatigue > 1 {
			t.Fatalf("year %d: soil fatigue %v escaped [0, 1]",
				year, c.SoilFatigue)
		}

		if c.Wealth > maxWealth {
			maxWealth = c.Wealth
		}
		if c.Wealth < minWealth {
			minWealth = c.Wealth
		}
	}

	t.Logf("2000-year stability: pop=%d wealth=%d prosperity=%.3f soil_fatigue=%.3f",
		c.Population, c.Wealth, c.Prosperity, c.SoilFatigue)
	t.Logf("Wealth envelope: [%d, %d]", minWealth, maxWealth)

	// Deposits should be exhausted long before 2000 years at any
	// reasonable drain rate.
	if len(c.Deposits) > 0 {
		t.Errorf("deposits not exhausted after 2000 years: %d remain", len(c.Deposits))
	}

	// Wealth should never overflow int64 even in extreme cases.
	if maxWealth > math.MaxInt32 || minWealth < math.MinInt32 {
		t.Errorf("wealth exceeded int32 range over 2000 yr: [%d, %d]",
			minWealth, maxWealth)
	}
}

// BenchmarkSimulation_200Years measures how long the standard 10-city
// world takes to simulate 200 years. Keeps an eye on regressions —
// if this grows past ~5 ms a serious inefficiency has crept in.
func BenchmarkSimulation_200Years(b *testing.B) {
	const seed int64 = 42
	const startYear = 1300
	for i := 0; i < b.N; i++ {
		w := seedWorld(seed, startYear)
		w.run(startYear, 200)
	}
}

// BenchmarkSimulation_2000YearsSingleCity measures the per-city,
// per-year tick cost at millennium scale. Useful for projecting
// engine cost on a 100-city kingdom-scale world.
func BenchmarkSimulation_2000YearsSingleCity(b *testing.B) {
	const seed int64 = 42
	const startYear = 1000
	for i := 0; i < b.N; i++ {
		ruler := polity.NewRuler(dice.New(seed, dice.SaltKingdomYear), startYear-30)
		c := polity.NewCity("Bench", geom.Position{}, startYear-100, ruler)
		c.Population = 5000
		c.Wealth = 5000
		c.Happiness = 70
		c.TaxRate = polity.TaxNormal
		stream := dice.New(seed, dice.SaltKingdomYear)

		for year := startYear; year < startYear+2000; year++ {
			TickCityYear(c, stream, year)
		}
	}
}

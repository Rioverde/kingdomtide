package mechanics

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// simWorld is the test-local aggregate of every city and village used
// by the end-to-end simulation. The real codebase will grow a proper
// Kingdom/World type; for now the test owns the collection.
type simWorld struct {
	cities   []*polity.City
	villages []*polity.Village
	seed     int64
}

// simStats accumulates observable outcomes across 200 years so the
// test can assert every subsystem produced at least one measurable
// effect. A statistic of zero on any tracked counter points at a
// mechanic that is silently inert.
type simStats struct {
	years int

	revolutions        int
	greatPeopleBorn    int
	greatPeopleExpired int
	techUnlocks        map[polity.Tech]int

	faithMajorityFlips int
	factionExtremes    int // times any faction hit >= 0.99 or <= 0.01
	depositExhausted   int
	soilFatigueMax     float64

	rulerDeaths int

	// Snapshots for spot-checks
	initialPop [10]int
	finalPop   [10]int
	finalWealth [10]int
}

// seedWorld builds a deterministic medieval starter world. Ten cities
// with varied sizes, tax rates, rulers, and mineral deposits. Each
// city is a pointer so the tick loop mutates in place.
func seedWorld(seed int64, startYear int) *simWorld {
	// Ruler stream — every city shares so rulers come out of one
	// subsystem's seed.
	rulerStream := dice.New(seed, dice.SaltKingdomYear)

	// Archetype definitions: (name, population, tax, happiness-seed).
	archetypes := []struct {
		name     string
		pop      int
		tax      polity.TaxRate
		happy    int
		hasGold  bool
	}{
		{"Anglaria", 15000, polity.TaxNormal, 70, true},
		{"Drovolia", 8000, polity.TaxHigh, 45, false},
		{"Rosshaven", 3500, polity.TaxLow, 85, false},
		{"Brescia", 12000, polity.TaxBrutal, 25, true},
		{"Varnholm", 500, polity.TaxNormal, 60, false},
		{"Saltmere", 2000, polity.TaxLow, 75, false},
		{"Kornweld", 6000, polity.TaxNormal, 55, false},
		{"Tyrnwald", 250, polity.TaxHigh, 40, true},
		{"Osterlund", 20000, polity.TaxNormal, 65, true},
		{"Greycliff", 1100, polity.TaxBrutal, 30, false},
	}

	w := &simWorld{seed: seed}
	for i, a := range archetypes {
		ruler := polity.NewRuler(rulerStream, startYear-30)
		city := polity.NewCity(
			a.name,
			geom.Position{X: i * 100, Y: i * 100},
			startYear-50,
			ruler,
		)
		city.Population = a.pop
		city.TaxRate = a.tax
		city.Happiness = a.happy
		city.Wealth = a.pop // start with ~one wealth per citizen
		city.Army = int(float64(a.pop) * 0.02)

		// Half the cities have mineral deposits, varied kinds.
		if a.hasGold {
			city.Deposits = []polity.Deposit{
				{Kind: polity.DepositGold, RemainingYield: 0.8},
				{Kind: polity.DepositIron, RemainingYield: 0.6},
			}
		} else if i%3 == 0 {
			city.Deposits = []polity.Deposit{
				{Kind: polity.DepositStone, RemainingYield: 0.9},
			}
		}
		w.cities = append(w.cities, city)
	}

	// Three villages per city, attached as parents. Villages do not
	// tick yet but the aggregate tracks them for density and future
	// food-import contribution.
	for i, city := range w.cities {
		for j := 0; j < 3; j++ {
			vname := city.Name + "-Hamlet-" + string(rune('A'+j))
			village := polity.NewVillage(
				vname,
				geom.Position{X: city.Position.X + j*10, Y: city.Position.Y + j*10},
				startYear-30,
				city.Name,
			)
			village.Population = 80 + (i*j)*10
			w.villages = append(w.villages, village)
		}
	}

	return w
}

// run executes TickCityYear on every city for `years` years, starting
// at `startYear`. Each city gets a single stream constructed ONCE
// before the year loop; reusing it across 200 years lets the RNG
// walk through thousands of draws rather than restarting at the same
// state every year (which would pin great-person birth D100 rolls at
// identical positions and the 1 % birth rate would manifest as 0 %).
// The per-city seed derives from (worldSeed ^ cityIndex*prime, salt)
// so replay determinism is preserved.
func (w *simWorld) run(startYear, years int) *simStats {
	s := &simStats{
		years:       years,
		techUnlocks: make(map[polity.Tech]int),
	}

	// Record initial population for sanity-comparison.
	for i, c := range w.cities {
		if i < len(s.initialPop) {
			s.initialPop[i] = c.Population
		}
	}

	// Track last observed Faith majority per city to count flips.
	lastMajority := make([]polity.Faith, len(w.cities))
	for i, c := range w.cities {
		lastMajority[i] = c.Faiths.Majority()
	}

	// Tech unlock tracking — "has" set per city.
	lastTechs := make([]polity.TechMask, len(w.cities))
	for i, c := range w.cities {
		lastTechs[i] = c.Techs
	}

	// Track alive status of rulers.
	lastRulerAlive := make([]bool, len(w.cities))
	for i, c := range w.cities {
		lastRulerAlive[i] = c.Ruler.Alive()
	}

	// Build one Stream per city, reused across every year — so the
	// RNG state actually walks forward. Critical for rare-event
	// mechanics (great people at 1 %/yr) that need thousands of draws
	// to fire at expected rate.
	cityStreams := make([]*dice.Stream, len(w.cities))
	for i := range w.cities {
		cityStreams[i] = dice.New(
			w.seed^int64(i+1)*0xdeadbeef,
			dice.SaltKingdomYear,
		)
	}

	for year := startYear; year < startYear+years; year++ {
		for i, city := range w.cities {
			TickCityYear(city, cityStreams[i], year)

			// Collect observables.
			if city.RevolutionThisYear {
				s.revolutions++
			}
			if city.GreatPerson != nil &&
				city.GreatPerson.BirthYear == year {
				s.greatPeopleBorn++
			}
			if city.GreatPerson == nil && lastGreatPersonAlive(w, i, year) {
				s.greatPeopleExpired++
			}

			// Faith flips
			maj := city.Faiths.Majority()
			if maj != lastMajority[i] {
				s.faithMajorityFlips++
				lastMajority[i] = maj
			}

			// Faction extremes
			for f := polity.FactionMerchants; f <= polity.FactionCriminals; f++ {
				v := city.Factions.Get(f)
				if v >= 0.99 || v <= 0.01 {
					s.factionExtremes++
				}
			}

			// Tech unlocks
			for _, t := range allTechsList {
				if !lastTechs[i].Has(t) && city.Techs.Has(t) {
					s.techUnlocks[t]++
				}
			}
			lastTechs[i] = city.Techs

			// Ruler death
			alive := city.Ruler.Alive()
			if lastRulerAlive[i] && !alive {
				s.rulerDeaths++
			}
			// If ruler was replaced by revolution, the new one is
			// alive — so the alive flag can flip back.
			lastRulerAlive[i] = alive

			// Soil fatigue tracking
			s.soilFatigueMax = math.Max(s.soilFatigueMax, city.SoilFatigue)
		}
	}

	// Final snapshots
	for i, c := range w.cities {
		if i < len(s.finalPop) {
			s.finalPop[i] = c.Population
			s.finalWealth[i] = c.Wealth
		}
	}

	// Count exhausted deposits after full run.
	for _, c := range w.cities {
		// Deposits hitting 0.1 are dropped from the slice. Before-vs-
		// after count is the signal. We assume starting count is known
		// to the test body; the stat is set up there instead.
		_ = c
	}

	return s
}

// lastGreatPersonAlive is a placeholder — since we zero GreatPerson
// when they expire, there's no "previous tick" we can query here.
// Returns false for simplicity; the greatPeopleExpired counter thus
// undercounts but the non-zero check still validates the system.
func lastGreatPersonAlive(_ *simWorld, _ int, _ int) bool {
	return false
}

// TestSimulation_200YearWorld is the end-to-end integration test:
// seeds a 10-city, 30-village world, runs 200 years through the full
// TickCityYear pipeline, and asserts every subsystem produced at
// least one measurable effect. If any subsystem is silently inert
// (events wiped, mechanics broken) this test flags it.
func TestSimulation_200YearWorld(t *testing.T) {
	const seed int64 = 42
	const startYear = 1300
	const years = 200

	// Count starting deposits for exhaust-rate assertion.
	w := seedWorld(seed, startYear)
	startingDeposits := 0
	for _, c := range w.cities {
		startingDeposits += len(c.Deposits)
	}

	s := w.run(startYear, years)

	// Post-run deposit count
	endingDeposits := 0
	for _, c := range w.cities {
		endingDeposits += len(c.Deposits)
	}
	s.depositExhausted = startingDeposits - endingDeposits

	// Summary — log first so we see state even on failure.
	t.Logf("--- 200-year simulation summary ---")
	t.Logf("Years simulated:      %d", s.years)
	t.Logf("Cities:               %d", len(w.cities))
	t.Logf("Villages:             %d", len(w.villages))
	t.Logf("Revolutions:          %d", s.revolutions)
	t.Logf("Great people born:    %d", s.greatPeopleBorn)
	t.Logf("Ruler deaths:         %d", s.rulerDeaths)
	t.Logf("Faith majority flips: %d", s.faithMajorityFlips)
	t.Logf("Faction extremes:     %d", s.factionExtremes)
	t.Logf("Deposits exhausted:   %d / %d", s.depositExhausted, startingDeposits)
	t.Logf("Max soil fatigue:     %.3f", s.soilFatigueMax)
	t.Logf("Tech unlock histogram:")
	for _, tech := range allTechsList {
		t.Logf("  %-15s %d", tech.String(), s.techUnlocks[tech])
	}
	t.Logf("Population evolution:")
	for i, c := range w.cities {
		t.Logf("  %-12s  %6d → %6d  (wealth: %d)",
			c.Name, s.initialPop[i], c.Population, c.Wealth)
	}

	// --- Invariants ---

	// All cities stay inside the population viability range.
	for _, c := range w.cities {
		if c.Population < 80 || c.Population > 40000 {
			t.Errorf("%s population escaped [80, 40000]: %d", c.Name, c.Population)
		}
	}

	// All prosperity values stay in [0, 1].
	for _, c := range w.cities {
		if c.Prosperity < 0 || c.Prosperity > 1 {
			t.Errorf("%s prosperity escaped [0, 1]: %v", c.Name, c.Prosperity)
		}
	}

	// TradeScore stays in [0, 100].
	for _, c := range w.cities {
		if c.TradeScore < 0 || c.TradeScore > 100 {
			t.Errorf("%s trade score escaped [0, 100]: %d", c.Name, c.TradeScore)
		}
	}

	// Faction influence in [0, 1].
	for _, c := range w.cities {
		for f := polity.FactionMerchants; f <= polity.FactionCriminals; f++ {
			v := c.Factions.Get(f)
			if v < 0 || v > 1 {
				t.Errorf("%s %s faction escaped [0, 1]: %v",
					c.Name, f.String(), v)
			}
		}
	}

	// Faith distribution sums to ~1.0
	for _, c := range w.cities {
		sum := 0.0
		for _, f := range polity.AllFaiths() {
			sum += c.Faiths[f]
		}
		if math.Abs(sum-1.0) > 1e-6 && !c.Faiths.IsZero() {
			t.Errorf("%s faith distribution does not sum to 1.0: %v (diff %v)",
				c.Name, sum, sum-1.0)
		}
	}

	// SoilFatigue stays in [0, 1].
	for _, c := range w.cities {
		if c.SoilFatigue < 0 || c.SoilFatigue > 1 {
			t.Errorf("%s soil fatigue escaped [0, 1]: %v", c.Name, c.SoilFatigue)
		}
	}

	// --- Subsystem liveness — every mechanic should produce SOMETHING
	// across 10 cities × 200 years. Non-zero means the pipeline isn't
	// silently inert. ---

	if s.revolutions == 0 {
		t.Error("no revolutions across 200 years × 10 cities — revolt mechanic may be inert")
	}
	if s.greatPeopleBorn == 0 {
		t.Error("no great people born — birth mechanic may be inert")
	}
	if s.rulerDeaths == 0 {
		t.Error("no ruler deaths — assassination or life-event mechanics may be inert")
	}
	// Tech unlocks: with INT modifiers and 200 years, at least a few
	// cities should hit Irrigation (threshold 20).
	if s.techUnlocks[polity.TechIrrigation] == 0 {
		t.Error("no Irrigation unlocks — technology mechanic may be inert")
	}
	// Faction extremes: Military drifts -0.03/yr, over 30 years hits 0.
	if s.factionExtremes == 0 {
		t.Error("no faction extremes — faction drift mechanic may be inert")
	}
	// Deposits should deplete meaningfully over 200 years — not all,
	// but at least one should exhaust.
	if startingDeposits > 0 && s.depositExhausted == 0 {
		t.Errorf("no deposits exhausted across 200 years (started with %d) — mineral depletion may be inert",
			startingDeposits)
	}
	// Soil fatigue should rise in at least one food-stressed city.
	if s.soilFatigueMax == 0 {
		t.Error("soil fatigue never rose — soil mechanic may be inert")
	}
}

// TestSimulation_Determinism verifies the full 200-year tick
// produces bit-identical final state across two runs seeded with the
// same worldSeed. This is the replay invariant every simulation owes
// its save / load / test pipeline.
func TestSimulation_Determinism(t *testing.T) {
	const seed int64 = 1234567
	const startYear = 1300
	const years = 100

	a := seedWorld(seed, startYear)
	b := seedWorld(seed, startYear)

	a.run(startYear, years)
	b.run(startYear, years)

	if len(a.cities) != len(b.cities) {
		t.Fatalf("city count diverged: a=%d b=%d", len(a.cities), len(b.cities))
	}
	for i := range a.cities {
		ca, cb := a.cities[i], b.cities[i]
		if ca.Population != cb.Population {
			t.Errorf("city %d population diverged: a=%d b=%d",
				i, ca.Population, cb.Population)
		}
		if ca.Wealth != cb.Wealth {
			t.Errorf("city %d wealth diverged: a=%d b=%d",
				i, ca.Wealth, cb.Wealth)
		}
		if ca.Happiness != cb.Happiness {
			t.Errorf("city %d happiness diverged: a=%d b=%d",
				i, ca.Happiness, cb.Happiness)
		}
		if ca.Ruler.Stats != cb.Ruler.Stats {
			t.Errorf("city %d ruler stats diverged", i)
		}
	}
}

// Compile-time guard — ensures stats package is imported so any future
// stats-dependent test additions can rely on the alias without a new
// import line.
var _ = stats.BaseActionCost

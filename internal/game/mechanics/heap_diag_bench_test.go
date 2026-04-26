package mechanics

import (
	"runtime"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// diagBenchSeed is the fixed seed used across all heap-diagnostic
// benchmarks so results are deterministic and comparable run-to-run.
const diagBenchSeed int64 = 0xdeadbeef

// diagCity builds a well-populated city that exercises every subsystem
// at realistic depth: 50+ historical mods (so recrystallize has real
// work), all four factions set, all five faiths seeded, an active
// ruler with full stats, two deposits, and non-trivial wealth/pop.
func diagCity() *polity.City {
	stream := dice.New(diagBenchSeed, dice.SaltKingdomYear)
	ruler := polity.NewRuler(stream, 1250, "")
	ruler.Stats = stats.CoreStats{
		Strength:     14,
		Dexterity:    12,
		Constitution: 13,
		Intelligence: 15,
		Wisdom:       11,
		Charisma:     14,
	}

	c := polity.NewCity("DiagBench", geom.Position{}, 1200, ruler)
	c.Population = 8000
	c.Wealth = 6000
	c.Army = 200
	c.Happiness = 65
	c.TaxRate = polity.TaxNormal
	c.TradeScore = 40
	c.SoilFatigue = 0.15
	c.Innovation = 30
	c.Culture = polity.CultureFeudal

	c.Deposits = []polity.Deposit{
		{Kind: polity.DepositGold, RemainingYield: 0.85},
		{Kind: polity.DepositIron, RemainingYield: 0.80},
	}

	// Seed all four factions so drift logic sees real competition.
	c.Factions.Set(polity.FactionMerchants, 0.55)
	c.Factions.Set(polity.FactionMilitary, 0.40)
	c.Factions.Set(polity.FactionMages, 0.30)
	c.Factions.Set(polity.FactionCriminals, 0.20)

	// Seed faiths with a realistic non-uniform distribution.
	c.Faiths[polity.FaithOldGods] = 0.60
	c.Faiths[polity.FaithSunCovenant] = 0.20
	c.Faiths[polity.FaithGreenSage] = 0.10
	c.Faiths[polity.FaithOneOath] = 0.07
	c.Faiths[polity.FaithStormPact] = 0.03

	// Build 55 historical mods so recrystallize is exercised at a
	// realistic queue depth that reflects long-running sims.
	mods := make([]polity.HistoricalMod, 55)
	kinds := []polity.HistoricalModKind{
		polity.HistoricalModHappiness,
		polity.HistoricalModWealth,
		polity.HistoricalModArmy,
		polity.HistoricalModFoodBalance,
	}
	for i := range mods {
		mods[i] = polity.HistoricalMod{
			Kind:        kinds[i%len(kinds)],
			Magnitude:   (i % 11) - 5,
			YearApplied: 1280 + i%20,
			DecayYears:  5 + i%10,
		}
	}
	c.HistoricalMods = mods
	return c
}

// diagStream returns a deterministic stream for subsystem benchmarks.
func diagStream() *dice.Stream {
	return dice.New(diagBenchSeed, dice.SaltKingdomYear)
}

// Part A — per-subsystem isolation benches. Each bench seeds a city
// and stream once outside the timed region, then hot-loops one
// subsystem call. All are named BenchmarkSubsystem_<Step> so a single
// -bench=BenchmarkSubsystem filter runs the full set.

func BenchmarkSubsystem_ApplyFoodYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyFoodYear(c, s)
	}
}

func BenchmarkSubsystem_ApplySoilFatigueYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplySoilFatigueYear(c)
	}
}

func BenchmarkSubsystem_ApplyPopulationYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyPopulationYear(c)
	}
}

func BenchmarkSubsystem_ApplyMineralDepletionYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyMineralDepletionYear(c)
	}
}

func BenchmarkSubsystem_ApplyTechnologyYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyTechnologyYear(c, s)
	}
}

func BenchmarkSubsystem_ApplyGreatPeopleYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyGreatPeopleYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyFactionDriftYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyFactionDriftYear(c, s)
	}
}

func BenchmarkSubsystem_ApplyReligionDiffusionYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyReligionDiffusionYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyRulerLifeEventsYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyRulerLifeEventsYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyNaturalDisastersYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyNaturalDisastersYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyDecreeYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyDecreeYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyEconomicYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyEconomicYear(c, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyHappinessYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyHappinessYear(c, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyRevolutionCheckYear(b *testing.B) {
	c := diagCity()
	s := diagStream()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyRevolutionCheckYear(c, s, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyRecrystallizeYear(b *testing.B) {
	// Rebuild the mod queue each iteration so the bench measures prune
	// cost, not a no-op on an already-empty queue.
	base := diagCity().HistoricalMods
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := polity.City{HistoricalMods: append([]polity.HistoricalMod(nil), base...)}
		ApplyRecrystallizeYear(&c, 1300+i%50)
	}
}

func BenchmarkSubsystem_ApplyProsperityYear(b *testing.B) {
	c := diagCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyProsperityYear(c, 1300+i%50)
	}
}

// Part B — phase-by-phase MemStats snapshot bench.
// Runs one full simulated year across 1000 cities in explicit ordered
// phases, capturing runtime.ReadMemStats deltas between each phase to
// attribute heap spend per phase. Results are printed via b.Logf so
// they appear with -v or -bench=... output.

// phaseMemStats accumulates TotalAlloc and Mallocs deltas per named
// phase. Package-level so the bench can accumulate across b.N.
var (
	phaseAllocBytes  [6]uint64
	phaseMallocCount [6]uint64
)

// BenchmarkTickYearPhases_MemStats_1000Cities runs one full year of
// all simulation phases for 1000 cities, measuring heap spend per
// phase. Build setup is outside the timed region.
func BenchmarkTickYearPhases_MemStats_1000Cities(b *testing.B) {
	const cityCount = 1000
	const startYear = 1300

	// Build world outside the timed region.
	cities := make([]*polity.City, cityCount)
	streams := make([]*dice.Stream, cityCount)
	for i := 0; i < cityCount; i++ {
		cities[i] = diagCity()
		streams[i] = dice.New(diagBenchSeed^int64(i+1), dice.SaltKingdomYear)
	}

	// Two kingdoms sharing the cities for kingdom/mulk phases.
	founder := polity.NewRuler(dice.New(diagBenchSeed, dice.SaltKingdomYear), 1250, "")
	founder.Stats = stats.CoreStats{Constitution: 14, Charisma: 13}

	k1 := polity.NewKingdom("K1", "Bench Alpha", founder, cities[0].Name,
		polity.SuccessionPrimogeniture, 1250)
	k2 := polity.NewKingdom("K2", "Bench Beta", founder, cities[500].Name,
		polity.SuccessionPrimogeniture, 1250)
	k1.Culture = polity.CultureFeudal
	k2.Culture = polity.CultureSteppe

	cityMap := make(map[string]*polity.City, cityCount)
	for i, c := range cities {
		cityMap[c.Name] = c
		if i > 0 && i < 500 {
			k1.CityIDs = append(k1.CityIDs, c.Name)
		} else if i > 500 {
			k2.CityIDs = append(k2.CityIDs, c.Name)
		}
	}

	k1Stream := dice.New(diagBenchSeed^0x1111, dice.SaltKingdomYear)
	k2Stream := dice.New(diagBenchSeed^0x2222, dice.SaltKingdomYear)
	mulkStream := dice.New(diagBenchSeed^0x3333, dice.SaltKingdomYear)

	league := polity.NewLeague("L1", "DiagLeague",
		cities[1].Name, cities[2].Name, 1280)
	leagueStream := dice.New(diagBenchSeed^0x4444, dice.SaltKingdomYear)

	// Phase names for logging.
	phaseNames := [6]string{
		"TickCitiesYear",
		"TickKingdomYear",
		"TickLeagueYear",
		"ApplyInterPolityEventsYear",
		"ApplyMulkCycleYear",
		"(no villages)",
	}

	var ms0, ms1 runtime.MemStats
	b.ReportAllocs()
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		year := startYear + n%100

		// Phase 0: city tick (parallelized).
		runtime.ReadMemStats(&ms0)
		TickCitiesYear(cities, streams, year)
		runtime.ReadMemStats(&ms1)
		phaseAllocBytes[0] += ms1.TotalAlloc - ms0.TotalAlloc
		phaseMallocCount[0] += ms1.Mallocs - ms0.Mallocs

		// Phase 1: kingdom ticks (serial).
		runtime.ReadMemStats(&ms0)
		TickKingdomYear(k1, cityMap, k1Stream, year)
		TickKingdomYear(k2, cityMap, k2Stream, year)
		runtime.ReadMemStats(&ms1)
		phaseAllocBytes[1] += ms1.TotalAlloc - ms0.TotalAlloc
		phaseMallocCount[1] += ms1.Mallocs - ms0.Mallocs

		// Phase 2: league tick (serial).
		runtime.ReadMemStats(&ms0)
		TickLeagueYear(league, cityMap, leagueStream, year)
		runtime.ReadMemStats(&ms1)
		phaseAllocBytes[2] += ms1.TotalAlloc - ms0.TotalAlloc
		phaseMallocCount[2] += ms1.Mallocs - ms0.Mallocs

		// Phase 3: inter-polity events (serial).
		runtime.ReadMemStats(&ms0)
		ApplyInterPolityEventsYear(InterPolityContext{
			Origin: k1, Neighbors: []*polity.Kingdom{k2},
			Cities: cityMap, Stream: k1Stream, Year: year,
		})
		runtime.ReadMemStats(&ms1)
		phaseAllocBytes[3] += ms1.TotalAlloc - ms0.TotalAlloc
		phaseMallocCount[3] += ms1.Mallocs - ms0.Mallocs

		// Phase 4: mulk cycle (serial, both kingdoms).
		runtime.ReadMemStats(&ms0)
		ApplyMulkCycleYear(k1, cityMap, mulkStream)
		ApplyMulkCycleYear(k2, cityMap, mulkStream)
		runtime.ReadMemStats(&ms1)
		phaseAllocBytes[4] += ms1.TotalAlloc - ms0.TotalAlloc
		phaseMallocCount[4] += ms1.Mallocs - ms0.Mallocs

		// Phase 5: no villages in this bench; record zeros for completeness.
		phaseAllocBytes[5] = 0
		phaseMallocCount[5] = 0
	}

	b.StopTimer()
	b.Logf("--- per-phase heap totals across %d iterations ---", b.N)
	for i, name := range phaseNames {
		b.Logf("  %-35s  allocBytes=%d  mallocs=%d",
			name, phaseAllocBytes[i], phaseMallocCount[i])
	}
}

// Part C — memprofile-ready bench. Named to produce a self-explanatory
// pprof profile: run with -memprofile=/tmp/world_mem.prof and then
// `go tool pprof -top -alloc_objects` to rank allocation sites.
func BenchmarkWorldMemProfile_1000Cities_10Years(b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		pool := make([]*polity.City, 1000)
		streams := make([]*dice.Stream, 1000)
		for i := range pool {
			pool[i] = diagCity()
			streams[i] = dice.New(diagBenchSeed^int64(i+1), dice.SaltKingdomYear)
		}
		for year := 1300; year < 1310; year++ {
			TickCitiesYear(pool, streams, year)
		}
	}
}

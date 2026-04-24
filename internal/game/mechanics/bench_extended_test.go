package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// benchHappinessCity mints a city set up to exercise every branch of
// ApplyHappinessYear — ruler charisma, food surplus, historical mods.
func benchHappinessCity() *polity.City {
	c := &polity.City{
		FoodBalance: 8,
		TaxRate:     polity.TaxNormal,
	}
	c.Ruler.Stats.Charisma = 14
	c.HistoricalMods = []polity.HistoricalMod{
		{Kind: polity.HistoricalModHappiness, Magnitude: 3, YearApplied: 1498, DecayYears: 5},
	}
	return c
}

// BenchmarkApplyHappinessYear_WithCharisma measures the cost of the
// yearly happiness recompute with a full set of contributing factors.
func BenchmarkApplyHappinessYear_WithCharisma(b *testing.B) {
	c := benchHappinessCity()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyHappinessYear(c, 1500)
	}
}

// BenchmarkApplyEconomicYear_WithTrade measures the cost of the
// economy tick with tax + trade + historical mods active.
func BenchmarkApplyEconomicYear_WithTrade(b *testing.B) {
	c := &polity.City{
		Settlement: polity.Settlement{Population: 5000},
		TaxRate:    polity.TaxNormal,
		Army:       50,
		TradeScore: 40,
		HistoricalMods: []polity.HistoricalMod{
			{Kind: polity.HistoricalModWealth, Magnitude: 100, YearApplied: 1498, DecayYears: 5},
		},
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyEconomicYear(c, 1500)
	}
}

// BenchmarkApplyVillageYear measures one village's tick cost. The
// D20 draw dominates — this is the baseline Stream-sensitive loop.
func BenchmarkApplyVillageYear(b *testing.B) {
	v := polity.NewVillage("bench", geom.Position{}, 1200, "parent")
	v.Population = 100
	stream := dice.New(42, dice.SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyVillageYear(v, stream)
	}
}

// BenchmarkResolveVillageToCity_10villages measures the O(N) scan
// cost of pushing village food contributions to their parents.
func BenchmarkResolveVillageToCity_10villages(b *testing.B) {
	city := polity.NewCity("Hub", geom.Position{}, 1200, polity.Ruler{})
	villages := make([]*polity.Village, 10)
	for i := range villages {
		v := polity.NewVillage("v", geom.Position{}, 1200, "Hub")
		v.Population = 100
		villages[i] = v
	}
	cities := map[string]*polity.City{"Hub": city}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		city.FoodBalance = 0
		ResolveVillageToCity(villages, cities)
	}
}

// benchModQueue builds a 50-entry HistoricalMods queue covering every
// kind, with varied decay windows. Reused by the sum / recrystallize
// benchmarks to measure realistic long-running queue costs.
func benchModQueue() []polity.HistoricalMod {
	mods := make([]polity.HistoricalMod, 50)
	for i := range mods {
		kind := polity.HistoricalModKind(i % 4)
		mods[i] = polity.HistoricalMod{
			Kind:        kind,
			Magnitude:   i - 25,
			YearApplied: 1490 + i%10,
			DecayYears:  5 + i%8,
		}
	}
	return mods
}

// BenchmarkApplyRecrystallizeYear_50mods — walks a realistic
// 50-entry queue, prunes expired mods. Measures the in-place
// compaction cost.
func BenchmarkApplyRecrystallizeYear_50mods(b *testing.B) {
	base := benchModQueue()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := polity.City{HistoricalMods: append([]polity.HistoricalMod(nil), base...)}
		ApplyRecrystallizeYear(&c, 1500)
	}
}

// BenchmarkHistoricalModSum_50mods — sum over a 50-entry queue,
// filtering by kind. Hot path because every city's ApplyHappinessYear
// and ApplyEconomicYear call this every tick.
func BenchmarkHistoricalModSum_50mods(b *testing.B) {
	c := polity.City{HistoricalMods: benchModQueue()}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = HistoricalModSum(&c, polity.HistoricalModHappiness, 1500)
	}
}

// BenchmarkApplyDecreeYear — measures the per-year decree attempt
// including the two D20 rolls.
func BenchmarkApplyDecreeYear(b *testing.B) {
	c := &polity.City{TaxRate: polity.TaxNormal}
	c.Ruler.Stats.Charisma = 14
	stream := dice.New(42, dice.SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ApplyDecreeYear(c, stream, 1500+i)
	}
}

// benchKingdomWorld constructs a 4-city kingdom (capital + 3 vassals).
// Used by kingdom-level benchmarks to measure per-year tick cost at a
// realistic scale.
func benchKingdomWorld() (*polity.Kingdom, map[string]*polity.City) {
	founder := polity.Ruler{Stats: stats.CoreStats{Constitution: 14},
		BirthYear: 1250}
	capital := polity.NewCity("Cap", geom.Position{}, 1250, founder)
	capital.Population = 10000
	capital.Wealth = 10000
	cities := map[string]*polity.City{"Cap": capital}
	k := polity.NewKingdom("K1", "Bench", founder, "Cap",
		polity.SuccessionPrimogeniture, 1250)
	for i := 0; i < 3; i++ {
		id := string(rune('V' + i))
		v := polity.NewCity(id, geom.Position{}, 1250, polity.Ruler{})
		v.Population = 3000
		v.Wealth = 2000
		cities[id] = v
		k.CityIDs = append(k.CityIDs, id)
	}
	return k, cities
}

// BenchmarkTickKingdomYear — a full kingdom tick (asabiya + tribute
// cadence + succession check + collapse check).
func BenchmarkTickKingdomYear(b *testing.B) {
	k, cities := benchKingdomWorld()
	stream := dice.New(42, dice.SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TickKingdomYear(k, cities, stream, 1250+i)
	}
}

// BenchmarkApplyAsabiyaYear — the pure asabiya evolution function in
// isolation. Catches regressions in the inner float math.
func BenchmarkApplyAsabiyaYear(b *testing.B) {
	k, _ := benchKingdomWorld()
	k.Asabiya = 0.5
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		applyAsabiyaYear(k)
		// Rehydrate so asabiya never collapses inside the benchmark;
		// we measure the compute, not the terminal state.
		if k.Asabiya < 0.2 {
			k.Asabiya = 0.5
		}
	}
}

// BenchmarkCollectTribute_10vassals — 10-vassal tribute collection is
// the realistic upper bound; measures the hot O(N) loop.
func BenchmarkCollectTribute_10vassals(b *testing.B) {
	founder := polity.Ruler{Stats: stats.CoreStats{Constitution: 14},
		BirthYear: 1250}
	capital := polity.NewCity("Cap", geom.Position{}, 1250, founder)
	capital.Wealth = 5000
	cities := map[string]*polity.City{"Cap": capital}
	k := polity.NewKingdom("K1", "Grand", founder, "Cap",
		polity.SuccessionPrimogeniture, 1250)
	for i := 0; i < 10; i++ {
		id := "v" + string(rune('0'+i))
		v := polity.NewCity(id, geom.Position{}, 1250, polity.Ruler{})
		v.Wealth = 2000
		cities[id] = v
		k.CityIDs = append(k.CityIDs, id)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		collectTribute(k, cities)
		// Refill so the benchmark doesn't drain to zero and flip its path.
		for _, id := range k.CityIDs[1:] {
			cities[id].Wealth = 2000
		}
	}
}

package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// benchCity mints the standard bench baseline city — mid-sized,
// Normal tax, a handful of deposits. Avoids random drift from
// different starting conditions across benchmark calls.
func benchCity(seed int64) *polity.City {
	ruler := polity.NewRuler(dice.New(seed, dice.SaltKingdomYear), 1270, "")
	c := polity.NewCity("Bench", geom.Position{}, 1200, ruler)
	c.Population = 5000
	c.Wealth = 5000
	c.Army = 100
	c.Happiness = 70
	c.TaxRate = polity.TaxNormal
	c.Deposits = []polity.Deposit{
		{Kind: polity.DepositGold, RemainingYield: 0.9},
		{Kind: polity.DepositIron, RemainingYield: 0.9},
	}
	return c
}

// benchRun runs a single city for the given horizon and resets the
// city pointer each iteration to keep memory footprint constant.
func benchRun(b *testing.B, years int) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c := benchCity(42)
		stream := dice.New(42, dice.SaltKingdomYear)
		for year := 1300; year < 1300+years; year++ {
			TickCityYear(c, stream, year)
		}
	}
}

// BenchmarkTick_1City_1Year measures pure per-tick cost — the inner
// loop that multiplies by city count and year count.
func BenchmarkTick_1City_1Year(b *testing.B) {
	c := benchCity(42)
	stream := dice.New(42, dice.SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TickCityYear(c, stream, 1300+i%5000)
	}
}

func BenchmarkTick_1City_100Years(b *testing.B)  { benchRun(b, 100) }
func BenchmarkTick_1City_200Years(b *testing.B)  { benchRun(b, 200) }
func BenchmarkTick_1City_500Years(b *testing.B)  { benchRun(b, 500) }
func BenchmarkTick_1City_2000Years(b *testing.B) { benchRun(b, 2000) }

// benchWorld runs a full N-city world for Y years. Uses per-city
// streams so behavior matches the real WorldManager model. Serial
// baseline — the parallel counterpart is benchWorldParallel below.
func benchWorld(b *testing.B, cities, years int) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		pool := make([]*polity.City, cities)
		streams := make([]*dice.Stream, cities)
		for i := 0; i < cities; i++ {
			pool[i] = benchCity(int64(i + 1))
			streams[i] = dice.New(int64(i+1), dice.SaltKingdomYear)
		}
		for year := 1300; year < 1300+years; year++ {
			for i := range pool {
				TickCityYear(pool[i], streams[i], year)
			}
		}
	}
}

// benchWorldParallel mirrors benchWorld but fans city ticks through
// TickCitiesYear. Run side-by-side with benchWorld to measure the
// fan-out speedup; the bodies are otherwise identical.
func benchWorldParallel(b *testing.B, cities, years int) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		pool := make([]*polity.City, cities)
		streams := make([]*dice.Stream, cities)
		for i := 0; i < cities; i++ {
			pool[i] = benchCity(int64(i + 1))
			streams[i] = dice.New(int64(i+1), dice.SaltKingdomYear)
		}
		for year := 1300; year < 1300+years; year++ {
			TickCitiesYear(pool, streams, year)
		}
	}
}

// BenchmarkWorld_10Cities_200Years — the reference engine load: 10
// cities × 200 years = 2 000 tick calls per iteration.
func BenchmarkWorld_10Cities_200Years(b *testing.B) { benchWorld(b, 10, 200) }

// BenchmarkWorld_100Cities_100Years simulates a realistic medieval
// world scale for a single century — 10 000 tick calls.
func BenchmarkWorld_100Cities_100Years(b *testing.B) { benchWorld(b, 100, 100) }

// BenchmarkWorld_500Cities_50Years — stress scale. 25 000 tick
// calls. Useful to catch O(N²) regressions in per-city code.
func BenchmarkWorld_500Cities_50Years(b *testing.B) { benchWorld(b, 500, 50) }

// BenchmarkWorld_1000Cities_10Years — extreme. 10 000 tick calls
// at a 1 000-city scale. Checks whether we hit constant-factor
// trouble or memory pressure at kingdom-scale.
func BenchmarkWorld_1000Cities_10Years(b *testing.B) { benchWorld(b, 1000, 10) }

// BenchmarkTickCitiesYear_* — parallel fan-out counterparts to the
// serial BenchmarkWorld_* runs. Compare ns/op pairwise to get the
// speedup ratio: serial ÷ parallel = workers-used efficiency.
func BenchmarkTickCitiesYear_10Cities_200Years(b *testing.B)  { benchWorldParallel(b, 10, 200) }
func BenchmarkTickCitiesYear_100Cities_100Years(b *testing.B) { benchWorldParallel(b, 100, 100) }
func BenchmarkTickCitiesYear_500Cities_50Years(b *testing.B)  { benchWorldParallel(b, 500, 50) }
func BenchmarkTickCitiesYear_1000Cities_10Years(b *testing.B) { benchWorldParallel(b, 1000, 10) }

// benchTickOnlySerial isolates the year-loop cost from world setup.
// Builds the world once, then measures one year's worth of ticks per
// iteration. Use alongside benchTickOnlyParallel to get the pure
// fan-out speedup without setup noise.
func benchTickOnlySerial(b *testing.B, cities int) {
	pool := make([]*polity.City, cities)
	streams := make([]*dice.Stream, cities)
	for i := 0; i < cities; i++ {
		pool[i] = benchCity(int64(i + 1))
		streams[i] = dice.New(int64(i+1), dice.SaltKingdomYear)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		for i := range pool {
			TickCityYear(pool[i], streams[i], 1300+n)
		}
	}
}

func benchTickOnlyParallel(b *testing.B, cities int) {
	pool := make([]*polity.City, cities)
	streams := make([]*dice.Stream, cities)
	for i := 0; i < cities; i++ {
		pool[i] = benchCity(int64(i + 1))
		streams[i] = dice.New(int64(i+1), dice.SaltKingdomYear)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		TickCitiesYear(pool, streams, 1300+n)
	}
}

// BenchmarkTickOnly_* — setup-excluded comparisons for the pure
// city-tick fan-out. One iteration ticks one year across N cities.
func BenchmarkTickOnly_Serial_100Cities(b *testing.B)    { benchTickOnlySerial(b, 100) }
func BenchmarkTickOnly_Parallel_100Cities(b *testing.B)  { benchTickOnlyParallel(b, 100) }
func BenchmarkTickOnly_Serial_500Cities(b *testing.B)    { benchTickOnlySerial(b, 500) }
func BenchmarkTickOnly_Parallel_500Cities(b *testing.B)  { benchTickOnlyParallel(b, 500) }
func BenchmarkTickOnly_Serial_1000Cities(b *testing.B)   { benchTickOnlySerial(b, 1000) }
func BenchmarkTickOnly_Parallel_1000Cities(b *testing.B) { benchTickOnlyParallel(b, 1000) }

// BenchmarkAllocs_SingleTick isolates per-tick allocations so future
// optimization can target the heap-hottest step.
func BenchmarkAllocs_SingleTick(b *testing.B) {
	c := benchCity(42)
	stream := dice.New(42, dice.SaltKingdomYear)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		TickCityYear(c, stream, 1300+i)
	}
}

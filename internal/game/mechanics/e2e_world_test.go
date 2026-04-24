package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestE2E_FullWorld_500yr orchestrates every implemented system —
// cities, villages, kingdoms, leagues, inter-polity events, Mulk
// cycle, decrees, events, disasters, historical mods — in a single
// 500-year sim. Passes when all invariants hold at year end.
func TestE2E_FullWorld_500yr(t *testing.T) {
	if testing.Short() {
		t.Skip("short — 500-yr full-world integration sim")
	}
	const seed int64 = 42
	const startYear = 1300
	const years = 500

	// Build two rival kingdoms, 4 cities each, 2 villages per city.
	buildKingdom := func(id string, cityCount int, founderSeed int64, culture polity.Culture) (*polity.Kingdom, []*polity.City) {
		founder := polity.NewRuler(dice.New(founderSeed, dice.SaltKingdomYear), startYear-30)
		var cs []*polity.City
		for i := 0; i < cityCount; i++ {
			c := polity.NewCity(
				id+"-C"+string(rune('A'+i)), geom.Position{X: i, Y: 0},
				startYear-80, founder,
			)
			c.Population = 5000 + i*2000
			c.Wealth = 3000
			c.TaxRate = polity.TaxNormal
			c.Happiness = 60
			c.Culture = culture
			cs = append(cs, c)
		}
		k := polity.NewKingdom(id, id+" Realm", founder, cs[0].Name,
			polity.SuccessionPrimogeniture, startYear-80)
		k.Culture = culture
		for _, c := range cs[1:] {
			k.CityIDs = append(k.CityIDs, c.Name)
		}
		return k, cs
	}

	k1, cities1 := buildKingdom("K1", 4, seed, polity.CultureFeudal)
	k2, cities2 := buildKingdom("K2", 4, seed+1, polity.CultureSteppe)

	allCities := append([]*polity.City{}, cities1...)
	allCities = append(allCities, cities2...)

	cityMap := make(map[string]*polity.City)
	for _, c := range allCities {
		cityMap[c.Name] = c
	}

	var villages []*polity.Village
	for _, c := range allCities {
		for i := 0; i < 2; i++ {
			v := polity.NewVillage(
				c.Name+"-v"+string(rune('A'+i)),
				geom.Position{}, startYear-50, c.Name,
			)
			v.Population = 100 + i*20
			villages = append(villages, v)
		}
	}

	// Founding league with 2 cities from k1.
	league := polity.NewLeague("L1", "Merchant League",
		cities1[1].Name, cities1[2].Name, startYear-20)

	// Streams.
	cityStreams := make([]*dice.Stream, len(allCities))
	for i := range allCities {
		cityStreams[i] = dice.New(seed^int64(i+1)*0xabc, dice.SaltKingdomYear)
	}
	villageStreams := make([]*dice.Stream, len(villages))
	for i := range villages {
		villageStreams[i] = dice.New(seed^int64(i+1)*0xdef, dice.SaltKingdomYear)
	}
	k1Stream := dice.New(seed^0x1111, dice.SaltKingdomYear)
	k2Stream := dice.New(seed^0x2222, dice.SaltKingdomYear)
	interStream := dice.New(seed^0x3333, dice.SaltKingdomYear)
	leagueStream := dice.New(seed^0x4444, dice.SaltKingdomYear)
	mulkStream := dice.New(seed^0x5555, dice.SaltKingdomYear)

	for year := startYear; year < startYear+years; year++ {
		// Villages feed cities.
		for i, v := range villages {
			ApplyVillageYear(v, villageStreams[i])
		}
		ResolveVillageToCity(villages, cityMap)

		// Cities tick.
		TickCitiesYear(allCities, cityStreams, year)

		// Kingdoms tick.
		TickKingdomYear(k1, cityMap, k1Stream, year)
		TickKingdomYear(k2, cityMap, k2Stream, year)

		// League ticks.
		TickLeagueYear(league, cityMap, leagueStream, year)

		// Inter-polity every 5 years to avoid cascading war.
		if year%5 == 0 {
			ApplyInterPolityEventsYear(InterPolityContext{
				Origin: k1, Neighbors: []*polity.Kingdom{k2},
				Cities: cityMap, Stream: interStream, Year: year,
			})
			ApplyInterPolityEventsYear(InterPolityContext{
				Origin: k2, Neighbors: []*polity.Kingdom{k1},
				Cities: cityMap, Stream: interStream, Year: year,
			})
		}

		// Mulk cycle each year.
		ApplyMulkCycleYear(k1, cityMap, mulkStream)
		ApplyMulkCycleYear(k2, cityMap, mulkStream)
	}

	// Summary log.
	t.Logf("--- 500yr world E2E ---")
	t.Logf("K1: alive=%v rulers=%d asabiya=%.3f cities=%d events=%d culture=%v",
		k1.Alive(), len(k1.Rulers), k1.Asabiya, len(k1.CityIDs),
		len(k1.InterPolityHistory), k1.Culture)
	t.Logf("K2: alive=%v rulers=%d asabiya=%.3f cities=%d events=%d culture=%v",
		k2.Alive(), len(k2.Rulers), k2.Asabiya, len(k2.CityIDs),
		len(k2.InterPolityHistory), k2.Culture)
	t.Logf("League: alive=%v members=%d", league.Alive(), len(league.MemberCityIDs))
	for _, c := range allCities {
		t.Logf("  %s: pop=%d wealth=%d happy=%d techs=%d mods=%d culture=%v",
			c.Name, c.Population, c.Wealth, c.Happiness,
			countBits(uint16(c.Techs)), len(c.HistoricalMods), c.Culture)
	}

	// Invariants — none should blow up even under full chaos.
	for _, c := range allCities {
		if c.Population < 80 || c.Population > 40000 {
			t.Errorf("%s pop out of range: %d", c.Name, c.Population)
		}
		if c.Prosperity < 0 || c.Prosperity > 1 {
			t.Errorf("%s prosperity out of range: %v", c.Name, c.Prosperity)
		}
		sum := 0.0
		for _, f := range polity.AllFaiths() {
			sum += c.Faiths[f]
		}
		if sum > 0 && (sum < 0.999 || sum > 1.001) {
			t.Errorf("%s faith sum broken: %v", c.Name, sum)
		}
	}
	for _, v := range villages {
		if v.Population < 20 || v.Population > 400 {
			t.Errorf("%s village pop out of range: %d", v.Name, v.Population)
		}
	}

	// Some event activity must have happened — complete inertia = bug.
	if len(k1.InterPolityHistory) == 0 && len(k2.InterPolityHistory) == 0 {
		t.Error("no inter-polity events fired across 500 yr — subsystem inert")
	}
}

// countBits returns popcount over a TechMask uint16.
func countBits(m uint16) int {
	count := 0
	for m != 0 {
		count += int(m & 1)
		m >>= 1
	}
	return count
}

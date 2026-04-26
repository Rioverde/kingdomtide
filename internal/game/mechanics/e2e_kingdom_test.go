package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestE2E_KingdomWithDemesnes_500yr exercises the full stack —
// cities + demesnes + kingdom + all new mechanics in one 500-year
// simulation. Verifies the tick pipeline integrates correctly under
// every subsystem firing at real rates.
func TestE2E_KingdomWithDemesnes_500yr(t *testing.T) {
	if testing.Short() {
		t.Skip("short — 500-yr kingdom+villages integration sim")
	}
	const seed int64 = 42
	const startYear = 1300
	const years = 500

	// Build capital + 3 vassals + 5 villages per city.
	capital := polity.NewCity("Capital", geom.Position{}, startYear-100, polity.Ruler{})
	capital.Population = 10000
	capital.Wealth = 10000
	capital.TaxRate = polity.TaxNormal
	capital.Happiness = 65
	capital.EffectiveRank = polity.RankCapital
	capital.Ruler.Stats.Charisma = 16 // decrees succeed more often

	vassals := []*polity.City{}
	for i := 0; i < 3; i++ {
		v := polity.NewCity(
			"Vassal"+string(rune('A'+i)),
			geom.Position{X: i + 1},
			startYear-50,
			polity.Ruler{},
		)
		v.Population = 3000 + i*1000
		v.Wealth = 2000
		v.TaxRate = polity.TaxNormal
		v.EffectiveRank = polity.RankVassal
		vassals = append(vassals, v)
	}

	cities := map[string]*polity.City{capital.Name: capital}
	var allCities []*polity.City
	allCities = append(allCities, capital)
	for _, v := range vassals {
		cities[v.Name] = v
		allCities = append(allCities, v)
	}

	// Build demesnes — 5 per city, all feeding their parent's food.
	var demesnes []*polity.Demesne
	for _, city := range allCities {
		for j := 0; j < 5; j++ {
			d := polity.NewDemesne(
				city.Name+"-V"+string(rune('A'+j)),
				geom.Position{},
				startYear-30,
				city.Name,
			)
			d.Population = 100
			demesnes = append(demesnes, d)
		}
	}

	// Kingdom over capital + vassals.
	k := polity.NewKingdom("K1", "Realm", capital.Ruler, capital.Name,
		polity.SuccessionPrimogeniture, startYear-100)
	for _, v := range vassals {
		k.CityIDs = append(k.CityIDs, v.Name)
	}

	// Run 500 years.
	cityStreams := make([]*dice.Stream, len(allCities))
	demesneStreams := make([]*dice.Stream, len(demesnes))
	for i := range allCities {
		cityStreams[i] = dice.New(seed^int64(i+1)*0xcafe, dice.SaltKingdomYear)
	}
	for i := range demesnes {
		demesneStreams[i] = dice.New(seed^int64(i+1)*0xbeef, dice.SaltKingdomYear)
	}
	kingdomStream := dice.New(seed, dice.SaltKingdomYear)

	var decreeCount, modPeakCount, demesneResolvePhase int

	for year := startYear; year < startYear+years; year++ {
		// Tick demesnes first (they feed food upstream).
		for i, d := range demesnes {
			ApplyDemesneYear(d, demesneStreams[i])
		}
		ResolveDemesneToCity(demesnes, cities)
		demesneResolvePhase++

		// Tick cities.
		for i, c := range allCities {
			modsBefore := len(c.HistoricalMods)
			TickCityYear(c, cityStreams[i], year)
			if len(c.HistoricalMods) > modsBefore {
				decreeCount++
			}
			if len(c.HistoricalMods) > modPeakCount {
				modPeakCount = len(c.HistoricalMods)
			}
		}

		// Tick kingdom.
		TickKingdomYear(k, cities, kingdomStream, year)
	}

	// Log summary.
	t.Logf("--- 500yr E2E integration ---")
	t.Logf("Demesnes resolved: %d", demesneResolvePhase)
	t.Logf("Historical-mod peak queue: %d", modPeakCount)
	t.Logf("Kingdom alive: %v, rulers through: %d, asabiya %.3f",
		k.Alive(), len(k.Rulers), k.Asabiya)
	for _, c := range allCities {
		t.Logf("  %s: pop=%d wealth=%d happy=%d prosp=%.3f trade=%d mods=%d",
			c.Name, c.Population, c.Wealth, c.Happiness,
			c.Prosperity, c.TradeScore, len(c.HistoricalMods))
	}

	// Invariants.
	for _, c := range allCities {
		if c.Population < 80 || c.Population > 40000 {
			t.Errorf("%s pop out of range: %d", c.Name, c.Population)
		}
		if c.Prosperity < 0 || c.Prosperity > 1 {
			t.Errorf("%s prosperity out of range: %v", c.Name, c.Prosperity)
		}
	}
	for _, d := range demesnes {
		if d.Population < 20 || d.Population > 400 {
			t.Errorf("%s demesne pop out of range: %d", d.Name, d.Population)
		}
	}

	// At least one of these must have fired in 500 yr:
	//   - Succession in the kingdom (k.Rulers grew)
	//   - Asabiya moved from 0.5 baseline
	if len(k.Rulers) == 1 && k.Asabiya == 0.5 {
		t.Error("kingdom static over 500 yr — no asabiya change, no succession")
	}

	// Historical mods must have accumulated and been pruned.
	if modPeakCount == 0 {
		t.Error("no historical mods ever queued — mod pipeline broken")
	}
	if modPeakCount > 200 {
		t.Errorf("mod queue grew unbounded (peak %d) — recrystallize not pruning", modPeakCount)
	}
	_ = decreeCount
}

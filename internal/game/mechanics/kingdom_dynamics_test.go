package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestKingdom_SpanLimitDrain verifies a kingdom whose CityIDs vastly
// exceed the administrative spanCap loses Asabiya faster than a
// compact kingdom.
func TestKingdom_SpanLimitDrain(t *testing.T) {
	mkKingdom := func(cityCount int) (*polity.Kingdom, map[string]*polity.City) {
		founder := polity.Ruler{Stats: stats.CoreStats{
			Strength: 10, Dexterity: 10, Constitution: 10,
			Intelligence: 10, Wisdom: 10, Charisma: 10,
		}, BirthYear: 1250}
		cities := make(map[string]*polity.City, cityCount)
		ids := make([]string, 0, cityCount)
		for i := 0; i < cityCount; i++ {
			id := polity.NewCity("C", geom.Position{}, 1250, founder).Name
			// Use the index as the map key to keep IDs unique.
			key := id + string(rune('A'+i))
			city := polity.NewCity(key, geom.Position{}, 1250, founder)
			city.Population = 1000
			city.Wealth = 500
			cities[key] = city
			ids = append(ids, key)
		}
		k := polity.NewKingdom("K", "Realm", founder, ids[0],
			polity.SuccessionPrimogeniture, 1250)
		if len(ids) > 1 {
			k.CityIDs = append(k.CityIDs, ids[1:]...)
		}
		k.Asabiya = 0.6
		return k, cities
	}
	small, smallCities := mkKingdom(2)
	huge, hugeCities := mkKingdom(15)
	stream := dice.New(42, dice.SaltKingdomYear)

	// Non-cadence years so only applyAsabiyaYear runs.
	for yr := 1251; yr < 1261; yr++ {
		TickKingdomYear(small, smallCities, stream, yr)
		TickKingdomYear(huge, hugeCities, stream, yr)
	}

	if huge.Asabiya >= small.Asabiya {
		t.Errorf("15-city kingdom should decay faster than 2-city: "+
			"small=%v huge=%v", small.Asabiya, huge.Asabiya)
	}
}

// TestKingdom_RumpState_Happens verifies an old, literate kingdom
// with a competent ruler trims to a rump state instead of dissolving.
func TestKingdom_RumpState_Happens(t *testing.T) {
	founder := polity.Ruler{Stats: stats.CoreStats{
		Strength: 10, Dexterity: 10, Constitution: 10,
		Intelligence: 10, Wisdom: 13, Charisma: 13, // WIS+CHA = 26
	}, BirthYear: 1100}
	cities := make(map[string]*polity.City)
	ids := make([]string, 6)
	for i := 0; i < 6; i++ {
		id := "City" + string(rune('A'+i))
		c := polity.NewCity(id, geom.Position{}, 1200, founder)
		c.Population = 2000
		c.EffectiveRank = polity.RankVassal
		ids[i] = id
		cities[id] = c
	}
	// Capital needs innovation above rumpStateInnovationMin.
	cities[ids[0]].Innovation = 70
	cities[ids[0]].EffectiveRank = polity.RankCapital

	k := polity.NewKingdom("K", "Realm", founder, ids[0],
		polity.SuccessionPrimogeniture, 1200)
	k.CityIDs = append(k.CityIDs, ids[1:]...)
	k.Asabiya = 0.05 // forces collapse branch
	stream := dice.New(42, dice.SaltKingdomYear)

	// Age 100 exactly (1300 - 1200 = 100) → rump-eligible.
	TickKingdomYear(k, cities, stream, 1300)

	if !k.Alive() {
		t.Errorf("rump-eligible kingdom should not fully dissolve")
	}
	if len(k.CityIDs) > rumpStateKeep {
		t.Errorf("rump kingdom should keep ≤ %d cities, got %d",
			rumpStateKeep, len(k.CityIDs))
	}
	if k.Asabiya != rumpStateAsabiyaReset {
		t.Errorf("rump asabiya reset: got %v want %v",
			k.Asabiya, rumpStateAsabiyaReset)
	}
}

// TestKingdom_RumpState_FailsOnYoung verifies a young kingdom with
// the same collapse conditions does NOT rump — it dissolves fully.
func TestKingdom_RumpState_FailsOnYoung(t *testing.T) {
	founder := polity.Ruler{Stats: stats.CoreStats{
		Strength: 10, Dexterity: 10, Constitution: 10,
		Intelligence: 10, Wisdom: 13, Charisma: 13,
	}, BirthYear: 1250}
	cities := make(map[string]*polity.City)
	ids := make([]string, 4)
	for i := 0; i < 4; i++ {
		id := "Young" + string(rune('A'+i))
		c := polity.NewCity(id, geom.Position{}, 1250, founder)
		c.Population = 2000
		c.EffectiveRank = polity.RankVassal
		ids[i] = id
		cities[id] = c
	}
	cities[ids[0]].Innovation = 70
	cities[ids[0]].EffectiveRank = polity.RankCapital

	k := polity.NewKingdom("K", "Young", founder, ids[0],
		polity.SuccessionPrimogeniture, 1250)
	k.CityIDs = append(k.CityIDs, ids[1:]...)
	k.Asabiya = 0.05
	stream := dice.New(42, dice.SaltKingdomYear)

	// Age 30 (1280 - 1250 = 30) — below rumpStateAgeMin.
	TickKingdomYear(k, cities, stream, 1280)

	if k.Alive() {
		t.Errorf("young kingdom should dissolve, not rump")
	}
	for _, id := range k.CityIDs {
		if cities[id].EffectiveRank != polity.RankIndependent {
			t.Errorf("dissolved kingdom city %s should be Independent, got %v",
				id, cities[id].EffectiveRank)
		}
	}
}

// TestKingdom_Corruption_ChainScaling verifies a 10-city tribute chain
// loses more of each collected transfer to corruption than a 2-city
// chain.
func TestKingdom_Corruption_ChainScaling(t *testing.T) {
	shortChain := retentionRate(2)
	longChain := retentionRate(10)

	if shortChain <= longChain {
		t.Errorf("long chain should retain less: 2=%v 10=%v",
			shortChain, longChain)
	}
	if shortChain <= 0 || shortChain > 1 {
		t.Errorf("short chain retention out of bounds: %v", shortChain)
	}
	if longChain < corruptionMinRetention {
		t.Errorf("long chain retention under floor: %v < %v",
			longChain, corruptionMinRetention)
	}
}

// TestKingdom_Corruption_CollectTributeChain verifies the integrated
// collectTribute path delivers less to the capital for a longer chain
// than for a shorter one (holding vassal wealth constant).
func TestKingdom_Corruption_CollectTributeChain(t *testing.T) {
	mk := func(vassals int) (*polity.Kingdom, map[string]*polity.City) {
		founder := polity.Ruler{}
		cities := make(map[string]*polity.City)
		ids := []string{"Cap"}
		cap := polity.NewCity("Cap", geom.Position{}, 1250, founder)
		cities["Cap"] = cap
		for i := 0; i < vassals; i++ {
			id := "V" + string(rune('A'+i))
			v := polity.NewCity(id, geom.Position{}, 1250, founder)
			v.Wealth = 1000
			cities[id] = v
			ids = append(ids, id)
		}
		k := polity.NewKingdom("K", "K", founder, "Cap",
			polity.SuccessionPrimogeniture, 1250)
		k.CityIDs = ids
		return k, cities
	}

	// Same vassal count but different chainSize reports via the ratio
	// of transferred wealth — but collectTribute's retention rate is
	// derived from the chain length, so we compare:
	//   Kingdom_A: 1 vassal  → chain size 2  → retention 1.0
	//   Kingdom_B: 9 vassals → chain size 10 → retention < 1.0
	kShort, csShort := mk(1)
	kLong, csLong := mk(9)

	beforeShort := csShort["Cap"].Wealth
	beforeLong := csLong["Cap"].Wealth
	collectTribute(kShort, csShort)
	collectTribute(kLong, csLong)
	gainedShort := csShort["Cap"].Wealth - beforeShort
	gainedLong := csLong["Cap"].Wealth - beforeLong

	perVassalShort := float64(gainedShort) / 1.0
	perVassalLong := float64(gainedLong) / 9.0

	if perVassalLong >= perVassalShort {
		t.Errorf("per-vassal capital retention should drop with chain size: "+
			"short=%.2f long=%.2f", perVassalShort, perVassalLong)
	}
}

// TestKingdom_BankingReducesTribute verifies a Banking-unlocked vassal
// hands over less tribute than a vassal without Banking.
func TestKingdom_BankingReducesTribute(t *testing.T) {
	founder := polity.Ruler{}
	plain := polity.NewCity("Plain", geom.Position{}, 1250, founder)
	plain.Wealth = 1000
	banked := polity.NewCity("Banked", geom.Position{}, 1250, founder)
	banked.Wealth = 1000
	banked.Techs.Set(polity.TechBanking)
	cap := polity.NewCity("Cap", geom.Position{}, 1250, founder)
	cities := map[string]*polity.City{
		"Cap":    cap,
		"Plain":  plain,
		"Banked": banked,
	}
	k := polity.NewKingdom("K", "K", founder, "Cap",
		polity.SuccessionPrimogeniture, 1250)
	k.CityIDs = []string{"Cap", "Plain", "Banked"}

	collectTribute(k, cities)

	plainLoss := 1000 - plain.Wealth
	bankedLoss := 1000 - banked.Wealth
	if bankedLoss >= plainLoss {
		t.Errorf("Banking vassal should pay less tribute: "+
			"plain=%d banked=%d", plainLoss, bankedLoss)
	}
}

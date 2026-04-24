package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// interPolityFixture builds two kingdoms — an origin with a
// charisma-heavy ruler that reliably clears the event DCs, and a
// neighbor that owns a capital plus one vassal. Returns the origin,
// the neighbor, the shared city registry, and a seeded stream.
func interPolityFixture(seed int64) (*polity.Kingdom, *polity.Kingdom, map[string]*polity.City, *dice.Stream) {
	rulerA := polity.Ruler{
		Stats:     stats.CoreStats{Charisma: 18},
		BirthYear: 1000,
		Faith:     polity.FaithSunCovenant,
	}
	capitalA := polity.NewCity("CapA", geom.Position{}, 1000, rulerA)
	capitalA.Wealth = 1000
	capitalA.Army = 500
	capitalA.TradeScore = 30
	capitalA.EffectiveRank = polity.RankCapital

	kingdomA := polity.NewKingdom("KA", "KingdomA", rulerA, "CapA",
		polity.SuccessionPrimogeniture, 1000)

	rulerB := polity.Ruler{
		Stats:     stats.CoreStats{Charisma: 10},
		BirthYear: 1000,
		Faith:     polity.FaithOldGods,
	}
	capitalB := polity.NewCity("CapB", geom.Position{}, 1000, rulerB)
	capitalB.Wealth = 1000
	capitalB.Army = 500
	capitalB.TradeScore = 30
	capitalB.EffectiveRank = polity.RankCapital

	vassalB := polity.NewCity("VassalB", geom.Position{}, 1010, polity.Ruler{})
	vassalB.Wealth = 500
	vassalB.Army = 100
	vassalB.TradeScore = 20
	vassalB.EffectiveRank = polity.RankVassal

	kingdomB := polity.NewKingdom("KB", "KingdomB", rulerB, "CapB",
		polity.SuccessionPrimogeniture, 1000)
	kingdomB.CityIDs = append(kingdomB.CityIDs, "VassalB")

	cities := map[string]*polity.City{
		"CapA":    capitalA,
		"CapB":    capitalB,
		"VassalB": vassalB,
	}
	return kingdomA, kingdomB, cities, dice.New(seed, dice.SaltKingdomYear)
}

// TestApplyInterPolityEventsYear_NoNeighborsNoop verifies the tick is
// a no-op when the neighbor slice is empty.
func TestApplyInterPolityEventsYear_NoNeighborsNoop(t *testing.T) {
	origin, _, cities, stream := interPolityFixture(42)
	ApplyInterPolityEventsYear(InterPolityContext{
		Origin:    origin,
		Neighbors: nil,
		Cities:    cities,
		Stream:    stream,
		Year:      1050,
	})
	if len(origin.InterPolityHistory) != 0 {
		t.Fatalf("expected no history, got %d entries", len(origin.InterPolityHistory))
	}
}

// TestApplyInterPolityEventsYear_DissolvedIsNoop verifies a dissolved
// origin does not fire events.
func TestApplyInterPolityEventsYear_DissolvedIsNoop(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	origin.Dissolved = 1040
	ApplyInterPolityEventsYear(InterPolityContext{
		Origin:    origin,
		Neighbors: []*polity.Kingdom{neighbor},
		Cities:    cities,
		Stream:    stream,
		Year:      1050,
	})
	if len(origin.InterPolityHistory) != 0 {
		t.Fatalf("dissolved kingdom should not emit events, got %d",
			len(origin.InterPolityHistory))
	}
}

// TestApplyInterPolityEventsYear_DeterministicReplay verifies two runs
// with the same seed produce identical histories.
func TestApplyInterPolityEventsYear_DeterministicReplay(t *testing.T) {
	o1, n1, c1, s1 := interPolityFixture(7)
	o2, n2, c2, s2 := interPolityFixture(7)
	ApplyInterPolityEventsYear(InterPolityContext{
		Origin: o1, Neighbors: []*polity.Kingdom{n1},
		Cities: c1, Stream: s1, Year: 1050,
	})
	ApplyInterPolityEventsYear(InterPolityContext{
		Origin: o2, Neighbors: []*polity.Kingdom{n2},
		Cities: c2, Stream: s2, Year: 1050,
	})
	if len(o1.InterPolityHistory) != len(o2.InterPolityHistory) {
		t.Fatalf("history lengths differ: %d vs %d",
			len(o1.InterPolityHistory), len(o2.InterPolityHistory))
	}
	for i := range o1.InterPolityHistory {
		if o1.InterPolityHistory[i] != o2.InterPolityHistory[i] {
			t.Fatalf("entry %d differs: %+v vs %+v",
				i, o1.InterPolityHistory[i], o2.InterPolityHistory[i])
		}
	}
}

// TestApplyInterPolityEventsYear_RecordsInHistory verifies at least one
// event is recorded on a well-seeded call.
func TestApplyInterPolityEventsYear_RecordsInHistory(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	ApplyInterPolityEventsYear(InterPolityContext{
		Origin:    origin,
		Neighbors: []*polity.Kingdom{neighbor},
		Cities:    cities,
		Stream:    stream,
		Year:      1050,
	})
	if len(origin.InterPolityHistory) == 0 {
		t.Fatal("expected at least one recorded event")
	}
	for _, ev := range origin.InterPolityHistory {
		if ev.AggressorID != "KA" {
			t.Errorf("wrong aggressor: %s", ev.AggressorID)
		}
		if ev.TargetID != "KB" {
			t.Errorf("wrong target: %s", ev.TargetID)
		}
		if ev.Year != 1050 {
			t.Errorf("wrong year: %d", ev.Year)
		}
	}
}

// TestApplyInterPolityEventsYear_RespectsMaxActions verifies at most
// maxInterPolityActionsPerYear events are recorded.
func TestApplyInterPolityEventsYear_RespectsMaxActions(t *testing.T) {
	for seed := int64(1); seed < 50; seed++ {
		origin, neighbor, cities, stream := interPolityFixture(seed)
		ApplyInterPolityEventsYear(InterPolityContext{
			Origin:    origin,
			Neighbors: []*polity.Kingdom{neighbor},
			Cities:    cities,
			Stream:    stream,
			Year:      1050,
		})
		if len(origin.InterPolityHistory) > maxInterPolityActionsPerYear {
			t.Fatalf("seed=%d recorded %d events, cap is %d",
				seed, len(origin.InterPolityHistory), maxInterPolityActionsPerYear)
		}
	}
}

// TestApplyRaid_DrainsWealth verifies applyRaid subtracts wealth from
// every city of the target and queues the happiness penalty.
func TestApplyRaid_DrainsWealth(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	beforeCap := cities["CapB"].Wealth
	beforeVassal := cities["VassalB"].Wealth
	applyRaid(InterPolityContext{
		Origin: origin, Neighbors: []*polity.Kingdom{neighbor},
		Cities: cities, Stream: stream, Year: 1050,
	}, neighbor)
	if cities["CapB"].Wealth != beforeCap-raidWealthDrain {
		t.Errorf("CapB wealth not drained: before=%d after=%d",
			beforeCap, cities["CapB"].Wealth)
	}
	if cities["VassalB"].Wealth != beforeVassal-raidWealthDrain {
		t.Errorf("VassalB wealth not drained: before=%d after=%d",
			beforeVassal, cities["VassalB"].Wealth)
	}
	if len(cities["CapB"].HistoricalMods) == 0 {
		t.Error("CapB should have a happiness mod queued")
	}
}

// TestApplyTradeCompact_BoostsBothSides verifies trade compact bumps
// TradeScore on every city of both kingdoms.
func TestApplyTradeCompact_BoostsBothSides(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	beforeA := cities["CapA"].TradeScore
	beforeB := cities["CapB"].TradeScore
	beforeVB := cities["VassalB"].TradeScore
	applyTradeCompact(InterPolityContext{
		Origin: origin, Neighbors: []*polity.Kingdom{neighbor},
		Cities: cities, Stream: stream, Year: 1050,
	}, neighbor)
	if cities["CapA"].TradeScore != beforeA+tradeCompactTradeBonus {
		t.Errorf("CapA trade not boosted: before=%d after=%d",
			beforeA, cities["CapA"].TradeScore)
	}
	if cities["CapB"].TradeScore != beforeB+tradeCompactTradeBonus {
		t.Errorf("CapB trade not boosted: before=%d after=%d",
			beforeB, cities["CapB"].TradeScore)
	}
	if cities["VassalB"].TradeScore != beforeVB+tradeCompactTradeBonus {
		t.Errorf("VassalB trade not boosted: before=%d after=%d",
			beforeVB, cities["VassalB"].TradeScore)
	}
}

// TestApplyMissionary_ShiftsTargetFaith verifies the missionary action
// raises the origin ruler's faith share in every target city.
func TestApplyMissionary_ShiftsTargetFaith(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	beforeCapB := cities["CapB"].Faiths[polity.FaithSunCovenant]
	beforeVassal := cities["VassalB"].Faiths[polity.FaithSunCovenant]
	applyMissionary(InterPolityContext{
		Origin: origin, Neighbors: []*polity.Kingdom{neighbor},
		Cities: cities, Stream: stream, Year: 1050,
	}, neighbor)
	if cities["CapB"].Faiths[polity.FaithSunCovenant] <= beforeCapB {
		t.Errorf("CapB faith not shifted: before=%f after=%f",
			beforeCapB, cities["CapB"].Faiths[polity.FaithSunCovenant])
	}
	if cities["VassalB"].Faiths[polity.FaithSunCovenant] <= beforeVassal {
		t.Errorf("VassalB faith not shifted: before=%f after=%f",
			beforeVassal, cities["VassalB"].Faiths[polity.FaithSunCovenant])
	}
	// Post-normalize invariant: distribution sums to ~1.
	var total float64
	for _, f := range polity.AllFaiths() {
		total += cities["CapB"].Faiths[f]
	}
	if total < 0.999 || total > 1.001 {
		t.Errorf("CapB faiths do not sum to 1 after normalize: %f", total)
	}
}

// TestInterPolityHistory_RollingCap fills 200 events into a kingdom's
// InterPolityHistory via record() and verifies the slice never exceeds
// interPolityHistoryCap and that only the newest entries are kept.
func TestInterPolityHistory_RollingCap(t *testing.T) {
	origin, neighbor, _, _ := interPolityFixture(1)
	for i := range 200 {
		record(origin, polity.InterPolityRaid, neighbor, 1000+i, "success")
		if len(origin.InterPolityHistory) > interPolityHistoryCap {
			t.Fatalf("after %d inserts, history len=%d exceeds cap=%d",
				i+1, len(origin.InterPolityHistory), interPolityHistoryCap)
		}
	}
	if len(origin.InterPolityHistory) != interPolityHistoryCap {
		t.Fatalf("expected exactly %d entries after 200 inserts, got %d",
			interPolityHistoryCap, len(origin.InterPolityHistory))
	}
	// Verify the newest entries are retained: last year inserted is 1000+199=1199.
	last := origin.InterPolityHistory[len(origin.InterPolityHistory)-1]
	if last.Year != 1199 {
		t.Errorf("expected last entry year=1199, got %d", last.Year)
	}
	first := origin.InterPolityHistory[0]
	if first.Year != 1100 {
		t.Errorf("expected first entry year=1100 (200-100=100 trimmed), got %d", first.Year)
	}
}

// TestApplyTributeDemand_TransfersWealth verifies tribute demand moves
// wealth from the target's capital to the origin's capital.
func TestApplyTributeDemand_TransfersWealth(t *testing.T) {
	origin, neighbor, cities, stream := interPolityFixture(42)
	beforeA := cities["CapA"].Wealth
	beforeB := cities["CapB"].Wealth
	applyTributeDemand(InterPolityContext{
		Origin: origin, Neighbors: []*polity.Kingdom{neighbor},
		Cities: cities, Stream: stream, Year: 1050,
	}, neighbor)
	expectedDemand := int(float64(beforeB) * tributeDemandRate)
	if cities["CapB"].Wealth != beforeB-expectedDemand {
		t.Errorf("CapB wealth not reduced correctly: before=%d after=%d expected=%d",
			beforeB, cities["CapB"].Wealth, beforeB-expectedDemand)
	}
	if cities["CapA"].Wealth != beforeA+expectedDemand {
		t.Errorf("CapA wealth not increased correctly: before=%d after=%d expected=%d",
			beforeA, cities["CapA"].Wealth, beforeA+expectedDemand)
	}
}

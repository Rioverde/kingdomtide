package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

func mkKingdomWorld() (*polity.Kingdom, map[string]*polity.City) {
	founder := polity.Ruler{Stats: stats.CoreStats{
		Strength: 12, Dexterity: 10, Constitution: 14,
		Intelligence: 10, Wisdom: 12, Charisma: 14,
	}, BirthYear: 1250}

	capital := polity.NewCity("Capital", geom.Position{}, 1250, founder)
	capital.Population = 15000
	capital.Wealth = 10000
	capital.EffectiveRank = polity.RankCapital

	vassalA := polity.NewCity("VassalA", geom.Position{}, 1260, polity.Ruler{})
	vassalA.Population = 5000
	vassalA.Wealth = 2000
	vassalA.EffectiveRank = polity.RankVassal

	vassalB := polity.NewCity("VassalB", geom.Position{}, 1260, polity.Ruler{})
	vassalB.Population = 3000
	vassalB.Wealth = 1000
	vassalB.EffectiveRank = polity.RankVassal

	cities := map[string]*polity.City{
		"Capital": capital, "VassalA": vassalA, "VassalB": vassalB,
	}
	k := polity.NewKingdom("K1", "Kingdom", founder, "Capital",
		polity.SuccessionPrimogeniture, 1250)
	k.CityIDs = append(k.CityIDs, "VassalA", "VassalB")
	return k, cities
}

func TestTickKingdomYear_TributeFlowsOnCadence(t *testing.T) {
	k, cities := mkKingdomWorld()
	stream := dice.New(42, dice.SaltKingdomYear)
	beforeCap := cities["Capital"].Wealth
	beforeA := cities["VassalA"].Wealth
	beforeB := cities["VassalB"].Wealth

	// Tick on a cadence year (1250 is % 10 = 0).
	TickKingdomYear(k, cities, stream, 1250)

	if cities["VassalA"].Wealth >= beforeA {
		t.Errorf("VassalA should lose tribute, before=%d after=%d",
			beforeA, cities["VassalA"].Wealth)
	}
	if cities["VassalB"].Wealth >= beforeB {
		t.Errorf("VassalB should lose tribute, before=%d after=%d",
			beforeB, cities["VassalB"].Wealth)
	}
	if cities["Capital"].Wealth <= beforeCap {
		t.Errorf("Capital should gain tribute, before=%d after=%d",
			beforeCap, cities["Capital"].Wealth)
	}
}

func TestTickKingdomYear_NoTributeOffCadence(t *testing.T) {
	k, cities := mkKingdomWorld()
	stream := dice.New(42, dice.SaltKingdomYear)
	beforeA := cities["VassalA"].Wealth
	TickKingdomYear(k, cities, stream, 1253) // not % 10 == 0
	if cities["VassalA"].Wealth != beforeA {
		t.Errorf("VassalA wealth changed on non-cadence year: %d → %d",
			beforeA, cities["VassalA"].Wealth)
	}
}

func TestTickKingdomYear_AsabiyaDecaysOverCentury(t *testing.T) {
	k, cities := mkKingdomWorld()
	stream := dice.New(42, dice.SaltKingdomYear)
	before := k.Asabiya
	for yr := 1250; yr < 1350; yr++ {
		TickKingdomYear(k, cities, stream, yr)
	}
	// Multi-city kingdom → interior decay → asabiya should trend downward.
	if k.Asabiya >= before {
		t.Errorf("multi-city asabiya should decay, before=%v after=%v",
			before, k.Asabiya)
	}
}

func TestTickKingdomYear_CollapseOnLowAsabiya(t *testing.T) {
	k, cities := mkKingdomWorld()
	k.Asabiya = 0.05 // below threshold
	stream := dice.New(42, dice.SaltKingdomYear)
	TickKingdomYear(k, cities, stream, 1250)
	if k.Alive() {
		t.Errorf("kingdom with asabiya=0.05 should dissolve")
	}
	if cities["Capital"].EffectiveRank != polity.RankIndependent {
		t.Errorf("Capital should revert to Independent after dissolve, got %v",
			cities["Capital"].EffectiveRank)
	}
}

func TestTickKingdomYear_DissolvedIsNoop(t *testing.T) {
	k, cities := mkKingdomWorld()
	k.Dissolved = 1250 // already dissolved
	stream := dice.New(42, dice.SaltKingdomYear)
	beforeWealth := cities["VassalA"].Wealth
	TickKingdomYear(k, cities, stream, 1260)
	if cities["VassalA"].Wealth != beforeWealth {
		t.Errorf("dissolved kingdom should not extract tribute")
	}
}

func TestNewKingdom_Fields(t *testing.T) {
	founder := polity.Ruler{BirthYear: 1200}
	k := polity.NewKingdom("K", "Realm", founder, "CapitalCity", polity.SuccessionElective, 1250)
	if k.ID != "K" || k.Name != "Realm" {
		t.Errorf("fields not set")
	}
	if len(k.Rulers) != 1 || k.Rulers[0] != founder {
		t.Errorf("founder not in Rulers list")
	}
	if len(k.CityIDs) != 1 || k.CityIDs[0] != "CapitalCity" {
		t.Errorf("capital not in CityIDs")
	}
	if k.SuccessionLaw != polity.SuccessionElective {
		t.Errorf("succession law not set")
	}
	if !k.Alive() {
		t.Errorf("new kingdom should be alive")
	}
}

func TestSuccessionLaw_String(t *testing.T) {
	cases := []struct {
		l    polity.SuccessionLaw
		want string
	}{
		{polity.SuccessionPrimogeniture, "Primogeniture"},
		{polity.SuccessionTanistry, "Tanistry"},
		{polity.SuccessionSalic, "Salic"},
		{polity.SuccessionLaw(99), "UnknownSuccessionLaw"},
	}
	for _, c := range cases {
		if got := c.l.String(); got != c.want {
			t.Errorf("SuccessionLaw(%d).String() = %q, want %q", c.l, got, c.want)
		}
	}
}

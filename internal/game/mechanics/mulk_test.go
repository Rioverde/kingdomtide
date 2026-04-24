package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// fixedD100 is a deterministic D100 source for tests. Each call
// returns the current value and increments by one.
type fixedD100 int

func (f *fixedD100) D100() int {
	v := int(*f)
	*f++
	return v
}

func aliveKingdom(culture polity.Culture) *polity.Kingdom {
	k := polity.NewKingdom("k1", "TestKingdom", polity.Ruler{}, "city1", polity.SuccessionPrimogeniture, 0)
	k.Culture = culture
	return k
}

func TestApplyMulkCycleYear_SameCultureNoop(t *testing.T) {
	k := aliveKingdom(polity.CultureFeudal)
	cities := map[string]*polity.City{
		"city1": {Culture: polity.CultureFeudal},
		"city2": {Culture: polity.CultureFeudal},
	}
	k.CityIDs = []string{"city1", "city2"}
	k.Asabiya = 0.8

	// D100 always returns 0 — would trigger a flip, but cultures match.
	d := fixedD100(0)
	ApplyMulkCycleYear(k, cities, &d)

	for id, c := range cities {
		if c.Culture != polity.CultureFeudal {
			t.Errorf("city %s culture changed unexpectedly", id)
		}
	}
	if k.Asabiya != 0.8 {
		t.Errorf("asabiya changed unexpectedly: got %v", k.Asabiya)
	}
}

func TestApplyMulkCycleYear_DifferentCulturesCanFlip(t *testing.T) {
	k := aliveKingdom(polity.CultureFeudal)
	city := &polity.City{}
	city.Culture = polity.CultureSteppe
	cities := map[string]*polity.City{"city1": city}
	k.CityIDs = []string{"city1"}
	k.Asabiya = 0.5

	// D100 returns 0 — below int(0.015*100)==1, so flip triggers.
	d := fixedD100(0)
	ApplyMulkCycleYear(k, cities, &d)

	if city.Culture != polity.CultureFeudal {
		t.Errorf("expected city culture to flip to Feudal, got %v", city.Culture)
	}
}

func TestApplyMulkCycleYear_FlipCostsAsabiya(t *testing.T) {
	k := aliveKingdom(polity.CultureFeudal)
	city := &polity.City{}
	city.Culture = polity.CultureSteppe
	cities := map[string]*polity.City{"city1": city}
	k.CityIDs = []string{"city1"}
	k.Asabiya = 0.5

	d := fixedD100(0)
	ApplyMulkCycleYear(k, cities, &d)

	want := 0.5 - mulkAsabiyaCostPerFlip
	if k.Asabiya != want {
		t.Errorf("asabiya: want %v, got %v", want, k.Asabiya)
	}
}

func TestApplyMulkCycleYear_DissolvedIsNoop(t *testing.T) {
	k := aliveKingdom(polity.CultureFeudal)
	k.Dissolved = 100
	city := &polity.City{}
	city.Culture = polity.CultureSteppe
	cities := map[string]*polity.City{"city1": city}
	k.CityIDs = []string{"city1"}
	k.Asabiya = 0.5

	d := fixedD100(0)
	ApplyMulkCycleYear(k, cities, &d)

	if city.Culture != polity.CultureSteppe {
		t.Errorf("dissolved kingdom should not mutate city culture")
	}
	if k.Asabiya != 0.5 {
		t.Errorf("dissolved kingdom should not mutate asabiya")
	}
}

func TestApplyMulkCycleYear_AsabiyaClampsAtZero(t *testing.T) {
	k := aliveKingdom(polity.CultureFeudal)
	// Many cities with different culture — all flip at once.
	cities := make(map[string]*polity.City)
	ids := []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	for _, id := range ids {
		c := &polity.City{}
		c.Culture = polity.CultureSteppe
		cities[id] = c
	}
	k.CityIDs = ids
	k.Asabiya = 0.2 // only 2 flips worth of asabiya, but 6 cities will flip

	// D100 always returns 0 — every city flips.
	d := fixedD100(0)
	ApplyMulkCycleYear(k, cities, &d)

	if k.Asabiya < 0 {
		t.Errorf("asabiya went negative: %v", k.Asabiya)
	}
}

func TestCulture_String(t *testing.T) {
	cases := []struct {
		culture polity.Culture
		want    string
	}{
		{polity.CultureFeudal, "Feudal"},
		{polity.CultureSteppe, "Steppe"},
		{polity.CultureCeltic, "Celtic"},
		{polity.CultureRepublican, "Republican"},
		{polity.CultureImperial, "Imperial"},
		{polity.CultureNorthernFeudal, "NorthernFeudal"},
		{polity.Culture(255), "UnknownCulture"},
	}
	for _, tc := range cases {
		if got := tc.culture.String(); got != tc.want {
			t.Errorf("Culture(%d).String() = %q, want %q", tc.culture, got, tc.want)
		}
	}
}

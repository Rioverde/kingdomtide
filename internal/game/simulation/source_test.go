package simulation

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestSource_AllCampsSortedByPosition(t *testing.T) {
	src := &stubCampSource{camps: []polity.Camp{
		*newCamp(2, 50, 50, polity.RegionNormal, 30, polity.FaithOldGods),
		*newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods),
		*newCamp(3, 25, 100, polity.RegionNormal, 30, polity.FaithOldGods),
	}}
	r := Run(42, src, WithYears(1))
	s := r.SettlementSource()
	camps := s.AllCamps()
	if len(camps) != 3 {
		t.Fatalf("expected 3 camps, got %d", len(camps))
	}
	for i := 1; i < len(camps); i++ {
		if !lessPos(camps[i-1].Position, camps[i].Position) {
			t.Errorf("camps not sorted: %v then %v", camps[i-1].Position, camps[i].Position)
		}
	}
}

func TestSource_PlacesInGroupsBySCs(t *testing.T) {
	src := &stubCampSource{camps: []polity.Camp{
		*newCamp(1, 5, 5, polity.RegionNormal, 30, polity.FaithOldGods),
		*newCamp(2, 70, 70, polity.RegionNormal, 30, polity.FaithOldGods),
	}}
	r := Run(42, src, WithYears(1))
	s := r.SettlementSource()
	sc1 := geom.WorldToSuperChunk(5, 5)
	if got := len(s.PlacesIn(sc1)); got != 1 {
		t.Errorf("expected 1 place in sc1, got %d", got)
	}
}

func TestSource_ImplementsInterface(t *testing.T) {
	src := &stubCampSource{}
	r := Run(42, src, WithYears(1))
	var _ polity.SettlementSource = r.SettlementSource() // compile-time
}

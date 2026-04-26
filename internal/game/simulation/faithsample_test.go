package simulation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// staticCampSource is a minimal CampSource for tests that need a fixed camp set.
type staticCampSource struct{ camps []polity.Camp }

func (s *staticCampSource) CampsIn(_ geom.SuperChunkCoord) []polity.Camp { return nil }
func (s *staticCampSource) All() []polity.Camp                           { return s.camps }

func TestFaithEvents_Sample(t *testing.T) {
	var buf bytes.Buffer
	src := &staticCampSource{camps: []polity.Camp{
		{Settlement: polity.Settlement{ID: 1, Name: "Eridu", Region: polity.RegionHoly,
			Faiths: polity.NewFaithDistribution(), Population: 30}},
		{Settlement: polity.Settlement{ID: 2, Name: "Kish", Region: polity.RegionNormal,
			Faiths: polity.NewFaithDistribution(), Population: 25}},
	}}
	_ = Run(42, src, WithLogger(&buf), WithYears(200))
	lines := strings.Split(buf.String(), "\n")
	var faith []string
	for _, l := range lines {
		if strings.Contains(l, "faith-emerged") || strings.Contains(l, "faith-flipped") {
			faith = append(faith, l)
		}
	}
	t.Logf("Total faith events: %d", len(faith))
	for i, l := range faith {
		if i >= 20 {
			t.Logf("... (%d more)", len(faith)-20)
			break
		}
		t.Log(l)
	}
}

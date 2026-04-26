package simulation

import (
	"bytes"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// stubCampSource implements polity.CampSource for tests.
type stubCampSource struct{ camps []polity.Camp }

func (s *stubCampSource) All() []polity.Camp                           { return s.camps }
func (s *stubCampSource) CampsIn(_ geom.SuperChunkCoord) []polity.Camp { return s.camps }

func TestRun_DeterministicSameSeed(t *testing.T) {
	camps := []polity.Camp{
		*newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods),
		*newCamp(2, 50, 50, polity.RegionHoly, 30, polity.FaithSunCovenant),
	}
	src := &stubCampSource{camps: camps}

	a := Run(42, src, WithYears(10))
	b := Run(42, src, WithYears(10))

	if len(a.settlements) != len(b.settlements) {
		t.Fatalf("settlement count diverged: %d vs %d", len(a.settlements), len(b.settlements))
	}
	for id, va := range a.settlements {
		vb, ok := b.settlements[id]
		if !ok {
			t.Fatalf("id %d in run A missing from run B", id)
		}
		if va.Base().Population != vb.Base().Population {
			t.Errorf("pop diverged for %d: %d vs %d", id, va.Base().Population, vb.Base().Population)
		}
	}
}

func TestRun_LogsInitAndEnd(t *testing.T) {
	var buf bytes.Buffer
	src := &stubCampSource{camps: []polity.Camp{
		*newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods),
	}}
	_ = Run(42, src, WithYears(5), WithLogger(&buf))

	out := buf.String()
	if !contains(out, "sim-init") {
		t.Errorf("expected sim-init in log, got: %q", out)
	}
	if !contains(out, "sim-end") {
		t.Errorf("expected sim-end in log, got: %q", out)
	}
}

func TestRun_ProducesSnapshots(t *testing.T) {
	src := &stubCampSource{camps: []polity.Camp{
		*newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods),
	}}
	r := Run(42, src, WithYears(10), WithSnapshotEvery(2))
	got := len(r.Snapshots())
	if got < 5 || got > 6 {
		t.Errorf("expected ~5 snapshots over 10 years with WithSnapshotEvery(2), got %d", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

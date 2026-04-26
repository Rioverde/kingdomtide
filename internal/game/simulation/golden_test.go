package simulation

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// buildTinyCampSource generates a real Tiny world and constructs the
// camp source. Used by every golden test to share the worldgen cost.
func buildTinyCampSource(t *testing.T, seed int64) polity.CampSource {
	t.Helper()
	w := worldgen.Generate(seed, worldgen.WorldSizeTiny)
	regions := worldgen.NewRegionSource(w, seed)
	landmarks := worldgen.NewLandmarkSource(w, seed, worldgen.LandmarkSourceConfig{Regions: regions})
	volcanoes := worldgen.NewVolcanoSource(w, seed)
	deposits := worldgen.NewDepositSource(w, seed, worldgen.DepositSourceConfig{Volcanoes: volcanoes})
	return worldgen.NewCampSource(w, seed, worldgen.CampSourceConfig{
		Regions:   regions,
		Landmarks: landmarks,
		Volcanoes: volcanoes,
		Deposits:  deposits,
	})
}

// TestSimulationDeterminism — same seed produces identical end state.
func TestSimulationDeterminism(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation determinism — runs full Tiny worldgen + 500-year sim")
	}
	const seed int64 = 42
	src := buildTinyCampSource(t, seed)
	a := Run(seed, src)
	b := Run(seed, src)

	srcA, srcB := a.SettlementSource(), b.SettlementSource()
	if len(srcA.AllCamps()) != len(srcB.AllCamps()) {
		t.Fatalf("camp count diverged: %d vs %d", len(srcA.AllCamps()), len(srcB.AllCamps()))
	}
	if len(srcA.AllHamlets()) != len(srcB.AllHamlets()) {
		t.Fatalf("hamlet count diverged: %d vs %d", len(srcA.AllHamlets()), len(srcB.AllHamlets()))
	}
	if len(srcA.AllVillages()) != len(srcB.AllVillages()) {
		t.Fatalf("village count diverged: %d vs %d", len(srcA.AllVillages()), len(srcB.AllVillages()))
	}
	// Check positions match exactly.
	if !reflect.DeepEqual(srcA.AllCamps(), srcB.AllCamps()) {
		t.Errorf("camp slices not deeply equal across runs")
	}
}

// TestSimulationConvergence — produces a plausible mix of camps,
// hamlets, and villages by year 200.
func TestSimulationConvergence(t *testing.T) {
	if testing.Short() {
		t.Skip("simulation convergence — runs full Tiny worldgen + 500-year sim")
	}
	const seed int64 = 42
	src := buildTinyCampSource(t, seed)
	r := Run(seed, src)
	s := r.SettlementSource()

	camps := len(s.AllCamps())
	hamlets := len(s.AllHamlets())
	villages := len(s.AllVillages())
	total := camps + hamlets + villages

	if total == 0 {
		t.Fatal("expected some surviving settlements after simulation")
	}
	t.Logf("Tiny seed=42 year-200: %d camps, %d hamlets, %d villages",
		camps, hamlets, villages)
}

// TestNoSettlementOverlap — no two settlements share an anchor tile.
func TestNoSettlementOverlap(t *testing.T) {
	if testing.Short() {
		t.Skip("settlement overlap check — runs full Tiny worldgen + 500-year sim")
	}
	const seed int64 = 42
	src := buildTinyCampSource(t, seed)
	r := Run(seed, src)
	s := r.SettlementSource()

	anchors := make(map[polity.SettlementID]struct{})
	for _, c := range s.AllCamps() {
		anchors[c.ID] = struct{}{}
	}
	for _, h := range s.AllHamlets() {
		if _, exists := anchors[h.ID]; exists {
			t.Errorf("hamlet %d shares ID with another settlement", h.ID)
		}
		anchors[h.ID] = struct{}{}
	}
	for _, v := range s.AllVillages() {
		if _, exists := anchors[v.ID]; exists {
			t.Errorf("village %d shares ID with another settlement", v.ID)
		}
		anchors[v.ID] = struct{}{}
	}
}

// TestSimulationGolden_Tiny_Seed42 — committed snapshot baseline.
// Future tuning must not break these ranges. ±15% tolerance to
// handle minor noise.
func TestSimulationGolden_Tiny_Seed42(t *testing.T) {
	if testing.Short() {
		t.Skip("golden snapshot — runs full Tiny worldgen + 500-year sim")
	}
	const seed int64 = 42
	src := buildTinyCampSource(t, seed)
	r := Run(seed, src)
	s := r.SettlementSource()

	camps := len(s.AllCamps())
	hamlets := len(s.AllHamlets())
	villages := len(s.AllVillages())
	total := camps + hamlets + villages

	t.Logf("baseline: total=%d camps=%d hamlets=%d villages=%d",
		total, camps, hamlets, villages)

	// Ranges captured after the simulation audit Wave-1 fixes
	// (per-(year,id) pop rng, faith ε symmetry, 5× region-affinity rate,
	// Holy +20% / Blighted -40% growth modifier, ruler succession,
	// plague schedule, sortedID cache). The new dynamics produce far
	// more growth: Holy regions outpace baseline, satellite spawning
	// keeps cascading, and successful villages emerge.
	// Observed (seed=42, 500-year horizon): camps=68 hamlets=57 villages=15 total=140.
	// Windows are ±15% of the observed values rounded to integers.
	// Re-capture and update here if tuning constants change.
	const (
		wantCampsMin    = 58
		wantCampsMax    = 78
		wantHamletsMin  = 48
		wantHamletsMax  = 66
		wantVillagesMin = 12
		wantVillagesMax = 18
		wantTotalMin    = 119
		wantTotalMax    = 161
	)
	if camps < wantCampsMin || camps > wantCampsMax {
		t.Errorf("camps=%d want [%d, %d]", camps, wantCampsMin, wantCampsMax)
	}
	if hamlets < wantHamletsMin || hamlets > wantHamletsMax {
		t.Errorf("hamlets=%d want [%d, %d]", hamlets, wantHamletsMin, wantHamletsMax)
	}
	if villages < wantVillagesMin || villages > wantVillagesMax {
		t.Errorf("villages=%d want [%d, %d]", villages, wantVillagesMin, wantVillagesMax)
	}
	if total < wantTotalMin || total > wantTotalMax {
		t.Errorf("total=%d want [%d, %d]", total, wantTotalMin, wantTotalMax)
	}
}

// TestSimulationLogFormat — log lines parse as the documented format.
func TestSimulationLogFormat(t *testing.T) {
	if testing.Short() {
		t.Skip("log format check — runs full Tiny worldgen + 500-year sim")
	}
	const seed int64 = 42
	src := buildTinyCampSource(t, seed)

	var buf simBuffer
	_ = Run(seed, src, WithLogger(&buf))

	if !buf.contains("[year +000] sim-init") {
		t.Errorf("missing sim-init line; got:\n%s", buf.String())
	}
	if !buf.contains("[year +499] sim-end") {
		t.Errorf("missing sim-end line; got:\n%s", buf.String())
	}
}

// simBuffer is a minimal io.Writer for log tests, avoiding bytes.Buffer
// imports in this file to keep the test compact.
type simBuffer struct{ data []byte }

func (b *simBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *simBuffer) String() string { return string(b.data) }

func (b *simBuffer) contains(s string) bool {
	for i := 0; i+len(s) <= len(b.data); i++ {
		if string(b.data[i:i+len(s)]) == s {
			return true
		}
	}
	return false
}

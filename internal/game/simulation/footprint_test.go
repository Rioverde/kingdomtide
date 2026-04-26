package simulation

import (
	"math/rand/v2"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestSettlementFootprintBudget checks the budget formula at tier boundaries.
func TestSettlementFootprintBudget(t *testing.T) {
	cases := []struct {
		tier polity.SettlementTier
		pop  int
		want int
	}{
		{polity.TierCamp, 10, 2},
		{polity.TierCamp, 25, 2},
		{polity.TierCamp, 26, 3},
		{polity.TierHamlet, 50, 4},
		{polity.TierHamlet, 80, 4},
		{polity.TierHamlet, 81, 5},
		{polity.TierHamlet, 120, 5},
		{polity.TierHamlet, 121, 6},
		{polity.TierVillage, 100, 8},  // sqrt(100/100)=1 → clamped to 8
		{polity.TierVillage, 6400, 8}, // sqrt(6400/100)=8 → exactly 8
		{polity.TierVillage, 900, 8},  // sqrt(900/100)=3 → clamped to 8
		{polity.TierVillage, 40000, 20},
	}
	for _, c := range cases {
		got := settlementFootprintBudget(c.tier, c.pop)
		if got != c.want {
			t.Errorf("budget(%v, pop=%d) = %d, want %d", c.tier, c.pop, got, c.want)
		}
	}
}

// TestSettlementFootprintBudget_VillageClamp verifies the [8,20] clamp.
func TestSettlementFootprintBudget_VillageClamp(t *testing.T) {
	if settlementFootprintBudget(polity.TierVillage, 1) < 8 {
		t.Errorf("village budget below 8 for pop=1")
	}
	if settlementFootprintBudget(polity.TierVillage, 1_000_000) > 20 {
		t.Errorf("village budget above 20 for pop=1000000")
	}
}

// TestRegrowFootprint_GrowsTowardBudget verifies tiles are appended up to budget.
func TestRegrowFootprint_GrowsTowardBudget(t *testing.T) {
	s := &polity.Settlement{
		Footprint: []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
	}
	rng := rand.New(rand.NewPCG(1, 2))
	regrowFootprint(s, 6, rng, nil)
	if len(s.Footprint) != 6 {
		t.Errorf("want 6 tiles, got %d", len(s.Footprint))
	}
}

// TestRegrowFootprint_NoShrink verifies existing tiles are never removed.
func TestRegrowFootprint_NoShrink(t *testing.T) {
	initial := []geom.Position{{X: 5, Y: 5}, {X: 6, Y: 5}, {X: 5, Y: 6}}
	s := &polity.Settlement{
		Footprint: append([]geom.Position(nil), initial...),
	}
	rng := rand.New(rand.NewPCG(99, 7))
	regrowFootprint(s, 7, rng, nil)

	// All original tiles must still be present.
	present := make(map[geom.Position]bool)
	for _, p := range s.Footprint {
		present[p] = true
	}
	for _, p := range initial {
		if !present[p] {
			t.Errorf("original tile %v missing after regrow", p)
		}
	}
}

// TestRegrowFootprint_AlreadyAtBudget verifies no mutation when footprint >= budget.
func TestRegrowFootprint_AlreadyAtBudget(t *testing.T) {
	s := &polity.Settlement{
		Footprint: []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}},
	}
	rng := rand.New(rand.NewPCG(0, 0))
	regrowFootprint(s, 2, rng, nil) // budget < current size
	if len(s.Footprint) != 3 {
		t.Errorf("footprint should not shrink; got %d tiles", len(s.Footprint))
	}
}

// TestRegrowFootprint_NoOverlap verifies grown tiles are distinct.
func TestRegrowFootprint_NoOverlap(t *testing.T) {
	s := &polity.Settlement{
		Footprint: []geom.Position{{X: 0, Y: 0}},
	}
	rng := rand.New(rand.NewPCG(42, 13))
	regrowFootprint(s, 20, rng, nil)

	seen := make(map[geom.Position]bool, len(s.Footprint))
	for _, p := range s.Footprint {
		if seen[p] {
			t.Errorf("duplicate tile %v in footprint", p)
		}
		seen[p] = true
	}
}

// TestRegrowFootprint_ValidatorRejectsAllNeighbours verifies a validator
// that rejects every candidate stops growth at the original footprint.
func TestRegrowFootprint_ValidatorRejectsAll(t *testing.T) {
	s := &polity.Settlement{
		Footprint: []geom.Position{{X: 0, Y: 0}},
	}
	rng := rand.New(rand.NewPCG(7, 11))
	regrowFootprint(s, 6, rng, func(geom.Position) bool { return false })
	if len(s.Footprint) != 1 {
		t.Errorf("validator rejected all tiles; expected 1 footprint tile, got %d",
			len(s.Footprint))
	}
}

// TestRegrowFootprint_ValidatorAcceptsAll verifies the validator-accepts-all
// path matches the nil-validator behaviour for the same seed.
func TestRegrowFootprint_ValidatorAcceptsAll(t *testing.T) {
	mk := func() *polity.Settlement {
		return &polity.Settlement{
			Footprint: []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}},
		}
	}
	a := mk()
	b := mk()
	rngA := rand.New(rand.NewPCG(3, 5))
	rngB := rand.New(rand.NewPCG(3, 5))
	regrowFootprint(a, 6, rngA, nil)
	regrowFootprint(b, 6, rngB, func(geom.Position) bool { return true })
	if len(a.Footprint) != len(b.Footprint) {
		t.Errorf("nil vs accept-all validator diverged: %d vs %d tiles",
			len(a.Footprint), len(b.Footprint))
	}
	for i := range a.Footprint {
		if a.Footprint[i] != b.Footprint[i] {
			t.Errorf("tile %d diverged: %v vs %v", i, a.Footprint[i], b.Footprint[i])
		}
	}
}

// TestPromoteCampToHamlet_FootprintGrows verifies footprint expands on promotion.
func TestPromoteCampToHamlet_FootprintGrows(t *testing.T) {
	st := newState(42, 5)
	c := newCamp(1, 0, 0, polity.RegionNormal, 100, polity.FaithOldGods)
	// Give the camp a minimal 2-tile footprint as worldgen would.
	c.Footprint = []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}}
	st.settlements[1] = c

	for y := 0; y < simHamletPromoteSustain; y++ {
		st.tickPromotions(y)
	}

	hamlet, ok := st.settlements[1].(*polity.Hamlet)
	if !ok {
		t.Fatalf("expected Hamlet, got %T", st.settlements[1])
	}
	minBudget := settlementFootprintBudget(polity.TierHamlet, hamlet.Population)
	if len(hamlet.Footprint) < minBudget {
		t.Errorf("hamlet footprint = %d tiles, want >= %d (budget for pop %d)",
			len(hamlet.Footprint), minBudget, hamlet.Population)
	}
}

// TestPromoteHamletToVillage_FootprintGrows verifies footprint expands on promotion.
func TestPromoteHamletToVillage_FootprintGrows(t *testing.T) {
	st := newState(42, 5)
	h := &polity.Hamlet{Settlement: polity.Settlement{
		ID:         2,
		Tier:       polity.TierHamlet,
		Population: 200,
		Region:     polity.RegionNormal,
		Footprint:  []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1}},
	}}
	st.settlements[2] = h

	for y := 0; y < simVillagePromoteSustain; y++ {
		st.tickPromotions(y)
	}

	village, ok := st.settlements[2].(*polity.Village)
	if !ok {
		t.Fatalf("expected Village, got %T", st.settlements[2])
	}
	minBudget := settlementFootprintBudget(polity.TierVillage, village.Population)
	if len(village.Footprint) < minBudget {
		t.Errorf("village footprint = %d tiles, want >= %d (budget for pop %d)",
			len(village.Footprint), minBudget, village.Population)
	}
}

// TestMerge_FootprintGrows verifies merged survivor gets a regrown footprint.
func TestMerge_FootprintGrows(t *testing.T) {
	st := newState(42, 5)
	// Two camps close enough to merge, same region/faith.
	a := newCamp(1, 0, 0, polity.RegionNormal, 30, polity.FaithOldGods)
	a.Footprint = []geom.Position{{X: 0, Y: 0}, {X: 1, Y: 0}}
	b := newCamp(2, 2, 0, polity.RegionNormal, 30, polity.FaithOldGods)
	b.Footprint = []geom.Position{{X: 2, Y: 0}, {X: 3, Y: 0}}
	st.settlements[1] = a
	st.settlements[2] = b

	st.mergeSettlements(1, 2, 0)

	// One settlement should survive.
	if len(st.settlements) != 1 {
		t.Fatalf("expected 1 survivor, got %d", len(st.settlements))
	}
	for _, p := range st.settlements {
		survivor := p.Base()
		budget := settlementFootprintBudget(survivor.Tier, survivor.Population)
		if len(survivor.Footprint) < budget {
			t.Errorf("merged survivor footprint = %d, want >= %d (budget for pop %d)",
				len(survivor.Footprint), budget, survivor.Population)
		}
	}
}

// TestFootprintSizes_FullSim samples avg footprint sizes by tier after a
// 200-year run on several seeds. Gated on !short because it runs worldgen.
func TestFootprintSizes_FullSim(t *testing.T) {
	if testing.Short() {
		t.Skip("footprint size sample — runs full Tiny worldgen + 200-year sim")
	}

	seeds := []int64{42, 1337, 999}
	for _, seed := range seeds {
		src := buildTinyCampSource(t, seed)
		r := Run(seed, src)
		s := r.SettlementSource()

		var campTotal, campCount int
		for _, c := range s.AllCamps() {
			campTotal += len(c.Footprint)
			campCount++
		}
		var hamletTotal, hamletCount int
		for _, h := range s.AllHamlets() {
			hamletTotal += len(h.Footprint)
			hamletCount++
		}
		var villageTotal, villageCount int
		for _, v := range s.AllVillages() {
			villageTotal += len(v.Footprint)
			villageCount++
		}

		avgOrZero := func(sum, n int) float64 {
			if n == 0 {
				return 0
			}
			return float64(sum) / float64(n)
		}
		t.Logf("seed=%d: camps=%d avg_fp=%.1f | hamlets=%d avg_fp=%.1f | villages=%d avg_fp=%.1f",
			seed, campCount, avgOrZero(campTotal, campCount),
			hamletCount, avgOrZero(hamletTotal, hamletCount),
			villageCount, avgOrZero(villageTotal, villageCount))

		// Hamlets must average >= 4 tiles; villages >= 8.
		if hamletCount > 0 && avgOrZero(hamletTotal, hamletCount) < 4 {
			t.Errorf("seed=%d: hamlet avg footprint %.1f < 4",
				seed, avgOrZero(hamletTotal, hamletCount))
		}
		if villageCount > 0 && avgOrZero(villageTotal, villageCount) < 8 {
			t.Errorf("seed=%d: village avg footprint %.1f < 8",
				seed, avgOrZero(villageTotal, villageCount))
		}
	}
}

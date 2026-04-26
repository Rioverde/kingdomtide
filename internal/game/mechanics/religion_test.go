package mechanics

import (
	"math"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyReligionDiffusionYear_MajorityGrows verifies that a
// baseline OldGods-dominant city sees OldGods share grow after one
// tick under self-diffusion. Explicitly zeroes the non-contesting
// faiths so the default seed's 0.02 minority shares don't dilute
// the majority's pulse growth past the assertion threshold.
func TestApplyReligionDiffusionYear_MajorityGrows(t *testing.T) {
	c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Faiths[polity.FaithOldGods] = 0.6
	c.Faiths[polity.FaithSunCovenant] = 0.4
	c.Faiths[polity.FaithGreenSage] = 0
	c.Faiths[polity.FaithOneOath] = 0
	c.Faiths[polity.FaithStormPact] = 0
	before := c.Faiths[polity.FaithOldGods]
	ApplyReligionDiffusionYear(c, dice.New(42, dice.SaltReligion), 1500)
	if c.Faiths[polity.FaithOldGods] <= before {
		t.Errorf("OldGods share did not grow: before=%v after=%v",
			before, c.Faiths[polity.FaithOldGods])
	}
}

// TestApplyReligionDiffusionYear_NormalizedSum verifies that after a
// diffusion tick the distribution sums to 1.0 within floating tol.
func TestApplyReligionDiffusionYear_NormalizedSum(t *testing.T) {
	c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Faiths[polity.FaithOldGods] = 0.5
	c.Faiths[polity.FaithSunCovenant] = 0.3
	c.Faiths[polity.FaithGreenSage] = 0.2
	stream := dice.New(42, dice.SaltReligion)
	for i := 0; i < 100; i++ {
		ApplyReligionDiffusionYear(c, stream, 1500+i)
		var total float64
		for _, f := range polity.AllFaiths() {
			total += c.Faiths[f]
		}
		if math.Abs(total-1.0) > 1e-9 {
			t.Fatalf("iter %d: sum=%v, want 1.0", i, total)
		}
	}
}

// TestApplyReligionDiffusionYear_SchismFires verifies the §6a
// four-gate schism condition triggers the 60/40 split when gates
// open (secondary ≥ 0.4, gap ≤ 0.2, Innovation ≥ 45).
func TestApplyReligionDiffusionYear_SchismFires(t *testing.T) {
	c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Faiths[polity.FaithOldGods] = 0.55
	c.Faiths[polity.FaithSunCovenant] = 0.45
	c.Innovation = 50 // above the DC 45 gate
	ApplyReligionDiffusionYear(c, dice.New(42, dice.SaltReligion), 1500)

	// Majority + minority should now sit close to the 0.6 / 0.4
	// split after Normalize (minor drift from other faiths = 0).
	major := c.Faiths[polity.FaithOldGods]
	minor := c.Faiths[polity.FaithSunCovenant]
	if math.Abs(major-0.6) > 0.05 || math.Abs(minor-0.4) > 0.05 {
		t.Errorf("schism did not produce ~60/40 split: major=%v minor=%v",
			major, minor)
	}
}

// TestApplyReligionDiffusionYear_SchismBlockedByLowInnovation
// verifies Innovation < 45 prevents the schism from firing even
// when the other three gates open.
func TestApplyReligionDiffusionYear_SchismBlockedByLowInnovation(t *testing.T) {
	c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Faiths[polity.FaithOldGods] = 0.55
	c.Faiths[polity.FaithSunCovenant] = 0.45
	c.Innovation = 30 // below the DC 45 gate
	ApplyReligionDiffusionYear(c, dice.New(42, dice.SaltReligion), 1500)

	// Diffusion nudges majority up by <= 0.03. Split should NOT
	// snap to 0.6 / 0.4 because the Innovation gate is closed.
	if math.Abs(c.Faiths[polity.FaithOldGods]-0.6) < 0.01 {
		t.Errorf("schism fired without Innovation gate: major=%v",
			c.Faiths[polity.FaithOldGods])
	}
}

// TestApplyReligionDiffusionYear_Determinism verifies two identical
// cities on identical streams produce identical distributions.
func TestApplyReligionDiffusionYear_Determinism(t *testing.T) {
	mk := func() *polity.City {
		c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
		c.Faiths[polity.FaithOldGods] = 0.6
		c.Faiths[polity.FaithSunCovenant] = 0.4
		return c
	}
	a, b := mk(), mk()
	streamA := dice.New(42, dice.SaltReligion)
	streamB := dice.New(42, dice.SaltReligion)
	for i := 0; i < 50; i++ {
		ApplyReligionDiffusionYear(a, streamA, 1500+i)
		ApplyReligionDiffusionYear(b, streamB, 1500+i)
	}
	for _, f := range polity.AllFaiths() {
		if a.Faiths[f] != b.Faiths[f] {
			t.Errorf("faith %s diverged: a=%v b=%v", f, a.Faiths[f], b.Faiths[f])
		}
	}
}

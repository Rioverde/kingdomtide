package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyMineralDepletionYear_NoDeposits verifies the no-op path
// — a city with no deposits returns cleanly without touching state.
func TestApplyMineralDepletionYear_NoDeposits(t *testing.T) {
	c := &polity.City{}
	ApplyMineralDepletionYear(c)
	if len(c.Deposits) != 0 {
		t.Errorf("Deposits grew from zero-value city: len=%d", len(c.Deposits))
	}
}

// TestApplyMineralDepletionYear_DrainReducesYield verifies the base
// drain applies to every deposit even with zero army.
func TestApplyMineralDepletionYear_DrainReducesYield(t *testing.T) {
	c := &polity.City{
		Deposits: []polity.Deposit{
			{Kind: polity.DepositIron, RemainingYield: 0.5},
			{Kind: polity.DepositGold, RemainingYield: 0.9},
		},
	}
	original := make([]float64, len(c.Deposits))
	for i, d := range c.Deposits {
		original[i] = d.RemainingYield
	}
	ApplyMineralDepletionYear(c)
	for i, d := range c.Deposits {
		if d.RemainingYield >= original[i] {
			t.Errorf("deposit %d: RemainingYield=%v did not decrease from %v",
				i, d.RemainingYield, original[i])
		}
	}
}

// TestApplyMineralDepletionYear_ExhaustedRemoved verifies deposits
// that fall at or below the exhaustion threshold are dropped from
// the city's slice.
func TestApplyMineralDepletionYear_ExhaustedRemoved(t *testing.T) {
	c := &polity.City{
		Deposits: []polity.Deposit{
			{Kind: polity.DepositIron, RemainingYield: 0.105},
			{Kind: polity.DepositGold, RemainingYield: 0.9},
		},
	}
	ApplyMineralDepletionYear(c)
	if len(c.Deposits) != 1 {
		t.Fatalf("expected 1 deposit after exhaustion, got %d", len(c.Deposits))
	}
	if c.Deposits[0].Kind != polity.DepositGold {
		t.Errorf("expected surviving deposit to be Gold, got %s", c.Deposits[0].Kind)
	}
}

// TestApplyMineralDepletionYear_DrainCapped verifies that even with
// an enormous army the per-year drain clamps to mineralDrainMax.
func TestApplyMineralDepletionYear_DrainCapped(t *testing.T) {
	c := &polity.City{
		Army: 1_000_000,
		Deposits: []polity.Deposit{
			{Kind: polity.DepositIron, RemainingYield: 1.0},
		},
	}
	ApplyMineralDepletionYear(c)
	drain := 1.0 - c.Deposits[0].RemainingYield
	if drain > mineralDrainMax+1e-9 {
		t.Errorf("drain %v exceeds cap %v", drain, mineralDrainMax)
	}
}

// TestApplyMineralDepletionYear_Determinism verifies two cities
// with identical deposits get identical yield after one tick.
func TestApplyMineralDepletionYear_Determinism(t *testing.T) {
	newCity := func() *polity.City {
		return &polity.City{
			Army: 2000,
			Deposits: []polity.Deposit{
				{Kind: polity.DepositIron, RemainingYield: 0.75},
				{Kind: polity.DepositCoal, RemainingYield: 0.55},
			},
		}
	}
	a, b := newCity(), newCity()
	ApplyMineralDepletionYear(a)
	ApplyMineralDepletionYear(b)
	for i := range a.Deposits {
		if a.Deposits[i].RemainingYield != b.Deposits[i].RemainingYield {
			t.Errorf("deposit %d diverged: a=%v b=%v",
				i, a.Deposits[i].RemainingYield, b.Deposits[i].RemainingYield)
		}
	}
}

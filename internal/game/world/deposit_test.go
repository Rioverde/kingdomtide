package world

import "testing"

func TestDepositKind_Key(t *testing.T) {
	cases := []struct {
		kind DepositKind
		want string
	}{
		{DepositNone, "none"},
		{DepositIron, "iron"},
		{DepositStone, "stone"},
		{DepositTimber, "timber"},
		{DepositFertile, "fertile"},
		{DepositFish, "fish"},
		{DepositGame, "game"},
		{DepositSalt, "salt"},
		{DepositGold, "gold"},
		{DepositSilver, "silver"},
		{DepositGems, "gems"},
		{DepositObsidian, "obsidian"},
		{DepositSulfur, "sulfur"},
	}
	for _, c := range cases {
		if got := c.kind.Key(); got != c.want {
			t.Errorf("DepositKind(%d).Key() = %q, want %q", c.kind, got, c.want)
		}
		if got := c.kind.String(); got != c.want {
			t.Errorf("DepositKind(%d).String() = %q, want %q", c.kind, got, c.want)
		}
	}
	if got := DepositKind(99).Key(); got != "" {
		t.Errorf(`DepositKind(99).Key() = %q, want ""`, got)
	}
}

func TestAllDepositKinds_NotEmpty(t *testing.T) {
	kinds := AllDepositKinds()
	if len(kinds) == 0 {
		t.Fatal("AllDepositKinds returned empty slice")
	}
	for _, k := range kinds {
		if k == DepositNone {
			t.Fatal("AllDepositKinds must not include DepositNone")
		}
	}
}

func TestDeposit_ZeroValue(t *testing.T) {
	var d Deposit
	if d.Kind != DepositNone {
		t.Errorf("zero-value Deposit.Kind = %v, want DepositNone", d.Kind)
	}
	if d.MaxAmount != 0 || d.CurrentAmount != 0 || d.LastRespawn != 0 {
		t.Errorf("zero-value Deposit has non-zero amount/respawn: %+v", d)
	}
}

package game

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

func TestAllDepositKinds_CoversEveryCategory(t *testing.T) {
	kinds := AllDepositKinds()
	if len(kinds) == 0 {
		t.Fatal("AllDepositKinds returned empty slice")
	}
	// Ensure DepositNone is NOT in the list — the slice represents
	// placeable kinds, not the zero sentinel.
	for _, k := range kinds {
		if k == DepositNone {
			t.Fatal("AllDepositKinds must not include DepositNone")
		}
	}
	counts := map[DepositCategory]int{}
	for _, k := range kinds {
		counts[CategoryOf(k)]++
	}
	if counts[CategoryZonal] == 0 {
		t.Error("no zonal deposits in AllDepositKinds")
	}
	if counts[CategoryPointLike] == 0 {
		t.Error("no point-like deposits in AllDepositKinds")
	}
	if counts[CategoryStructural] == 0 {
		t.Error("no structural deposits in AllDepositKinds")
	}
}

func TestCategoryOf_ExplicitMapping(t *testing.T) {
	cases := []struct {
		kind DepositKind
		want DepositCategory
	}{
		{DepositFertile, CategoryZonal},
		{DepositTimber, CategoryZonal},
		{DepositGame, CategoryZonal},

		{DepositIron, CategoryPointLike},
		{DepositStone, CategoryPointLike},
		{DepositSalt, CategoryPointLike},
		{DepositGold, CategoryPointLike},
		{DepositSilver, CategoryPointLike},
		{DepositGems, CategoryPointLike},

		{DepositFish, CategoryStructural},
		{DepositObsidian, CategoryStructural},
		{DepositSulfur, CategoryStructural},
	}
	for _, c := range cases {
		if got := CategoryOf(c.kind); got != c.want {
			t.Errorf("CategoryOf(%s) = %s, want %s", c.kind, got, c.want)
		}
	}
}

func TestDepositCategory_String(t *testing.T) {
	cases := []struct {
		c    DepositCategory
		want string
	}{
		{CategoryZonal, "zonal"},
		{CategoryPointLike, "point_like"},
		{CategoryStructural, "structural"},
		{DepositCategory(99), ""},
	}
	for _, c := range cases {
		if got := c.c.String(); got != c.want {
			t.Errorf("DepositCategory(%d).String() = %q, want %q", c.c, got, c.want)
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

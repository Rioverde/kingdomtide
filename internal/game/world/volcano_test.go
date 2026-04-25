package world

import "testing"

func TestVolcanoState_Key(t *testing.T) {
	cases := []struct {
		state VolcanoState
		want  string
	}{
		{VolcanoStateUnknown, "unknown"},
		{VolcanoActive, "active"},
		{VolcanoDormant, "dormant"},
		{VolcanoExtinct, "extinct"},
	}
	for _, c := range cases {
		if got := c.state.Key(); got != c.want {
			t.Fatalf("VolcanoState(%d).Key() = %q, want %q", c.state, got, c.want)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("VolcanoState(99).Key() panicked: %v", r)
		}
	}()
	if got := VolcanoState(99).Key(); got != "" {
		t.Fatalf(`VolcanoState(99).Key() = %q, want ""`, got)
	}
}

func TestVolcanoState_String(t *testing.T) {
	states := []VolcanoState{
		VolcanoStateUnknown,
		VolcanoActive,
		VolcanoDormant,
		VolcanoExtinct,
	}
	for _, s := range states {
		if got, want := s.String(), s.Key(); got != want {
			t.Fatalf("VolcanoState(%d): String() = %q, Key() = %q; want equal", s, got, want)
		}
	}
	if got := VolcanoState(99).String(); got != "" {
		t.Fatalf(`VolcanoState(99).String() = %q, want ""`, got)
	}
}

func TestVolcano_ZeroValue(t *testing.T) {
	var v Volcano
	if v.State != VolcanoStateUnknown {
		t.Fatalf("zero-value Volcano.State = %v, want VolcanoStateUnknown", v.State)
	}
	if v.CoreTiles != nil || v.SlopeTiles != nil || v.AshlandTiles != nil {
		t.Fatalf("zero-value Volcano footprint slices should be nil, got core=%v slope=%v ash=%v",
			v.CoreTiles, v.SlopeTiles, v.AshlandTiles)
	}
}

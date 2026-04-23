package game

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

func TestVolcanoZone_Key(t *testing.T) {
	cases := []struct {
		zone VolcanoZone
		want string
	}{
		{VolcanoZoneNone, ""},
		{VolcanoZoneCore, "core"},
		{VolcanoZoneSlope, "slope"},
		{VolcanoZoneAshland, "ashland"},
		{VolcanoZone(99), ""},
	}
	for _, c := range cases {
		if got := c.zone.Key(); got != c.want {
			t.Fatalf("VolcanoZone(%d).Key() = %q, want %q", c.zone, got, c.want)
		}
		if got := c.zone.String(); got != c.want {
			t.Fatalf("VolcanoZone(%d).String() = %q, want %q", c.zone, got, c.want)
		}
	}
}

func TestVolcano_ZoneAt(t *testing.T) {
	core := Position{X: 0, Y: 0}
	slopeA := Position{X: 1, Y: 0}
	slopeB := Position{X: 0, Y: 1}
	ash := Position{X: 2, Y: 0}
	miss := Position{X: 10, Y: 10}

	v := Volcano{
		Anchor:       core,
		State:        VolcanoActive,
		CoreTiles:    []Position{core},
		SlopeTiles:   []Position{slopeA, slopeB},
		AshlandTiles: []Position{ash},
	}

	cases := []struct {
		tile Position
		want VolcanoZone
	}{
		{core, VolcanoZoneCore},
		{slopeA, VolcanoZoneSlope},
		{slopeB, VolcanoZoneSlope},
		{ash, VolcanoZoneAshland},
		{miss, VolcanoZoneNone},
	}
	for _, c := range cases {
		if got := v.ZoneAt(c.tile); got != c.want {
			t.Fatalf("Volcano.ZoneAt(%+v) = %v, want %v", c.tile, got, c.want)
		}
	}
}

func TestVolcano_ZeroValue(t *testing.T) {
	var v Volcano
	if v.State != VolcanoStateUnknown {
		t.Fatalf("zero-value Volcano.State = %v, want VolcanoStateUnknown", v.State)
	}
	if got := v.ZoneAt(Position{X: 0, Y: 0}); got != VolcanoZoneNone {
		t.Fatalf("zero-value Volcano.ZoneAt = %v, want VolcanoZoneNone", got)
	}
	if v.CoreTiles != nil || v.SlopeTiles != nil || v.AshlandTiles != nil {
		t.Fatalf("zero-value Volcano footprint slices should be nil, got core=%v slope=%v ash=%v",
			v.CoreTiles, v.SlopeTiles, v.AshlandTiles)
	}
}

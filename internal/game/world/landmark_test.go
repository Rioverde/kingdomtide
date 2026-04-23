package world

import "testing"

func TestLandmarkKindKey(t *testing.T) {
	cases := []struct {
		kind LandmarkKind
		want string
	}{
		{LandmarkNone, "none"},
		{LandmarkTower, "tower"},
		{LandmarkGiantTree, "giant_tree"},
		{LandmarkStandingStones, "standing_stones"},
		{LandmarkObelisk, "obelisk"},
		{LandmarkChasm, "chasm"},
		{LandmarkShrine, "shrine"},
	}
	for _, c := range cases {
		if got := c.kind.Key(); got != c.want {
			t.Fatalf("LandmarkKind(%d).Key() = %q, want %q", c.kind, got, c.want)
		}
	}
}

func TestLandmarkKindString(t *testing.T) {
	kinds := []LandmarkKind{
		LandmarkNone,
		LandmarkTower,
		LandmarkGiantTree,
		LandmarkStandingStones,
		LandmarkObelisk,
		LandmarkChasm,
		LandmarkShrine,
	}
	for _, k := range kinds {
		if got, want := k.String(), k.Key(); got != want {
			t.Fatalf("LandmarkKind(%d): String() = %q, Key() = %q; want equal", k, got, want)
		}
	}
}

func TestLandmarkKindOutOfRange(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("LandmarkKind(99).Key() panicked: %v", r)
		}
	}()
	if got := LandmarkKind(99).Key(); got != "" {
		t.Fatalf(`LandmarkKind(99).Key() = %q, want ""`, got)
	}
	if got := LandmarkKind(99).String(); got != "" {
		t.Fatalf(`LandmarkKind(99).String() = %q, want ""`, got)
	}
}

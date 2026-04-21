package ui

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

func TestRegionAccentForProfileTruecolor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   pb.RegionCharacter
		want lipgloss.Color
	}{
		{pb.RegionCharacter_REGION_CHARACTER_BLIGHTED, "#3d3d2e"},
		{pb.RegionCharacter_REGION_CHARACTER_FEY, "#7c3aff"},
		{pb.RegionCharacter_REGION_CHARACTER_ANCIENT, "#a8723a"},
		{pb.RegionCharacter_REGION_CHARACTER_SAVAGE, "#8b2a1d"},
		{pb.RegionCharacter_REGION_CHARACTER_HOLY, "#f2c74f"},
		{pb.RegionCharacter_REGION_CHARACTER_WILD, "#1e5a2e"},
	}
	for _, tc := range cases {
		got := regionAccentForProfile(tc.in, termenv.TrueColor)
		if got != tc.want {
			t.Errorf("truecolor %v: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRegionAccentForProfileANSI256(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   pb.RegionCharacter
		want lipgloss.Color
	}{
		{pb.RegionCharacter_REGION_CHARACTER_BLIGHTED, "242"},
		{pb.RegionCharacter_REGION_CHARACTER_FEY, "93"},
		{pb.RegionCharacter_REGION_CHARACTER_ANCIENT, "130"},
		{pb.RegionCharacter_REGION_CHARACTER_SAVAGE, "124"},
		{pb.RegionCharacter_REGION_CHARACTER_HOLY, "220"},
		{pb.RegionCharacter_REGION_CHARACTER_WILD, "22"},
	}
	for _, tc := range cases {
		got := regionAccentForProfile(tc.in, termenv.ANSI256)
		if got != tc.want {
			t.Errorf("ansi256 %v: got %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRegionAccentForProfileDisabledOnLowColor(t *testing.T) {
	t.Parallel()

	// Ascii and 16-colour ANSI both disable tint: blending into an 8-bit
	// ANSI index would collapse the per-character distinction anyway.
	profiles := []termenv.Profile{termenv.Ascii, termenv.ANSI}
	chars := []pb.RegionCharacter{
		pb.RegionCharacter_REGION_CHARACTER_BLIGHTED,
		pb.RegionCharacter_REGION_CHARACTER_HOLY,
		pb.RegionCharacter_REGION_CHARACTER_WILD,
	}
	for _, p := range profiles {
		for _, c := range chars {
			if got := regionAccentForProfile(c, p); got != "" {
				t.Errorf("profile=%v char=%v got %q, want empty", p, c, got)
			}
		}
	}
}

func TestRegionAccentForProfileNormalAlwaysEmpty(t *testing.T) {
	t.Parallel()
	profiles := []termenv.Profile{
		termenv.Ascii, termenv.ANSI, termenv.ANSI256, termenv.TrueColor,
	}
	for _, p := range profiles {
		got := regionAccentForProfile(
			pb.RegionCharacter_REGION_CHARACTER_NORMAL, p)
		if got != "" {
			t.Errorf("normal profile=%v got %q, want empty", p, got)
		}
	}
}

func TestRegionHeaderStyleFallsBackOnUnmapped(t *testing.T) {
	t.Parallel()

	// A future enum value that no palette knows about must still render in
	// the default status colour rather than crashing or producing an empty
	// style. We can't force a specific terminal profile here, so we just
	// assert the function returns without panicking and produces a style
	// that renders non-empty text.
	out := regionHeaderStyle(pb.RegionCharacter(99))
	if got := out.Render("probe"); got == "" {
		t.Fatalf("unmapped character rendered empty string")
	}
}

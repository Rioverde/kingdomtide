package ui

import (
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// regionAccentsTruecolor holds the per-character tint palette used on
// terminals that report COLORTERM=truecolor (or equivalent). The hex values
// lean muted: tinted terrain should read as "this biome, only bluer" rather
// than a neon overlay that drowns out the base rune.
var regionAccentsTruecolor = map[pb.RegionCharacter]lipgloss.Color{
	pb.RegionCharacter_REGION_CHARACTER_BLIGHTED: "#3d3d2e",
	pb.RegionCharacter_REGION_CHARACTER_FEY:      "#7c3aff",
	pb.RegionCharacter_REGION_CHARACTER_ANCIENT:  "#a8723a",
	pb.RegionCharacter_REGION_CHARACTER_SAVAGE:   "#8b2a1d",
	pb.RegionCharacter_REGION_CHARACTER_HOLY:     "#f2c74f",
	pb.RegionCharacter_REGION_CHARACTER_WILD:     "#1e5a2e",
}

// regionAccentsANSI256 is the 8-bit palette fallback. Indices picked to
// survive quantization without colliding with neighbouring characters
// (Blighted 242 grey-green, Fey 93 purple, Holy 220 gold, etc.).
var regionAccentsANSI256 = map[pb.RegionCharacter]lipgloss.Color{
	pb.RegionCharacter_REGION_CHARACTER_BLIGHTED: "242",
	pb.RegionCharacter_REGION_CHARACTER_FEY:      "93",
	pb.RegionCharacter_REGION_CHARACTER_ANCIENT:  "130",
	pb.RegionCharacter_REGION_CHARACTER_SAVAGE:   "124",
	pb.RegionCharacter_REGION_CHARACTER_HOLY:     "220",
	pb.RegionCharacter_REGION_CHARACTER_WILD:     "22",
}

// colorProfile is cached at first use. termenv.ColorProfile() queries the
// TTY, which is cheap but non-trivial and can hit the filesystem on some
// platforms; resolving it once keeps renderCell allocation-free.
var (
	colorProfileOnce sync.Once
	colorProfile     termenv.Profile
)

// detectColorProfile returns the cached terminal color profile. Tests that
// need to exercise the palette selector without a real TTY should use
// regionAccentForProfile directly.
func detectColorProfile() termenv.Profile {
	colorProfileOnce.Do(func() {
		colorProfile = termenv.ColorProfile()
	})
	return colorProfile
}

// regionAccent returns the tint accent for the given character on the local
// terminal. Grayscale-only terminals (Ascii, ANSI) return the empty color so
// the tint pipeline short-circuits — blending a foreground into near-black
// basic ANSI indices muddies the map without adding any real colour cue.
func regionAccent(c pb.RegionCharacter) lipgloss.Color {
	return regionAccentForProfile(c, detectColorProfile())
}

// regionAccentForProfile is the pure lookup behind regionAccent. Exposed so
// unit tests can assert each branch of the palette selector without touching
// termenv global state.
func regionAccentForProfile(c pb.RegionCharacter, profile termenv.Profile) lipgloss.Color {
	switch profile {
	case termenv.TrueColor:
		if col, ok := regionAccentsTruecolor[c]; ok {
			return col
		}
	case termenv.ANSI256:
		if col, ok := regionAccentsANSI256[c]; ok {
			return col
		}
	}
	return ""
}

// regionHeaderStyle returns the status-bar style for the region header line
// corresponding to character c. Non-tinted characters (Normal plus any
// unmapped value) render in the default status colour so the header reads as
// unchanged when the player stands in a plain region.
func regionHeaderStyle(c pb.RegionCharacter) lipgloss.Style {
	accent := regionAccent(c)
	if accent == "" {
		return styles.status
	}
	return styles.status.Foreground(accent)
}

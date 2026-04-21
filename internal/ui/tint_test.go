package ui

import (
	"math"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// hexEqual compares two #RRGGBB strings by reinterpreting them through
// colorful to tolerate casing and 3-vs-6 character forms. Using strings
// directly would fail on equivalent-but-different representations of the
// same colour ("#abc" vs "#aabbcc").
func hexEqual(t *testing.T, got, want string) {
	t.Helper()
	gc, err := colorful.Hex(got)
	if err != nil {
		t.Fatalf("parse got hex %q: %v", got, err)
	}
	wc, err := colorful.Hex(want)
	if err != nil {
		t.Fatalf("parse want hex %q: %v", want, err)
	}
	// Allow a tiny tolerance because BlendLab round-trips through Lab and
	// back to sRGB, which introduces sub-quantum drift.
	const eps = 1.0 / 512
	if math.Abs(gc.R-wc.R) > eps || math.Abs(gc.G-wc.G) > eps || math.Abs(gc.B-wc.B) > eps {
		t.Fatalf("colour mismatch: got %q (%+v), want %q (%+v)", got, gc, want, wc)
	}
}

func TestBlendColorsEndpoints(t *testing.T) {
	t.Parallel()
	base := lipgloss.Color("#204080")
	accent := lipgloss.Color("#ff8040")

	if got := blendColors(base, accent, 0); got != base {
		t.Fatalf("strength=0 got %q, want base %q", got, base)
	}
	if got := blendColors(base, accent, 1); got == "" {
		t.Fatalf("strength=1 produced empty output")
	} else {
		hexEqual(t, string(got), string(accent))
	}
}

func TestBlendColorsClampsStrength(t *testing.T) {
	t.Parallel()
	base := lipgloss.Color("#102030")
	accent := lipgloss.Color("#f0e0d0")

	low := blendColors(base, accent, -5)
	if low != base {
		t.Fatalf("negative strength got %q, want base %q", low, base)
	}
	high := blendColors(base, accent, 5)
	hexEqual(t, string(high), string(accent))
}

func TestBlendColorsMidpointMatchesLabFormula(t *testing.T) {
	t.Parallel()
	base := lipgloss.Color("#1e5a2e")
	accent := lipgloss.Color("#7c3aff")

	got := blendColors(base, accent, 0.5)

	baseCol, err := colorful.Hex(string(base))
	if err != nil {
		t.Fatalf("parse base: %v", err)
	}
	accentCol, err := colorful.Hex(string(accent))
	if err != nil {
		t.Fatalf("parse accent: %v", err)
	}
	want := baseCol.BlendLab(accentCol, 0.5).Clamped().Hex()
	hexEqual(t, string(got), want)
}

func TestBlendColorsGracefulFallback(t *testing.T) {
	t.Parallel()

	// ANSI-256 index strings are not hex — blendColors should refuse and
	// return the base colour unchanged so the tint pipeline is a no-op on
	// palettes that can't be blended.
	base := lipgloss.Color("93")
	accent := lipgloss.Color("220")
	if got := blendColors(base, accent, 0.5); got != base {
		t.Fatalf("ansi256 blend got %q, want base %q", got, base)
	}

	if got := blendColors("", "#ff00ff", 0.5); got != "" {
		t.Fatalf("empty base: got %q, want empty", got)
	}
	if got := blendColors("#ff00ff", "", 0.5); got != lipgloss.Color("#ff00ff") {
		t.Fatalf("empty accent: got %q, want base", got)
	}
}

func TestTintedStyleZeroStrengthPassesThrough(t *testing.T) {
	t.Parallel()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#204080"))
	out := tintedStyle(base, lipgloss.Color("#ff0000"), 0)
	if out.GetForeground() != base.GetForeground() {
		t.Fatalf("zero strength mutated style")
	}
}

func TestTintedStyleEmptyAccentPassesThrough(t *testing.T) {
	t.Parallel()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#204080"))
	out := tintedStyle(base, "", 0.5)
	if out.GetForeground() != base.GetForeground() {
		t.Fatalf("empty accent mutated style")
	}
}

func TestTintedStyleBlendsForeground(t *testing.T) {
	t.Parallel()
	base := lipgloss.NewStyle().Foreground(lipgloss.Color("#204080"))
	out := tintedStyle(base, lipgloss.Color("#ff8040"), 0.5)
	fg, ok := out.GetForeground().(lipgloss.Color)
	if !ok {
		t.Fatalf("foreground not lipgloss.Color: %T", out.GetForeground())
	}
	if fg == lipgloss.Color("#204080") {
		t.Fatalf("foreground unchanged; expected blended result")
	}
}

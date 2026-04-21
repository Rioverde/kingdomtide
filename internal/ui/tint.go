package ui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// blendColors interpolates between a base and an accent lipgloss color in CIE
// Lab space and returns the resulting lipgloss.Color as a truecolor hex. Lab
// blending keeps midpoints perceptually smooth — a linear RGB blend of green
// and magenta passes through a muddy grey, whereas Lab stays vivid.
//
// strength is clamped to [0, 1]. 0 returns base unchanged; 1 returns accent.
// Empty or non-hex colors (adaptive colors, ANSI palette entries, empty
// string) are left alone — the base style is returned as-is by
// tintedStyle, and blendColors itself returns base in that case so the
// tint pipeline degrades gracefully on terminals without truecolor.
func blendColors(base, accent lipgloss.Color, strength float64) lipgloss.Color {
	if base == "" || accent == "" {
		return base
	}
	a, aOK := parseHexColor(string(base))
	b, bOK := parseHexColor(string(accent))
	if !aOK || !bOK {
		return base
	}
	t := clamp01(strength)
	mixed := a.BlendLab(b, t).Clamped()
	return lipgloss.Color(mixed.Hex())
}

// tintedStyle returns a copy of base with its foreground blended toward
// accent by the given strength. The caller's base style is left unchanged so
// this is safe to call on package-level shared styles.
//
// strength <= 0 or an empty accent yields the base style unchanged, so the
// ANSI-fallback path (which hands back an empty accent from region_accent.go)
// simply skips the tint step.
func tintedStyle(base lipgloss.Style, accent lipgloss.Color, strength float64) lipgloss.Style {
	if accent == "" || strength <= 0 {
		return base
	}
	fg := base.GetForeground()
	fgColor, ok := fg.(lipgloss.Color)
	if !ok {
		return base
	}
	blended := blendColors(fgColor, accent, strength)
	if blended == fgColor {
		return base
	}
	return base.Foreground(blended)
}

// parseHexColor decodes a #RRGGBB / #RGB string into a colorful.Color. It
// returns ok=false for empty strings, ANSI-256 indices ("220"), and adaptive
// strings lipgloss emits for theme-aware colors. The two-character case is
// not a valid hex color either; anything that doesn't start with '#' is
// rejected up-front so the parser stays boring.
func parseHexColor(s string) (colorful.Color, bool) {
	if s == "" || s[0] != '#' {
		return colorful.Color{}, false
	}
	c, err := colorful.Hex(s)
	if err != nil {
		return colorful.Color{}, false
	}
	return c, true
}

// clamp01 bounds x into [0, 1]. Using min/max from the Go 1.21 builtins keeps
// the blend math branch-free.
func clamp01(x float64) float64 { return max(0.0, min(1.0, x)) }

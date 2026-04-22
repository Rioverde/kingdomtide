package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// Layout constants for the character-creation screen. Named so future
// tweaks do not require chasing magic numbers through the render helper.
const (
	// statRowInnerWidth is the content width (excluding box chrome and
	// cursor marker) of one stat row. Wide enough to hold the label,
	// the bracketed score, and the localized cost column with a small
	// gap between them on a 80-column terminal.
	statRowInnerWidth = 32

	// statCursorActive is drawn to the left of the selected stat row.
	statCursorActive = "\u25b6"

	// statCursorInactive occupies the same rune width as statCursorActive
	// so unselected rows align vertically with the selected one.
	statCursorInactive = " "
)

// creationStatStyle colours the selected stat row yellow and leaves every
// other row at the default terminal foreground so the active row is
// immediately obvious. The style is recomputed per render rather than
// pre-cached to stay in sync with any future lipgloss theme tweaks.
var (
	creationStatSelectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("11")).
					Bold(true)
	creationStatErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9"))
	creationStatHeaderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)
)

// creationStatKey is the locale key for each ability row. Index must match
// the stat constants in state.go so statLabels()[statIdxStrength] returns
// the STR key — keeps the row/label wiring strictly positional.
var creationStatKeys = [statsCount]string{
	locale.KeyCreationStatStrength,
	locale.KeyCreationStatDexterity,
	locale.KeyCreationStatConstitution,
	locale.KeyCreationStatIntelligence,
	locale.KeyCreationStatWisdom,
	locale.KeyCreationStatCharisma,
}

// viewCharacterCreation renders the Point Buy distributor for
// phaseCharacterCreation. The box mirrors the enter-name screen's chrome
// (centred, rounded border, styled header) so the two screens read as
// siblings.
//
// Layout:
//
//	┌──────────────────────────────────────────┐
//	│          Character Creation              │
//	│                                          │
//	│   Remaining points: 27 / 27              │
//	│                                          │
//	│   ▶ STR   [ 8]   cost 0                  │
//	│     DEX   [ 8]   cost 0                  │
//	│     ...                                  │
//	│                                          │
//	│   ←/→ to adjust · ↑/↓ to select          │
//	│   Enter: confirm · Esc: back             │
//	│                                          │
//	│   <localized error, when present>        │
//	└──────────────────────────────────────────┘
func (m *Model) viewCharacterCreation() string {
	header := creationStatHeaderStyle.Render(
		locale.Tr(m.lang, locale.KeyCreationHeader))
	remaining := locale.Tr(m.lang, locale.KeyCreationRemaining,
		locale.ArgRemaining, m.pointBuyRemaining())

	rows := make([]string, 0, statsCount)
	for i := range m.stats {
		rows = append(rows, m.renderStatRow(i))
	}

	hintAdjust := styles.status.Render(
		locale.Tr(m.lang, locale.KeyCreationHintAdjust))
	hintConfirm := styles.status.Render(
		locale.Tr(m.lang, locale.KeyCreationHintConfirm))

	parts := []string{
		header,
		"",
		styles.prompt.Render(remaining),
		"",
	}
	parts = append(parts, rows...)
	parts = append(parts,
		"",
		hintAdjust,
		hintConfirm,
	)
	if m.statsError != "" {
		parts = append(parts, "", creationStatErrorStyle.Render(m.statsError))
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return m.centeredBox(styles.box, inner)
}

// renderStatRow returns one line of the stat table. The selected row gets
// a filled cursor marker and yellow-bold styling; other rows are rendered
// at the default terminal foreground with a blank marker so vertical
// alignment holds.
func (m *Model) renderStatRow(i int) string {
	cursor := statCursorInactive
	style := lipgloss.NewStyle()
	if i == m.selectedStat {
		cursor = statCursorActive
		style = creationStatSelectedStyle
	}
	label := locale.Tr(m.lang, creationStatKeys[i])
	value := m.stats[i]
	cost := locale.Tr(m.lang, locale.KeyCreationCost,
		locale.ArgCost, game.PointBuyCost(value))

	// Fixed column layout: "<label> [<val>] <cost>". strconv would be
	// marginally faster than fmt.Sprintf here but this path is not hot
	// (one render per keypress) so readability wins.
	body := fmt.Sprintf("%-4s [%2d]  %s", label, value, cost)
	padded := padRight(body, statRowInnerWidth)
	return cursor + " " + style.Render(padded)
}

// padRight returns s padded with trailing spaces so its display width
// (in terminal cells, per lipgloss.Width) is at least width. Preserves
// any embedded ANSI escapes because we append outside the styled span.
func padRight(s string, width int) string {
	gap := width - lipgloss.Width(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

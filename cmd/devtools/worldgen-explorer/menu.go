package main

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// Menu styling.
var (
	menuBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("12")).
			Padding(1, 3)
	menuTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)
	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11")).
				Bold(true)
	menuDimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("243"))
	menuStarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11"))
	menuErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("9"))
	menuActiveHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)
	menuIdleHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("243"))
)

// updateMenu routes the key message to the active field's handler.
func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch key.String() {
	case "q", "esc", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.activeField = m.activeField.next()
		m.syncSeedFocus()
		return m, nil
	case "shift+tab":
		m.activeField = m.activeField.prev()
		m.syncSeedFocus()
		return m, nil
	case "enter":
		seed, err := parseSeed(m.seedInput.Value())
		if err != nil {
			m.menuErr = err.Error()
			return m, nil
		}
		m.menuErr = ""
		m.pendingSize = m.sizes[m.sizeIdx]
		m.pendingContinents = m.continents[m.continentIdx]
		m.pendingSeed = seed
		m.phase = phaseBuilding
		return m, buildCmd(m.pendingSize, m.pendingContinents, m.pendingSeed)
	case "r":
		if m.activeField != fieldSeed {
			m.seedInput.SetValue(formatSeed(int64(rand.Uint64())))
			return m, nil
		}
	}

	switch m.activeField {
	case fieldSize:
		return m.updateSizeField(key)
	case fieldContinent:
		return m.updateContinentField(key)
	case fieldSeed:
		var cmd tea.Cmd
		m.seedInput, cmd = m.seedInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

// updateSizeField handles arrow-key navigation while size is active.
func (m Model) updateSizeField(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.sizeIdx > 0 {
			m.sizeIdx--
		}
	case "down", "j":
		if m.sizeIdx < len(m.sizes)-1 {
			m.sizeIdx++
		}
	}
	return m, nil
}

// updateContinentField handles arrow-key navigation while continent is active.
func (m Model) updateContinentField(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "up", "k":
		if m.continentIdx > 0 {
			m.continentIdx--
		}
	case "down", "j":
		if m.continentIdx < len(m.continents)-1 {
			m.continentIdx++
		}
	}
	return m, nil
}

// syncSeedFocus mirrors activeField onto the textinput's focus state so
// the blinking cursor is only visible when the seed field is active.
func (m *Model) syncSeedFocus() {
	if m.activeField == fieldSeed {
		m.seedInput.Focus()
	} else {
		m.seedInput.Blur()
	}
}

// viewMenu renders the full menu: title, size list, continent list,
// seed input, hints, and inline error line. Each section's heading is
// highlighted when its field is active so the dev always knows where
// input will land.
func (m Model) viewMenu() string {
	var b strings.Builder

	b.WriteString(menuTitleStyle.Render("Gongeons Worldgen Explorer"))
	b.WriteString("\n\n")

	b.WriteString(headingFor("World size", m.activeField == fieldSize))
	b.WriteString("\n")
	for i, size := range m.sizes {
		b.WriteString(renderSizeRow(size, i == m.sizeIdx, m.activeField == fieldSize))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(headingFor("Continents", m.activeField == fieldContinent))
	b.WriteString("\n")
	for i, cp := range m.continents {
		b.WriteString(renderContinentRow(cp, i == m.continentIdx, m.activeField == fieldContinent))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	b.WriteString(headingFor("Seed", m.activeField == fieldSeed))
	b.WriteString("  ")
	b.WriteString(m.seedInput.View())
	b.WriteString("\n\n")

	hints := []string{
		"tab: next field",
		"up/down: select",
		"r: random seed",
		"enter: generate",
		"q: quit",
	}
	b.WriteString(menuDimStyle.Render(strings.Join(hints, " · ")))

	if m.menuErr != "" {
		b.WriteString("\n\n")
		b.WriteString(menuErrorStyle.Render(m.menuErr))
	}

	box := menuBoxStyle.Render(b.String())
	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, box)
}

// headingFor renders a section heading in active or idle colour.
func headingFor(label string, active bool) string {
	if active {
		return menuActiveHeadingStyle.Render(label + ":")
	}
	return menuIdleHeadingStyle.Render(label + ":")
}

// renderSizeRow formats one row of the size list.
func renderSizeRow(size worldgen.WorldSize, selected, active bool) string {
	w, h := size.Dimensions()
	dims := fmt.Sprintf("%dx%d", w, h)
	timeStr := fmt.Sprintf("~%ds", size.EstimatedGenSeconds())
	kings := fmt.Sprintf("%d kingdoms", size.ExpectedKingdoms())
	star := "  "
	if size == worldgen.WorldSizeStandard {
		star = menuStarStyle.Render(" *")
	}

	row := fmt.Sprintf("  %-11s %-10s %6s   %-13s%s",
		size.Label(), dims, timeStr, kings, star)
	return styleMenuRow(row, selected, active)
}

// renderContinentRow formats one row of the continent preset list.
func renderContinentRow(cp worldgen.ContinentPreset, selected, active bool) string {
	star := "  "
	if cp == worldgen.ContinentTrinity {
		star = menuStarStyle.Render(" *")
	}
	row := fmt.Sprintf("  %-13s %-28s%s", cp.Label(), cp.Description(), star)
	return styleMenuRow(row, selected, active)
}

// styleMenuRow applies the selected/active cursor + colouring to one
// row. Active and selected highlights the row yellow and adds a caret;
// selected but not active greys the row with a dim caret; neither just
// greys.
func styleMenuRow(row string, selected, active bool) string {
	if selected && active {
		return menuSelectedStyle.Render("▶") + row[1:]
	}
	if selected {
		return menuDimStyle.Render("▷") + row[1:]
	}
	return menuDimStyle.Render(row)
}

// viewBuilding renders the short generating-progress screen.
func (m Model) viewBuilding() string {
	line1 := menuTitleStyle.Render("Generating world...")
	w, h := m.pendingSize.Dimensions()
	line2 := fmt.Sprintf("Size: %s (%dx%d) · Continents: %s · Seed: %s",
		m.pendingSize.Label(), w, h,
		m.pendingContinents.Label(),
		formatSeed(m.pendingSeed))
	line3 := menuDimStyle.Render(fmt.Sprintf(
		"Estimated: ~%d seconds on M1 Max", m.pendingSize.EstimatedGenSeconds()))

	body := line1 + "\n\n" + line2 + "\n" + line3
	box := menuBoxStyle.Render(body)
	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, box)
}

// formatSeed renders a seed int64 as a plain decimal string.
func formatSeed(seed int64) string { return strconv.FormatInt(seed, 10) }

// parseSeed accepts either a decimal int64 or a hex value prefixed with 0x.
func parseSeed(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("seed is empty")
	}
	if strings.HasPrefix(strings.ToLower(s), "0x") {
		u, err := strconv.ParseUint(s[2:], 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hex seed: %w", err)
		}
		return int64(u), nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid seed: %w", err)
	}
	return v, nil
}

var _ = textinput.Blink // ensure import stays live; Blink is referenced via tea.Cmd indirectly

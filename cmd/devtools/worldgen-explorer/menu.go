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

// Menu styling. Kept in one block so future palette tweaks flow from
// one place rather than chasing constants through the handler.
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
	menuEditingStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)
)

// updateMenu is the phaseMenu key handler. It routes keys to either the
// size list or the seed textinput depending on whether the dev is
// currently editing the seed field.
func (m Model) updateMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.editingSeed {
		switch key.String() {
		case "enter", "esc":
			m.editingSeed = false
			m.seedInput.Blur()
			return m, nil
		}
		var cmd tea.Cmd
		m.seedInput, cmd = m.seedInput.Update(msg)
		return m, cmd
	}

	switch key.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if m.sizeIdx > 0 {
			m.sizeIdx--
		}
	case "down", "j":
		if m.sizeIdx < len(m.sizes)-1 {
			m.sizeIdx++
		}
	case "r":
		m.seedInput.SetValue(formatSeed(int64(rand.Uint64())))
	case "tab":
		m.editingSeed = true
		m.seedInput.Focus()
		return m, textinput.Blink
	case "enter":
		seed, err := parseSeed(m.seedInput.Value())
		if err != nil {
			m.menuErr = err.Error()
			return m, nil
		}
		m.menuErr = ""
		m.pendingSize = m.sizes[m.sizeIdx]
		m.pendingSeed = seed
		m.phase = phaseBuilding
		return m, buildCmd(m.pendingSize, m.pendingSeed)
	}
	return m, nil
}

// viewMenu renders the size picker + seed input inside a rounded border
// centred on the terminal. The layout mirrors character_creation.go from
// the main client so the visual language stays consistent across tools.
func (m Model) viewMenu() string {
	var b strings.Builder

	b.WriteString(menuTitleStyle.Render("Gongeons Worldgen Explorer"))
	b.WriteString("\n\n")
	b.WriteString("Select world size:")
	b.WriteString("\n\n")

	for i, size := range m.sizes {
		marker := "  "
		label := size.Label()
		w, h := size.Dimensions()
		dims := fmt.Sprintf("%dx%d", w, h)
		time := fmt.Sprintf("~%ds", size.EstimatedGenSeconds())
		kings := fmt.Sprintf("%d kingdoms", size.ExpectedKingdoms())
		star := "  "
		if size == worldgen.WorldSizeStandard {
			star = menuStarStyle.Render(" *")
		}

		row := fmt.Sprintf("%-10s %-10s %6s   %-13s%s", label, dims, time, kings, star)

		if i == m.sizeIdx {
			marker = menuSelectedStyle.Render("▶ ")
			row = menuSelectedStyle.Render(row)
		} else {
			row = menuDimStyle.Render(row)
		}
		b.WriteString(marker)
		b.WriteString(row)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	seedLabel := "Seed: "
	if m.editingSeed {
		seedLabel = menuEditingStyle.Render("Seed: ")
	}
	b.WriteString(seedLabel)
	b.WriteString(m.seedInput.View())
	b.WriteString("\n\n")

	hints := []string{
		"up/down: size",
		"r: random seed",
		"tab: edit seed",
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

// viewBuilding renders the short generating-progress screen. tea fires
// buildDoneMsg when the goroutine finishes, so this view is visible
// only for the duration of one world generation.
func (m Model) viewBuilding() string {
	line1 := menuTitleStyle.Render("Generating world...")
	w, h := m.pendingSize.Dimensions()
	line2 := fmt.Sprintf("Size: %s (%dx%d) · Seed: %s",
		m.pendingSize.Label(), w, h, formatSeed(m.pendingSeed))
	line3 := menuDimStyle.Render(fmt.Sprintf(
		"Estimated: ~%d seconds on M1 Max", m.pendingSize.EstimatedGenSeconds()))

	body := line1 + "\n\n" + line2 + "\n" + line3
	box := menuBoxStyle.Render(body)
	return lipgloss.Place(m.termW, m.termH, lipgloss.Center, lipgloss.Center, box)
}

// formatSeed renders a seed int64 as a plain decimal string. Used both
// for placeholder generation in the textinput and for display in the
// building screen.
func formatSeed(seed int64) string { return strconv.FormatInt(seed, 10) }

// parseSeed accepts either a decimal int64 or a hex value prefixed with
// 0x. Any parse failure surfaces to the menu as an inline error — the
// textinput stays focused so the dev can fix it without losing state.
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

// textinput is imported lazily here to keep main.go focused on program
// boot. Bubbletea's textinput.Blink Cmd is returned from the tab handler
// so the cursor starts blinking as soon as the seed field takes focus.
var _ = textinput.Blink // keep import live; blink is referenced above

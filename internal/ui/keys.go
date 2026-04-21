package ui

import tea "github.com/charmbracelet/bubbletea"

// moveFor maps a KeyMsg pressed in phasePlaying to a (dx, dy) direction.
// The second return reports whether the key was a movement key at all.
//
// WASD is the documented scheme; hjkl and the arrow keys are aliased in
// because the cost is zero and "w to go up but arrows don't work" is a
// frustration readers should never feel.
func moveFor(msg tea.KeyMsg) (dx, dy int, ok bool) {
	switch msg.Type {
	case tea.KeyUp:
		return 0, -1, true
	case tea.KeyDown:
		return 0, 1, true
	case tea.KeyLeft:
		return -1, 0, true
	case tea.KeyRight:
		return 1, 0, true
	}
	if msg.Type != tea.KeyRunes {
		return 0, 0, false
	}
	if len(msg.Runes) != 1 {
		return 0, 0, false
	}
	switch msg.Runes[0] {
	case 'w', 'W', 'k', 'K':
		return 0, -1, true
	case 's', 'S', 'j', 'J':
		return 0, 1, true
	case 'a', 'A', 'h', 'H':
		return -1, 0, true
	case 'd', 'D', 'l', 'L':
		return 1, 0, true
	}
	return 0, 0, false
}

// isQuit reports whether a key press should terminate the program.
func isQuit(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 {
		r := msg.Runes[0]
		return r == 'q' || r == 'Q'
	}
	return false
}

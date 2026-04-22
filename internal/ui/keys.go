package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap groups every key binding the client listens for. The layout is
// flat on purpose — the bubbles help.Model consumes ShortHelp / FullHelp
// directly and renders the bottom-bar hint without any extra glue.
type KeyMap struct {
	Up, Down, Left, Right key.Binding
	Quit                  key.Binding

	// Character-creation screen (phaseCharacterCreation).
	// StatPrev / StatNext move the cursor between ability rows, with wrap.
	// StatIncrease / StatDecrease adjust the selected ability score subject
	// to range and budget guards. Confirm validates the distribution and
	// advances to phaseConnecting; Back returns to phaseEnterName.
	StatPrev     key.Binding
	StatNext     key.Binding
	StatIncrease key.Binding
	StatDecrease key.Binding
	Confirm      key.Binding
	Back         key.Binding
}

// ShortHelp returns the single-line help rendered in the status bar.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Left, k.Right, k.Quit}
}

// FullHelp returns the expanded two-column help (for the ? toggle, if added).
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right},
		{k.Quit},
	}
}

// Keys is the single source of truth for every binding the client reacts to.
// WASD is the documented scheme; arrow keys are aliased in because the cost
// is zero and "w to go up but arrows don't work" is a frustration readers
// should never feel.
var Keys = KeyMap{
	Up:    key.NewBinding(key.WithKeys("w", "up"), key.WithHelp("w/↑", "north")),
	Down:  key.NewBinding(key.WithKeys("s", "down"), key.WithHelp("s/↓", "south")),
	Left:  key.NewBinding(key.WithKeys("a", "left"), key.WithHelp("a/←", "west")),
	Right: key.NewBinding(key.WithKeys("d", "right"), key.WithHelp("d/→", "east")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q/ctrl+c", "quit")),

	StatPrev:     key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "prev stat")),
	StatNext:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next stat")),
	StatIncrease: key.NewBinding(key.WithKeys("right", "=", "+", "l"), key.WithHelp("→/+", "increase")),
	StatDecrease: key.NewBinding(key.WithKeys("left", "-", "h"), key.WithHelp("←/-", "decrease")),
	Confirm:      key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "confirm")),
	Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
}

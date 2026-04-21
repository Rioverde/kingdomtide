package ui

import "github.com/charmbracelet/bubbles/key"

// KeyMap groups every key binding the client listens for. The layout is
// flat on purpose — the bubbles help.Model consumes ShortHelp / FullHelp
// directly and renders the bottom-bar hint without any extra glue.
type KeyMap struct {
	Up, Down, Left, Right key.Binding
	Quit                  key.Binding
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
// WASD is the documented scheme; hjkl and the arrow keys are aliased in
// because the cost is zero and "w to go up but arrows don't work" is a
// frustration readers should never feel.
var Keys = KeyMap{
	Up:    key.NewBinding(key.WithKeys("w", "k", "up"), key.WithHelp("w/k/↑", "north")),
	Down:  key.NewBinding(key.WithKeys("s", "j", "down"), key.WithHelp("s/j/↓", "south")),
	Left:  key.NewBinding(key.WithKeys("a", "h", "left"), key.WithHelp("a/h/←", "west")),
	Right: key.NewBinding(key.WithKeys("d", "l", "right"), key.WithHelp("d/l/→", "east")),
	Quit:  key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q/esc", "quit")),
}

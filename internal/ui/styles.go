package ui

import "github.com/charmbracelet/lipgloss"

// Styles are kept in a single package-level struct so every renderer
// pulls colour from the same source. Colours are chosen as ANSI indices
// to stay readable on both dark and light terminals — no raw hex.
var styles = struct {
	floor       lipgloss.Style
	wall        lipgloss.Style
	water       lipgloss.Style
	unspecified lipgloss.Style

	selfPlayer  lipgloss.Style
	otherPlayer lipgloss.Style

	title   lipgloss.Style
	box     lipgloss.Style
	status  lipgloss.Style
	prompt  lipgloss.Style
	input   lipgloss.Style
	cursor  lipgloss.Style
	log     lipgloss.Style
	playerL lipgloss.Style
	errBox  lipgloss.Style
}{
	floor:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),   // bright black / grey
	wall:        lipgloss.NewStyle().Foreground(lipgloss.Color("3")),   // yellow (readable on dark and light)
	water:       lipgloss.NewStyle().Foreground(lipgloss.Color("4")),   // blue
	unspecified: lipgloss.NewStyle().Foreground(lipgloss.Color("240")), // dim grey

	selfPlayer:  lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true), // bright green
	otherPlayer: lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true), // bright magenta

	title: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Padding(0, 1),
	box: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(1, 2),
	status: lipgloss.NewStyle().
		Foreground(lipgloss.Color("11")),
	prompt: lipgloss.NewStyle().
		Foreground(lipgloss.Color("6")),
	input: lipgloss.NewStyle().
		Foreground(lipgloss.Color("15")),
	cursor: lipgloss.NewStyle().
		Reverse(true),
	log: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1),
	playerL: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 1),
	errBox: lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("9")).
		Foreground(lipgloss.Color("9")).
		Padding(1, 2),
}

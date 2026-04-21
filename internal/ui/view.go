package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// Every glyph this file renders comes from runes.go — runeSelf / runeOther /
// runeUnspecified / riverRune / terrainRunes for the map, plus the
// chrome constants (InputPrompt, LogBullet, etc.) for the surrounding UI.

// View renders the full screen for the current phase.
func (m *Model) View() string {
	switch m.phase {
	case phaseEnterName:
		return m.viewEnterName()
	case phaseConnecting:
		return m.viewConnecting()
	case phasePlaying:
		return m.viewPlaying()
	case phaseDisconnected:
		return m.viewDisconnected()
	}
	return ""
}

// centeredBox renders inner with the given style, then centres the result
// in the terminal. Falls back to the uncentred box before the first
// WindowSizeMsg so lipgloss.Place is never asked to fit into a 0×0 canvas.
func (m *Model) centeredBox(style lipgloss.Style, inner string) string {
	box := style.Render(inner)
	if m.termWidth <= 0 || m.termHeight <= 0 {
		return box
	}
	return lipgloss.Place(
		m.termWidth, m.termHeight,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

// viewEnterName draws the sword banner, game title, tagline, name input and
// quit hint — all stacked and centred inside a bordered box, which is itself
// centred on the terminal.
func (m *Model) viewEnterName() string {
	inner := lipgloss.JoinVertical(lipgloss.Center,
		styles.title.Render(TitleArt),
		"",
		styles.title.Render(TitleText),
		styles.prompt.Render(Tagline),
		"",
		styles.prompt.Render(NameInputLabel),
		renderInput(m),
		"",
		styles.status.Render(QuitLongHint),
	)
	return m.centeredBox(styles.box, inner)
}

// viewConnecting shows a centred "connecting" notice with an animated
// spinner so the wait never feels frozen.
func (m *Model) viewConnecting() string {
	inner := lipgloss.JoinVertical(lipgloss.Left,
		styles.status.Render(m.spinner.View()+"Connecting to "+m.serverAddr),
		"",
		styles.status.Render(QuitHint),
	)
	return m.centeredBox(styles.box, inner)
}

// viewDisconnected shows the error in a red box plus a quit hint,
// centred in the terminal area.
func (m *Model) viewDisconnected() string {
	msg := "disconnected"
	if m.err != nil {
		msg = "disconnected: " + m.err.Error()
	}
	inner := lipgloss.JoinVertical(lipgloss.Left,
		msg,
		"",
		DisconnectHint,
	)
	return m.centeredBox(styles.errBox, inner)
}

// viewPlaying composes the grid, the player list and the event log.
func (m *Model) viewPlaying() string {
	grid := m.renderGrid()
	playerList := m.renderPlayerList()
	log := m.renderLog()
	rightCol := lipgloss.JoinVertical(lipgloss.Left, playerList, log)
	main := lipgloss.JoinHorizontal(lipgloss.Top, grid, " ", rightCol)
	status := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Left, main, status)
}

// renderGrid turns the tile slice into a styled multi-line string.
func (m *Model) renderGrid() string {
	if m.width <= 0 || m.height <= 0 || len(m.tiles) == 0 {
		return styles.box.Render(EmptyMapLabel)
	}
	var b strings.Builder
	b.Grow(m.width * m.height * 4)
	for y := range m.height {
		for x := range m.width {
			idx := y*m.width + x
			if idx >= len(m.tiles) {
				b.WriteString(styles.unknownTile.Render(runeUnspecified))
				continue
			}
			b.WriteString(m.renderCell(m.tiles[idx]))
		}
		if y < m.height-1 {
			b.WriteByte('\n')
		}
	}
	return styles.box.Render(b.String())
}

// renderCell picks the rune + style for one tile. Layer precedence:
// occupant > structure (village / castle) > river overlay > terrain.
// Self vs other player is decided by myID.
func (m *Model) renderCell(t *pb.Tile) string {
	if t == nil {
		return styles.unknownTile.Render(runeUnspecified)
	}
	if t.GetOccupant() == pb.OccupantKind_OCCUPANT_PLAYER && t.GetEntityId() != "" {
		if t.GetEntityId() == m.myID {
			return styles.selfPlayer.Render(runeSelf)
		}
		return styles.otherPlayer.Render(runeOther)
	}
	if s := t.GetStructure(); s != pb.Structure_STRUCTURE_UNSPECIFIED {
		glyph, gOK := structureRunes[s]
		style, sOK := structureStyles[s]
		if gOK && sOK && glyph != "" {
			return style.Render(glyph)
		}
		// Unknown / version-skew structure from the server: render the "what is this"
		// marker so the player SEES something is there, rather than silently
		// falling through to the terrain underneath.
		return styles.unknownTile.Render(runeUnspecified)
	}
	overlays := game.TileOverlay(t.GetOverlays())
	if overlays.Has(game.OverlayRiver) {
		return styles.river.Render(riverRune)
	}
	r, s := lookTile(t)
	return s.Render(r)
}

// sortedPlayers returns the Model's players ordered by ID.
func (m *Model) sortedPlayers() []playerInfo {
	return sortedMapValues(m.players)
}

// renderPlayerList draws the "players online" panel, with the local
// player's name highlighted.
func (m *Model) renderPlayerList() string {
	return renderPanel(PlayersHeader, EmptyListLabel, styles.playerL, m.sortedPlayers(),
		func(p playerInfo) string {
			line := fmt.Sprintf("%s %s %d,%d",
				LogBullet, displayName(p.Name, p.ID), p.Pos.X, p.Pos.Y)
			if p.ID == m.myID {
				line = styles.selfPlayer.Render(line)
			}
			return line
		})
}

// renderLog draws the rolling event log panel. Empty state gets its own
// label so the box doesn't collapse to a single border-only line.
func (m *Model) renderLog() string {
	return renderPanel(EventsHeader, EmptyLogLabel, styles.log, m.logLines,
		func(s string) string { return s })
}

// renderStatus draws the bottom status line. The keybinding hint comes
// from the bubbles help.Model so its layout stays consistent with the
// rest of the Charm ecosystem (and auto-truncates on narrow terminals).
func (m *Model) renderStatus() string {
	me := displayName(m.nameInput.Value(), m.myID)
	parts := []string{
		fmt.Sprintf("you: %s", me),
		fmt.Sprintf("server: %s", m.serverAddr),
		m.help.View(Keys),
	}
	if m.status != "" {
		parts = append(parts, m.status)
	}
	return styles.status.Render(strings.Join(parts, StatusDivider))
}

// renderInput renders the name buffer. Bubbles' textinput.Model draws its
// own cursor and echoes typed runes; we only prepend the styled InputPrompt
// chevron so the visual affordance from the old hand-rolled widget survives.
func renderInput(m *Model) string {
	return styles.input.Render(InputPrompt) + m.nameInput.View()
}

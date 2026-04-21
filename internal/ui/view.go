package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// Every glyph this file renders comes from runes.go — RuneSelf / RuneOther /
// RuneUnspecified / RiverRune / TerrainRunes for the map, plus the
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

// viewEnterName draws a bordered prompt asking for a nickname.
func (m *Model) viewEnterName() string {
	title := styles.title.Render(TitleText)
	prompt := styles.prompt.Render(m.nameInput.prompt)
	input := renderInput(m.nameInput)
	hint := styles.status.Render(QuitLongHint)

	inner := lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		prompt,
		input,
		"",
		hint,
	)
	return styles.box.Render(inner)
}

// viewConnecting shows a centred "connecting" notice.
func (m *Model) viewConnecting() string {
	line := styles.status.Render("Connecting to " + m.serverAddr + "...")
	hint := styles.status.Render(QuitHint)
	return styles.box.Render(lipgloss.JoinVertical(lipgloss.Left, line, "", hint))
}

// viewDisconnected shows the error in a red box plus a quit hint.
func (m *Model) viewDisconnected() string {
	msg := "disconnected"
	if m.err != nil {
		msg = "disconnected: " + m.err.Error()
	}
	body := lipgloss.JoinVertical(lipgloss.Left,
		msg,
		"",
		DisconnectHint,
	)
	return styles.errBox.Render(body)
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
				b.WriteString(styles.unknownTile.Render(RuneUnspecified))
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
// occupant > world-object (village / castle) > river > terrain. Self vs
// other player is decided by myID.
func (m *Model) renderCell(t *pb.Tile) string {
	if t == nil {
		return styles.unknownTile.Render(RuneUnspecified)
	}
	if t.GetOccupant() == pb.OccupantKind_OCCUPANT_PLAYER && t.GetEntityId() != "" {
		if t.GetEntityId() == m.myID {
			return styles.selfPlayer.Render(RuneSelf)
		}
		return styles.otherPlayer.Render(RuneOther)
	}
	if obj := t.GetObject(); obj != pb.WorldObject_WORLD_OBJECT_UNSPECIFIED {
		if glyph, ok := ObjectRunes[obj]; ok {
			switch obj {
			case pb.WorldObject_WORLD_OBJECT_VILLAGE:
				return styles.village.Render(glyph)
			case pb.WorldObject_WORLD_OBJECT_CASTLE:
				return styles.castle.Render(glyph)
			case pb.WorldObject_WORLD_OBJECT_UNSPECIFIED:
				// unreachable — guarded by outer if
			}
		}
	}
	r, s := lookTile(t)
	return s.Render(r)
}

// renderPlayerList draws the "players online" panel, with the local
// player's name highlighted.
func (m *Model) renderPlayerList() string {
	if len(m.players) == 0 {
		return styles.playerL.Render(PlayersHeader + "\n" + EmptyListLabel)
	}
	ids := make([]string, 0, len(m.players))
	for id := range m.players {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString(PlayersHeader + "\n")
	for _, id := range ids {
		info := m.players[id]
		line := fmt.Sprintf("%s %s %d,%d", LogBullet, displayName(info.Name, info.ID), info.Pos.X, info.Pos.Y)
		if id == m.myID {
			line = styles.selfPlayer.Render(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return styles.playerL.Render(strings.TrimRight(b.String(), "\n"))
}

// renderLog draws the rolling event log panel. Empty state gets its own
// label so the box doesn't collapse to a single border-only line.
func (m *Model) renderLog() string {
	if len(m.logLines) == 0 {
		return styles.log.Render(EventsHeader + "\n" + EmptyLogLabel)
	}
	var b strings.Builder
	b.WriteString(EventsHeader + "\n")
	for _, line := range m.logLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return styles.log.Render(strings.TrimRight(b.String(), "\n"))
}

// renderStatus draws the bottom status line.
func (m *Model) renderStatus() string {
	me := displayName(m.nameInput.Value(), m.myID)
	parts := []string{
		fmt.Sprintf("you: %s", me),
		fmt.Sprintf("server: %s", m.serverAddr),
		"WASD move, q quit",
	}
	if m.status != "" {
		parts = append(parts, m.status)
	}
	return styles.status.Render(strings.Join(parts, StatusDivider))
}

// renderInput renders the name buffer with a block cursor.
func renderInput(t textInput) string {
	chars := []rune(t.Value())
	var b strings.Builder
	b.WriteString(styles.input.Render(InputPrompt))
	for i := 0; i <= len(chars); i++ {
		if i == t.cursor && t.focus {
			cell := " "
			if i < len(chars) {
				cell = string(chars[i])
			}
			b.WriteString(styles.cursor.Render(cell))
			if i < len(chars) {
				continue
			}
			break
		}
		if i < len(chars) {
			b.WriteString(styles.input.Render(string(chars[i])))
		}
	}
	return b.String()
}

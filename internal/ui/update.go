package ui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
)

// Init is the Bubble Tea entry Cmd. The client does nothing until the
// user submits a name: no dial, no goroutines, no side effects.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Update is the Bubble Tea reducer. It receives both input events
// (tea.KeyMsg, tea.WindowSizeMsg) and internal messages emitted by the
// Cmd factories in net.go. Every branch returns the Model plus an
// optional Cmd — no direct mutation of outside state.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = v.Width
		m.termHeight = v.Height
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(v)

	case connectingMsg:
		return m, nil

	case connectedMsg:
		return m.handleConnected(v)

	case acceptedMsg:
		m.myID = v.PlayerID
		if m.stream != nil {
			return m, listenCmd(m.stream)
		}
		return m, nil

	case snapshotMsg:
		applySnapshot(m, v.Snapshot)
		m.phase = phasePlaying
		m.status = ""
		if m.stream != nil {
			return m, listenCmd(m.stream)
		}
		return m, nil

	case eventMsg:
		applyEvent(m, v.Event)
		if m.stream != nil {
			return m, listenCmd(m.stream)
		}
		return m, nil

	case serverErrorMsg:
		// Rule violations (move into wall / occupied tile) are expected UX —
		// the player simply stays put. No log spam. Just keep listening.
		_ = v
		if m.stream != nil {
			return m, listenCmd(m.stream)
		}
		return m, nil

	case netErrorMsg:
		return m.handleNetError(v), nil
	}
	return m, nil
}

// handleKey dispatches a key press to the right sub-handler based on
// the current phase.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if isQuit(msg) {
		if m.cancel != nil {
			m.cancel()
		}
		return m, tea.Quit
	}
	switch m.phase {
	case phaseEnterName:
		return m.handleKeyEnterName(msg)
	case phasePlaying:
		return m.handleKeyPlaying(msg)
	case phaseConnecting, phaseDisconnected:
		// No input accepted beyond quit. Already handled above.
		return m, nil
	}
	return m, nil
}

// handleKeyEnterName edits the name buffer and, on Enter with a non-
// empty value, transitions to phaseConnecting and fires the dial Cmd.
func (m *Model) handleKeyEnterName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			return m, nil
		}
		m.nameInput.SetValue(name)
		m.phase = phaseConnecting
		m.status = "connecting to " + m.serverAddr + "..."
		return m, connectCmd(m.ctx, m.serverAddr)
	case tea.KeyBackspace:
		m.nameInput.Backspace()
		return m, nil
	case tea.KeyLeft:
		m.nameInput.MoveLeft()
		return m, nil
	case tea.KeyRight:
		m.nameInput.MoveRight()
		return m, nil
	case tea.KeySpace:
		m.nameInput.InsertRune(' ')
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if !unicode.IsPrint(r) {
				continue
			}
			m.nameInput.InsertRune(r)
		}
		return m, nil
	}
	return m, nil
}

// handleKeyPlaying turns WASD/hjkl/arrows into a MoveCmd on the outbox.
func (m *Model) handleKeyPlaying(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	dx, dy, ok := moveFor(msg)
	if !ok {
		return m, nil
	}
	if m.outbox == nil {
		return m, nil
	}
	return m, sendMoveCmd(m.outbox, dx, dy)
}

// handleConnected stores the live stream handles on the Model and
// immediately sends Join + installs the first Recv.
func (m *Model) handleConnected(v connectedMsg) (tea.Model, tea.Cmd) {
	m.conn = v.conn
	m.stream = v.stream
	m.cancel = v.cancel
	m.outbox = v.outbox
	m.status = "connected, joining..."
	name := m.nameInput.Value()
	return m, tea.Batch(
		sendJoinCmd(v.outbox, name),
		listenCmd(v.stream),
	)
}

// handleNetError moves the UI into the disconnected phase and tears
// down every piece of network state owned by the Model. It is safe to
// call repeatedly; idempotency keeps the teardown path simple.
func (m *Model) handleNetError(v netErrorMsg) *Model {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	if m.outbox != nil {
		// Closing signals the writer goroutine to exit if ctx didn't
		// already. Safe: we own the channel and no one else writes.
		close(m.outbox)
		m.outbox = nil
	}
	if m.conn != nil {
		_ = m.conn.Close()
		m.conn = nil
	}
	m.stream = nil
	m.err = v.Err
	if v.Err != nil {
		m.status = v.Err.Error()
	} else {
		m.status = "disconnected"
	}
	m.phase = phaseDisconnected
	return m
}

// Compile-time assertion — Model must satisfy tea.Model. Catches
// accidental signature drift as a build-time error instead of at
// program start.
var _ tea.Model = (*Model)(nil)

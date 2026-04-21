package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
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
		return m.handleResize(v)
	case tea.KeyMsg:
		return m.handleKey(v)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(v)
		return m, cmd
	case connectingMsg:
		return m, nil
	case connectedMsg:
		return m.handleConnected(v)
	case acceptedMsg:
		m.myID = v.PlayerID
		cmd := m.keepListening()
		return m, cmd
	case snapshotMsg:
		applySnapshot(m, v.Snapshot)
		m.phase = phasePlaying
		m.status = ""
		cmd := m.keepListening()
		return m, cmd
	case eventMsg:
		applyEvent(m, v.Event)
		cmd := m.keepListening()
		return m, cmd
	case serverErrorMsg:
		// Rule violations (move into wall / occupied tile) are expected UX —
		// the player simply stays put. No log spam. Just keep listening.
		_ = v
		cmd := m.keepListening()
		return m, cmd
	case netErrorMsg:
		return m.handleNetError(v), nil
	}
	return m, nil
}

// handleKey is the tea.KeyMsg dispatcher. Quit is handled globally so
// no per-phase handler has to remember to check for it; every other
// key press flows to the sub-handler for the current phase.
func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, Keys.Quit) {
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
		// No input accepted beyond quit, which was handled above.
		return m, nil
	}
	return m, nil
}

// keepListening returns a Cmd that re-arms the Recv goroutine if a
// stream is live, or nil otherwise. Centralising the nil check keeps
// the message-specific branches of Update one line apiece.
func (m *Model) keepListening() tea.Cmd {
	if m.stream == nil {
		return nil
	}
	return listenCmd(m.stream)
}

// handleKeyEnterName edits the name buffer and, on Enter with a non-
// empty value, transitions to phaseConnecting and fires the dial Cmd.
// Non-Enter keys are delegated to the bubbles textinput.Model, which
// handles typing, cursor motion, backspace, word jumps, etc.
func (m *Model) handleKeyEnterName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEnter {
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			return m, nil
		}
		m.nameInput.SetValue(name)
		m.phase = phaseConnecting
		m.status = "connecting to " + m.serverAddr + "..."
		return m, tea.Batch(connectCmd(m.ctx, m.serverAddr), m.spinner.Tick)
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

// handleKeyPlaying turns WASD/hjkl/arrows into a MoveCmd on the outbox.
func (m *Model) handleKeyPlaying(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.outbox == nil {
		return m, nil
	}
	switch {
	case key.Matches(msg, Keys.Up):
		return m, sendMoveCmd(m.outbox, 0, -1)
	case key.Matches(msg, Keys.Down):
		return m, sendMoveCmd(m.outbox, 0, +1)
	case key.Matches(msg, Keys.Left):
		return m, sendMoveCmd(m.outbox, -1, 0)
	case key.Matches(msg, Keys.Right):
		return m, sendMoveCmd(m.outbox, +1, 0)
	}
	return m, nil
}

// handleResize stores the new terminal dimensions and, if we are already
// playing, pushes the fresh viewport size to the server so the next
// snapshot is rendered at the new terminal area.
func (m *Model) handleResize(v tea.WindowSizeMsg) (tea.Model, tea.Cmd) {
	m.termWidth = v.Width
	m.termHeight = v.Height
	m.help.Width = v.Width
	if m.phase == phasePlaying && m.outbox != nil {
		w, h := viewportForTerm(v.Width, v.Height)
		return m, sendViewportCmd(m.outbox, w, h)
	}
	return m, nil
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
	viewW, viewH := viewportForTerm(m.termWidth, m.termHeight)
	return m, tea.Batch(
		sendJoinCmd(v.outbox, name, viewW, viewH),
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

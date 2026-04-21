// Package ui is the Bubble Tea terminal client for Gongeons.
//
// The client is a thin renderer on top of a gRPC bidirectional stream:
// it never mutates world state on its own. Every key press turns into a
// ClientMessage sent to the server, and every ServerMessage received from
// the server is applied to the local snapshot held in Model.
package ui

import (
	"context"

	"google.golang.org/grpc"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// UI sizing and formatting constants. Named so call sites stay expressive and
// the linter stops complaining about magic numbers.
const (
	// logLinesCap bounds the rolling event-log panel in the playing phase.
	logLinesCap = 6

	// nameInputMaxLen is the longest name a player can type in the
	// enter-name screen. 24 rune-cells fits comfortably inside the menu box
	// on any sane terminal.
	nameInputMaxLen = 24

	// textInputMaxVisual caps how many rune cells a text input is allowed
	// to display on screen before scrolling kicks in (not implemented yet,
	// but the constant documents the intent).
	textInputMaxVisual = 32
)

// phase is the top-level UI state machine of the client.
type phase int

const (
	phaseEnterName phase = iota
	phaseConnecting
	phasePlaying
	phaseDisconnected
)

// playerInfo is everything the UI knows about a remote or local player.
type playerInfo struct {
	ID   string
	Name string
	Pos  game.Position
}

// textInput is a minimal single-line text input. It intentionally avoids a
// dependency on bubbles/textinput: the requirements here (one line, no
// masking, no validation) are small enough that a purpose-built struct
// keeps the call graph obvious.
type textInput struct {
	value     []rune
	cursor    int
	maxLen    int
	prompt    string
	focus     bool
	maxVisual int
}

// newTextInput returns a focused, empty text input.
func newTextInput(prompt string, maxLen int) textInput {
	return textInput{
		prompt:    prompt,
		maxLen:    maxLen,
		focus:     true,
		maxVisual: textInputMaxVisual,
	}
}

// Value returns the current contents as a string.
func (t textInput) Value() string {
	return string(t.value)
}

// SetValue replaces the buffer and parks the cursor at the end.
func (t *textInput) SetValue(s string) {
	t.value = []rune(s)
	t.cursor = len(t.value)
}

// InsertRune appends a rune at the cursor position. Silently drops input
// once maxLen is reached.
func (t *textInput) InsertRune(r rune) {
	if len(t.value) >= t.maxLen {
		return
	}
	// Insert at cursor. A one-line buffer is small; the O(n) copy is fine.
	out := make([]rune, 0, len(t.value)+1)
	out = append(out, t.value[:t.cursor]...)
	out = append(out, r)
	out = append(out, t.value[t.cursor:]...)
	t.value = out
	t.cursor++
}

// Backspace deletes the rune before the cursor, if any.
func (t *textInput) Backspace() {
	if t.cursor == 0 {
		return
	}
	out := make([]rune, 0, len(t.value)-1)
	out = append(out, t.value[:t.cursor-1]...)
	out = append(out, t.value[t.cursor:]...)
	t.value = out
	t.cursor--
}

// MoveLeft shifts the cursor one rune left, clamped to the buffer start.
func (t *textInput) MoveLeft() {
	if t.cursor > 0 {
		t.cursor--
	}
}

// MoveRight shifts the cursor one rune right, clamped to the buffer end.
func (t *textInput) MoveRight() {
	if t.cursor < len(t.value) {
		t.cursor++
	}
}

// Model is the Bubble Tea model. It tracks the current UI phase, the
// server-authoritative snapshot, and the outbound command channel.
//
// Invariants:
//   - tiles is row-major with exactly width*height cells once phasePlaying
//     is reached; before that it is nil.
//   - outbox is non-nil only in phasePlaying or phaseConnecting after
//     connectedMsg has been handled.
//   - cancel is installed when a stream is live and is nil otherwise.
type Model struct {
	phase phase

	// Enter-name screen.
	nameInput textInput

	// Connecting / disconnected screen status strings.
	serverAddr string
	status     string
	err        error

	// Play-screen state.
	myID     string
	width    int
	height   int
	tiles    []*pb.Tile
	players  map[string]playerInfo
	logLines []string

	// Network plumbing.
	ctx    context.Context
	cancel context.CancelFunc
	conn   *grpc.ClientConn
	stream pb.GameService_PlayClient
	outbox chan *pb.ClientMessage

	// Terminal dimensions reported by tea.WindowSizeMsg.
	termWidth  int
	termHeight int
}

// New returns an initial Model parked in phaseEnterName. The returned
// pointer is what tea.NewProgram should be constructed with.
//
// ctx is the program-wide context — when it is cancelled (for example by
// signal.NotifyContext in main), any in-flight stream goroutine owned by
// this Model winds down. ctx must be non-nil.
func New(ctx context.Context, addr string) *Model {
	return &Model{
		phase:      phaseEnterName,
		nameInput:  newTextInput("Your name:", nameInputMaxLen),
		serverAddr: addr,
		players:    make(map[string]playerInfo),
		ctx:        ctx,
	}
}

// setOutbox is a test hook — it installs an outbox channel so Update can
// emit sendMoveCmd without a live gRPC stream.
func (m *Model) setOutbox(ch chan *pb.ClientMessage) {
	m.outbox = ch
}

// setPhase is a test hook.
func (m *Model) setPhase(p phase) {
	m.phase = p
}

// appendLog pushes a human-readable line into the rolling log, trimming
// from the head if it would exceed logLinesCap.
func (m *Model) appendLog(line string) {
	m.logLines = append(m.logLines, line)
	if len(m.logLines) > logLinesCap {
		// Drop the oldest. Copy into a new slice so we don't keep a large
		// backing array pinned when a player spams moves.
		trimmed := make([]string, logLinesCap)
		copy(trimmed, m.logLines[len(m.logLines)-logLinesCap:])
		m.logLines = trimmed
	}
}

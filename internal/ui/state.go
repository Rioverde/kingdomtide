// Package ui is the Bubble Tea terminal client for Gongeons.
//
// The client is a thin renderer on top of a gRPC bidirectional stream:
// it never mutates world state on its own. Every key press turns into a
// ClientMessage sent to the server, and every ServerMessage received from
// the server is applied to the local snapshot held in Model.
package ui

import (
	"context"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
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

	// NameInputLabel is the prompt shown above the name-entry box. Rendered
	// separately from the bubbles textinput's own Prompt field (which we
	// suppress) so lipgloss styling stays under our control.
	NameInputLabel = "Your name:"
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

// newNameInput returns a focused, empty textinput.Model sized for the
// enter-name screen. The bubbles model renders its own cursor and prompt;
// we suppress its Prompt and render the "Your name:" label separately in
// view.go so lipgloss styling stays in our hands.
func newNameInput() textinput.Model {
	ti := textinput.New()
	ti.CharLimit = nameInputMaxLen
	ti.Prompt = ""
	ti.Focus()
	return ti
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
	nameInput textinput.Model

	// help renders the bottom-bar keybinding hint from Keys. Width is
	// updated on every tea.WindowSizeMsg so the short view truncates
	// gracefully on narrow terminals.
	help help.Model

	// spinner animates the connecting-phase status line. It keeps ticking
	// once started; we simply stop rendering it when the phase advances.
	spinner spinner.Model

	// Connecting / disconnected screen status strings.
	serverAddr string
	status     string
	err        error

	// Play-screen state.
	myID     string
	width    int
	height   int
	origin   game.Position // world coord of tiles[0]
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
	sp := spinner.New()
	sp.Spinner = spinner.Ellipsis
	return &Model{
		phase:      phaseEnterName,
		nameInput:  newNameInput(),
		help:       help.New(),
		spinner:    sp,
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

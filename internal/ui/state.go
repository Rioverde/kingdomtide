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
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// UI sizing and formatting constants. Named so call sites stay expressive and
// the linter stops complaining about magic numbers.
const (
	// logLinesCap bounds the rolling event-log panel in the playing phase.
	// The wide bottom-strip layout can show up to eventsRows lines minus
	// the box chrome, so we keep roughly twice that in memory to allow
	// scrollback without losing events on resize.
	logLinesCap = 20

	// nameInputMaxLen is the longest name a player can type in the
	// enter-name screen. 24 rune-cells fits comfortably inside the menu box
	// on any sane terminal.
	nameInputMaxLen = 24
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

// logKind classifies an event-log entry so renderEventsBox can apply
// per-kind colour styles. The zero value is logKindDefault.
type logKind int

const (
	logKindDefault logKind = iota
	logKindJoin
	logKindLeave
)

// logEntry is one line in the rolling event log. Text is the rendered
// string (already includes bullet and localised message); Kind drives
// the lipgloss style applied in renderEventsBox.
type logEntry struct {
	Text string
	Kind logKind
}

// newNameInput returns a focused, empty textinput.Model sized for the
// enter-name screen. The bubbles model renders its own cursor and prompt;
// we suppress its Prompt and render the localized name label (input.name_label)
// separately in view.go so lipgloss styling stays in our hands.
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
	logLines []logEntry

	// Region tracking. region is the latest anchor-resolved Region sent in
	// the most recent Snapshot; lastRegionCoord is its SuperChunkCoord, used
	// for crossing-detection comparisons without relying on the region name
	// (which a Phase-5 history event could mutate without the player moving).
	// initialised guards the first snapshot so joining a world does not emit
	// a spurious "you enter X" log line.
	region          *pb.Region
	lastRegionCoord game.SuperChunkCoord
	initialised     bool

	// World seed delivered by JoinAccepted. Stored read-only; used to drive
	// the local influenceSource for per-tile tint sampling and the same
	// Voronoi-anchor queries the server ran authoritatively. Zero means we
	// have not joined yet (the server always sends a non-zero seed even for
	// seed=0 worlds because JoinAccepted arrives strictly after dial).
	worldSeed int64

	// influenceSource is the client's local copy of the region noise pipeline,
	// used exclusively for cosmetic per-tile tint sampling in renderCell.
	// Identity (name/character) always comes from the server-authoritative
	// Region in the Snapshot — the client never rederives those.
	influenceSource worldgen.InfluenceSampler

	// Localization. lang is the BCP-47 short tag the client renders UI in
	// and sends to the server via JoinRequest.Language. Derived once at
	// construction from locale.Detect.
	lang string

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
		lang:       locale.Detect(),
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

// appendLog pushes a typed log entry into the rolling log, trimming from
// the head if it would exceed logLinesCap.
func (m *Model) appendLog(line string, kind logKind) {
	m.logLines = append(m.logLines, logEntry{Text: line, Kind: kind})
	if len(m.logLines) > logLinesCap {
		// Drop the oldest. Copy into a new slice so we don't keep a large
		// backing array pinned when a player spams moves.
		trimmed := make([]logEntry, logLinesCap)
		copy(trimmed, m.logLines[len(m.logLines)-logLinesCap:])
		m.logLines = trimmed
	}
}

// appendLogDefault appends a default-styled log entry. Used by all call
// sites except join/leave so they remain one-liners without repeating the
// kind argument.
func (m *Model) appendLogDefault(line string) {
	m.appendLog(line, logKindDefault)
}

// appendLogJoin appends a green-styled join event entry.
func (m *Model) appendLogJoin(line string) {
	m.appendLog(line, logKindJoin)
}

// appendLogLeave appends a grey-styled leave event entry.
func (m *Model) appendLogLeave(line string) {
	m.appendLog(line, logKindLeave)
}

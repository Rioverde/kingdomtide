package ui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/geom"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// keyRunes synthesises a tea.KeyMsg as if the user typed one rune.
func keyRunes(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestEnterNameTypingAndSubmit(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")

	// Type "bob".
	for _, r := range "bob" {
		model, _ := m.Update(keyRunes(r))
		m = model.(*Model)
	}
	if m.nameInput.Value() != "bob" {
		t.Fatalf("name buffer = %q, want %q", m.nameInput.Value(), "bob")
	}

	// Submit: a valid name advances to phaseCharacterCreation so the
	// player can distribute their Point Buy budget before we dial.
	// No Cmd is returned at this step — dialing is deferred to the
	// subsequent confirm on the creation screen.
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)
	if m.phase != phaseCharacterCreation {
		t.Fatalf("phase = %d, want phaseCharacterCreation", m.phase)
	}
}

func TestEnterNameEmptyDoesNothing(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)
	if m.phase != phaseEnterName {
		t.Fatalf("phase = %d, want phaseEnterName", m.phase)
	}
	if cmd != nil {
		t.Fatalf("expected nil Cmd for empty-name Enter")
	}
}

func TestBackspaceInName(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	for _, r := range "abc" {
		model, _ := m.Update(keyRunes(r))
		m = model.(*Model)
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = model.(*Model)
	if m.nameInput.Value() != "ab" {
		t.Fatalf("after backspace: %q, want %q", m.nameInput.Value(), "ab")
	}
}

func TestSnapshotTransitionsToPlaying(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	m.setPhase(phaseConnecting)

	snap := &pb.Snapshot{
		Width:  2,
		Height: 1,
		Tiles: []*pb.Tile{
			{Terrain: pb.Terrain_TERRAIN_PLAINS},
			{Terrain: pb.Terrain_TERRAIN_PLAINS, Occupant: pb.OccupantKind_OCCUPANT_PLAYER, EntityId: "me"},
		},
		Entities: []*pb.Entity{
			{Id: "me", Name: "me", Position: &pb.Position{X: 1, Y: 0}},
		},
	}
	model, _ := m.Update(snapshotMsg{Snapshot: snap})
	m = model.(*Model)

	if m.phase != phasePlaying {
		t.Fatalf("phase = %d, want phasePlaying", m.phase)
	}
	if m.width != 2 || m.height != 1 || len(m.tiles) != 2 {
		t.Fatalf("snapshot not applied: %dx%d tiles=%d", m.width, m.height, len(m.tiles))
	}
	if _, ok := m.players["me"]; !ok {
		t.Fatalf("snapshot did not populate players map")
	}
}

func TestEventMsgAppendsLogAndUpdatesPlayers(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	m.setPhase(phasePlaying)
	m.width = 2
	m.height = 1
	m.tiles = []*pb.Tile{
		{Terrain: pb.Terrain_TERRAIN_PLAINS},
		{Terrain: pb.Terrain_TERRAIN_PLAINS},
	}

	ev := &pb.Event{
		Payload: &pb.Event_PlayerJoined{
			PlayerJoined: &pb.PlayerJoined{
				Entity: &pb.Entity{
					Id:       "alice",
					Name:     "alice",
					Position: &pb.Position{X: 0, Y: 0},
				},
			},
		},
	}
	model, _ := m.Update(eventMsg{Event: ev})
	m = model.(*Model)

	if _, ok := m.players["alice"]; !ok {
		t.Fatalf("alice missing from players map")
	}
	if len(m.logLines) != 1 {
		t.Fatalf("log len = %d, want 1", len(m.logLines))
	}
}

func TestMoveKeyInPlayingEmitsOutbox(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	m.setPhase(phasePlaying)
	outbox := make(chan *pb.ClientMessage, 1)
	m.setOutbox(outbox)

	_, cmd := m.Update(keyRunes('d'))
	if cmd == nil {
		t.Fatalf("expected Cmd from d key press")
	}
	// Run the Cmd to trigger the send. tea.Cmd returns a tea.Msg.
	_ = cmd()

	select {
	case got := <-outbox:
		move := got.GetMove()
		if move == nil {
			t.Fatalf("expected MoveCmd payload, got %v", got.GetPayload())
		}
		if move.GetDx() != 1 || move.GetDy() != 0 {
			t.Fatalf("MoveCmd = (%d,%d), want (1,0)", move.GetDx(), move.GetDy())
		}
	default:
		t.Fatalf("no message on outbox")
	}
}

func TestMoveKeysAllDirections(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		key         tea.KeyMsg
		wantDX, wDY int32
	}{
		{"w up", keyRunes('w'), 0, -1},
		{"s down", keyRunes('s'), 0, 1},
		{"a left", keyRunes('a'), -1, 0},
		{"d right", keyRunes('d'), 1, 0},
		{"arrow up", tea.KeyMsg{Type: tea.KeyUp}, 0, -1},
		{"arrow right", tea.KeyMsg{Type: tea.KeyRight}, 1, 0},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := New(context.Background(), "localhost:50051")
			m.setPhase(phasePlaying)
			outbox := make(chan *pb.ClientMessage, 1)
			m.setOutbox(outbox)

			_, cmd := m.Update(tc.key)
			if cmd == nil {
				t.Fatalf("expected Cmd")
			}
			_ = cmd()

			got := <-outbox
			move := got.GetMove()
			if move == nil {
				t.Fatalf("expected MoveCmd")
			}
			if move.GetDx() != tc.wantDX || move.GetDy() != tc.wDY {
				t.Fatalf("got (%d,%d) want (%d,%d)", move.GetDx(), move.GetDy(), tc.wantDX, tc.wDY)
			}
		})
	}
}

func TestQuitKeyReturnsQuitCmd(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	m.setPhase(phasePlaying)

	_, cmd := m.Update(keyRunes('q'))
	if cmd == nil {
		t.Fatalf("expected tea.Quit Cmd")
	}
	// tea.Quit is a Cmd function — just check it returns a tea.Msg.
	if got := cmd(); got == nil {
		t.Fatalf("quit cmd returned nil msg")
	}
}

func TestNetErrorTransitionsToDisconnected(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	m.setPhase(phaseConnecting)

	errIn := testError("boom")
	model, _ := m.Update(netErrorMsg{Err: errIn})
	m = model.(*Model)
	if m.phase != phaseDisconnected {
		t.Fatalf("phase = %d, want phaseDisconnected", m.phase)
	}
	if m.err == nil {
		t.Fatalf("err not stored")
	}
}

func TestWindowSizeMsgStored(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	model, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = model.(*Model)
	if m.termWidth != 80 || m.termHeight != 24 {
		t.Fatalf("term size = %dx%d, want 80x24", m.termWidth, m.termHeight)
	}
}

func TestAcceptedMsgSetsID(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	model, _ := m.Update(acceptedMsg{PlayerID: "player-7"})
	m = model.(*Model)
	if m.myID != "player-7" {
		t.Fatalf("myID = %q, want player-7", m.myID)
	}
}

// testError is a minimal error implementation so tests don't need
// errors.New (keeps the import list focused).
type testError string

func (e testError) Error() string { return string(e) }

// Compile-time sanity: geom.Position is still the value type UI uses.
var _ = geom.Position{}

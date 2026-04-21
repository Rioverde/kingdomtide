package server

import (
	"errors"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

func TestClientMessageToCommandJoin(t *testing.T) {
	msg := &pb.ClientMessage{Payload: &pb.ClientMessage_Join{Join: &pb.JoinRequest{Name: "alice"}}}
	cmd, err := clientMessageToCommand(msg, "pid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jc, ok := cmd.(game.JoinCmd)
	if !ok {
		t.Fatalf("expected JoinCmd, got %T", cmd)
	}
	if jc.PlayerID != "pid-1" || jc.Name != "alice" {
		t.Fatalf("bad mapping: %+v", jc)
	}
}

func TestClientMessageToCommandMove(t *testing.T) {
	msg := &pb.ClientMessage{Payload: &pb.ClientMessage_Move{Move: &pb.MoveCmd{Dx: 1, Dy: 0}}}
	cmd, err := clientMessageToCommand(msg, "pid-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mc, ok := cmd.(game.MoveCmd)
	if !ok {
		t.Fatalf("expected MoveCmd, got %T", cmd)
	}
	if mc.PlayerID != "pid-2" || mc.DX != 1 || mc.DY != 0 {
		t.Fatalf("bad mapping: %+v", mc)
	}
}

func TestClientMessageToCommandEmpty(t *testing.T) {
	_, err := clientMessageToCommand(nil, "pid-3")
	if !errors.Is(err, errEmptyPayload) {
		t.Fatalf("want errEmptyPayload, got %v", err)
	}
	_, err = clientMessageToCommand(&pb.ClientMessage{}, "pid-3")
	if !errors.Is(err, errEmptyPayload) {
		t.Fatalf("want errEmptyPayload on bare ClientMessage, got %v", err)
	}
}

func TestEventToServerMessagePlayerJoined(t *testing.T) {
	ev := game.PlayerJoinedEvent{PlayerID: "p1", Name: "alice", Position: game.Position{X: 3, Y: 4}}
	msg := eventToServerMessage(ev)
	pj := msg.GetEvent().GetPlayerJoined()
	if pj == nil {
		t.Fatalf("expected PlayerJoined payload, got %v", msg)
	}
	if pj.GetEntity().GetId() != "p1" || pj.GetEntity().GetName() != "alice" {
		t.Fatalf("entity fields wrong: %+v", pj.GetEntity())
	}
	if pj.GetEntity().GetPosition().GetX() != 3 || pj.GetEntity().GetPosition().GetY() != 4 {
		t.Fatalf("position wrong: %+v", pj.GetEntity().GetPosition())
	}
	if pj.GetEntity().GetKind() != pb.OccupantKind_OCCUPANT_PLAYER {
		t.Fatalf("kind wrong: %v", pj.GetEntity().GetKind())
	}
}

func TestEventToServerMessagePlayerLeft(t *testing.T) {
	ev := game.PlayerLeftEvent{PlayerID: "p1"}
	msg := eventToServerMessage(ev)
	pl := msg.GetEvent().GetPlayerLeft()
	if pl == nil || pl.GetPlayerId() != "p1" {
		t.Fatalf("bad mapping: %v", msg)
	}
}

func TestEventToServerMessageEntityMoved(t *testing.T) {
	ev := game.EntityMovedEvent{
		EntityID: "p1",
		From:     game.Position{X: 1, Y: 2},
		To:       game.Position{X: 2, Y: 2},
	}
	msg := eventToServerMessage(ev)
	em := msg.GetEvent().GetEntityMoved()
	if em == nil {
		t.Fatalf("expected EntityMoved, got %v", msg)
	}
	if em.GetEntityId() != "p1" {
		t.Fatalf("entity id: %v", em.GetEntityId())
	}
	if em.GetFrom().GetX() != 1 || em.GetTo().GetX() != 2 {
		t.Fatalf("from/to: %v / %v", em.GetFrom(), em.GetTo())
	}
}

func TestTerrainToPBBuckets(t *testing.T) {
	cases := map[game.Terrain]pb.Terrain{
		game.TerrainPlains:    pb.Terrain_TERRAIN_FLOOR,
		game.TerrainGrassland: pb.Terrain_TERRAIN_FLOOR,
		game.TerrainForest:    pb.Terrain_TERRAIN_FLOOR,
		game.TerrainMountain:  pb.Terrain_TERRAIN_WALL,
		game.TerrainSnowyPeak: pb.Terrain_TERRAIN_WALL,
		game.TerrainOcean:     pb.Terrain_TERRAIN_WATER,
		game.TerrainDeepOcean: pb.Terrain_TERRAIN_WATER,
		game.Terrain(""):      pb.Terrain_TERRAIN_UNSPECIFIED,
		game.Terrain("xyz"):   pb.Terrain_TERRAIN_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

func TestSnapshotOfDefaultWorldWithPlayer(t *testing.T) {
	w := game.NewMockWorld()
	events, err := w.ApplyCommand(game.JoinCmd{PlayerID: "p1", Name: "alice"})
	if err != nil {
		t.Fatalf("apply join: %v", err)
	}
	pj, ok := events[0].(game.PlayerJoinedEvent)
	if !ok {
		t.Fatalf("expected PlayerJoinedEvent, got %T", events[0])
	}

	snap := snapshotOf(w)
	if snap.GetWidth() != int32(w.Width()) || snap.GetHeight() != int32(w.Height()) {
		t.Fatalf("snapshot size %dx%d, want %dx%d",
			snap.GetWidth(), snap.GetHeight(), w.Width(), w.Height())
	}
	if len(snap.GetTiles()) != w.Width()*w.Height() {
		t.Fatalf("snapshot tile count: got %d, want %d",
			len(snap.GetTiles()), w.Width()*w.Height())
	}

	spawnIdx := pj.Position.Y*w.Width() + pj.Position.X
	spawnTile := snap.GetTiles()[spawnIdx]
	if spawnTile.GetOccupant() != pb.OccupantKind_OCCUPANT_PLAYER {
		t.Fatalf("spawn tile occupant: %v", spawnTile.GetOccupant())
	}
	if spawnTile.GetEntityId() != "p1" {
		t.Fatalf("spawn tile entity id: %v", spawnTile.GetEntityId())
	}

	if len(snap.GetEntities()) != 1 || snap.GetEntities()[0].GetId() != "p1" {
		t.Fatalf("entities: %v", snap.GetEntities())
	}
}

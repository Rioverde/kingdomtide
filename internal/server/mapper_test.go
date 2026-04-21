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

func TestTerrainToPBMapping(t *testing.T) {
	cases := map[game.Terrain]pb.Terrain{
		game.TerrainPlains:    pb.Terrain_TERRAIN_PLAINS,
		game.TerrainGrassland: pb.Terrain_TERRAIN_GRASSLAND,
		game.TerrainForest:    pb.Terrain_TERRAIN_FOREST,
		game.TerrainMountain:  pb.Terrain_TERRAIN_MOUNTAIN,
		game.TerrainOcean:     pb.Terrain_TERRAIN_OCEAN,
		game.TerrainDeepOcean: pb.Terrain_TERRAIN_DEEP_OCEAN,
		game.TerrainBeach:     pb.Terrain_TERRAIN_BEACH,
		game.TerrainHills:     pb.Terrain_TERRAIN_HILLS,
		game.Terrain(""):      pb.Terrain_TERRAIN_UNSPECIFIED,
		game.Terrain("xyz"):   pb.Terrain_TERRAIN_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

func TestSnapshotOfShape(t *testing.T) {
	w := game.NewWorld(42)
	events, err := w.ApplyCommand(game.JoinCmd{PlayerID: "p1", Name: "alice"})
	if err != nil {
		t.Fatalf("apply join: %v", err)
	}
	spawn := events[0].(game.PlayerJoinedEvent).Position

	snap := snapshotOf(w, spawn, DefaultViewportWidth, DefaultViewportHeight)
	if snap.GetWidth() != int32(DefaultViewportWidth) || snap.GetHeight() != int32(DefaultViewportHeight) {
		t.Fatalf("snapshot size: %dx%d, want %dx%d",
			snap.GetWidth(), snap.GetHeight(), DefaultViewportWidth, DefaultViewportHeight)
	}
	if len(snap.GetTiles()) != DefaultViewportWidth*DefaultViewportHeight {
		t.Fatalf("snapshot tile count: got %d, want %d",
			len(snap.GetTiles()), DefaultViewportWidth*DefaultViewportHeight)
	}
	if snap.GetOrigin().GetX() != int32(spawn.X-DefaultViewportWidth/2) ||
		snap.GetOrigin().GetY() != int32(spawn.Y-DefaultViewportHeight/2) {
		t.Fatalf("origin: %+v, want centred on spawn %+v", snap.GetOrigin(), spawn)
	}
	if len(snap.GetEntities()) != 1 || snap.GetEntities()[0].GetId() != "p1" {
		t.Fatalf("entities: %v", snap.GetEntities())
	}
}

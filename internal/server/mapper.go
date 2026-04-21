package server

import (
	"errors"
	"fmt"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// errEmptyPayload is returned when a ClientMessage carries no recognised oneof.
var errEmptyPayload = errors.New("empty payload")

// clientMessageToCommand converts a wire message from the given player into a
// domain command. The playerID argument is the server-assigned identity for
// this stream — clients never send their own id.
func clientMessageToCommand(m *pb.ClientMessage, playerID string) (game.Command, error) {
	if m == nil {
		return nil, errEmptyPayload
	}
	switch v := m.GetPayload().(type) {
	case *pb.ClientMessage_Join:
		name := ""
		if v.Join != nil {
			name = v.Join.GetName()
		}
		return game.JoinCmd{PlayerID: playerID, Name: name}, nil
	case *pb.ClientMessage_Move:
		if v.Move == nil {
			return nil, fmt.Errorf("move: %w", errEmptyPayload)
		}
		return game.MoveCmd{
			PlayerID: playerID,
			DX:       int(v.Move.GetDx()),
			DY:       int(v.Move.GetDy()),
		}, nil
	default:
		return nil, errEmptyPayload
	}
}

// eventToServerMessage wraps a domain Event in the right oneof branches of
// Event and ServerMessage. Returns nil for unknown event types so the caller
// can simply skip them.
func eventToServerMessage(e game.Event) *pb.ServerMessage {
	var ev *pb.Event
	switch v := e.(type) {
	case game.PlayerJoinedEvent:
		ev = &pb.Event{Payload: &pb.Event_PlayerJoined{PlayerJoined: &pb.PlayerJoined{
			Entity: &pb.Entity{
				Id:       v.PlayerID,
				Name:     v.Name,
				Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
				Position: positionPB(v.Position),
			},
		}}}
	case game.PlayerLeftEvent:
		ev = &pb.Event{Payload: &pb.Event_PlayerLeft{PlayerLeft: &pb.PlayerLeft{
			PlayerId: v.PlayerID,
		}}}
	case game.EntityMovedEvent:
		ev = &pb.Event{Payload: &pb.Event_EntityMoved{EntityMoved: &pb.EntityMoved{
			EntityId: v.EntityID,
			From:     positionPB(v.From),
			To:       positionPB(v.To),
		}}}
	default:
		return nil
	}
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Event{Event: ev}}
}

// positionPB converts a domain Position into its wire form.
func positionPB(p game.Position) *pb.Position {
	return &pb.Position{X: int32(p.X), Y: int32(p.Y)}
}

// terrainToPB collapses domain biomes onto the small wire Terrain bucket.
// Land biomes (plains, grass, meadow, beach, savanna, desert, forests, hills,
// tundra, taiga, snow) become FLOOR. Mountains become WALL. Oceans become
// WATER. Anything unrecognised stays UNSPECIFIED.
func terrainToPB(t game.Terrain) pb.Terrain {
	switch t {
	case game.TerrainPlains, game.TerrainGrassland, game.TerrainMeadow,
		game.TerrainBeach, game.TerrainSavanna, game.TerrainDesert,
		game.TerrainSnow, game.TerrainTundra, game.TerrainTaiga,
		game.TerrainForest, game.TerrainJungle, game.TerrainHills:
		return pb.Terrain_TERRAIN_FLOOR
	case game.TerrainMountain, game.TerrainSnowyPeak:
		return pb.Terrain_TERRAIN_WALL
	case game.TerrainOcean, game.TerrainDeepOcean:
		return pb.Terrain_TERRAIN_WATER
	default:
		return pb.Terrain_TERRAIN_UNSPECIFIED
	}
}

// snapshotOf builds a full Snapshot of the given world. Caller holds any
// locks needed to keep the world quiescent during the scan.
func snapshotOf(w *game.World) *pb.Snapshot {
	width := w.Width()
	height := w.Height()
	tiles := make([]*pb.Tile, 0, width*height)
	for y := range height {
		for x := range width {
			t, _ := w.TileAt(game.Position{X: x, Y: y})
			out := &pb.Tile{Terrain: terrainToPB(t.Terrain)}
			if p, ok := t.Occupant.(*game.Player); ok && p != nil {
				out.Occupant = pb.OccupantKind_OCCUPANT_PLAYER
				out.EntityId = p.ID
			}
			tiles = append(tiles, out)
		}
	}
	return &pb.Snapshot{
		Width:    int32(width),
		Height:   int32(height),
		Tiles:    tiles,
		Entities: entitiesOf(w),
	}
}

// entitiesOf returns one Entity per player currently in the world, sorted by
// ID for stability (World.Players guarantees that).
func entitiesOf(w *game.World) []*pb.Entity {
	players := w.Players()
	out := make([]*pb.Entity, 0, len(players))
	for _, p := range players {
		pos, _ := w.PositionOf(p.ID)
		out = append(out, &pb.Entity{
			Id:       p.ID,
			Name:     p.Name,
			Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
			Position: positionPB(pos),
		})
	}
	return out
}

// errorResponse wraps a short error into a ServerMessage for targeted delivery.
func errorResponse(msg, code string) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Error{Error: &pb.ErrorResponse{
		Message: msg,
		Code:    code,
	}}}
}

// acceptedResponse is the initial JoinAccepted message carrying the assigned id.
func acceptedResponse(playerID string) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Accepted{Accepted: &pb.JoinAccepted{
		PlayerId: playerID,
	}}}
}

// snapshotResponse wraps a Snapshot into a ServerMessage.
func snapshotResponse(s *pb.Snapshot) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: s}}
}

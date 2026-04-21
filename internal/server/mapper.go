package server

import (
	"errors"
	"fmt"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// errEmptyPayload is returned when a ClientMessage carries no recognised oneof.
var errEmptyPayload = errors.New("empty payload")

// DefaultViewportWidth/Height are the Snapshot dimensions the server uses
// when the client hasn't reported its own. Odd on both axes so the spawn
// tile sits in the exact centre.
const (
	DefaultViewportWidth  = 41
	DefaultViewportHeight = 21

	// Minimum viewport size the server will honour. Below this the UI has
	// no room for an interior; forcing a floor prevents a broken client
	// from making the server render 1×1 snapshots.
	MinViewportWidth  = 11
	MinViewportHeight = 7
)

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

// structurePBMapping translates the domain StructureKind enum to its wire
// counterpart. Unknown values fall back to UNSPECIFIED.
var structurePBMapping = map[game.StructureKind]pb.Structure{
	game.StructureVillage: pb.Structure_STRUCTURE_VILLAGE,
	game.StructureCastle:  pb.Structure_STRUCTURE_CASTLE,
}

// structureToPB looks up k in structurePBMapping. StructureNone maps
// implicitly to UNSPECIFIED via the default return.
func structureToPB(k game.StructureKind) pb.Structure {
	return lookupOr(structurePBMapping, k, pb.Structure_STRUCTURE_UNSPECIFIED)
}

// terrainPBMapping is the 1:1 translation table from the domain Terrain
// enum to its wire counterpart. Kept as a map (not a switch) so adding a
// new biome is a single-line change and cyclomatic complexity stays flat.
var terrainPBMapping = map[game.Terrain]pb.Terrain{
	game.TerrainPlains:    pb.Terrain_TERRAIN_PLAINS,
	game.TerrainGrassland: pb.Terrain_TERRAIN_GRASSLAND,
	game.TerrainMeadow:    pb.Terrain_TERRAIN_MEADOW,
	game.TerrainBeach:     pb.Terrain_TERRAIN_BEACH,
	game.TerrainDesert:    pb.Terrain_TERRAIN_DESERT,
	game.TerrainSavanna:   pb.Terrain_TERRAIN_SAVANNA,
	game.TerrainForest:    pb.Terrain_TERRAIN_FOREST,
	game.TerrainJungle:    pb.Terrain_TERRAIN_JUNGLE,
	game.TerrainTaiga:     pb.Terrain_TERRAIN_TAIGA,
	game.TerrainTundra:    pb.Terrain_TERRAIN_TUNDRA,
	game.TerrainSnow:      pb.Terrain_TERRAIN_SNOW,
	game.TerrainHills:     pb.Terrain_TERRAIN_HILLS,
	game.TerrainMountain:  pb.Terrain_TERRAIN_MOUNTAIN,
	game.TerrainSnowyPeak: pb.Terrain_TERRAIN_SNOWY_PEAK,
	game.TerrainOcean:     pb.Terrain_TERRAIN_OCEAN,
	game.TerrainDeepOcean: pb.Terrain_TERRAIN_DEEP_OCEAN,
}

// terrainToPB looks up t in terrainPBMapping. Unknown terrains fall back
// to UNSPECIFIED so the client can render a clear "what is this" glyph.
func terrainToPB(t game.Terrain) pb.Terrain {
	return lookupOr(terrainPBMapping, t, pb.Terrain_TERRAIN_UNSPECIFIED)
}

// regionCharacterPBMapping is the 1:1 translation table from the domain
// RegionCharacter enum to its wire counterpart. Kept as a map so adding a
// seventh character (e.g. Phase 5 "Cultured") stays a one-line change.
var regionCharacterPBMapping = map[game.RegionCharacter]pb.RegionCharacter{
	game.RegionNormal:   pb.RegionCharacter_REGION_CHARACTER_NORMAL,
	game.RegionBlighted: pb.RegionCharacter_REGION_CHARACTER_BLIGHTED,
	game.RegionFey:      pb.RegionCharacter_REGION_CHARACTER_FEY,
	game.RegionAncient:  pb.RegionCharacter_REGION_CHARACTER_ANCIENT,
	game.RegionSavage:   pb.RegionCharacter_REGION_CHARACTER_SAVAGE,
	game.RegionHoly:     pb.RegionCharacter_REGION_CHARACTER_HOLY,
	game.RegionWild:     pb.RegionCharacter_REGION_CHARACTER_WILD,
}

// regionCharacterPB translates the domain RegionCharacter enum to its wire
// counterpart. Unknown values fall back to NORMAL — the safe rendering
// default (no tint, no crossing-verb prefix).
func regionCharacterPB(c game.RegionCharacter) pb.RegionCharacter {
	return lookupOr(regionCharacterPBMapping, c, pb.RegionCharacter_REGION_CHARACTER_NORMAL)
}

// regionInfluencePB builds a wire RegionInfluence from the domain struct.
// The field order is intentional: it matches the proto declaration so a
// visual diff against the .proto reads top-to-bottom.
func regionInfluencePB(r game.RegionInfluence) *pb.RegionInfluence {
	return &pb.RegionInfluence{
		Blight:  r.Blight,
		Fae:     r.Fae,
		Ancient: r.Ancient,
		Savage:  r.Savage,
		Holy:    r.Holy,
		Wild:    r.Wild,
	}
}

// regionPB converts a domain Region to its wire form. The anchor position
// is intentionally not sent — clients derive it locally from the world
// seed via game.AnchorOf to keep Snapshot region payload compact.
func regionPB(r game.Region) *pb.Region {
	return &pb.Region{
		SuperChunkX: int32(r.Coord.X),
		SuperChunkY: int32(r.Coord.Y),
		Name:        r.Name,
		Character:   regionCharacterPB(r.Character),
		Influence:   regionInfluencePB(r.Influence),
	}
}

// clampViewport enforces the minimum size rule and defaults zero values to
// the server defaults. Unused here — see dispatch in service.go.
func clampViewport(w, h int) (int, int) {
	if w <= 0 {
		w = DefaultViewportWidth
	}
	if h <= 0 {
		h = DefaultViewportHeight
	}
	return max(w, MinViewportWidth), max(h, MinViewportHeight)
}

// tileFromDomain builds a wire Tile from a domain tile, overlaying the
// player occupant when present. The terrain / overlays / structure
// conversions all live in one spot. overlays is carried through as an
// opaque bitmask — the domain and the client agree on flag values.
func tileFromDomain(t game.Tile) *pb.Tile {
	out := &pb.Tile{
		Terrain:   terrainToPB(t.Terrain),
		Overlays:  uint32(t.Overlays),
		Structure: structureToPB(t.Structure),
	}
	if p, ok := t.Occupant.(*game.Player); ok && p != nil {
		out.Occupant = pb.OccupantKind_OCCUPANT_PLAYER
		out.EntityId = p.ID
	}
	return out
}

// snapshotOf builds a viewport Snapshot of viewW × viewH tiles centred on
// the given world position. Zero or too-small dimensions are replaced by
// the server defaults via clampViewport. The returned Snapshot also carries
// the region covering center — resolved from region on the caller's side
// so the cache is owned by the service, not re-entered per snapshot here.
// Pass a nil region when no RegionSource is configured (tests, legacy paths)
// and the Snapshot omits the region field.
func snapshotOf(w *game.World, center game.Position, viewW, viewH int, region *pb.Region) *pb.Snapshot {
	viewW, viewH = clampViewport(viewW, viewH)
	halfW := viewW / 2
	halfH := viewH / 2
	originX := center.X - halfW
	originY := center.Y - halfH
	tiles := make([]*pb.Tile, 0, viewW*viewH)
	for dy := range viewH {
		for dx := range viewW {
			p := game.Position{X: originX + dx, Y: originY + dy}
			t, _ := w.TileAt(p)
			tiles = append(tiles, tileFromDomain(t))
		}
	}
	return &pb.Snapshot{
		Width:    int32(viewW),
		Height:   int32(viewH),
		Origin:   positionPB(game.Position{X: originX, Y: originY}),
		Tiles:    tiles,
		Entities: entitiesOf(w),
		Region:   region,
	}
}

// entitiesOf returns one Entity per player currently in the world, sorted
// by ID for stability (World.Players guarantees that).
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

// acceptedResponse is the initial JoinAccepted message carrying the assigned
// id, the player's spawn position, and the world seed. Clients use the seed
// to construct a local region source for per-tile influence sampling.
func acceptedResponse(playerID string, spawn game.Position, worldSeed int64) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Accepted{Accepted: &pb.JoinAccepted{
		PlayerId:  playerID,
		Spawn:     positionPB(spawn),
		WorldSeed: worldSeed,
	}}}
}

// snapshotResponse wraps a Snapshot into a ServerMessage.
func snapshotResponse(s *pb.Snapshot) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: s}}
}

package ui

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// positionFromPB converts a proto Position to the domain value type. A nil
// receiver maps to the origin — callers don't have to guard every field
// access.
func positionFromPB(p *pb.Position) game.Position {
	if p == nil {
		return game.Position{}
	}
	return game.Position{X: int(p.GetX()), Y: int(p.GetY())}
}

// applySnapshot replaces the local world state with a full server snapshot.
// Origin is recorded so world-space event coordinates can be translated to
// local tile-array indices.
func applySnapshot(m *Model, s *pb.Snapshot) {
	if s == nil {
		return
	}
	m.width = int(s.GetWidth())
	m.height = int(s.GetHeight())
	m.origin = positionFromPB(s.GetOrigin())

	src := s.GetTiles()
	tiles := make([]*pb.Tile, len(src))
	copy(tiles, src)
	m.tiles = tiles

	m.players = make(map[string]playerInfo, len(s.GetEntities()))
	for _, e := range s.GetEntities() {
		if e == nil {
			continue
		}
		m.players[e.GetId()] = playerInfo{
			ID:   e.GetId(),
			Name: e.GetName(),
			Pos:  positionFromPB(e.GetPosition()),
		}
	}
}

// applyEvent folds one server event into the local model. Each branch also
// appends a log line so the event log panel on the playing screen gives a
// running narrative of other players' actions.
func applyEvent(m *Model, ev *pb.Event) {
	if ev == nil {
		return
	}
	switch payload := ev.GetPayload().(type) {
	case *pb.Event_PlayerJoined:
		ent := payload.PlayerJoined.GetEntity()
		if ent == nil {
			return
		}
		id := ent.GetId()
		pos := positionFromPB(ent.GetPosition())
		m.players[id] = playerInfo{ID: id, Name: ent.GetName(), Pos: pos}
		m.setOccupant(pos, id, pb.OccupantKind_OCCUPANT_PLAYER)
		m.appendLog(fmt.Sprintf(LogBullet+" %s joined", displayName(ent.GetName(), id)))
	case *pb.Event_PlayerLeft:
		id := payload.PlayerLeft.GetPlayerId()
		if info, ok := m.players[id]; ok {
			m.clearOccupantAt(info.Pos, id)
			delete(m.players, id)
			m.appendLog(fmt.Sprintf(LogBullet+" %s left", displayName(info.Name, id)))
			return
		}
		m.appendLog(fmt.Sprintf(LogBullet+" %s left", displayName("", id)))
	case *pb.Event_EntityMoved:
		id := payload.EntityMoved.GetEntityId()
		from := positionFromPB(payload.EntityMoved.GetFrom())
		to := positionFromPB(payload.EntityMoved.GetTo())
		m.clearOccupantAt(from, id)
		m.setOccupant(to, id, pb.OccupantKind_OCCUPANT_PLAYER)
		info, ok := m.players[id]
		if !ok {
			info = playerInfo{ID: id}
		}
		info.Pos = to
		m.players[id] = info
		m.appendLog(fmt.Sprintf(LogBullet+" %s moved", displayName(info.Name, id)))
	}
}

// displayIDPrefixLen is the number of leading ID characters shown when the
// entity's name is not yet known. Six keeps the first chunk of a UUID
// distinctive without eating the narrow player-list column.
const displayIDPrefixLen = 6

// displayName prefers the entity's human name; falls back to a short ID
// prefix so the log is still readable when names aren't known yet.
func displayName(name, id string) string {
	if name != "" {
		return name
	}
	if len(id) > displayIDPrefixLen {
		return id[:displayIDPrefixLen]
	}
	return id
}

// tileIndex translates a world-space position into the local tile-array
// offset, or -1 when the position falls outside the current viewport.
func (m *Model) tileIndex(p game.Position) int {
	if m.width <= 0 || m.height <= 0 {
		return -1
	}
	localX := p.X - m.origin.X
	localY := p.Y - m.origin.Y
	if localX < 0 || localY < 0 || localX >= m.width || localY >= m.height {
		return -1
	}
	return localY*m.width + localX
}

// withTile looks up the tile at p in the local viewport and invokes fn
// if it exists. No-op for out-of-viewport positions or nil tile slots.
func (m *Model) withTile(p game.Position, fn func(*pb.Tile)) {
	idx := m.tileIndex(p)
	if idx < 0 || idx >= len(m.tiles) {
		return
	}
	if t := m.tiles[idx]; t != nil {
		fn(t)
	}
}

// setOccupant writes occupant metadata to a tile. No-op if the position is
// out of the viewport or the tile slot is nil.
func (m *Model) setOccupant(p game.Position, entityID string, kind pb.OccupantKind) {
	m.withTile(p, func(t *pb.Tile) {
		t.Occupant = kind
		t.EntityId = entityID
	})
}

// clearOccupantAt blanks occupant metadata only when the entity currently
// listed on that tile matches id. Prevents out-of-order events from wiping a
// cell a different entity has since moved onto.
func (m *Model) clearOccupantAt(p game.Position, id string) {
	m.withTile(p, func(t *pb.Tile) {
		if t.GetEntityId() != id {
			return
		}
		t.Occupant = pb.OccupantKind_OCCUPANT_UNSPECIFIED
		t.EntityId = ""
	})
}

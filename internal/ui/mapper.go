package ui

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// terrainBucket is the reduced render category for a tile. It collapses
// pb.Terrain variants down to just the cases the view cares about so the
// render path does not spell out a big switch per cell.
type terrainBucket int

const (
	terrainBucketUnspecified terrainBucket = iota
	terrainBucketFloor
	terrainBucketWall
	terrainBucketWater
)

// bucketOf maps a pb.Terrain to its render bucket.
func bucketOf(t pb.Terrain) terrainBucket {
	switch t {
	case pb.Terrain_TERRAIN_FLOOR, pb.Terrain_TERRAIN_GRASS:
		return terrainBucketFloor
	case pb.Terrain_TERRAIN_WALL:
		return terrainBucketWall
	case pb.Terrain_TERRAIN_WATER:
		return terrainBucketWater
	default:
		return terrainBucketUnspecified
	}
}

// positionFromPB converts a proto Position to the domain value type.
// A nil receiver maps to the origin — a deliberate choice so callers
// don't have to guard every field access.
func positionFromPB(p *pb.Position) game.Position {
	if p == nil {
		return game.Position{}
	}
	return game.Position{X: int(p.GetX()), Y: int(p.GetY())}
}

// applySnapshot replaces the local world state with a full server
// snapshot. It clears any previous tiles/players and rebuilds from
// scratch — snapshots are authoritative.
func applySnapshot(m *Model, s *pb.Snapshot) {
	if s == nil {
		return
	}
	m.width = int(s.GetWidth())
	m.height = int(s.GetHeight())

	src := s.GetTiles()
	// Copy the slice header so callers cannot mutate our tiles through
	// the Snapshot they passed in. The element pointers are shared —
	// acceptable here because the server never reuses them.
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

// applyEvent folds one server event into the local model. Each branch
// also appends a log line so the event log panel on the playing screen
// gives a running narrative of other players' actions.
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
		m.appendLog(fmt.Sprintf("* %s joined", displayName(ent.GetName(), id)))
	case *pb.Event_PlayerLeft:
		id := payload.PlayerLeft.GetPlayerId()
		if info, ok := m.players[id]; ok {
			m.clearOccupantAt(info.Pos, id)
			delete(m.players, id)
			m.appendLog(fmt.Sprintf("* %s left", displayName(info.Name, id)))
			return
		}
		m.appendLog(fmt.Sprintf("* %s left", displayName("", id)))
	case *pb.Event_EntityMoved:
		id := payload.EntityMoved.GetEntityId()
		from := positionFromPB(payload.EntityMoved.GetFrom())
		to := positionFromPB(payload.EntityMoved.GetTo())
		// Keep the tiles slice in sync with the authoritative position so
		// re-renders don't show ghosts at the old cell.
		m.clearOccupantAt(from, id)
		m.setOccupant(to, id, pb.OccupantKind_OCCUPANT_PLAYER)
		info, ok := m.players[id]
		if !ok {
			info = playerInfo{ID: id}
		}
		info.Pos = to
		m.players[id] = info
		m.appendLog(fmt.Sprintf("* %s moved", displayName(info.Name, id)))
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

// tileIndex returns the row-major offset of (p.X, p.Y) or -1 if the
// position is out of bounds.
func (m *Model) tileIndex(p game.Position) int {
	if m.width <= 0 || m.height <= 0 {
		return -1
	}
	if p.X < 0 || p.Y < 0 || p.X >= m.width || p.Y >= m.height {
		return -1
	}
	return p.Y*m.width + p.X
}

// setOccupant writes occupant metadata to a tile. No-op if the position
// is out of bounds or the tile slot is nil (server didn't ship this
// cell — shouldn't happen, but the guard is cheap).
func (m *Model) setOccupant(p game.Position, entityID string, kind pb.OccupantKind) {
	idx := m.tileIndex(p)
	if idx < 0 || idx >= len(m.tiles) {
		return
	}
	t := m.tiles[idx]
	if t == nil {
		return
	}
	t.Occupant = kind
	t.EntityId = entityID
}

// clearOccupantAt blanks occupant metadata only when the entity currently
// listed on that tile matches id. Prevents out-of-order events from
// wiping a cell a different entity has since moved onto.
func (m *Model) clearOccupantAt(p game.Position, id string) {
	idx := m.tileIndex(p)
	if idx < 0 || idx >= len(m.tiles) {
		return
	}
	t := m.tiles[idx]
	if t == nil {
		return
	}
	if t.GetEntityId() != id {
		return
	}
	t.Occupant = pb.OccupantKind_OCCUPANT_UNSPECIFIED
	t.EntityId = ""
}

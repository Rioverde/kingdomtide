package ui

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
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

// applyJoinAccepted folds the JoinAccepted reply into the Model. The player
// ID anchors "me" on every subsequent snapshot; the world seed spins up a
// local NoiseRegionSource whose only job is per-tile tint sampling in
// renderCell. Region identity — the name and character shown in the status
// bar, and the SuperChunkCoord used for crossing detection — always arrives
// via Snapshot.Region, never derived client-side. Keeping the two flows
// separate means a history mutation of a region reaches the UI
// without the client needing a new pipeline.
func applyJoinAccepted(m *Model, v acceptedMsg) {
	m.myID = v.PlayerID
	m.worldSeed = v.WorldSeed
	m.influenceSource = worldgen.NewInfluenceSampler(v.WorldSeed)
}

// applySnapshot replaces the local world state with a full server snapshot.
// Origin is recorded so world-space event coordinates can be translated to
// local tile-array indices. The region is compared against the previously
// observed anchor coord; a change emits one localized crossing log entry
// (suppressed on the very first snapshot so joining doesn't announce itself).
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

	applyRegion(m, s.GetRegion())
}

// applyRegion folds the per-player region field from Snapshot into the
// Model. A change in anchor SuperChunkCoord relative to the previous value
// emits a localized crossing log line; identical-coord snapshots and the
// very first snapshot after join produce no log line.
func applyRegion(m *Model, r *pb.Region) {
	if r == nil {
		return
	}
	sc := regionCoord(r)
	if m.initialised && m.lastRegionCoord != sc {
		key := locale.CharacterCrossingKey(regionCharacterKey(r.GetCharacter()))
		msg := locale.Tr(m.lang, key, "Region", r.GetName())
		m.appendLogDefault(msg)
	}
	m.lastRegionCoord = sc
	m.region = r
	m.initialised = true
}

// regionCoord extracts the anchor's SuperChunkCoord from a wire Region.
// Equivalent to constructing the value inline but named for readability at
// call sites.
func regionCoord(r *pb.Region) game.SuperChunkCoord {
	return game.SuperChunkCoord{
		X: int(r.GetSuperChunkX()),
		Y: int(r.GetSuperChunkY()),
	}
}

// fromPBCharacter converts a wire RegionCharacter to the domain value type.
// Unknown wire values map to RegionNormal so version-skewed enums degrade
// gracefully. This is the single authoritative pb→domain mapping; the
// reverse (domain→pb) lives in view.go's pbCharacter function.
func fromPBCharacter(c pb.RegionCharacter) game.RegionCharacter {
	switch c {
	case pb.RegionCharacter_REGION_CHARACTER_BLIGHTED:
		return game.RegionBlighted
	case pb.RegionCharacter_REGION_CHARACTER_FEY:
		return game.RegionFey
	case pb.RegionCharacter_REGION_CHARACTER_ANCIENT:
		return game.RegionAncient
	case pb.RegionCharacter_REGION_CHARACTER_SAVAGE:
		return game.RegionSavage
	case pb.RegionCharacter_REGION_CHARACTER_HOLY:
		return game.RegionHoly
	case pb.RegionCharacter_REGION_CHARACTER_WILD:
		return game.RegionWild
	}
	return game.RegionNormal
}

// regionCharacterKey maps a wire RegionCharacter to the lowercase catalog
// suffix used for crossing keys. Delegates to fromPBCharacter + Key so the
// string mapping is maintained in a single place (game.regionCharacterNames).
func regionCharacterKey(c pb.RegionCharacter) string {
	return fromPBCharacter(c).Key()
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
		m.logJoinEvent(locale.KeyLogJoined, displayName(ent.GetName(), id))
	case *pb.Event_PlayerLeft:
		id := payload.PlayerLeft.GetPlayerId()
		if info, ok := m.players[id]; ok {
			m.clearOccupantAt(info.Pos, id)
			delete(m.players, id)
			m.logLeaveEvent(locale.KeyLogLeft, displayName(info.Name, id))
			return
		}
		m.logLeaveEvent(locale.KeyLogLeft, displayName("", id))
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
		m.logEvent(locale.KeyLogMoved, displayName(info.Name, id))
	}
}

// logEvent appends a default-styled bulleted, localized event-log entry.
// Centralising the bullet + locale.Tr call keeps every event branch in
// applyEvent one line.
func (m *Model) logEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, "Name", name)
	m.appendLogDefault(fmt.Sprintf("%s %s", LogBullet, msg))
}

// logJoinEvent appends a green-styled join log entry.
func (m *Model) logJoinEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, "Name", name)
	m.appendLogJoin(fmt.Sprintf("%s %s", LogBullet, msg))
}

// logLeaveEvent appends a grey-styled leave log entry.
func (m *Model) logLeaveEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, "Name", name)
	m.appendLogLeave(fmt.Sprintf("%s %s", LogBullet, msg))
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

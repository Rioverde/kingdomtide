package ui

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// positionFromPB converts a proto Position to the domain value type. A nil
// receiver maps to the origin — callers don't have to guard every field
// access.
func positionFromPB(p *pb.Position) geom.Position {
	if p == nil {
		return geom.Position{}
	}
	return geom.Position{X: int(p.GetX()), Y: int(p.GetY())}
}

// applyJoinAccepted folds the JoinAccepted reply into the Model. The player
// ID anchors "me" on every subsequent snapshot; the world seed spins up a
// local NoiseRegionSource whose only job is per-tile tint sampling in
// renderCell. Region identity — the name and character shown in the status
// bar, and the SuperChunkCoord used for crossing detection — always arrives
// via Snapshot.Region, never derived client-side. Keeping the two flows
// separate means a history mutation of a region reaches the UI
// without the client needing a new pipeline.
//
// The calendar cadence is cached raw in calendarCfg for any future UI
// that wants the cadence fields; live calendar position arrives via
// Snapshot.game_time on join and is refreshed by the periodic
// TimeTickEvent the server broadcasts once per wall-clock second. No
// client-side calendar mirror is built — the server is authoritative.
func applyJoinAccepted(m *Model, v acceptedMsg) {
	m.myID = v.PlayerID
	m.worldSeed = v.WorldSeed
	m.influenceSource = worldgen.NewInfluenceSampler(v.WorldSeed)
	m.calendarCfg = v.Calendar
}

// calendarConfigFromPB reshapes the wire CalendarConfig into the UI's
// local cache type. A nil receiver yields the zero value, which the
// date HUD renderer treats as "not yet configured" and therefore draws
// nothing. The epoch offset is forwarded so applyJoinAccepted can build
// a calendar.Calendar mirror aligned with the server's epoch.
func calendarConfigFromPB(c *pb.CalendarConfig) calendarConfig {
	if c == nil {
		return calendarConfig{}
	}
	return calendarConfig{
		TicksPerDay:     c.GetTicksPerDay(),
		DaysPerMonth:    c.GetDaysPerMonth(),
		MonthsPerYear:   c.GetMonthsPerYear(),
		EpochTickOffset: c.GetEpochTickOffset(),
	}
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
	// Refresh the live calendar position. A valid GameTime has
	// Month != MonthZero; a zero-value GameTime arrives when the world
	// has no Calendar wired (legacy / test servers). Preserve the
	// previous gameTime rather than blanking the HUD on such snapshots
	// so the display stays stable if a single snapshot happens to be
	// calendar-less after TimeTick has already seeded a value.
	if gt := gameTimeFromPB(s.GetGameTime()); gt.Month != calendar.MonthZero {
		m.gameTime = gt
	}
	m.detectLandmarkApproach()
}

// applyRegion folds the per-player region field from Snapshot into the
// Model. A change in anchor SuperChunkCoord relative to the previous
// value emits a localized crossing log line; identical-coord snapshots
// and the very first snapshot after join produce no log line. The
// region name is composed client-side from the structured NameParts
// using the player's Model.lang.
func applyRegion(m *Model, r *pb.Region) {
	if r == nil {
		return
	}
	sc := regionCoord(r)
	if m.initialised && m.lastRegionCoord != sc {
		key := locale.CharacterCrossingKey(regionCharacterKey(r.GetCharacter()))
		msg := locale.Tr(m.lang, key, locale.ArgRegion, composeName(naming.DomainRegion, r.GetName(), m.lang))
		m.appendLogDefault(msg)
	}
	m.lastRegionCoord = sc
	m.region = r
	m.initialised = true
}

// regionCoord extracts the anchor's SuperChunkCoord from a wire Region.
// Equivalent to constructing the value inline but named for readability at
// call sites.
func regionCoord(r *pb.Region) geom.SuperChunkCoord {
	return geom.SuperChunkCoord{
		X: int(r.GetSuperChunkX()),
		Y: int(r.GetSuperChunkY()),
	}
}

// fromPBCharacter converts a wire RegionCharacter to the domain value type.
// Unknown wire values map to RegionNormal so version-skewed enums degrade
// gracefully. This is the single authoritative pb→domain mapping; the
// reverse (domain→pb) lives in view.go's pbCharacter function.
func fromPBCharacter(c pb.RegionCharacter) world.RegionCharacter {
	switch c {
	case pb.RegionCharacter_REGION_CHARACTER_BLIGHTED:
		return world.RegionBlighted
	case pb.RegionCharacter_REGION_CHARACTER_FEY:
		return world.RegionFey
	case pb.RegionCharacter_REGION_CHARACTER_ANCIENT:
		return world.RegionAncient
	case pb.RegionCharacter_REGION_CHARACTER_SAVAGE:
		return world.RegionSavage
	case pb.RegionCharacter_REGION_CHARACTER_HOLY:
		return world.RegionHoly
	case pb.RegionCharacter_REGION_CHARACTER_WILD:
		return world.RegionWild
	}
	return world.RegionNormal
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
	case *pb.Event_TimeTick:
		// Server-authoritative calendar broadcast: adopt the carried
		// GameTime directly so the date HUD tracks the world clock
		// without local extrapolation. Guard against a zero-value
		// payload (legacy server or calendar-less world) the same way
		// applySnapshot does so a malformed broadcast never wipes a
		// previously valid HUD.
		if gt := gameTimeFromPB(payload.TimeTick.GetGameTime()); gt.Month != calendar.MonthZero {
			m.gameTime = gt
		}
	}
}

// gameTimeFromPB reshapes a wire pb.GameTime into its domain form. A
// nil receiver yields the zero GameTime (Month == MonthZero) so
// renderers can tell "no calendar configured" apart from any valid
// in-world date without a separate null check. Mirrors gameTimeToPB on
// the server side.
func gameTimeFromPB(p *pb.GameTime) calendar.GameTime {
	if p == nil {
		return calendar.GameTime{}
	}
	return calendar.GameTime{
		Year:       p.GetYear(),
		Month:      monthFromPB(p.GetMonth()),
		DayOfMonth: p.GetDayOfMonth(),
		TickOfDay:  p.GetTickOfDay(),
		Season:     seasonFromPB(p.GetSeason()),
	}
}

// monthFromPBMapping is the 1:1 translation table from the wire
// CalendarMonth enum to the domain Month. Kept as a map so adding a
// thirteenth month stays a single-line change and CALENDAR_MONTH_
// UNSPECIFIED falls through to MonthZero via the default return.
var monthFromPBMapping = map[pb.CalendarMonth]calendar.Month{
	pb.CalendarMonth_CALENDAR_MONTH_JANUARY:   calendar.MonthJanuary,
	pb.CalendarMonth_CALENDAR_MONTH_FEBRUARY:  calendar.MonthFebruary,
	pb.CalendarMonth_CALENDAR_MONTH_MARCH:     calendar.MonthMarch,
	pb.CalendarMonth_CALENDAR_MONTH_APRIL:     calendar.MonthApril,
	pb.CalendarMonth_CALENDAR_MONTH_MAY:       calendar.MonthMay,
	pb.CalendarMonth_CALENDAR_MONTH_JUNE:      calendar.MonthJune,
	pb.CalendarMonth_CALENDAR_MONTH_JULY:      calendar.MonthJuly,
	pb.CalendarMonth_CALENDAR_MONTH_AUGUST:    calendar.MonthAugust,
	pb.CalendarMonth_CALENDAR_MONTH_SEPTEMBER: calendar.MonthSeptember,
	pb.CalendarMonth_CALENDAR_MONTH_OCTOBER:   calendar.MonthOctober,
	pb.CalendarMonth_CALENDAR_MONTH_NOVEMBER:  calendar.MonthNovember,
	pb.CalendarMonth_CALENDAR_MONTH_DECEMBER:  calendar.MonthDecember,
}

// monthFromPB translates a wire CalendarMonth enum to the domain
// Month. Unknown wire values (including CALENDAR_MONTH_UNSPECIFIED)
// return MonthZero so the renderer treats them as "no calendar".
func monthFromPB(m pb.CalendarMonth) calendar.Month {
	if v, ok := monthFromPBMapping[m]; ok {
		return v
	}
	return calendar.MonthZero
}

// seasonFromPBMapping is the 1:1 translation table from the wire
// CalendarSeason enum to the domain Season. Kept as a map for
// symmetry with monthFromPBMapping; the zero-value UNSPECIFIED falls
// through to SeasonWinter via the default return (matches SeasonOf
// behaviour for out-of-range months).
var seasonFromPBMapping = map[pb.CalendarSeason]calendar.Season{
	pb.CalendarSeason_CALENDAR_SEASON_WINTER: calendar.SeasonWinter,
	pb.CalendarSeason_CALENDAR_SEASON_SPRING: calendar.SeasonSpring,
	pb.CalendarSeason_CALENDAR_SEASON_SUMMER: calendar.SeasonSummer,
	pb.CalendarSeason_CALENDAR_SEASON_AUTUMN: calendar.SeasonAutumn,
}

// seasonFromPB translates a wire CalendarSeason enum to the domain
// Season. Unknown wire values (including CALENDAR_SEASON_UNSPECIFIED)
// return SeasonWinter as the safe default — matches the server-side
// SeasonOf behaviour for an out-of-range Month.
func seasonFromPB(s pb.CalendarSeason) calendar.Season {
	if v, ok := seasonFromPBMapping[s]; ok {
		return v
	}
	return calendar.SeasonWinter
}

// logEvent appends a default-styled bulleted, localized event-log entry.
// Centralising the bullet + locale.Tr call keeps every event branch in
// applyEvent one line.
func (m *Model) logEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, locale.ArgName, name)
	m.appendLogDefault(fmt.Sprintf("%s %s", LogBullet, msg))
}

// logJoinEvent appends a green-styled join log entry.
func (m *Model) logJoinEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, locale.ArgName, name)
	m.appendLogJoin(fmt.Sprintf("%s %s", LogBullet, msg))
}

// logLeaveEvent appends a grey-styled leave log entry.
func (m *Model) logLeaveEvent(messageID, name string) {
	msg := locale.Tr(m.lang, messageID, locale.ArgName, name)
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
func (m *Model) tileIndex(p geom.Position) int {
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
func (m *Model) withTile(p geom.Position, fn func(*pb.Tile)) {
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
func (m *Model) setOccupant(p geom.Position, entityID string, kind pb.OccupantKind) {
	m.withTile(p, func(t *pb.Tile) {
		t.Occupant = kind
		t.EntityId = entityID
	})
}

// clearOccupantAt blanks occupant metadata only when the entity currently
// listed on that tile matches id. Prevents out-of-order events from wiping a
// cell a different entity has since moved onto.
func (m *Model) clearOccupantAt(p geom.Position, id string) {
	m.withTile(p, func(t *pb.Tile) {
		if t.GetEntityId() != id {
			return
		}
		t.Occupant = pb.OccupantKind_OCCUPANT_UNSPECIFIED
		t.EntityId = ""
	})
}

package server

import (
	"errors"
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/entity"
	"github.com/Rioverde/gongeons/internal/game/event"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming/parts"
	"github.com/Rioverde/gongeons/internal/game/stats"
	"github.com/Rioverde/gongeons/internal/game/world"
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
func clientMessageToCommand(m *pb.ClientMessage, playerID string) (world.Command, error) {
	if m == nil {
		return nil, errEmptyPayload
	}
	switch v := m.GetPayload().(type) {
	case *pb.ClientMessage_Join:
		name := ""
		var cs stats.CoreStats
		if v.Join != nil {
			name = v.Join.GetName()
			cs = coreStatsFromPB(v.Join.GetStats())
		} else {
			cs = stats.DefaultCoreStats()
		}
		return world.JoinCmd{PlayerID: playerID, Name: name, Stats: cs}, nil
	case *pb.ClientMessage_Move:
		if v.Move == nil {
			return nil, fmt.Errorf("move: %w", errEmptyPayload)
		}
		return world.MoveCmd{
			PlayerID: playerID,
			DX:       int(v.Move.GetDx()),
			DY:       int(v.Move.GetDy()),
		}, nil
	default:
		return nil, errEmptyPayload
	}
}

// coreStatsFromPB converts a wire CoreStats into the domain type. A nil
// input is treated as "client omitted the field" and yields the neutral
// baseline, so callers that want to reject missing stats must check for
// nil themselves before calling.
func coreStatsFromPB(s *pb.CoreStats) stats.CoreStats {
	if s == nil {
		return stats.DefaultCoreStats()
	}
	return stats.CoreStats{
		Strength:     int(s.GetStrength()),
		Dexterity:    int(s.GetDexterity()),
		Constitution: int(s.GetConstitution()),
		Intelligence: int(s.GetIntelligence()),
		Wisdom:       int(s.GetWisdom()),
		Charisma:     int(s.GetCharisma()),
	}
}

// coreStatsToPB converts a domain CoreStats into its wire form. Returns a
// non-nil pointer for every input so the caller never has to check.
func coreStatsToPB(s stats.CoreStats) *pb.CoreStats {
	return &pb.CoreStats{
		Strength:     int32(s.Strength),
		Dexterity:    int32(s.Dexterity),
		Constitution: int32(s.Constitution),
		Intelligence: int32(s.Intelligence),
		Wisdom:       int32(s.Wisdom),
		Charisma:     int32(s.Charisma),
	}
}

// eventToServerMessage wraps a domain Event in the right oneof branches of
// Event and ServerMessage. Returns nil for unknown event types so the caller
// can simply skip them.
func eventToServerMessage(e event.Event) *pb.ServerMessage {
	var ev *pb.Event
	switch v := e.(type) {
	case event.PlayerJoinedEvent:
		ev = &pb.Event{Payload: &pb.Event_PlayerJoined{PlayerJoined: &pb.PlayerJoined{
			Entity: &pb.Entity{
				Id:       v.PlayerID,
				Name:     v.Name,
				Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
				Position: positionPB(v.Position),
			},
		}}}
	case event.PlayerLeftEvent:
		ev = &pb.Event{Payload: &pb.Event_PlayerLeft{PlayerLeft: &pb.PlayerLeft{
			PlayerId: v.PlayerID,
		}}}
	case event.EntityMovedEvent:
		ev = &pb.Event{Payload: &pb.Event_EntityMoved{EntityMoved: &pb.EntityMoved{
			EntityId: v.EntityID,
			From:     positionPB(v.From),
			To:       positionPB(v.To),
		}}}
	case event.IntentFailedEvent:
		ev = &pb.Event{Payload: &pb.Event_IntentFailed{IntentFailed: intentFailedPB(v)}}
	case event.TimeTickEvent:
		ev = &pb.Event{Payload: &pb.Event_TimeTick{TimeTick: &pb.TimeTick{
			CurrentTick: v.CurrentTick,
			GameTime:    gameTimeToPB(v.GameTime),
		}}}
	default:
		return nil
	}
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Event{Event: ev}}
}

// positionPB converts a domain Position into its wire form.
func positionPB(p geom.Position) *pb.Position {
	return &pb.Position{X: int32(p.X), Y: int32(p.Y)}
}

// intentFailedPB converts a domain IntentFailedEvent into its wire form.
// Reason is forwarded unchanged — it is a stable locale catalog key (see
// game.ReasonIntent* constants) the client resolves through its i18n
// bundle, keeping the server language-agnostic.
func intentFailedPB(e event.IntentFailedEvent) *pb.IntentFailed {
	return &pb.IntentFailed{
		EntityId: e.EntityID,
		Reason:   e.Reason,
	}
}

// structurePBMapping translates the domain StructureKind enum to its wire
// counterpart. Unknown values fall back to UNSPECIFIED.
var structurePBMapping = map[world.StructureKind]pb.Structure{
	world.StructureVillage: pb.Structure_STRUCTURE_VILLAGE,
	world.StructureCastle:  pb.Structure_STRUCTURE_CASTLE,
}

// structureToPB looks up k in structurePBMapping. StructureNone maps
// implicitly to UNSPECIFIED via the default return.
func structureToPB(k world.StructureKind) pb.Structure {
	return lookupOr(structurePBMapping, k, pb.Structure_STRUCTURE_UNSPECIFIED)
}

// terrainPBMapping is the 1:1 translation table from the domain Terrain
// enum to its wire counterpart. Kept as a map (not a switch) so adding a
// new biome is a single-line change and cyclomatic complexity stays flat.
var terrainPBMapping = map[world.Terrain]pb.Terrain{
	world.TerrainPlains:    pb.Terrain_TERRAIN_PLAINS,
	world.TerrainGrassland: pb.Terrain_TERRAIN_GRASSLAND,
	world.TerrainMeadow:    pb.Terrain_TERRAIN_MEADOW,
	world.TerrainBeach:     pb.Terrain_TERRAIN_BEACH,
	world.TerrainDesert:    pb.Terrain_TERRAIN_DESERT,
	world.TerrainSavanna:   pb.Terrain_TERRAIN_SAVANNA,
	world.TerrainForest:    pb.Terrain_TERRAIN_FOREST,
	world.TerrainJungle:    pb.Terrain_TERRAIN_JUNGLE,
	world.TerrainTaiga:     pb.Terrain_TERRAIN_TAIGA,
	world.TerrainTundra:    pb.Terrain_TERRAIN_TUNDRA,
	world.TerrainSnow:      pb.Terrain_TERRAIN_SNOW,
	world.TerrainHills:     pb.Terrain_TERRAIN_HILLS,
	world.TerrainMountain:  pb.Terrain_TERRAIN_MOUNTAIN,
	world.TerrainSnowyPeak: pb.Terrain_TERRAIN_SNOWY_PEAK,
	world.TerrainOcean:     pb.Terrain_TERRAIN_OCEAN,
	world.TerrainDeepOcean: pb.Terrain_TERRAIN_DEEP_OCEAN,

	// Volcanic terrains — multi-tile volcano footprints overwrite the
	// base biome at worldgen query time and travel the wire as ordinary
	// Terrain values so the client renders from per-tile terrain alone.
	world.TerrainVolcanoCore:        pb.Terrain_TERRAIN_VOLCANO_CORE,
	world.TerrainVolcanoCoreDormant: pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT,
	world.TerrainCraterLake:         pb.Terrain_TERRAIN_CRATER_LAKE,
	world.TerrainVolcanoSlope:       pb.Terrain_TERRAIN_VOLCANO_SLOPE,
	world.TerrainAshland:            pb.Terrain_TERRAIN_ASHLAND,
}

// terrainToPB looks up t in terrainPBMapping. Unknown terrains fall back
// to UNSPECIFIED so the client can render a clear "what is this" glyph.
func terrainToPB(t world.Terrain) pb.Terrain {
	return lookupOr(terrainPBMapping, t, pb.Terrain_TERRAIN_UNSPECIFIED)
}

// regionCharacterPBMapping is the 1:1 translation table from the domain
// RegionCharacter enum to its wire counterpart. Kept as a map so adding a
// seventh character (e.g. "Cultured") stays a one-line change.
var regionCharacterPBMapping = map[world.RegionCharacter]pb.RegionCharacter{
	world.RegionNormal:   pb.RegionCharacter_REGION_CHARACTER_NORMAL,
	world.RegionBlighted: pb.RegionCharacter_REGION_CHARACTER_BLIGHTED,
	world.RegionFey:      pb.RegionCharacter_REGION_CHARACTER_FEY,
	world.RegionAncient:  pb.RegionCharacter_REGION_CHARACTER_ANCIENT,
	world.RegionSavage:   pb.RegionCharacter_REGION_CHARACTER_SAVAGE,
	world.RegionHoly:     pb.RegionCharacter_REGION_CHARACTER_HOLY,
	world.RegionWild:     pb.RegionCharacter_REGION_CHARACTER_WILD,
}

// regionCharacterPB translates the domain RegionCharacter enum to its wire
// counterpart. Unknown values fall back to NORMAL — the safe rendering
// default (no tint, no crossing-verb prefix).
func regionCharacterPB(c world.RegionCharacter) pb.RegionCharacter {
	return lookupOr(regionCharacterPBMapping, c, pb.RegionCharacter_REGION_CHARACTER_NORMAL)
}

// regionInfluencePB builds a wire RegionInfluence from the domain struct.
// The field order is intentional: it matches the proto declaration so a
// visual diff against the .proto reads top-to-bottom.
func regionInfluencePB(r world.RegionInfluence) *pb.RegionInfluence {
	return &pb.RegionInfluence{
		Blight:  r.Blight,
		Fae:     r.Fae,
		Ancient: r.Ancient,
		Savage:  r.Savage,
		Holy:    r.Holy,
		Wild:    r.Wild,
	}
}

// regionPB converts a domain Region to its wire form. The anchor
// position is intentionally not sent — clients derive it locally from
// the world seed via geom.AnchorOf to keep the Snapshot region payload
// compact. Name ships as structured, language-agnostic NameParts; the
// client composes the display string locally.
func regionPB(r world.Region) *pb.Region {
	return &pb.Region{
		SuperChunkX: int32(r.Coord.X),
		SuperChunkY: int32(r.Coord.Y),
		Name:        namePartsPB(r.Name),
		Character:   regionCharacterPB(r.Character),
		Influence:   regionInfluencePB(r.Influence),
	}
}

// nameFormatPBMapping translates the leaf parts.Format enum to its wire
// counterpart. Kept as a map so adding a new format stays a one-line
// change and the zero value (UNSPECIFIED) surfaces any out-of-range
// domain value.
var nameFormatPBMapping = map[parts.Format]pb.NameFormat{
	parts.FormatBodyOnly:        pb.NameFormat_NAME_FORMAT_BODY_ONLY,
	parts.FormatCharacterPrefix: pb.NameFormat_NAME_FORMAT_CHARACTER_PREFIX,
	parts.FormatKindPattern:     pb.NameFormat_NAME_FORMAT_KIND_PATTERN,
}

// namePartsPB converts a naming Parts record to its wire form. Only the
// structured fields travel the wire — Body text is produced client-side
// from body_seed against the client's embedded Markov corpus.
func namePartsPB(p parts.Parts) *pb.NameParts {
	return &pb.NameParts{
		Character:    p.Character,
		SubKind:      p.SubKind,
		Format:       lookupOr(nameFormatPBMapping, p.Format, pb.NameFormat_NAME_FORMAT_UNSPECIFIED),
		PrefixIndex:  uint32(p.PrefixIndex),
		PatternIndex: uint32(p.PatternIndex),
		BodySeed:     p.BodySeed,
	}
}

// landmarkPB converts a domain Landmark to its wire form. Kind maps via
// the existing table; Name is a structured NameParts.
func landmarkPB(l world.Landmark) *pb.Landmark {
	return &pb.Landmark{
		Kind: landmarkKindPB(l.Kind),
		Name: namePartsPB(l.Name),
	}
}

// clampViewport enforces the minimum size rule and defaults zero values to
// the server defaults. Called by snapshotOf and indirectly by updateViewport.
func clampViewport(w, h int) (int, int) {
	if w <= 0 {
		w = DefaultViewportWidth
	}
	if h <= 0 {
		h = DefaultViewportHeight
	}
	return max(w, MinViewportWidth), max(h, MinViewportHeight)
}

// monthPBMapping is the 1:1 translation table from the domain Month
// enum to its wire counterpart. Domain values are 1-indexed
// (MonthJanuary = 1 … MonthDecember = 12) so the table covers the twelve
// real months; MonthZero has no wire entry and falls back to
// CALENDAR_MONTH_UNSPECIFIED via lookupOr.
var monthPBMapping = map[calendar.Month]pb.CalendarMonth{
	calendar.MonthJanuary:   pb.CalendarMonth_CALENDAR_MONTH_JANUARY,
	calendar.MonthFebruary:  pb.CalendarMonth_CALENDAR_MONTH_FEBRUARY,
	calendar.MonthMarch:     pb.CalendarMonth_CALENDAR_MONTH_MARCH,
	calendar.MonthApril:     pb.CalendarMonth_CALENDAR_MONTH_APRIL,
	calendar.MonthMay:       pb.CalendarMonth_CALENDAR_MONTH_MAY,
	calendar.MonthJune:      pb.CalendarMonth_CALENDAR_MONTH_JUNE,
	calendar.MonthJuly:      pb.CalendarMonth_CALENDAR_MONTH_JULY,
	calendar.MonthAugust:    pb.CalendarMonth_CALENDAR_MONTH_AUGUST,
	calendar.MonthSeptember: pb.CalendarMonth_CALENDAR_MONTH_SEPTEMBER,
	calendar.MonthOctober:   pb.CalendarMonth_CALENDAR_MONTH_OCTOBER,
	calendar.MonthNovember:  pb.CalendarMonth_CALENDAR_MONTH_NOVEMBER,
	calendar.MonthDecember:  pb.CalendarMonth_CALENDAR_MONTH_DECEMBER,
}

// monthToPB translates the domain Month enum to its wire counterpart.
// The zero value (MonthZero) and any out-of-range value fall back to
// CALENDAR_MONTH_UNSPECIFIED — the proto3 sentinel that marks an
// uninitialised GameTime.
func monthToPB(m calendar.Month) pb.CalendarMonth {
	return lookupOr(monthPBMapping, m, pb.CalendarMonth_CALENDAR_MONTH_UNSPECIFIED)
}

// seasonPBMapping is the 1:1 translation table from the domain Season
// enum to its wire counterpart. The domain iota starts at
// SeasonWinter = 0 while the wire enum reserves 0 for UNSPECIFIED, so
// the table is explicit rather than a cast; any out-of-range domain
// value (corrupted memory) falls back to UNSPECIFIED via lookupOr.
var seasonPBMapping = map[calendar.Season]pb.CalendarSeason{
	calendar.SeasonWinter: pb.CalendarSeason_CALENDAR_SEASON_WINTER,
	calendar.SeasonSpring: pb.CalendarSeason_CALENDAR_SEASON_SPRING,
	calendar.SeasonSummer: pb.CalendarSeason_CALENDAR_SEASON_SUMMER,
	calendar.SeasonAutumn: pb.CalendarSeason_CALENDAR_SEASON_AUTUMN,
}

// seasonToPB translates the domain Season enum to its wire counterpart.
// Out-of-range values fall back to CALENDAR_SEASON_UNSPECIFIED so a
// corrupted value renders as "no season" rather than as a wrong one.
func seasonToPB(s calendar.Season) pb.CalendarSeason {
	return lookupOr(seasonPBMapping, s, pb.CalendarSeason_CALENDAR_SEASON_UNSPECIFIED)
}

// gameTimeToPB converts a domain GameTime into its wire form. Returns a
// non-nil pointer for every input — a zero-value GameTime maps through
// UNSPECIFIED enums so callers never need to nil-check. Pairs with the
// zero-calendar path on the server: a World without WithCalendar
// returns GameTime{}, which wire-encodes to an all-zero pb.GameTime
// that decodes cleanly on the client as "no calendar configured".
func gameTimeToPB(gt calendar.GameTime) *pb.GameTime {
	return &pb.GameTime{
		Year:       gt.Year,
		Month:      monthToPB(gt.Month),
		DayOfMonth: gt.DayOfMonth,
		TickOfDay:  gt.TickOfDay,
		Season:     seasonToPB(gt.Season),
	}
}

// calendarConfigToPB converts a domain Calendar into the wire
// CalendarConfig carried in JoinAccepted. Returns a non-nil pointer
// even for a zero-value Calendar (ticksPerDay = 0) so the wire layout
// is uniform — a client reading ticks_per_day == 0 knows the server
// has no Calendar wired without a separate null check.
//
// The epoch offset travels alongside the cadence fields so the client
// can construct its own calendar.Calendar mirror and derive GameTime
// locally between snapshots via the Snapshot.current_tick anchor.
func calendarConfigToPB(c calendar.Calendar) *pb.CalendarConfig {
	return &pb.CalendarConfig{
		TicksPerDay:     c.TicksPerDay(),
		DaysPerMonth:    int32(c.DaysPerMonth()),
		MonthsPerYear:   int32(c.MonthsPerYear()),
		EpochTickOffset: c.EpochTickOffset(),
	}
}

// landmarkKindPBMapping is the 1:1 translation table from the domain
// LandmarkKind enum to its wire counterpart. Kept as a map (not a switch)
// so adding a new landmark kind stays a single-line change, matching the
// convention used for terrain and region-character mappings above.
var landmarkKindPBMapping = map[world.LandmarkKind]pb.LandmarkKind{
	world.LandmarkNone:           pb.LandmarkKind_LANDMARK_KIND_NONE,
	world.LandmarkTower:          pb.LandmarkKind_LANDMARK_KIND_TOWER,
	world.LandmarkGiantTree:      pb.LandmarkKind_LANDMARK_KIND_GIANT_TREE,
	world.LandmarkStandingStones: pb.LandmarkKind_LANDMARK_KIND_STANDING_STONES,
	world.LandmarkObelisk:        pb.LandmarkKind_LANDMARK_KIND_OBELISK,
	world.LandmarkChasm:          pb.LandmarkKind_LANDMARK_KIND_CHASM,
	world.LandmarkShrine:         pb.LandmarkKind_LANDMARK_KIND_SHRINE,
}

// landmarkKindPB translates the domain LandmarkKind enum to its wire
// counterpart. Unknown values fall back to NONE — the safe zero value
// meaning "no landmark on this tile".
func landmarkKindPB(k world.LandmarkKind) pb.LandmarkKind {
	return lookupOr(landmarkKindPBMapping, k, pb.LandmarkKind_LANDMARK_KIND_NONE)
}

// tileFromDomain builds a wire Tile from a domain tile, applying any
// volcano terrain override before mapping, overlaying the player
// occupant when present, and stamping the landmark onto the tile. The
// terrain / overlays / structure conversions all live in one spot.
// overlays is carried through as an opaque bitmask — the domain and
// the client agree on flag values. A zero-value Landmark (Kind ==
// LandmarkNone) yields a nil wire landmark and is therefore omitted
// from the encoding.
//
// override is a per-tile terrain substitution from the volcano
// pipeline — when hasOverride is false the base Terrain is used
// unchanged. Passing the override in explicitly (rather than
// re-querying the world from inside the mapper) keeps tileFromDomain
// deterministic and unit-testable without a world.
func tileFromDomain(t world.Tile, landmark world.Landmark, override world.Terrain, hasOverride bool) *pb.Tile {
	terrain := t.Terrain
	if hasOverride {
		terrain = override
	}
	out := &pb.Tile{
		Terrain:   terrainToPB(terrain),
		Overlays:  uint32(t.Overlays),
		Structure: structureToPB(t.Structure),
	}
	if landmark.Kind != world.LandmarkNone {
		out.Landmark = landmarkPB(landmark)
	}
	if p, ok := t.Occupant.(*entity.Player); ok && p != nil {
		out.Occupant = pb.OccupantKind_OCCUPANT_PLAYER
		out.EntityId = p.ID
	}
	return out
}

// landmarkAtTile looks up the landmark at the given world coordinate
// by fetching the containing super-chunk's landmark slice from lc and
// scanning for a matching Coord. Returns the zero-value Landmark when
// lc is nil, the super-chunk has no landmarks, or no landmark occupies
// this exact tile.
//
// The scan is O(k) where k is the number of landmarks per super-chunk
// (always 4 in the current implementation). A viewport covers at most
// 4 super-chunks, so the total work per snapshot is viewW×viewH×4 =
// ~41×21×4 ≈ 3 444 simple comparisons — well within the <500µs budget.
func landmarkAtTile(lc *landmarkCache, worldX, worldY int) world.Landmark {
	if lc == nil {
		return world.Landmark{}
	}
	sc := geom.WorldToSuperChunk(worldX, worldY)
	for _, l := range lc.LandmarksIn(sc) {
		if l.Coord.X == worldX && l.Coord.Y == worldY {
			return l
		}
	}
	return world.Landmark{}
}

// snapshotOf builds a viewport Snapshot of viewW × viewH tiles centred
// on the given world position. Zero or too-small dimensions are
// replaced by the server defaults via clampViewport. The returned
// Snapshot also carries the region covering center — resolved from
// region on the caller's side so the cache is owned by the service,
// not re-entered per snapshot here. Pass a nil region when no
// RegionSource is configured (tests, legacy paths) and the Snapshot
// omits the region field. Pass a nil lc when no LandmarkSource is
// configured; tiles will carry no landmark. Pass a nil vc when no
// VolcanoSource is configured; tiles will carry their base terrain
// unchanged.
func snapshotOf(w *world.World, center geom.Position, viewW, viewH int, region *pb.Region, lc *landmarkCache, vc *volcanoCache) *pb.Snapshot {
	viewW, viewH = clampViewport(viewW, viewH)
	halfW := viewW / 2
	halfH := viewH / 2
	originX := center.X - halfW
	originY := center.Y - halfH
	tiles := make([]*pb.Tile, 0, viewW*viewH)
	for dy := range viewH {
		for dx := range viewW {
			p := geom.Position{X: originX + dx, Y: originY + dy}
			t, _ := w.TileAt(p)
			lm := landmarkAtTile(lc, p.X, p.Y)
			override, hasOverride := volcanoOverrideAtTile(vc, p)
			tiles = append(tiles, tileFromDomain(t, lm, override, hasOverride))
		}
	}
	return &pb.Snapshot{
		Width:       int32(viewW),
		Height:      int32(viewH),
		Origin:      positionPB(geom.Position{X: originX, Y: originY}),
		Tiles:       tiles,
		Entities:    entitiesOf(w),
		Region:      region,
		GameTime:    gameTimeToPB(w.GameTime()),
		CurrentTick: w.CurrentTick(),
	}
}

// volcanoOverrideAtTile returns the override terrain at p via the
// volcano cache, or ("", false) when vc is nil (no volcano source
// configured). Split into a named helper so snapshotOf stays readable
// at a glance and the nil-source branch has one obvious home.
func volcanoOverrideAtTile(vc *volcanoCache, p geom.Position) (world.Terrain, bool) {
	if vc == nil {
		return "", false
	}
	return vc.TerrainOverrideAt(p)
}

// entitiesOf returns one Entity per player currently in the world, sorted
// by ID for stability (World.Players guarantees that).
func entitiesOf(w *world.World) []*pb.Entity {
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
// id, the player's spawn position, the world seed, and the server's
// Calendar cadence. Clients use the seed to construct a local region
// source for per-tile influence sampling and the CalendarConfig to
// render in-game time locally.
func acceptedResponse(playerID string, spawn geom.Position, worldSeed int64, cal calendar.Calendar) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Accepted{Accepted: &pb.JoinAccepted{
		PlayerId:  playerID,
		Spawn:     positionPB(spawn),
		WorldSeed: worldSeed,
		Calendar:  calendarConfigToPB(cal),
	}}}
}

// snapshotResponse wraps a Snapshot into a ServerMessage.
func snapshotResponse(s *pb.Snapshot) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Snapshot{Snapshot: s}}
}

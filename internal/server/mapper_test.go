package server

import (
	"errors"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/event"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestClientMessageToCommandJoin(t *testing.T) {
	msg := &pb.ClientMessage{Payload: &pb.ClientMessage_Join{Join: &pb.JoinRequest{Name: "alice"}}}
	cmd, err := clientMessageToCommand(msg, "pid-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jc, ok := cmd.(world.JoinCmd)
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
	mc, ok := cmd.(world.MoveCmd)
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
	ev := event.PlayerJoinedEvent{PlayerID: "p1", Name: "alice", Position: geom.Position{X: 3, Y: 4}}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_PlayerJoined{
					PlayerJoined: &pb.PlayerJoined{
						Entity: &pb.Entity{
							Id:   "p1",
							Name: "alice",
							Kind: pb.OccupantKind_OCCUPANT_PLAYER,
							Position: &pb.Position{
								X: 3,
								Y: 4,
							},
						},
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(PlayerJoinedEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestEventToServerMessagePlayerLeft(t *testing.T) {
	ev := event.PlayerLeftEvent{PlayerID: "p1"}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_PlayerLeft{
					PlayerLeft: &pb.PlayerLeft{
						PlayerId: "p1",
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(PlayerLeftEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestEventToServerMessageEntityMoved(t *testing.T) {
	ev := event.EntityMovedEvent{
		EntityID: "p1",
		From:     geom.Position{X: 1, Y: 2},
		To:       geom.Position{X: 2, Y: 2},
	}
	got := eventToServerMessage(ev)

	want := &pb.ServerMessage{
		Payload: &pb.ServerMessage_Event{
			Event: &pb.Event{
				Payload: &pb.Event_EntityMoved{
					EntityMoved: &pb.EntityMoved{
						EntityId: "p1",
						From:     &pb.Position{X: 1, Y: 2},
						To:       &pb.Position{X: 2, Y: 2},
					},
				},
			},
		},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("eventToServerMessage(EntityMovedEvent) mismatch (-want +got):\n%s", diff)
	}
}

func TestTerrainToPBMapping(t *testing.T) {
	cases := map[world.Terrain]pb.Terrain{
		world.TerrainPlains:    pb.Terrain_TERRAIN_PLAINS,
		world.TerrainGrassland: pb.Terrain_TERRAIN_GRASSLAND,
		world.TerrainForest:    pb.Terrain_TERRAIN_FOREST,
		world.TerrainMountain:  pb.Terrain_TERRAIN_MOUNTAIN,
		world.TerrainOcean:     pb.Terrain_TERRAIN_OCEAN,
		world.TerrainDeepOcean: pb.Terrain_TERRAIN_DEEP_OCEAN,
		world.TerrainBeach:     pb.Terrain_TERRAIN_BEACH,
		world.TerrainHills:     pb.Terrain_TERRAIN_HILLS,
		world.Terrain(""):      pb.Terrain_TERRAIN_UNSPECIFIED,
		world.Terrain("xyz"):   pb.Terrain_TERRAIN_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

func TestStructureToPBMapping(t *testing.T) {
	cases := map[world.StructureKind]pb.Structure{
		world.StructureVillage:     pb.Structure_STRUCTURE_VILLAGE,
		world.StructureCastle:      pb.Structure_STRUCTURE_CASTLE,
		world.StructureNone:        pb.Structure_STRUCTURE_UNSPECIFIED,
		world.StructureKind("xyz"): pb.Structure_STRUCTURE_UNSPECIFIED,
	}
	for in, want := range cases {
		if got := structureToPB(in); got != want {
			t.Errorf("structureToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

// villageTileSource is a TileSource that paints a village over plains at a
// fixed target coordinate and plain plains everywhere else. Used by
// TestSnapshotOfIncludesStructures to assert the wire Snapshot carries the
// structure field through.
type villageTileSource struct {
	target geom.Position
}

func (s villageTileSource) TileAt(x, y int) world.Tile {
	if (geom.Position{X: x, Y: y}) == s.target {
		return world.Tile{Terrain: world.TerrainPlains, Structure: world.StructureVillage}
	}
	return world.Tile{Terrain: world.TerrainPlains}
}

func TestSnapshotOfIncludesStructures(t *testing.T) {
	target := geom.Position{X: 3, Y: 4}
	w := world.NewWorldFromSource(villageTileSource{target: target})

	// Centre the viewport on the target so the local index is trivially
	// computable from the viewport dimensions.
	snap := snapshotOf(w, target, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)
	localX := int(snap.GetWidth()) / 2
	localY := int(snap.GetHeight()) / 2
	idx := localY*int(snap.GetWidth()) + localX

	tiles := snap.GetTiles()
	if idx >= len(tiles) {
		t.Fatalf("target index %d out of range (%d tiles)", idx, len(tiles))
	}

	want := &pb.Tile{
		Terrain:   pb.Terrain_TERRAIN_PLAINS,
		Structure: pb.Structure_STRUCTURE_VILLAGE,
	}
	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, tiles[idx], opts...); diff != "" {
		t.Fatalf("snapshotOf centre tile mismatch (-want +got):\n%s", diff)
	}
}

func TestSnapshotOfShape(t *testing.T) {
	w := worldgen.NewWorld(42)
	events, err := w.ApplyCommand(world.JoinCmd{PlayerID: "p1", Name: "alice"})
	if err != nil {
		t.Fatalf("apply join: %v", err)
	}
	spawn := events[0].(event.PlayerJoinedEvent).Position

	got := snapshotOf(w, spawn, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	// Verify structural fields via cmp.Diff; tiles are verified by count only
	// since their content is world-seed-dependent.
	wantOrigin := &pb.Position{
		X: int32(spawn.X - DefaultViewportWidth/2),
		Y: int32(spawn.Y - DefaultViewportHeight/2),
	}
	wantEntities := []*pb.Entity{
		{Id: "p1", Name: "alice", Kind: pb.OccupantKind_OCCUPANT_PLAYER, Position: got.GetEntities()[0].GetPosition()},
	}

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(wantOrigin, got.GetOrigin(), opts...); diff != "" {
		t.Fatalf("snapshotOf origin mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(wantEntities, got.GetEntities(), opts...); diff != "" {
		t.Fatalf("snapshotOf entities mismatch (-want +got):\n%s", diff)
	}
	if got.GetWidth() != int32(DefaultViewportWidth) || got.GetHeight() != int32(DefaultViewportHeight) {
		t.Fatalf("snapshot size: %dx%d, want %dx%d",
			got.GetWidth(), got.GetHeight(), DefaultViewportWidth, DefaultViewportHeight)
	}
	if len(got.GetTiles()) != DefaultViewportWidth*DefaultViewportHeight {
		t.Fatalf("snapshot tile count: got %d, want %d",
			len(got.GetTiles()), DefaultViewportWidth*DefaultViewportHeight)
	}
}

// plainsTileSource is a TileSource that paints pure plains everywhere.
// Used by the volcano-override snapshot tests so the base biome is
// predictable and any deviation is attributable to the override.
type plainsTileSource struct{}

func (plainsTileSource) TileAt(x, y int) world.Tile {
	_ = x
	_ = y
	return world.Tile{Terrain: world.TerrainPlains}
}

// fakeOverrideVolcanoSource is a test-only world.VolcanoSource that
// returns the configured overrides map for TerrainOverrideAt and an
// empty slice for VolcanoAt. The snapshot path only reads
// TerrainOverrideAt, so VolcanoAt is left as a no-op that satisfies
// the interface without additional fixture wiring.
type fakeOverrideVolcanoSource struct {
	overrides map[geom.Position]world.Terrain
}

func (f fakeOverrideVolcanoSource) VolcanoAt(sc geom.SuperChunkCoord) []world.Volcano {
	_ = sc
	return nil
}

func (f fakeOverrideVolcanoSource) TerrainOverrideAt(p geom.Position) (world.Terrain, bool) {
	t, ok := f.overrides[p]
	return t, ok
}

// TestSnapshotOf_VolcanoTerrainOverride verifies the volcano override
// reaches the wire. A world built with a base plains tile source plus a
// volcano source that overrides one tile with TerrainVolcanoCore must
// emit TERRAIN_VOLCANO_CORE at that tile's snapshot slot while every
// other slot stays TERRAIN_PLAINS.
func TestSnapshotOf_VolcanoTerrainOverride(t *testing.T) {
	target := geom.Position{X: 2, Y: 3}
	overrides := map[geom.Position]world.Terrain{
		target: world.TerrainVolcanoCore,
	}
	src := fakeOverrideVolcanoSource{overrides: overrides}
	w := world.NewWorldFromSource(plainsTileSource{}, world.WithVolcanoSource(src))
	vc := newVolcanoCache(src, DefaultVolcanoCacheCapacity)

	// Centre the viewport on target so the index is trivial.
	snap := snapshotOf(w, target, DefaultViewportWidth, DefaultViewportHeight, nil, nil, vc)
	localX := int(snap.GetWidth()) / 2
	localY := int(snap.GetHeight()) / 2
	idx := localY*int(snap.GetWidth()) + localX

	tiles := snap.GetTiles()
	if idx >= len(tiles) {
		t.Fatalf("target index %d out of range (%d tiles)", idx, len(tiles))
	}
	if got := tiles[idx].GetTerrain(); got != pb.Terrain_TERRAIN_VOLCANO_CORE {
		t.Fatalf("override terrain at target: want %v, got %v",
			pb.Terrain_TERRAIN_VOLCANO_CORE, got)
	}

	// All other tiles must carry the base plains terrain; a stray
	// override elsewhere would indicate a wiring bug.
	for i, tile := range tiles {
		if i == idx {
			continue
		}
		if got := tile.GetTerrain(); got != pb.Terrain_TERRAIN_PLAINS {
			t.Fatalf("tile %d: want base %v, got %v", i, pb.Terrain_TERRAIN_PLAINS, got)
		}
	}
}

// TestSnapshotOf_NilVolcanoCache_NoOverride verifies the mapper
// tolerates a nil volcanoCache and emits base biomes unchanged for
// every tile.
func TestSnapshotOf_NilVolcanoCache_NoOverride(t *testing.T) {
	centre := geom.Position{X: 0, Y: 0}
	w := world.NewWorldFromSource(plainsTileSource{})

	snap := snapshotOf(w, centre, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	for i, tile := range snap.GetTiles() {
		if got := tile.GetTerrain(); got != pb.Terrain_TERRAIN_PLAINS {
			t.Fatalf("tile %d with nil vc: want %v, got %v",
				i, pb.Terrain_TERRAIN_PLAINS, got)
		}
	}
}

// TestFillTile_OverrideApplied verifies fillTile swaps the base terrain
// for the override when hasOverride is true, and leaves it unchanged
// when hasOverride is false. Unit-level so a future refactor cannot
// silently drop the override branch.
func TestFillTile_OverrideApplied(t *testing.T) {
	base := world.Tile{Terrain: world.TerrainForest}
	lm := world.Landmark{}

	var withOverride pb.Tile
	fillTile(&withOverride, base, lm, world.TerrainVolcanoCore, true)
	if got := withOverride.GetTerrain(); got != pb.Terrain_TERRAIN_VOLCANO_CORE {
		t.Fatalf("override applied: want %v, got %v",
			pb.Terrain_TERRAIN_VOLCANO_CORE, got)
	}

	var withoutOverride pb.Tile
	fillTile(&withoutOverride, base, lm, world.TerrainVolcanoCore, false)
	if got := withoutOverride.GetTerrain(); got != pb.Terrain_TERRAIN_FOREST {
		t.Fatalf("override ignored: want %v, got %v",
			pb.Terrain_TERRAIN_FOREST, got)
	}
}

// TestTerrainToPB_VolcanicMapping verifies every volcanic domain
// terrain maps to its proto counterpart. A regression here would mean
// the server silently emits TERRAIN_UNSPECIFIED for volcano tiles and
// the client would render the fallback glyph.
func TestTerrainToPB_VolcanicMapping(t *testing.T) {
	cases := map[world.Terrain]pb.Terrain{
		world.TerrainVolcanoCore:        pb.Terrain_TERRAIN_VOLCANO_CORE,
		world.TerrainVolcanoCoreDormant: pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT,
		world.TerrainCraterLake:         pb.Terrain_TERRAIN_CRATER_LAKE,
		world.TerrainVolcanoSlope:       pb.Terrain_TERRAIN_VOLCANO_SLOPE,
		world.TerrainAshland:            pb.Terrain_TERRAIN_ASHLAND,
	}
	for in, want := range cases {
		if got := terrainToPB(in); got != want {
			t.Errorf("terrainToPB(%q): want %v, got %v", string(in), want, got)
		}
	}
}

// TestGameTimeToPB_RoundTrip verifies gameTimeToPB copies every scalar
// field verbatim and routes Month / Season through the translation
// tables so a known domain fixture matches its expected wire form
// field-for-field.
func TestGameTimeToPB_RoundTrip(t *testing.T) {
	gt := calendar.GameTime{
		Year:       1042,
		Month:      calendar.MonthOctober,
		DayOfMonth: 7,
		TickOfDay:  123,
		Season:     calendar.SeasonAutumn,
	}
	want := &pb.GameTime{
		Year:       1042,
		Month:      pb.CalendarMonth_CALENDAR_MONTH_OCTOBER,
		DayOfMonth: 7,
		TickOfDay:  123,
		Season:     pb.CalendarSeason_CALENDAR_SEASON_AUTUMN,
	}
	got := gameTimeToPB(gt)

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("gameTimeToPB mismatch (-want +got):\n%s", diff)
	}
}

// TestMonthToPB_CoversEveryMonth verifies every valid domain Month maps
// to its specific wire counterpart — never to UNSPECIFIED. A regression
// here would surface as a client rendering the calendar with blank
// month labels.
func TestMonthToPB_CoversEveryMonth(t *testing.T) {
	cases := map[calendar.Month]pb.CalendarMonth{
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
	for in, want := range cases {
		got := monthToPB(in)
		if got == pb.CalendarMonth_CALENDAR_MONTH_UNSPECIFIED {
			t.Errorf("monthToPB(%v) returned UNSPECIFIED, want %v", in, want)
		}
		if got != want {
			t.Errorf("monthToPB(%v): want %v, got %v", in, want, got)
		}
	}
	// MonthZero is the "not set" sentinel; expect UNSPECIFIED.
	if got := monthToPB(calendar.MonthZero); got != pb.CalendarMonth_CALENDAR_MONTH_UNSPECIFIED {
		t.Errorf("monthToPB(MonthZero): want UNSPECIFIED, got %v", got)
	}
}

// TestSeasonToPB_CoversEverySeason verifies every valid domain Season
// maps to its specific wire counterpart — never to UNSPECIFIED. A
// regression here would mean season-tinted UI renders as uncoloured.
func TestSeasonToPB_CoversEverySeason(t *testing.T) {
	cases := map[calendar.Season]pb.CalendarSeason{
		calendar.SeasonWinter: pb.CalendarSeason_CALENDAR_SEASON_WINTER,
		calendar.SeasonSpring: pb.CalendarSeason_CALENDAR_SEASON_SPRING,
		calendar.SeasonSummer: pb.CalendarSeason_CALENDAR_SEASON_SUMMER,
		calendar.SeasonAutumn: pb.CalendarSeason_CALENDAR_SEASON_AUTUMN,
	}
	for in, want := range cases {
		got := seasonToPB(in)
		if got == pb.CalendarSeason_CALENDAR_SEASON_UNSPECIFIED {
			t.Errorf("seasonToPB(%v) returned UNSPECIFIED, want %v", in, want)
		}
		if got != want {
			t.Errorf("seasonToPB(%v): want %v, got %v", in, want, got)
		}
	}
}

// TestCalendarConfigToPB verifies calendarConfigToPB copies the three
// cadence values into the wire shape. Uses DefaultCalendarConfig so a
// future tuning change in the domain ripples into this assertion.
func TestCalendarConfigToPB(t *testing.T) {
	const epoch = int64(987654321)
	cal := calendar.NewCalendar(
		calendar.DefaultCalendarConfig.TicksPerDay,
		calendar.DefaultCalendarConfig.DaysPerMonth,
		calendar.DefaultCalendarConfig.MonthsPerYear,
		epoch,
	)
	want := &pb.CalendarConfig{
		TicksPerDay:     calendar.DefaultCalendarConfig.TicksPerDay,
		DaysPerMonth:    int32(calendar.DefaultCalendarConfig.DaysPerMonth),
		MonthsPerYear:   int32(calendar.DefaultCalendarConfig.MonthsPerYear),
		EpochTickOffset: epoch,
	}
	got := calendarConfigToPB(cal)

	opts := []cmp.Option{protocmp.Transform()}
	if diff := cmp.Diff(want, got, opts...); diff != "" {
		t.Fatalf("calendarConfigToPB mismatch (-want +got):\n%s", diff)
	}
}

// TestSnapshotOf_IncludesGameTime verifies snapshotOf populates
// GameTime from the world's configured Calendar. With
// NewCalendar(600, 10, 12, 0) and a freshly built world, Derive(0)
// yields Year 0, January, DayOfMonth 1 — that's what must reach the
// wire.
func TestSnapshotOf_IncludesGameTime(t *testing.T) {
	centre := geom.Position{X: 0, Y: 0}
	cal := calendar.NewCalendar(600, 10, 12, 0)
	w := world.NewWorldFromSource(plainsTileSource{}, world.WithCalendar(cal))

	snap := snapshotOf(w, centre, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	gt := snap.GetGameTime()
	if gt == nil {
		t.Fatalf("snapshot GameTime is nil, want populated")
	}
	if gt.GetYear() != 0 {
		t.Errorf("GameTime.Year: want 0, got %d", gt.GetYear())
	}
	if gt.GetMonth() != pb.CalendarMonth_CALENDAR_MONTH_JANUARY {
		t.Errorf("GameTime.Month: want JANUARY, got %v", gt.GetMonth())
	}
	if gt.GetDayOfMonth() != 1 {
		t.Errorf("GameTime.DayOfMonth: want 1, got %d", gt.GetDayOfMonth())
	}
	if gt.GetTickOfDay() != 0 {
		t.Errorf("GameTime.TickOfDay: want 0, got %d", gt.GetTickOfDay())
	}
	if gt.GetSeason() != pb.CalendarSeason_CALENDAR_SEASON_WINTER {
		t.Errorf("GameTime.Season: want WINTER (Jan is winter), got %v", gt.GetSeason())
	}
	// CurrentTick on a fresh world is 0 — the snapshot carries the
	// raw counter so the client can extrapolate GameTime between
	// snapshots via wall-clock elapsed × server tick rate.
	if got := snap.GetCurrentTick(); got != 0 {
		t.Errorf("Snapshot.CurrentTick on fresh world: got %d, want 0", got)
	}
}

// TestSnapshotOf_IncludesCurrentTick asserts Snapshot.CurrentTick
// tracks the world's tick counter — after N manual Tick calls the wire
// field equals N. This is the anchor the client uses to extrapolate
// GameTime between snapshots.
func TestSnapshotOf_IncludesCurrentTick(t *testing.T) {
	centre := geom.Position{X: 0, Y: 0}
	cal := calendar.NewCalendar(600, 10, 12, 0)
	w := world.NewWorldFromSource(plainsTileSource{}, world.WithCalendar(cal))

	const want = 5
	for i := 0; i < want; i++ {
		w.Tick()
	}

	snap := snapshotOf(w, centre, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)
	if got := snap.GetCurrentTick(); got != int64(want) {
		t.Errorf("Snapshot.CurrentTick after %d ticks: got %d, want %d", want, got, want)
	}
}

// TestSnapshotOf_NoCalendar_EmitsZeroGameTime verifies a World built
// without WithCalendar emits the zero-value GameTime on the wire:
// every numeric field zero, Month = UNSPECIFIED (the discriminator the
// client checks to detect "server has no calendar"). Season is the
// documented exception: the domain enum is iota-based with
// SeasonWinter = 0, so the zero-value Season maps through
// seasonPBMapping to CALENDAR_SEASON_WINTER rather than UNSPECIFIED.
// Clients keying off calendar presence must inspect Month, not Season.
func TestSnapshotOf_NoCalendar_EmitsZeroGameTime(t *testing.T) {
	centre := geom.Position{X: 0, Y: 0}
	w := world.NewWorldFromSource(plainsTileSource{})

	snap := snapshotOf(w, centre, DefaultViewportWidth, DefaultViewportHeight, nil, nil, nil)

	gt := snap.GetGameTime()
	if gt == nil {
		t.Fatalf("snapshot GameTime is nil, want non-nil zero value")
	}
	if gt.GetYear() != 0 {
		t.Errorf("GameTime.Year: want 0, got %d", gt.GetYear())
	}
	if gt.GetMonth() != pb.CalendarMonth_CALENDAR_MONTH_UNSPECIFIED {
		t.Errorf("GameTime.Month: want UNSPECIFIED, got %v", gt.GetMonth())
	}
	if gt.GetDayOfMonth() != 0 {
		t.Errorf("GameTime.DayOfMonth: want 0, got %d", gt.GetDayOfMonth())
	}
	if gt.GetTickOfDay() != 0 {
		t.Errorf("GameTime.TickOfDay: want 0, got %d", gt.GetTickOfDay())
	}
	// Season falls out of the iota-based domain enum as SeasonWinter
	// (value 0); the mapper reports that faithfully. The calendar-
	// presence discriminator is Month, not Season.
	if gt.GetSeason() != pb.CalendarSeason_CALENDAR_SEASON_WINTER {
		t.Errorf("GameTime.Season: want WINTER (domain zero-value), got %v", gt.GetSeason())
	}
}

// TestEventToServerMessage_TimeTick verifies the mapper wraps a domain
// TimeTickEvent in the right oneof branches and carries the GameTime
// payload intact. Catches any future drift in the wire shape
// independently of the DoTick emission cadence.
func TestEventToServerMessage_TimeTick(t *testing.T) {
	t.Parallel()
	ev := event.TimeTickEvent{
		CurrentTick: 42,
		GameTime: calendar.GameTime{
			Year:       1042,
			Month:      calendar.MonthOctober,
			DayOfMonth: 15,
			TickOfDay:  120,
			Season:     calendar.SeasonAutumn,
		},
		AtTick: 42,
	}
	msg := eventToServerMessage(ev)
	if msg == nil {
		t.Fatalf("eventToServerMessage(TimeTickEvent) returned nil")
	}
	tt := msg.GetEvent().GetTimeTick()
	if tt == nil {
		t.Fatalf("expected TimeTick payload, got %T", msg.GetEvent().GetPayload())
	}
	if tt.GetCurrentTick() != 42 {
		t.Errorf("TimeTick.CurrentTick: got %d, want 42", tt.GetCurrentTick())
	}
	gt := tt.GetGameTime()
	if gt == nil {
		t.Fatalf("TimeTick.GameTime is nil, want populated")
	}
	if gt.GetMonth() != pb.CalendarMonth_CALENDAR_MONTH_OCTOBER {
		t.Errorf("TimeTick.GameTime.Month: got %v, want OCTOBER", gt.GetMonth())
	}
	if gt.GetDayOfMonth() != 15 {
		t.Errorf("TimeTick.GameTime.DayOfMonth: got %d, want 15", gt.GetDayOfMonth())
	}
	if gt.GetSeason() != pb.CalendarSeason_CALENDAR_SEASON_AUTUMN {
		t.Errorf("TimeTick.GameTime.Season: got %v, want AUTUMN", gt.GetSeason())
	}
}

// TestDoTick_EmitsTimeTickEverySecond verifies Service.DoTick fans out
// a TimeTick broadcast once per wall-clock second — i.e. exactly when
// CurrentTick is a multiple of 10 under the 10 Hz server cadence.
// Ticking 20 times must produce exactly 2 TimeTick broadcasts (on
// tick 10 and tick 20); the test subscribes to the service's hub so
// the same broadcast path the client sees is exercised.
func TestDoTick_EmitsTimeTickEverySecond(t *testing.T) {
	t.Parallel()
	cal := calendar.NewCalendar(600, 10, 12, 0)
	w := world.NewWorldFromSource(plainsTileSource{}, world.WithCalendar(cal))
	svc := NewService(w, silentLog())

	outbox, unsub := svc.hub.Subscribe("observer")
	defer unsub()

	const tickCount = 20
	for i := 0; i < tickCount; i++ {
		svc.DoTick()
	}

	var timeTicks int
	// Drain the outbox non-blockingly: every DoTick that emits a
	// TimeTick pushes one ServerMessage; we assert the count rather
	// than exact ordering.
	for {
		select {
		case msg := <-outbox:
			if msg.GetEvent().GetTimeTick() != nil {
				timeTicks++
			}
		default:
			if timeTicks != tickCount/timeTickEveryNTicks {
				t.Fatalf("TimeTick broadcasts after %d ticks: got %d, want %d",
					tickCount, timeTicks, tickCount/timeTickEveryNTicks)
			}
			return
		}
	}
}

// TestDoTick_TimeTickCarriesCurrentTickAndGameTime verifies the payload
// on the emitted TimeTick matches the world's authoritative counter
// and derived GameTime at emission time. On tick 10 with the default
// cadence (600 ticks/day) CurrentTick = 10 and Derive(10) lives on
// DayOfMonth 1, Month January, Year 0 — anchoring the test against
// the calendar math keeps the check meaningful independent of the
// broadcast cadence.
func TestDoTick_TimeTickCarriesCurrentTickAndGameTime(t *testing.T) {
	t.Parallel()
	cal := calendar.NewCalendar(600, 10, 12, 0)
	w := world.NewWorldFromSource(plainsTileSource{}, world.WithCalendar(cal))
	svc := NewService(w, silentLog())

	outbox, unsub := svc.hub.Subscribe("observer")
	defer unsub()

	for i := 0; i < timeTickEveryNTicks; i++ {
		svc.DoTick()
	}

	var found *pb.TimeTick
	for {
		select {
		case msg := <-outbox:
			if tt := msg.GetEvent().GetTimeTick(); tt != nil {
				found = tt
			}
		default:
			goto drained
		}
	}
drained:
	if found == nil {
		t.Fatalf("no TimeTick broadcast observed after %d ticks", timeTickEveryNTicks)
	}
	if got := found.GetCurrentTick(); got != int64(timeTickEveryNTicks) {
		t.Errorf("TimeTick.CurrentTick: got %d, want %d", got, timeTickEveryNTicks)
	}
	gt := found.GetGameTime()
	if gt == nil {
		t.Fatalf("TimeTick.GameTime is nil, want populated")
	}
	wantDerived := cal.Derive(int64(timeTickEveryNTicks))
	if gt.GetYear() != wantDerived.Year {
		t.Errorf("TimeTick.GameTime.Year: got %d, want %d", gt.GetYear(), wantDerived.Year)
	}
	if gt.GetDayOfMonth() != wantDerived.DayOfMonth {
		t.Errorf("TimeTick.GameTime.DayOfMonth: got %d, want %d",
			gt.GetDayOfMonth(), wantDerived.DayOfMonth)
	}
}

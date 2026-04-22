package ui

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/naming"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

func TestPositionFromPB(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   *pb.Position
		want game.Position
	}{
		{"nil returns origin", nil, game.Position{}},
		{"positive", &pb.Position{X: 3, Y: 5}, game.Position{X: 3, Y: 5}},
		{"negative", &pb.Position{X: -1, Y: -2}, game.Position{X: -1, Y: -2}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := positionFromPB(tc.in)
			if got != tc.want {
				t.Fatalf("positionFromPB(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// newTestModel returns a Model whose local viewport covers world columns
// 10..12 and rows 10..11. Picking a non-zero origin catches index-math
// bugs that a (0,0)-origin viewport would hide.
func newTestModel() *Model {
	m := &Model{
		players: make(map[string]playerInfo),
		width:   3,
		height:  2,
		origin:  game.Position{X: 10, Y: 10},
	}
	tiles := make([]*pb.Tile, 6)
	for i := range tiles {
		tiles[i] = &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS}
	}
	m.tiles = tiles
	return m
}

func TestApplySnapshotResetsState(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.players["stale"] = playerInfo{ID: "stale"}
	m.logLines = []logEntry{{Text: "old", Kind: logKindDefault}}

	snap := &pb.Snapshot{
		Width:  2,
		Height: 2,
		Origin: &pb.Position{X: 5, Y: 5},
		Tiles: []*pb.Tile{
			{Terrain: pb.Terrain_TERRAIN_MOUNTAIN},
			{Terrain: pb.Terrain_TERRAIN_PLAINS},
			{Terrain: pb.Terrain_TERRAIN_PLAINS, Occupant: pb.OccupantKind_OCCUPANT_PLAYER, EntityId: "a"},
			{Terrain: pb.Terrain_TERRAIN_OCEAN},
		},
		Entities: []*pb.Entity{
			{Id: "a", Name: "alice", Kind: pb.OccupantKind_OCCUPANT_PLAYER, Position: &pb.Position{X: 5, Y: 6}},
		},
	}
	applySnapshot(m, snap)

	if m.width != 2 || m.height != 2 {
		t.Fatalf("dims = %dx%d, want 2x2", m.width, m.height)
	}
	if m.origin != (game.Position{X: 5, Y: 5}) {
		t.Fatalf("origin = %+v, want (5,5)", m.origin)
	}
	if len(m.tiles) != 4 {
		t.Fatalf("tiles len = %d, want 4", len(m.tiles))
	}
	if _, ok := m.players["stale"]; ok {
		t.Fatalf("stale player was not cleared")
	}
	got, ok := m.players["a"]
	if !ok {
		t.Fatalf("entity a missing from players map")
	}
	if got.Name != "alice" || got.Pos != (game.Position{X: 5, Y: 6}) {
		t.Fatalf("entity a = %+v", got)
	}
}

func TestApplyEventPlayerJoined(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	ev := &pb.Event{
		Payload: &pb.Event_PlayerJoined{
			PlayerJoined: &pb.PlayerJoined{
				Entity: &pb.Entity{
					Id:       "bob",
					Name:     "bob",
					Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
					Position: &pb.Position{X: 11, Y: 10}, // local (1,0) given origin (10,10)
				},
			},
		},
	}
	applyEvent(m, ev)

	if _, ok := m.players["bob"]; !ok {
		t.Fatalf("bob not added to players map")
	}
	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(m.logLines))
	}
	tile := m.tiles[1] // local (1,0)
	if tile.GetEntityId() != "bob" || tile.GetOccupant() != pb.OccupantKind_OCCUPANT_PLAYER {
		t.Fatalf("tile local(1,0) = %+v, want bob occupant", tile)
	}
}

func TestApplyEventEntityMovedTracksMyPosition(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.myID = "me"
	m.players["me"] = playerInfo{ID: "me", Name: "me", Pos: game.Position{X: 10, Y: 10}}
	m.tiles[0].Occupant = pb.OccupantKind_OCCUPANT_PLAYER
	m.tiles[0].EntityId = "me"

	ev := &pb.Event{
		Payload: &pb.Event_EntityMoved{
			EntityMoved: &pb.EntityMoved{
				EntityId: "me",
				From:     &pb.Position{X: 10, Y: 10},
				To:       &pb.Position{X: 11, Y: 10},
			},
		},
	}
	applyEvent(m, ev)

	if pos := m.players["me"].Pos; pos != (game.Position{X: 11, Y: 10}) {
		t.Fatalf("my position = %v, want (11,10)", pos)
	}
	if m.tiles[0].GetEntityId() != "" {
		t.Fatalf("old tile still claims me: %+v", m.tiles[0])
	}
	if m.tiles[1].GetEntityId() != "me" {
		t.Fatalf("new tile missing me: %+v", m.tiles[1])
	}
}

func TestApplyEventPlayerLeft(t *testing.T) {
	t.Parallel()
	m := newTestModel()
	m.players["a"] = playerInfo{ID: "a", Name: "alice", Pos: game.Position{X: 12, Y: 11}}
	idx := 1*m.width + 2 // local (2,1)
	m.tiles[idx].Occupant = pb.OccupantKind_OCCUPANT_PLAYER
	m.tiles[idx].EntityId = "a"

	ev := &pb.Event{
		Payload: &pb.Event_PlayerLeft{
			PlayerLeft: &pb.PlayerLeft{PlayerId: "a"},
		},
	}
	applyEvent(m, ev)

	if _, ok := m.players["a"]; ok {
		t.Fatalf("player a not removed")
	}
	if m.tiles[idx].GetEntityId() != "" {
		t.Fatalf("tile still claims a: %+v", m.tiles[idx])
	}
}

func TestAppendLogCap(t *testing.T) {
	t.Parallel()
	m := &Model{}
	for range logLinesCap + 3 {
		m.appendLog("line", logKindDefault)
	}
	if len(m.logLines) != logLinesCap {
		t.Fatalf("log len = %d, want %d", len(m.logLines), logLinesCap)
	}
}

// snapshotWithRegion builds a bare-minimum snapshot carrying just a
// Region, enough to exercise applyRegion without forcing tests to fill
// width/height and the tiles array.
func snapshotWithRegion(r *pb.Region) *pb.Snapshot {
	return &pb.Snapshot{Region: r}
}

// regionFixture builds a wire Region at the given anchor with the
// named character. The returned NameParts uses FormatBodyOnly and a
// fixed BodySeed so composeName produces a deterministic body from the
// embedded Markov corpora — tests that depend on the composed text
// derive their expected value via composeName rather than asserting a
// literal.
func regionFixture(scX, scY int32, character pb.RegionCharacter, bodySeed int64) *pb.Region {
	return &pb.Region{
		SuperChunkX: scX,
		SuperChunkY: scY,
		Character:   character,
		Name: &pb.NameParts{
			Character: regionCharacterKey(character),
			SubKind:   "forest",
			Format:    pb.NameFormat_NAME_FORMAT_BODY_ONLY,
			BodySeed:  bodySeed,
		},
	}
}

func TestCrossingSuppressedOnFirstSnapshot(t *testing.T) {
	t.Parallel()
	m := &Model{
		players: make(map[string]playerInfo),
		lang:    "en",
	}
	r := regionFixture(0, 0, pb.RegionCharacter_REGION_CHARACTER_BLIGHTED, 11)
	snap := snapshotWithRegion(r)
	applySnapshot(m, snap)

	if len(m.logLines) != 0 {
		t.Fatalf("first snapshot emitted %d log lines, want 0: %v", len(m.logLines), m.logLines)
	}
	if !m.initialised {
		t.Fatal("initialised flag not set after first snapshot")
	}
	if m.region == nil || m.region.GetName() == nil {
		t.Fatalf("region not stored on model: %+v", m.region)
	}
}

func TestCrossingNoLogOnSameRegion(t *testing.T) {
	t.Parallel()
	m := &Model{
		players: make(map[string]playerInfo),
		lang:    "en",
	}
	reg := regionFixture(1, 1, pb.RegionCharacter_REGION_CHARACTER_FEY, 22)
	applySnapshot(m, snapshotWithRegion(reg))
	applySnapshot(m, snapshotWithRegion(reg))

	if len(m.logLines) != 0 {
		t.Fatalf("identical regions emitted %d log lines, want 0: %v", len(m.logLines), m.logLines)
	}
}

func TestCrossingEmitsLocalizedLogLineEn(t *testing.T) {
	t.Parallel()
	m := &Model{
		players: make(map[string]playerInfo),
		lang:    "en",
	}
	a := regionFixture(0, 0, pb.RegionCharacter_REGION_CHARACTER_NORMAL, 101)
	b := regionFixture(1, 0, pb.RegionCharacter_REGION_CHARACTER_BLIGHTED, 202)
	applySnapshot(m, snapshotWithRegion(a))
	applySnapshot(m, snapshotWithRegion(b))

	if len(m.logLines) != 1 {
		t.Fatalf("log len = %d, want 1: %v", len(m.logLines), m.logLines)
	}
	got := m.logLines[0].Text
	wantName := composeName(naming.DomainRegion, b.GetName(), "en")
	if !strings.Contains(got, "You feel the weight of") || !strings.Contains(got, wantName) {
		t.Fatalf("crossing log = %q, want English Blighted verb with region name %q", got, wantName)
	}
}

func TestCrossingEmitsLocalizedLogLineRu(t *testing.T) {
	t.Parallel()
	m := &Model{
		players: make(map[string]playerInfo),
		lang:    "ru",
	}
	a := regionFixture(0, 0, pb.RegionCharacter_REGION_CHARACTER_NORMAL, 303)
	b := regionFixture(1, 0, pb.RegionCharacter_REGION_CHARACTER_BLIGHTED, 404)
	applySnapshot(m, snapshotWithRegion(a))
	applySnapshot(m, snapshotWithRegion(b))

	if len(m.logLines) != 1 {
		t.Fatalf("log len = %d, want 1: %v", len(m.logLines), m.logLines)
	}
	got := m.logLines[0].Text
	wantName := composeName(naming.DomainRegion, b.GetName(), "ru")
	if !strings.Contains(got, "тяжесть") || !strings.Contains(got, wantName) {
		t.Fatalf("crossing log = %q, want Russian Blighted verb with region name %q", got, wantName)
	}
}

func TestCrossingCharacterCoverage(t *testing.T) {
	t.Parallel()
	// Walk every character; each must produce a log line and the line
	// must not be the bare catalog key (that would signal a missing
	// entry).
	chars := []pb.RegionCharacter{
		pb.RegionCharacter_REGION_CHARACTER_NORMAL,
		pb.RegionCharacter_REGION_CHARACTER_BLIGHTED,
		pb.RegionCharacter_REGION_CHARACTER_FEY,
		pb.RegionCharacter_REGION_CHARACTER_ANCIENT,
		pb.RegionCharacter_REGION_CHARACTER_SAVAGE,
		pb.RegionCharacter_REGION_CHARACTER_HOLY,
		pb.RegionCharacter_REGION_CHARACTER_WILD,
	}
	for i, c := range chars {
		c := c
		t.Run(c.String(), func(t *testing.T) {
			t.Parallel()
			m := &Model{
				players: make(map[string]playerInfo),
				lang:    "en",
			}
			first := regionFixture(0, 0, pb.RegionCharacter_REGION_CHARACTER_NORMAL, int64(i*2+1))
			second := regionFixture(int32(i+1), 0, c, int64(i*2+2))
			applySnapshot(m, snapshotWithRegion(first))
			applySnapshot(m, snapshotWithRegion(second))
			if len(m.logLines) != 1 {
				t.Fatalf("log len = %d, want 1", len(m.logLines))
			}
			line := m.logLines[0].Text
			key := "crossing." + regionCharacterKey(c)
			if line == key {
				t.Fatalf("log line is raw catalog key %q — catalog entry missing", key)
			}
			wantName := composeName(naming.DomainRegion, second.GetName(), "en")
			if wantName != "" && !strings.Contains(line, wantName) {
				t.Fatalf("log line %q missing composed region name %q", line, wantName)
			}
		})
	}
}

func TestRegionCoordDerivesFromProto(t *testing.T) {
	t.Parallel()
	r := regionFixture(-3, 7, pb.RegionCharacter_REGION_CHARACTER_NORMAL, 0)
	got := regionCoord(r)
	want := game.SuperChunkCoord{X: -3, Y: 7}
	if got != want {
		t.Fatalf("regionCoord = %+v, want %+v", got, want)
	}
}

func TestLookTileKnownAndUnknown(t *testing.T) {
	t.Parallel()
	known := &pb.Tile{Terrain: pb.Terrain_TERRAIN_FOREST}
	r, _ := lookTile(known)
	if r == runeUnspecified {
		t.Fatalf("lookTile(known biome) returned unspecified rune")
	}
	unknown := &pb.Tile{Terrain: pb.Terrain(999)}
	r, _ = lookTile(unknown)
	if r != runeUnspecified {
		t.Fatalf("lookTile(unknown) = %q, want %q", r, runeUnspecified)
	}
}

// TestTerrainRunesCoverAllWireValues asserts every non-UNSPECIFIED Terrain
// enum value has both a rune and a style entry. A future proto addition
// that forgets to update runes.go or styles.go trips this test instead of
// silently rendering as "?" on the player's screen.
func TestTerrainRunesCoverAllWireValues(t *testing.T) {
	t.Parallel()
	for n, name := range pb.Terrain_name {
		if name == "TERRAIN_UNSPECIFIED" {
			continue
		}
		v := pb.Terrain(n)
		if _, ok := terrainRunes[v]; !ok {
			t.Errorf("terrainRunes missing entry for %s", name)
		}
		if _, ok := terrainStyles[v]; !ok {
			t.Errorf("terrainStyles missing entry for %s", name)
		}
	}
}

// TestTerrainRunesVolcanicGlyphsDistinct asserts the five volcanic biomes
// each use a glyph different from every non-volcanic biome. Ashland and
// CraterLake in particular must NOT collide with plains / ocean / river
// glyphs — the whole point of new runes is that a player reading the map
// instantly knows a volcano is on-screen.
func TestTerrainRunesVolcanicGlyphsDistinct(t *testing.T) {
	t.Parallel()
	volcanic := []pb.Terrain{
		pb.Terrain_TERRAIN_VOLCANO_CORE,
		pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT,
		pb.Terrain_TERRAIN_CRATER_LAKE,
		pb.Terrain_TERRAIN_VOLCANO_SLOPE,
		pb.Terrain_TERRAIN_ASHLAND,
	}
	nonVolcanic := map[pb.Terrain]string{}
	for n, name := range pb.Terrain_name {
		v := pb.Terrain(n)
		if name == "TERRAIN_UNSPECIFIED" {
			continue
		}
		isVolcanic := false
		for _, vb := range volcanic {
			if v == vb {
				isVolcanic = true
				break
			}
		}
		if !isVolcanic {
			nonVolcanic[v] = name
		}
	}
	for _, vb := range volcanic {
		r := terrainRunes[vb]
		for nv, name := range nonVolcanic {
			if terrainRunes[nv] == r {
				t.Errorf("volcanic terrain %s rune %q collides with %s",
					pb.Terrain_name[int32(vb)], r, name)
			}
		}
	}
}

// landmarkSnapshot builds a Snapshot that places the local player at
// playerWorld and a landmark of the given kind at landmarkWorld. Both coords
// must fit inside a viewport large enough to hold them; the helper creates a
// viewport that spans both points with a small margin.
func landmarkSnapshot(playerWorld, landmarkWorld game.Position, kind pb.LandmarkKind) *pb.Snapshot {
	// Viewport origin is the top-left of the bounding box minus 1 tile of margin.
	minX := playerWorld.X
	if landmarkWorld.X < minX {
		minX = landmarkWorld.X
	}
	minY := playerWorld.Y
	if landmarkWorld.Y < minY {
		minY = landmarkWorld.Y
	}
	origin := game.Position{X: minX - 1, Y: minY - 1}

	maxX := playerWorld.X
	if landmarkWorld.X > maxX {
		maxX = landmarkWorld.X
	}
	maxY := playerWorld.Y
	if landmarkWorld.Y > maxY {
		maxY = landmarkWorld.Y
	}
	w := maxX - origin.X + 2
	h := maxY - origin.Y + 2

	tiles := make([]*pb.Tile, w*h)
	for i := range tiles {
		tiles[i] = &pb.Tile{Terrain: pb.Terrain_TERRAIN_PLAINS}
	}

	// Place landmark tile.
	lx := landmarkWorld.X - origin.X
	ly := landmarkWorld.Y - origin.Y
	tiles[ly*w+lx] = &pb.Tile{
		Terrain: pb.Terrain_TERRAIN_PLAINS,
		Landmark: &pb.Landmark{
			Kind: kind,
			// A nil Name is safe: composeName returns "" for a nil NameParts.
		},
	}

	return &pb.Snapshot{
		Width:  int32(w),
		Height: int32(h),
		Origin: &pb.Position{X: int32(origin.X), Y: int32(origin.Y)},
		Tiles:  tiles,
		Entities: []*pb.Entity{
			{
				Id:       "me",
				Name:     "hero",
				Kind:     pb.OccupantKind_OCCUPANT_PLAYER,
				Position: &pb.Position{X: int32(playerWorld.X), Y: int32(playerWorld.Y)},
			},
		},
	}
}

// newLandmarkModel returns a Model ready for landmark-approach tests: the
// player ID is "me" and the language is set to lang.
func newLandmarkModel(lang string) *Model {
	return &Model{
		myID:    "me",
		players: make(map[string]playerInfo),
		lang:    lang,
	}
}

// TestDetectLandmarkApproachFires verifies that a landmark 2 tiles (Chebyshev)
// away from the player causes exactly one event-log line after applySnapshot.
func TestDetectLandmarkApproachFires(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("en")
	player := game.Position{X: 10, Y: 10}
	lmPos := game.Position{X: 12, Y: 10} // Chebyshev distance 2

	snap := landmarkSnapshot(player, lmPos, pb.LandmarkKind_LANDMARK_KIND_TOWER)
	applySnapshot(m, snap)

	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d: %v", len(m.logLines), m.logLines)
	}
	if !strings.Contains(m.logLines[0].Text, "tower") && !strings.Contains(m.logLines[0].Text, "Tower") && !strings.Contains(m.logLines[0].Text, "looms") {
		t.Fatalf("approach log %q missing expected tower text", m.logLines[0].Text)
	}
}

// TestDetectLandmarkApproachDebounced verifies that two consecutive snapshots
// with the player still within approachRadius of the same landmark produce only
// one log line — the second snapshot is suppressed by the debounce guard.
func TestDetectLandmarkApproachDebounced(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("en")
	player := game.Position{X: 10, Y: 10}
	lmPos := game.Position{X: 12, Y: 10} // Chebyshev distance 2

	snap := landmarkSnapshot(player, lmPos, pb.LandmarkKind_LANDMARK_KIND_TOWER)
	applySnapshot(m, snap)
	applySnapshot(m, snap) // identical second snapshot — must not re-fire

	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line after duplicate snapshot, got %d: %v", len(m.logLines), m.logLines)
	}
}

// TestDetectLandmarkApproachRearm verifies the full rearm cycle: approach fires,
// player moves beyond approachExitRadius, then re-approach fires again.
func TestDetectLandmarkApproachRearm(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("en")
	lmPos := game.Position{X: 20, Y: 20}

	// First snapshot: player 2 tiles away → fires.
	near := game.Position{X: 18, Y: 20}
	applySnapshot(m, landmarkSnapshot(near, lmPos, pb.LandmarkKind_LANDMARK_KIND_SHRINE))
	if len(m.logLines) != 1 {
		t.Fatalf("first approach: expected 1 log line, got %d", len(m.logLines))
	}

	// Second snapshot: player 6 tiles away (outside exit ring) → no new log.
	far := game.Position{X: 14, Y: 20} // Chebyshev 6
	applySnapshot(m, landmarkSnapshot(far, lmPos, pb.LandmarkKind_LANDMARK_KIND_SHRINE))
	if len(m.logLines) != 1 {
		t.Fatalf("after leaving: expected still 1 log line, got %d", len(m.logLines))
	}

	// Third snapshot: player 2 tiles away again → fires again (rearmed).
	applySnapshot(m, landmarkSnapshot(near, lmPos, pb.LandmarkKind_LANDMARK_KIND_SHRINE))
	if len(m.logLines) != 2 {
		t.Fatalf("rearm approach: expected 2 log lines, got %d: %v", len(m.logLines), m.logLines)
	}
}

// TestDetectLandmarkApproachSkipsNoneKind verifies that a tile carrying
// LandmarkKind_NONE is ignored and produces no log line.
func TestDetectLandmarkApproachSkipsNoneKind(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("en")
	player := game.Position{X: 10, Y: 10}
	lmPos := game.Position{X: 11, Y: 10} // only 1 tile away — very close

	snap := landmarkSnapshot(player, lmPos, pb.LandmarkKind_LANDMARK_KIND_NONE)
	applySnapshot(m, snap)

	if len(m.logLines) != 0 {
		t.Fatalf("NONE kind produced %d log lines, want 0: %v", len(m.logLines), m.logLines)
	}
}

// TestDetectLandmarkApproachLocalizedEn verifies the approach log contains
// expected English text (the tower message starts with "A tower looms").
func TestDetectLandmarkApproachLocalizedEn(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("en")
	player := game.Position{X: 10, Y: 10}
	lmPos := game.Position{X: 12, Y: 10}

	applySnapshot(m, landmarkSnapshot(player, lmPos, pb.LandmarkKind_LANDMARK_KIND_TOWER))

	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(m.logLines))
	}
	if !strings.Contains(m.logLines[0].Text, "looms") {
		t.Fatalf("en approach log %q missing expected English verb 'looms'", m.logLines[0].Text)
	}
}

// TestDetectLandmarkApproachLocalizedRu verifies the approach log contains
// expected Russian text (the tower message contains "возвышается").
func TestDetectLandmarkApproachLocalizedRu(t *testing.T) {
	t.Parallel()
	m := newLandmarkModel("ru")
	player := game.Position{X: 10, Y: 10}
	lmPos := game.Position{X: 12, Y: 10}

	applySnapshot(m, landmarkSnapshot(player, lmPos, pb.LandmarkKind_LANDMARK_KIND_TOWER))

	if len(m.logLines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(m.logLines))
	}
	if !strings.Contains(m.logLines[0].Text, "возвышается") {
		t.Fatalf("ru approach log %q missing expected Russian verb 'возвышается'", m.logLines[0].Text)
	}
}

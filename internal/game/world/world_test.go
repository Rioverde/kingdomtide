package game

import (
	"reflect"
	"testing"
)

// stubLandmarkSource is a minimal LandmarkSource used to verify the
// World delegates LandmarksIn to its configured backend. It records
// the queried coord so the test can assert the World forwarded the
// argument unchanged.
type stubLandmarkSource struct {
	got  SuperChunkCoord
	out  []Landmark
	hits int
}

func (s *stubLandmarkSource) LandmarksIn(sc SuperChunkCoord) []Landmark {
	s.got = sc
	s.hits++
	return s.out
}

func TestWorldLandmarksInNilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	got := w.LandmarksIn(SuperChunkCoord{X: 3, Y: -2})
	if got != nil {
		t.Fatalf("LandmarksIn with nil source = %v, want nil", got)
	}
}

func TestWorldLandmarksInDelegation(t *testing.T) {
	want := []Landmark{
		{Coord: Position{X: 10, Y: 20}, Kind: LandmarkTower},
		{Coord: Position{X: 30, Y: 40}, Kind: LandmarkShrine},
	}
	stub := &stubLandmarkSource{out: want}
	w := NewWorldFromSource(testTiles{}, WithLandmarkSource(stub))

	sc := SuperChunkCoord{X: 7, Y: 11}
	got := w.LandmarksIn(sc)

	if stub.hits != 1 {
		t.Fatalf("source hits = %d, want 1", stub.hits)
	}
	if stub.got != sc {
		t.Fatalf("source received sc = %+v, want %+v", stub.got, sc)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LandmarksIn = %+v, want %+v", got, want)
	}
}

func TestWorld_VolcanoAt_NilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	if got := w.VolcanoAt(SuperChunkCoord{X: 3, Y: -2}); got != nil {
		t.Fatalf("VolcanoAt with nil source = %v, want nil", got)
	}
}

func TestWorld_VolcanoTerrainOverride_NilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	terr, ok := w.VolcanoTerrainOverride(Position{X: 1, Y: 2})
	if ok {
		t.Fatalf("VolcanoTerrainOverride with nil source ok = true, want false")
	}
	if terr != "" {
		t.Fatalf(`VolcanoTerrainOverride with nil source terrain = %q, want ""`, terr)
	}
}

func TestNewWorldInBoundsAlwaysTrue(t *testing.T) {
	w := newTestWorld(testTiles{})
	if !w.InBounds(Position{X: -1e6, Y: 1e6}) {
		t.Fatalf("expected infinite world to report InBounds for any coord")
	}
}

func TestPlayersDefensiveCopyAndSort(t *testing.T) {
	w := newTestWorld(testTiles{})
	for _, id := range []string{"charlie", "alice", "bob"} {
		if _, err := w.ApplyCommand(JoinCmd{PlayerID: id, Name: id}); err != nil {
			t.Fatalf("join %q: %v", id, err)
		}
	}
	got := w.Players()
	if len(got) != 3 {
		t.Fatalf("len(Players()) = %d, want 3", len(got))
	}
	wantOrder := []string{"alice", "bob", "charlie"}
	for i, id := range wantOrder {
		if got[i].ID != id {
			t.Fatalf("Players()[%d].ID = %q, want %q", i, got[i].ID, id)
		}
	}
	got[0] = nil
	again := w.Players()
	if again[0] == nil || again[0].ID != "alice" {
		t.Fatalf("Players() not defensively copied: %+v", again)
	}
}

func TestTerrainPassable(t *testing.T) {
	passable := map[Terrain]bool{
		TerrainPlains:    true,
		TerrainGrassland: true,
		TerrainMeadow:    true,
		TerrainBeach:     true,
		TerrainSavanna:   true,
		TerrainDesert:    true,
		TerrainSnow:      true,
		TerrainTundra:    true,
		TerrainTaiga:     true,
		TerrainForest:    true,
		TerrainJungle:    true,
		TerrainHills:     true,
		TerrainDeepOcean:          false,
		TerrainOcean:              false,
		TerrainMountain:           false,
		TerrainSnowyPeak:          false,
		TerrainVolcanoSlope:       true,
		TerrainAshland:            true,
		TerrainVolcanoCore:        false,
		TerrainVolcanoCoreDormant: false,
		TerrainCraterLake:         false,
	}
	for _, terr := range AllTerrains() {
		want, ok := passable[terr]
		if !ok {
			t.Fatalf("test is missing an expectation for terrain %q", terr)
		}
		if got := terr.Passable(); got != want {
			t.Fatalf("Terrain(%q).Passable() = %v, want %v", terr, got, want)
		}
	}
	if Terrain("").Passable() {
		t.Fatalf(`Terrain("").Passable() = true, want false`)
	}
	if Terrain("garbage").Passable() {
		t.Fatalf(`Terrain("garbage").Passable() = true, want false`)
	}
}

func TestWorldDepositAtNilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	got, ok := w.DepositAt(Position{X: 3, Y: 4})
	if ok {
		t.Fatalf("DepositAt with nil source = ok=true, want false")
	}
	if got != (Deposit{}) {
		t.Fatalf("DepositAt with nil source = %+v, want zero value", got)
	}
}

func TestWorldDepositsInNilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	got := w.DepositsIn(Rect{MinX: 0, MinY: 0, MaxX: 10, MaxY: 10})
	if got != nil {
		t.Fatalf("DepositsIn with nil source = %v, want nil", got)
	}
}

func TestWorldDepositsNearNilSource(t *testing.T) {
	w := newTestWorld(testTiles{})
	got := w.DepositsNear(Position{X: 0, Y: 0}, 5)
	if got != nil {
		t.Fatalf("DepositsNear with nil source = %v, want nil", got)
	}
}

// stubDepositSource captures calls into DepositSource so tests can
// verify the World forwards queries unchanged to its configured backend.
type stubDepositSource struct {
	gotPos    Position
	gotRect   Rect
	gotRadius int
	outAt     Deposit
	outAtOK   bool
	outRect   []Deposit
	outNear   []Deposit
}

func (s *stubDepositSource) DepositAt(p Position) (Deposit, bool) {
	s.gotPos = p
	return s.outAt, s.outAtOK
}
func (s *stubDepositSource) DepositsIn(r Rect) []Deposit {
	s.gotRect = r
	return s.outRect
}
func (s *stubDepositSource) DepositsNear(p Position, radius int) []Deposit {
	s.gotPos = p
	s.gotRadius = radius
	return s.outNear
}

func TestWorldDepositAccessorsDelegate(t *testing.T) {
	stub := &stubDepositSource{
		outAt:   Deposit{Position: Position{X: 1, Y: 2}, Kind: DepositIron, MaxAmount: 10, CurrentAmount: 10},
		outAtOK: true,
		outRect: []Deposit{{Kind: DepositStone}},
		outNear: []Deposit{{Kind: DepositFish}},
	}
	w := NewWorld(tileSourceFn(func(x, y int) Tile { return Tile{Terrain: TerrainPlains} }),
		WithDepositSource(stub))

	if d, ok := w.DepositAt(Position{X: 7, Y: 8}); !ok || d.Kind != DepositIron {
		t.Fatalf("DepositAt did not forward: got %+v ok=%v", d, ok)
	}
	if stub.gotPos != (Position{X: 7, Y: 8}) {
		t.Fatalf("DepositAt: stub got %+v, want (7,8)", stub.gotPos)
	}

	rect := Rect{MinX: -5, MinY: -5, MaxX: 5, MaxY: 5}
	if got := w.DepositsIn(rect); len(got) != 1 || got[0].Kind != DepositStone {
		t.Fatalf("DepositsIn did not forward: got %+v", got)
	}
	if stub.gotRect != rect {
		t.Fatalf("DepositsIn: stub got %+v, want %+v", stub.gotRect, rect)
	}

	if got := w.DepositsNear(Position{X: 0, Y: 0}, 3); len(got) != 1 || got[0].Kind != DepositFish {
		t.Fatalf("DepositsNear did not forward: got %+v", got)
	}
	if stub.gotRadius != 3 {
		t.Fatalf("DepositsNear: stub got radius %d, want 3", stub.gotRadius)
	}
}

// tileSourceFn is a one-off TileSource adapter for stub-based tests.
type tileSourceFn func(x, y int) Tile

func (f tileSourceFn) TileAt(x, y int) Tile { return f(x, y) }

var _ = reflect.DeepEqual // silence unused if tests above shrink

func TestWorldTick_IncrementsCurrentTick(t *testing.T) {
	w := newTestWorld(testTiles{})
	if w.CurrentTick() != 0 {
		t.Fatalf("freshly built world has currentTick=%d, want 0", w.CurrentTick())
	}
	_ = w.Tick()
	if w.CurrentTick() != 1 {
		t.Fatalf("after one Tick, currentTick=%d, want 1", w.CurrentTick())
	}
	for range 10 {
		_ = w.Tick()
	}
	if w.CurrentTick() != 11 {
		t.Fatalf("after 11 Ticks, currentTick=%d, want 11", w.CurrentTick())
	}
}

func TestWorldTick_NoCalendarEmitsNoBoundary(t *testing.T) {
	w := newTestWorld(testTiles{})
	for range 100 {
		events := w.Tick()
		for _, e := range events {
			switch e.(type) {
			case MonthChangedEvent, SeasonChangedEvent, YearStartedEvent:
				t.Fatalf("calendar boundary event emitted without a wired Calendar: %T", e)
			}
		}
	}
}

func TestWorldTick_CalendarBoundaryEvents(t *testing.T) {
	// Short cadence: 2 ticksPerDay, 3 daysPerMonth, 4 monthsPerYear = 24 ticks/year
	cal := NewCalendar(2, 3, 4, 0)
	w := NewWorld(
		tileSourceFn(func(x, y int) Tile { return Tile{Terrain: TerrainPlains} }),
		WithCalendar(cal),
	)

	type counts struct {
		month, season, year int
	}
	observed := map[int64]counts{} // tick -> counts

	for i := 1; i <= 48; i++ { // 2 full years
		events := w.Tick()
		c := counts{}
		for _, e := range events {
			switch ev := e.(type) {
			case MonthChangedEvent:
				c.month++
				if ev.AtTick != int64(i) {
					t.Errorf("tick %d: MonthChanged AtTick=%d", i, ev.AtTick)
				}
			case SeasonChangedEvent:
				c.season++
			case YearStartedEvent:
				c.year++
			}
		}
		observed[int64(i)] = c
	}

	// Month boundaries: every 6 ticks (3 days × 2 t/day) for 48 ticks = 8 rollovers.
	// Season boundaries: every 3 months (once per 18 ticks) = 2 per year × 2 years = 4... wait,
	// seasons derive from month via SeasonOf. With 4 months/year (Jan..Apr), season map:
	//   Jan=Winter, Feb=Winter, Mar=Spring, Apr=Spring
	// So per year we get 1 season change: Feb→Mar (at tick 12, start of month 3).
	// Over 2 years: 4 season changes (one at each Mar start and one at each Jan wrap back to Winter).
	// Actually Apr→Jan is Spring→Winter, so tick 24 (year rollover) fires season change too.
	// Total: season changes at ticks 12, 24, 36, 48 = 4.
	totalMonth, totalSeason, totalYear := 0, 0, 0
	for _, c := range observed {
		totalMonth += c.month
		totalSeason += c.season
		totalYear += c.year
	}
	if totalMonth != 8 {
		t.Errorf("MonthChangedEvent total over 2 years = %d, want 8", totalMonth)
	}
	if totalYear != 2 {
		t.Errorf("YearStartedEvent total over 2 years = %d, want 2", totalYear)
	}
	if totalSeason != 4 {
		t.Errorf("SeasonChangedEvent total over 2 years = %d, want 4", totalSeason)
	}

	// Spot-check: year boundary tick = 24 has all three events.
	yearTick := observed[24]
	if yearTick.year != 1 || yearTick.season != 1 || yearTick.month != 1 {
		t.Errorf("tick 24 (year boundary) counts = %+v, want {1,1,1}", yearTick)
	}
}

func TestWorldGameTime_NoCalendar(t *testing.T) {
	w := newTestWorld(testTiles{})
	gt := w.GameTime()
	if gt != (GameTime{}) {
		t.Errorf("GameTime() without calendar = %+v, want zero value", gt)
	}
}

func TestWorldGameTime_WithCalendar(t *testing.T) {
	cal := NewCalendar(10, 5, 4, 0)
	w := NewWorld(
		tileSourceFn(func(x, y int) Tile { return Tile{Terrain: TerrainPlains} }),
		WithCalendar(cal),
	)
	gt0 := w.GameTime()
	if gt0.Year != 0 || gt0.Month != MonthJanuary || gt0.DayOfMonth != 1 {
		t.Errorf("initial GameTime = %+v, want Year 0 January day 1", gt0)
	}
	for range 50 {
		_ = w.Tick()
	}
	// Tick 50 = day 5, month 1 = (5 days × 10 ticks/day = 50 ticks in month 1, day 5+1=6... wait)
	// actually 50 ticks / 10 = 5 whole days, so dayIdx = 0, totalMonths = 1 → Month = February day 1.
	gt := w.GameTime()
	if gt.Month != MonthFebruary || gt.DayOfMonth != 1 {
		t.Errorf("GameTime after 50 ticks = %+v, want February day 1", gt)
	}
}

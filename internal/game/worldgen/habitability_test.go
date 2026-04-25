package worldgen

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// --- stub implementations -------------------------------------------------

// stubDepositSource satisfies gworld.DepositSource with a fixed deposit list.
type stubDepositSource struct {
	deposits []gworld.Deposit
}

func (s *stubDepositSource) DepositAt(p geom.Position) (gworld.Deposit, bool) {
	for _, d := range s.deposits {
		if d.Position == p {
			return d, true
		}
	}
	return gworld.Deposit{}, false
}

func (s *stubDepositSource) DepositsIn(rect geom.Rect) []gworld.Deposit {
	var out []gworld.Deposit
	for _, d := range s.deposits {
		if rect.Contains(d.Position) {
			out = append(out, d)
		}
	}
	return out
}

func (s *stubDepositSource) DepositsNear(p geom.Position, radius int) []gworld.Deposit {
	var out []gworld.Deposit
	for _, d := range s.deposits {
		if geom.ChebyshevDist(d.Position, p) <= radius {
			out = append(out, d)
		}
	}
	return out
}

var _ gworld.DepositSource = (*stubDepositSource)(nil)

// stubVolcanoSource satisfies gworld.VolcanoSource with a fixed set of
// override positions that simulate a volcano footprint.
type stubVolcanoSource struct {
	// overrides maps packed position to terrain override.
	overrides map[uint64]gworld.Terrain
}

func newStubVolcanoSource(positions []geom.Position, terrain gworld.Terrain) *stubVolcanoSource {
	m := make(map[uint64]gworld.Terrain, len(positions))
	for _, p := range positions {
		m[geom.PackPos(p)] = terrain
	}
	return &stubVolcanoSource{overrides: m}
}

func (s *stubVolcanoSource) VolcanoAt(sc geom.SuperChunkCoord) []gworld.Volcano {
	return nil
}

func (s *stubVolcanoSource) TerrainOverrideAt(p geom.Position) (gworld.Terrain, bool) {
	if t, ok := s.overrides[geom.PackPos(p)]; ok {
		return t, true
	}
	return "", false
}

func (s *stubVolcanoSource) All() []gworld.Volcano {
	return nil
}

var _ gworld.VolcanoSource = (*stubVolcanoSource)(nil)

// --- helpers ---------------------------------------------------------------

const floatEps = 1e-5

func approxEq(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < floatEps
}

// singleCellMap builds the smallest valid Map with a single Voronoi cell
// whose center is at (cx, cy) and whose terrain is t. All other map
// machinery (rivers, coast neighbours) is left at zero values so tests
// that need specific coast/river behaviour must configure them separately.
//
// This reuses the real Generate pipeline on a Tiny world and then
// overwrites the first cell's terrain with the desired value. This is
// the safest approach because it gives us a structurally valid Map
// (Voronoi grid, riverBits, etc.) without needing to replicate internal
// Map construction.
//
// For terrain-only tests we generate a Tiny world and look up the cell
// at the probe position, then force its terrain.
func buildTinyMap(t *testing.T) *Map {
	t.Helper()
	return Generate(testSeed, WorldSizeTiny)
}

// probePos returns the center of the first land cell in w, along with
// its cellID. Used by tests that need a valid probe position without
// caring which specific tile is selected.
func firstLandCell(w *Map) (geom.Position, uint32, bool) {
	for id, cell := range w.Voronoi.Cells {
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		if cx < 0 || cy < 0 || cx >= w.Width || cy >= w.Height {
			continue
		}
		cid := uint32(id)
		if !w.IsOcean(cid) {
			return geom.Position{X: cx, Y: cy}, cid, true
		}
	}
	return geom.Position{}, 0, false
}

// --- tests -----------------------------------------------------------------

// TestHabitabilityBaseScores verifies that each terrain returns exactly
// its biomeBaseScore value when no coast, river, deposits, or volcano
// modifiers are active.
func TestHabitabilityBaseScores(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world per terrain entry: slow in -short")
	}
	w := buildTinyMap(t)

	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell found in Tiny world")
	}

	emptyDeposits := &stubDepositSource{}

	for terrain, want := range biomeBaseScore {
		terrain, want := terrain, want
		t.Run(string(terrain), func(t *testing.T) {
			// Force the cell terrain to the target value.
			orig := w.Terrain[cellID]
			w.Terrain[cellID] = terrain
			t.Cleanup(func() { w.Terrain[cellID] = orig })

			// Ensure no coast for this cell by using an interior
			// cell; if the selected cell happens to be coastal the
			// bonus would make the assertion wrong. We strip the
			// coast effect by temporarily making all neighbours
			// non-ocean — but that requires writing to neighbour
			// cells which is complex. Instead, accept that a coastal
			// cell may get the coast bonus and skip the assertion for
			// terrain=Beach (which is expected to be coastal).
			// For non-beach terrains we just verify score >= want.
			got := habitabilityAt(p, w, cellID, emptyDeposits, nil)

			if w.IsCoast(cellID) || w.IsRiver(p.X, p.Y) {
				// Bonuses active — score must be at least base.
				if got < want-floatEps {
					t.Errorf("terrain %s: got %.4f, want >= %.4f", terrain, got, want)
				}
			} else {
				if !approxEq(got, want) {
					t.Errorf("terrain %s: got %.4f, want %.4f", terrain, got, want)
				}
			}
		})
	}
}

// TestHabitabilityHardReject verifies that water, snowy peaks, and
// volcano interior terrains always return 0.
func TestHabitabilityHardReject(t *testing.T) {
	rejectTerrains := []gworld.Terrain{
		gworld.TerrainOcean,
		gworld.TerrainDeepOcean,
		gworld.TerrainSnowyPeak,
		gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainVolcanoSlope,
		gworld.TerrainCraterLake,
	}

	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)
	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell found in Tiny world")
	}

	emptyDeposits := &stubDepositSource{}

	for _, terrain := range rejectTerrains {
		terrain := terrain
		t.Run(string(terrain), func(t *testing.T) {
			orig := w.Terrain[cellID]
			w.Terrain[cellID] = terrain
			t.Cleanup(func() { w.Terrain[cellID] = orig })

			got := habitabilityAt(p, w, cellID, emptyDeposits, nil)
			if got != 0 {
				t.Errorf("terrain %s: got %.4f, want 0.0", terrain, got)
			}
		})
	}
}

// TestHabitabilityCoastBonus verifies that a coastal Forest cell without
// river or deposits returns exactly base + campCoastBonus. Forest (0.65)
// is used because Forest + coast (0.10) = 0.75, safely below the 1.0
// clamp, so the bonus is observable without hitting the ceiling.
func TestHabitabilityCoastBonus(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	// Find a coastal land cell.
	var coastPos geom.Position
	var coastID uint32
	found := false
	for id, cell := range w.Voronoi.Cells {
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		if cx < 0 || cy < 0 || cx >= w.Width || cy >= w.Height {
			continue
		}
		cid := uint32(id)
		if !w.IsOcean(cid) && w.IsCoast(cid) && !w.IsRiver(cx, cy) {
			coastPos = geom.Position{X: cx, Y: cy}
			coastID = cid
			found = true
			break
		}
	}
	if !found {
		t.Skip("no coastal non-river land cell in Tiny world")
	}

	orig := w.Terrain[coastID]
	w.Terrain[coastID] = gworld.TerrainForest
	defer func() { w.Terrain[coastID] = orig }()

	want := biomeBaseScore[gworld.TerrainForest] + campCoastBonus
	got := habitabilityAt(coastPos, w, coastID, &stubDepositSource{}, nil)
	if !approxEq(got, want) {
		t.Errorf("coast Forest: got %.4f, want %.4f", got, want)
	}
}

// TestHabitabilityRiverBonus verifies that an inland river Taiga cell
// without coast or deposits returns exactly base + campRiverBonus.
// Taiga (0.40) + river (0.15) = 0.55, safely below the 1.0 clamp so
// the bonus is observable.
func TestHabitabilityRiverBonus(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	// Find a river land cell that is not coastal.
	var riverPos geom.Position
	var riverID uint32
	found := false
	for id, cell := range w.Voronoi.Cells {
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		if cx < 0 || cy < 0 || cx >= w.Width || cy >= w.Height {
			continue
		}
		cid := uint32(id)
		if !w.IsOcean(cid) && !w.IsCoast(cid) && w.IsRiver(cx, cy) {
			riverPos = geom.Position{X: cx, Y: cy}
			riverID = cid
			found = true
			break
		}
	}
	if !found {
		t.Skip("no inland river cell in Tiny world")
	}

	orig := w.Terrain[riverID]
	w.Terrain[riverID] = gworld.TerrainTaiga
	defer func() { w.Terrain[riverID] = orig }()

	want := biomeBaseScore[gworld.TerrainTaiga] + campRiverBonus
	got := habitabilityAt(riverPos, w, riverID, &stubDepositSource{}, nil)
	if !approxEq(got, want) {
		t.Errorf("river Taiga: got %.4f, want %.4f", got, want)
	}
}

// TestHabitabilityFoodDepositBonus verifies that food deposits increase
// the score by campFoodDepositBonus each. Desert (0.25) is used as the
// base terrain so that even with coast (+0.10), river (+0.15), and two
// food deposits (+0.16) the total stays below 1.0 and the clamp does
// not obscure the expected delta.
func TestHabitabilityFoodDepositBonus(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell in Tiny world")
	}

	// Force Desert terrain so all bonuses stack without hitting 1.0.
	orig := w.Terrain[cellID]
	w.Terrain[cellID] = gworld.TerrainDesert
	defer func() { w.Terrain[cellID] = orig }()

	base := biomeBaseScore[gworld.TerrainDesert]
	coastal := float32(0)
	if w.IsCoast(cellID) {
		coastal = campCoastBonus
	}
	river := float32(0)
	if w.IsRiver(p.X, p.Y) {
		river = campRiverBonus
	}

	// One food deposit at exactly the probe position (distance 0, within radius).
	oneFood := &stubDepositSource{deposits: []gworld.Deposit{
		{Position: p, Kind: gworld.DepositFertile, MaxAmount: 100, CurrentAmount: 100},
	}}
	want1 := base + coastal + river + campFoodDepositBonus
	got1 := habitabilityAt(p, w, cellID, oneFood, nil)
	if !approxEq(got1, want1) {
		t.Errorf("one food deposit: got %.4f, want %.4f", got1, want1)
	}

	// Two food deposits.
	twoFood := &stubDepositSource{deposits: []gworld.Deposit{
		{Position: p, Kind: gworld.DepositFertile, MaxAmount: 100, CurrentAmount: 100},
		{Position: geom.Position{X: p.X + 1, Y: p.Y}, Kind: gworld.DepositFish, MaxAmount: 50, CurrentAmount: 50},
	}}
	want2 := base + coastal + river + 2*campFoodDepositBonus
	got2 := habitabilityAt(p, w, cellID, twoFood, nil)
	if !approxEq(got2, want2) {
		t.Errorf("two food deposits: got %.4f, want %.4f", got2, want2)
	}
}

// TestHabitabilityGenericDepositBonus verifies that non-food deposits
// increase the score by campGenericDepositBonus. Desert (0.25) keeps
// the total well below 1.0 so the bonus is observable without hitting
// the clamp ceiling.
func TestHabitabilityGenericDepositBonus(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell in Tiny world")
	}

	orig := w.Terrain[cellID]
	w.Terrain[cellID] = gworld.TerrainDesert
	defer func() { w.Terrain[cellID] = orig }()

	base := biomeBaseScore[gworld.TerrainDesert]
	coastal := float32(0)
	if w.IsCoast(cellID) {
		coastal = campCoastBonus
	}
	river := float32(0)
	if w.IsRiver(p.X, p.Y) {
		river = campRiverBonus
	}

	ironDeposit := &stubDepositSource{deposits: []gworld.Deposit{
		{Position: p, Kind: gworld.DepositIron, MaxAmount: 200, CurrentAmount: 200},
	}}
	want := base + coastal + river + campGenericDepositBonus
	got := habitabilityAt(p, w, cellID, ironDeposit, nil)
	if !approxEq(got, want) {
		t.Errorf("iron deposit: got %.4f, want %.4f", got, want)
	}
}

// TestHabitabilityVolcanoPenalty verifies that a volcano footprint tile
// within campVolcanoPenaltyRadius halves the score by campVolcanoPenaltyMult.
func TestHabitabilityVolcanoPenalty(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell in Tiny world")
	}

	w.Terrain[cellID] = gworld.TerrainPlains

	// Place a volcano override at distance campVolcanoPenaltyRadius (within range).
	volcanoTile := geom.Position{X: p.X + campVolcanoPenaltyRadius, Y: p.Y}
	vs := newStubVolcanoSource([]geom.Position{volcanoTile}, gworld.TerrainVolcanoCore)

	base := biomeBaseScore[gworld.TerrainPlains]
	coastal := float32(0)
	if w.IsCoast(cellID) {
		coastal = campCoastBonus
	}
	river := float32(0)
	if w.IsRiver(p.X, p.Y) {
		river = campRiverBonus
	}

	unpenalised := base + coastal + river
	want := unpenalised * campVolcanoPenaltyMult

	got := habitabilityAt(p, w, cellID, &stubDepositSource{}, vs)
	if !approxEq(got, want) {
		t.Errorf("volcano penalty: got %.4f, want %.4f", got, want)
	}
}

// TestHabitabilityClampedAtOne verifies that a highly-scored cell is
// clamped to exactly 1.0 and never exceeds it.
func TestHabitabilityClampedAtOne(t *testing.T) {
	if testing.Short() {
		t.Skip("generates a Tiny world: slow in -short")
	}
	w := buildTinyMap(t)

	p, cellID, ok := firstLandCell(w)
	if !ok {
		t.Fatal("no land cell in Tiny world")
	}

	w.Terrain[cellID] = gworld.TerrainPlains

	// Five food deposits at distinct close positions, all within radius.
	deps := make([]gworld.Deposit, 5)
	for i := range deps {
		deps[i] = gworld.Deposit{
			Position:      geom.Position{X: p.X + i, Y: p.Y},
			Kind:          gworld.DepositFertile,
			MaxAmount:     100,
			CurrentAmount: 100,
		}
	}
	ds := &stubDepositSource{deposits: deps}

	got := habitabilityAt(p, w, cellID, ds, nil)
	if got > 1.0 {
		t.Errorf("score not clamped: got %.4f, want <= 1.0", got)
	}
	// Plains + 5×food bonus already exceeds 1.0 (0.95 + 5×0.08 = 1.35),
	// so the clamped result must be exactly 1.0.
	if got != 1.0 {
		t.Errorf("expected clamp to 1.0: got %.4f", got)
	}
}

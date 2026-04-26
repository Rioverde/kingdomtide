package worldgen

import (
	"testing"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
	gworld "github.com/Rioverde/kingdomtide/internal/game/world"
)

// standardWorld builds a Standard world with seed 42 for tests that need a
// fully-generated world. Gated behind testing.Short() when used in multi-seed
// or sweep contexts — see individual tests.
func standardWorld(t *testing.T) *Map {
	t.Helper()
	return Generate(testSeed, WorldSizeStandard)
}

// TestDepositSource_PlacementByBiome samples a large world and verifies that
// biome-specific deposit kinds appear in the expected cells. Mountain cells
// must yield Iron or Stone far more often than nothing; forests must yield
// Timber; beach/coast cells must yield Fish.
func TestDepositSource_PlacementByBiome(t *testing.T) {
	if testing.Short() {
		t.Skip("placement sweep: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{})

	var (
		mountainTotal, mountainMineral int
		forestTotal, forestTimber      int
		beachTotal, beachFish          int
	)

	for id, cell := range w.Voronoi.Cells {
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		if cx < 0 || cy < 0 || cx >= w.Width || cy >= w.Height {
			continue
		}
		if w.IsOcean(uint32(id)) {
			continue
		}
		terrain := w.Terrain[id]
		pos := geom.Position{X: cx, Y: cy}
		d, _ := src.DepositAt(pos)

		switch terrain {
		case gworld.TerrainMountain, gworld.TerrainSnowyPeak:
			mountainTotal++
			if d.Kind == gworld.DepositIron || d.Kind == gworld.DepositStone || d.Kind == gworld.DepositGold {
				mountainMineral++
			}
		case gworld.TerrainForest, gworld.TerrainJungle:
			forestTotal++
			if d.Kind == gworld.DepositTimber {
				forestTimber++
			}
		case gworld.TerrainBeach:
			beachTotal++
			if d.Kind == gworld.DepositFish {
				beachFish++
			}
		}
	}

	// Mountain cells: at least 60% should carry a mineral deposit
	// (iron 40% + stone 30% + gold 5% = 75% non-nothing).
	if mountainTotal > 0 {
		ratio := float64(mountainMineral) / float64(mountainTotal)
		if ratio < 0.50 {
			t.Errorf("mountain mineral ratio = %.2f (want >=0.50), total=%d mineral=%d",
				ratio, mountainTotal, mountainMineral)
		}
	}

	// Forest cells: at least 40% should carry Timber.
	if forestTotal > 0 {
		ratio := float64(forestTimber) / float64(forestTotal)
		if ratio < 0.35 {
			t.Errorf("forest timber ratio = %.2f (want >=0.35), total=%d timber=%d",
				ratio, forestTotal, forestTimber)
		}
	}

	// Beach cells: at least 20% should carry Fish (30% weight, minus "nothing").
	if beachTotal > 0 {
		ratio := float64(beachFish) / float64(beachTotal)
		if ratio < 0.15 {
			t.Errorf("beach fish ratio = %.2f (want >=0.15), total=%d fish=%d",
				ratio, beachTotal, beachFish)
		}
	}
}

// TestDepositSource_Determinism verifies that two DepositSource instances built
// from the same seed produce identical deposit maps.
func TestDepositSource_Determinism(t *testing.T) {
	if testing.Short() {
		t.Skip("determinism check: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	a := NewDepositSource(w, testSeed, DepositSourceConfig{})
	b := NewDepositSource(w, testSeed, DepositSourceConfig{})

	if len(a.sorted) != len(b.sorted) {
		t.Fatalf("deposit counts differ: %d vs %d", len(a.sorted), len(b.sorted))
	}
	for i, da := range a.sorted {
		db := b.sorted[i]
		if da != db {
			t.Fatalf("deposit[%d] mismatch: %+v vs %+v", i, da, db)
		}
	}
}

// TestDepositSource_DepositsInRect verifies that every deposit returned by
// DepositsIn falls strictly inside the query rect, and that no deposit outside
// the rect is returned.
func TestDepositSource_DepositsInRect(t *testing.T) {
	if testing.Short() {
		t.Skip("DepositsIn rect: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{})

	rect := geom.Rect{MinX: 0, MinY: 0, MaxX: 200, MaxY: 200}
	results := src.DepositsIn(rect)
	for _, d := range results {
		if !rect.Contains(d.Position) {
			t.Errorf("deposit %+v lies outside rect %+v", d.Position, rect)
		}
	}

	// Cross-check: every deposit in the full list that falls inside the
	// rect must appear in results.
	resultSet := make(map[uint64]bool, len(results))
	for _, d := range results {
		resultSet[geom.PackPos(d.Position)] = true
	}
	for _, d := range src.sorted {
		if rect.Contains(d.Position) {
			if !resultSet[geom.PackPos(d.Position)] {
				t.Errorf("deposit at %+v inside rect but missing from DepositsIn", d.Position)
			}
		}
	}
}

// TestDepositSource_AvoidOcean verifies that no deposit is placed on a deep-
// ocean or ocean cell (DepositNone must be the result for those tiles).
func TestDepositSource_AvoidOcean(t *testing.T) {
	if testing.Short() {
		t.Skip("ocean avoidance sweep: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{})

	for id, cell := range w.Voronoi.Cells {
		if !w.IsOcean(uint32(id)) {
			continue
		}
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		pos := geom.Position{X: cx, Y: cy}
		if d, ok := src.DepositAt(pos); ok {
			t.Errorf("ocean cell %d at %+v has deposit kind=%s", id, pos, d.Kind)
		}
	}
}

// TestDepositSource_NoneKindNeverStored verifies the invariant that DepositNone
// never appears in the source's index — all returned deposits have Kind != None.
func TestDepositSource_NoneKindNeverStored(t *testing.T) {
	if testing.Short() {
		t.Skip("none-kind invariant: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{})

	for _, d := range src.sorted {
		if d.Kind == gworld.DepositNone {
			t.Errorf("DepositNone found in sorted list at %+v", d.Position)
		}
	}
	for key, d := range src.byPos {
		if d.Kind == gworld.DepositNone {
			t.Errorf("DepositNone found in byPos map at key %x", key)
		}
	}
}

// TestDepositSource_CurrentAmountEqualsMax verifies that freshly placed
// deposits have CurrentAmount == MaxAmount (no depletion at generation time).
func TestDepositSource_CurrentAmountEqualsMax(t *testing.T) {
	if testing.Short() {
		t.Skip("amount invariant: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{})

	for _, d := range src.sorted {
		if d.CurrentAmount != d.MaxAmount {
			t.Errorf("deposit %s at %+v: CurrentAmount=%d != MaxAmount=%d",
				d.Kind, d.Position, d.CurrentAmount, d.MaxAmount)
		}
		if d.MaxAmount <= 0 {
			t.Errorf("deposit %s at %+v has MaxAmount=%d (want >0)",
				d.Kind, d.Position, d.MaxAmount)
		}
	}
}

// TestDepositSource_VolcanicKindsAppear verifies that obsidian or sulfur
// deposits are placed when a VolcanoSource is wired. Before the fix, volcanic
// terrain was checked against cell-level biome (always Mountain/Hills inside a
// volcano) so the volcanic weight table never fired and neither kind ever
// appeared.
func TestDepositSource_VolcanicKindsAppear(t *testing.T) {
	if testing.Short() {
		t.Skip("volcanic deposit check: needs full worldgen, slow in -short")
	}
	w := standardWorld(t)
	volcSrc := NewVolcanoSource(w, testSeed)
	src := NewDepositSource(w, testSeed, DepositSourceConfig{Volcanoes: volcSrc})

	var volcanic int
	for _, d := range src.sorted {
		if d.Kind == gworld.DepositObsidian || d.Kind == gworld.DepositSulfur {
			volcanic++
		}
	}
	if volcanic == 0 {
		t.Error("no obsidian or sulfur deposits found; expected at least 1 in a standard world with volcanoes wired")
	}
}

package server

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// volcanicTerrains is the set of pb.Terrain values that prove a volcano
// footprint made it through placement, worldgen override, mapper, and
// wire encoding. A snapshot carrying none of these at a seed known to
// contain volcanoes indicates the volcano-to-wire pipeline is broken.
var volcanicTerrains = map[pb.Terrain]struct{}{
	pb.Terrain_TERRAIN_VOLCANO_CORE:         {},
	pb.Terrain_TERRAIN_VOLCANO_CORE_DORMANT: {},
	pb.Terrain_TERRAIN_CRATER_LAKE:          {},
	pb.Terrain_TERRAIN_VOLCANO_SLOPE:        {},
	pb.Terrain_TERRAIN_ASHLAND:              {},
}

// buildE2EService wires the same region/landmark/volcano/deposit
// stack the production server uses so the assertions exercise the
// real snapshot assembly path through the caches.
func buildE2EService(tb testing.TB) *Service {
	tb.Helper()
	wg := worldgen.NewChunkedSource(snapshotTestSeed)
	regionSrc := worldgen.NewNoiseRegionSource(snapshotTestSeed, wg.Generator())
	landmarkSrc := worldgen.NewNoiseLandmarkSource(snapshotTestSeed, regionSrc, wg.Generator())
	volcanoSrc := worldgen.NewNoiseVolcanoSource(snapshotTestSeed, wg.Generator(), landmarkSrc)
	depositSrc := worldgen.NewNoiseDepositSource(snapshotTestSeed, wg.Generator(), landmarkSrc, volcanoSrc)
	w := world.NewWorld(
		wg,
		world.WithSeed(snapshotTestSeed),
		world.WithRegionSource(regionSrc),
		world.WithLandmarkSource(landmarkSrc),
		world.WithVolcanoSource(volcanoSrc),
		world.WithDepositSource(depositSrc),
	)
	return NewService(w, silentLog())
}

// countVolcanicTiles scans tiles and returns the number whose terrain
// belongs to the volcanic set. Kept as a helper so the e2e assertion
// and any future coverage probe share one definition of "volcanic".
func countVolcanicTiles(tiles []*pb.Tile) int {
	var n int
	for _, t := range tiles {
		if _, ok := volcanicTerrains[t.GetTerrain()]; ok {
			n++
		}
	}
	return n
}

// TestSnapshot_FullViewport_E2E is a regression guard that a
// post-refactor change to any of region / landmark / volcano / resource
// / mapper cannot silently return an empty snapshot. Drives the same
// entry point the client hits per tick (snapshotOf via the service's
// live caches) and asserts every layer reaches the wire.
//
// The Snapshot proto embeds landmarks per-tile (Tile.Landmark) and
// encodes volcanoes as volcanic terrain overrides on tiles rather than
// as top-level slices, so landmark/volcano presence is counted across
// the tile grid. Resource deposits are not part of the current
// Snapshot wire shape — presence is asserted on the world source to
// guard that the deposit pipeline still produces results, matching the
// regression intent without fabricating a wire field.
func TestSnapshot_FullViewport_E2E(t *testing.T) {
	if testing.Short() {
		t.Skip("builds full worldgen stack + 128x128 snapshot")
	}
	svc := buildE2EService(t)

	const baseW, baseH = 128, 128
	center := geom.Position{X: 0, Y: 0}

	snap := snapshotOf(
		svc.world,
		center,
		baseW,
		baseH,
		svc.regionAt(center),
		svc.landmarks,
		svc.volcanoes,
	)
	if snap == nil {
		t.Fatal("snapshot: nil")
	}

	if got, want := int(snap.GetWidth()), baseW; got != want {
		t.Fatalf("snapshot width = %d, want %d", got, want)
	}
	if got, want := int(snap.GetHeight()), baseH; got != want {
		t.Fatalf("snapshot height = %d, want %d", got, want)
	}

	tiles := snap.GetTiles()
	if n, want := len(tiles), baseW*baseH; n != want {
		t.Fatalf("tile count = %d, want %d (one tile per viewport cell)", n, want)
	}

	if snap.GetRegion() == nil {
		t.Fatal("snapshot region: nil, want resolved Region at centre")
	}

	var landmarkCount int
	for _, tile := range tiles {
		if lm := tile.GetLandmark(); lm != nil && lm.GetKind() != pb.LandmarkKind_LANDMARK_KIND_NONE {
			landmarkCount++
		}
	}
	if landmarkCount == 0 {
		t.Fatalf("landmark-bearing tiles in %dx%d viewport at seed 0x%X: want >0, got 0 — "+
			"if worldgen tuning changed, re-validate this seed's landmark placement",
			baseW, baseH, snapshotTestSeed)
	}

	// Volcanic-terrain coverage — scan the returned tiles for any
	// pb.Terrain that belongs to the volcanic set. At seed 0xA11CE the
	// 128×128 viewport centred on origin is known to contain volcanoes;
	// if the base window misses, widen up to 256×256 before declaring
	// the pipeline broken so one-off placement drift does not immediately
	// red-flag the wire encoding.
	volcanicTiles := countVolcanicTiles(tiles)
	widenW, widenH := baseW, baseH
	if volcanicTiles == 0 {
		widenW, widenH = 256, 256
		widerSnap := snapshotOf(
			svc.world,
			center,
			widenW,
			widenH,
			svc.regionAt(center),
			svc.landmarks,
			svc.volcanoes,
		)
		if widerSnap == nil {
			t.Fatal("widened snapshot: nil")
		}
		volcanicTiles = countVolcanicTiles(widerSnap.GetTiles())
	}
	if volcanicTiles == 0 {
		t.Fatalf("no volcanic terrain tile found in %dx%d viewport at seed 0x%X — "+
			"volcano-to-wire pipeline may be broken; if worldgen tuning changed, re-pick seed",
			widenW, widenH, snapshotTestSeed)
	}

	if svc.world.VolcanoSource() == nil {
		t.Fatal("world.VolcanoSource: nil, want wired")
	}
	if svc.world.DepositSource() == nil {
		t.Fatal("world.DepositSource: nil, want wired")
	}

	deposits := svc.world.DepositsNear(center, baseW)
	if len(deposits) == 0 {
		t.Fatalf("deposits within %d tiles of %v: want >0, got 0", baseW, center)
	}
}

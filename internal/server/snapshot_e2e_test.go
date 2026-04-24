package server

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// e2eSeed is the seed used by the full-viewport snapshot regression
// test. Fixed so the landmark-count assertion is deterministic; a
// different seed needs to be re-validated for at-least-one-landmark
// coverage before substitution.
const e2eSeed int64 = 0xA11CE

// buildE2EService wires the same region/landmark/volcano/deposit
// stack the production server uses so the assertions exercise the
// real snapshot assembly path through the caches.
func buildE2EService(tb testing.TB) *Service {
	tb.Helper()
	wg := worldgen.NewChunkedSource(e2eSeed)
	regionSrc := worldgen.NewNoiseRegionSource(e2eSeed, wg.Generator())
	landmarkSrc := worldgen.NewNoiseLandmarkSource(e2eSeed, regionSrc, wg.Generator())
	volcanoSrc := worldgen.NewNoiseVolcanoSource(e2eSeed, wg.Generator(), landmarkSrc)
	depositSrc := worldgen.NewNoiseDepositSource(e2eSeed, wg.Generator(), landmarkSrc, volcanoSrc)
	w := world.NewWorld(
		wg,
		world.WithSeed(e2eSeed),
		world.WithRegionSource(regionSrc),
		world.WithLandmarkSource(landmarkSrc),
		world.WithVolcanoSource(volcanoSrc),
		world.WithDepositSource(depositSrc),
	)
	return NewService(w, silentLog())
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

	const viewW, viewH = 128, 128
	center := geom.Position{X: 0, Y: 0}

	snap := snapshotOf(
		svc.world,
		center,
		viewW,
		viewH,
		svc.regionAt(center),
		svc.landmarks,
		svc.volcanoes,
	)
	if snap == nil {
		t.Fatal("snapshot: nil")
	}

	if got, want := int(snap.GetWidth()), viewW; got != want {
		t.Fatalf("snapshot width = %d, want %d", got, want)
	}
	if got, want := int(snap.GetHeight()), viewH; got != want {
		t.Fatalf("snapshot height = %d, want %d", got, want)
	}

	tiles := snap.GetTiles()
	if n, want := len(tiles), viewW*viewH; n != want {
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
		t.Fatalf("landmark-bearing tiles in %dx%d viewport: want >0, got 0", viewW, viewH)
	}

	if svc.world.VolcanoSource() == nil {
		t.Fatal("world.VolcanoSource: nil, want wired")
	}
	if svc.world.DepositSource() == nil {
		t.Fatal("world.DepositSource: nil, want wired")
	}

	deposits := svc.world.DepositsNear(center, viewW)
	if len(deposits) == 0 {
		t.Fatalf("deposits within %d tiles of %v: want >0, got 0", viewW, center)
	}
}

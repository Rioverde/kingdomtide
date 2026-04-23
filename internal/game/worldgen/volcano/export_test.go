package volcano

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
)

// MinSpacingTilesForTest exposes the Poisson-disk minimum spacing so the
// external test package can assert the invariant for co-super-region
// anchor pairs.
const MinSpacingTilesForTest = volcanoMinSpacingTiles

// FootprintNeighbourOffsetsForTest mirrors the unexported offsets table
// so the 4-connectivity flood-fill test in the external package can
// traverse zone slices the same way the growth walk did.
func FootprintNeighbourOffsetsForTest() [4][2]int { return footprintNeighbourOffsets }

// TierSizesForTest returns the zone-size min/max table for state. Used
// by TestGrowFootprint_ZoneSizes in the external package.
func TierSizesForTest(state world.VolcanoState) (core, slope, ashland [2]int, ok bool) {
	sz, exists := tierSizes[state]
	if !exists {
		return
	}
	return sz.core, sz.slope, sz.ashland, true
}

// GrowFootprintForTest exposes growFootprint to external tests. Wraps
// the unexported function rather than exporting it directly so the
// production API surface stays minimal.
func GrowFootprintForTest(
	anchor geom.Position,
	state world.VolcanoState,
	seed int64,
	terrain TerrainSampler,
	landmarks []world.Landmark,
) (core, slope, ashland []geom.Position) {
	return growFootprint(anchor, state, seed, terrain, landmarks)
}

// TerrainForZoneForTest exposes terrainForZone to external tests.
func TerrainForZoneForTest(zone world.VolcanoZone, state world.VolcanoState) world.Terrain {
	return terrainForZone(zone, state)
}

// BridsonSampleForTest exposes the Poisson-disk sampler so the
// external test package can exercise it with custom rectangles.
func BridsonSampleForTest(
	rng *rand.Rand,
	minX, minY, width, height int,
	minDistance, k int,
) []geom.Position {
	return genprim.BridsonSample(rng, minX, minY, width, height, minDistance, k)
}

// SuperRegionOfForTest wraps genprim.SuperRegionOf so tests in the
// external volcano_test package don't need to import genprim directly.
func SuperRegionOfForTest(sc geom.SuperChunkCoord) genprim.SuperRegion {
	return genprim.SuperRegionOf(sc)
}

// PickVolcanoAnchorsForTest exposes pickVolcanoAnchors for benchmarks in
// the external volcano_test package.
func PickVolcanoAnchorsForTest(
	seed int64,
	sr genprim.SuperRegion,
	lm world.LandmarkSource,
	terrain TerrainSampler,
) []geom.Position {
	return pickVolcanoAnchors(seed, sr, lm, terrain)
}

// EnsureSuperRegionForTest warms the per-SR cache so benchmarks measure
// the hot path rather than generation cost.
func (s *NoiseVolcanoSource) EnsureSuperRegionForTest(sr genprim.SuperRegion) {
	s.ensureSuperRegion(sr)
}

package landmark

import "github.com/Rioverde/gongeons/internal/game/world"

// FitsTerrainForTest exposes the unexported fitsTerrain to the external
// landmark_test package so tests can assert the same affinity check the
// production path used. Lives in an export_test.go file so it only
// compiles when the test binary is built.
func FitsTerrainForTest(
	kind world.LandmarkKind,
	tile world.Tile,
	elevation, gradient float64,
) bool {
	return fitsTerrain(kind, tile, elevation, gradient)
}

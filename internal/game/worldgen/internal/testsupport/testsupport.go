// Package testsupport assembles the shared worldgen stack used by the
// volcano and resource sub-package test suites. Lives under internal/ so
// production code cannot depend on it and so it can reach into
// worldgen-tier concrete types without polluting the public API.
//
// This file intentionally imports testing.TB from a non-_test.go source
// because the helper is shared across multiple _test packages and Go
// disallows cross-package imports of _test.go files. The production
// binary never links testsupport — the internal/ boundary keeps it
// confined to test builds of its sibling packages.
package testsupport

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
	"github.com/Rioverde/gongeons/internal/game/worldgen/landmark"
	"github.com/Rioverde/gongeons/internal/game/worldgen/region"
)

// Stack bundles the three worldgen layers a placement-source test usually
// needs: the tile-level terrain generator, a region source for character
// biasing, and a landmark source for collision rejection. Consumers pick
// the fields they need and ignore the rest.
type Stack struct {
	Generator *worldgen.WorldGenerator
	Regions   *region.NoiseRegionSource
	Landmarks *landmark.NoiseLandmarkSource
}

// NewStack wires a fresh Generator → Regions → Landmarks chain from seed.
// The tb parameter lets the helper mark itself as a test helper so failure
// messages point at the caller, not at this file.
func NewStack(tb testing.TB, seed int64) Stack {
	tb.Helper()
	wg := worldgen.NewWorldGenerator(seed)
	regions := region.NewNoiseRegionSource(seed, wg)
	landmarks := landmark.NewNoiseLandmarkSource(seed, regions, wg)
	return Stack{Generator: wg, Regions: regions, Landmarks: landmarks}
}

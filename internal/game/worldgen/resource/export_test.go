package resource

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/noise"
)

// PointMinDistanceForTest exposes the per-kind Poisson-disk minimum
// spacing table to the external resource_test package so spacing and
// rarity-classification tests can reason about point kinds without
// guessing at the unexported map.
func PointMinDistanceForTest(kind world.DepositKind) (int, bool) {
	v, ok := pointMinDistance[kind]
	return v, ok
}

// PointDepositsInRegionForTest wraps pointDepositsInRegion so the
// external test package can exercise a single super-region's point
// placement without going through DepositsIn.
func PointDepositsInRegionForTest(
	seed int64,
	sr genprim.SuperRegion,
	terrain TerrainSampler,
	lm world.LandmarkSource,
	vs world.VolcanoSource,
) []world.Deposit {
	return pointDepositsInRegion(seed, sr, terrain, lm, vs)
}

// PointBiomeAcceptsForTest exposes the per-kind biome gate.
func PointBiomeAcceptsForTest(kind world.DepositKind, ter world.Terrain) bool {
	return pointBiomeAccepts(kind, ter)
}

// TileBlockedForTest exposes tileBlocked so the external test package
// can assert water / landmark / volcano rejection without inlining the
// entire gate. It builds a 3x3 SC landmark set around p to match the
// coverage the production path provides via the full SR neighbourhood.
func TileBlockedForTest(p geom.Position, terrain TerrainSampler, lm world.LandmarkSource, vs world.VolcanoSource) bool {
	var lmSet map[geom.Position]struct{}
	if lm != nil {
		lmSet = make(map[geom.Position]struct{})
		home := geom.WorldToSuperChunk(p.X, p.Y)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
				for _, l := range lm.LandmarksIn(sc) {
					lmSet[l.Coord] = struct{}{}
				}
			}
		}
	}
	return tileBlocked(p, terrain, lmSet, vs)
}

// FishDepositAtForTest wraps fishDepositAt for external tests.
func FishDepositAtForTest(seed int64, t geom.Position, terrain TerrainSampler) (world.Deposit, bool) {
	return fishDepositAt(seed, t, terrain)
}

// BeachFacesOpenOceanForTest exposes beachFacesOpenOcean.
func BeachFacesOpenOceanForTest(t geom.Position, terrain TerrainSampler) bool {
	return beachFacesOpenOcean(t, terrain)
}

// FishDensityFractionForTest exposes the target selection fraction so the
// density probe can reference it without duplicating the constant.
const FishDensityFractionForTest = fishDensityFraction

// ObsidianDepositAtForTest wraps obsidianDepositAt.
func ObsidianDepositAtForTest(seed int64, t geom.Position, vs world.VolcanoSource) (world.Deposit, bool) {
	return obsidianDepositAt(seed, t, vs)
}

// SulfurDepositAtForTest wraps sulfurDepositAt.
func SulfurDepositAtForTest(seed int64, t geom.Position, vs world.VolcanoSource) (world.Deposit, bool) {
	return sulfurDepositAt(seed, t, vs)
}

// SlopeAdjacentToCoreForTest exposes slopeAdjacentToCore.
func SlopeAdjacentToCoreForTest(t geom.Position, v world.Volcano) bool {
	return slopeAdjacentToCore(t, v)
}

// ObsidianDensityFractionForTest exposes the target fraction.
const ObsidianDensityFractionForTest = obsidianDensityFraction

// SulfurDormantFractionForTest exposes the dormant gate fraction.
const SulfurDormantFractionForTest = sulfurDormantFraction

// ChebyshevForTest exposes chebyshev for the external test package.
func ChebyshevForTest(a, b geom.Position) int {
	return chebyshev(a, b)
}

// EnsureRegionForTest warms the per-SR cache so benchmarks measure the
// hot path rather than generation cost.
func (s *NoiseDepositSource) EnsureRegionForTest(sr genprim.SuperRegion) {
	s.ensureRegion(sr)
}

// ZonalNoiseMapForTest builds the per-kind noise array the same way
// NewNoiseDepositSource does. Used by zonal tests that exercise
// ZonalDepositAtForTest without constructing a full source.
func ZonalNoiseMapForTest(seed int64) [zonalKindCount]noise.OctaveNoise {
	var out [zonalKindCount]noise.OctaveNoise
	for i := range zonalSpecs {
		out[i] = noise.NewOctaveNoise(seed^zonalSpecs[i].subSalt, zonalNoiseOpts)
	}
	return out
}

// ZonalDepositAtForTest exposes zonalDepositAt.
func ZonalDepositAtForTest(
	t geom.Position,
	ter world.Terrain,
	noises [zonalKindCount]noise.OctaveNoise,
) (world.Deposit, bool) {
	return zonalDepositAt(t, ter, &noises)
}

// ZonalBiomeAcceptsForTest exposes zonalBiomeAccepts.
func ZonalBiomeAcceptsForTest(kind world.DepositKind, ter world.Terrain) bool {
	return zonalBiomeAccepts(kind, ter)
}

// ZonalPerlinScaleForTest exposes the per-tile scale constant.
const ZonalPerlinScaleForTest = zonalPerlinScale

// ZonalThresholdForTest looks up a per-kind threshold.
func ZonalThresholdForTest(kind world.DepositKind) (float64, bool) {
	for i := range zonalSpecs {
		if zonalSpecs[i].kind == kind {
			return zonalSpecs[i].threshold, true
		}
	}
	return 0, false
}

// ZonalNoiseSlotForTest reports the zonalSpecs slot for kind. Used by
// external tests that want to probe a specific kind's noise field
// without replicating the slot-lookup logic.
func ZonalNoiseSlotForTest(kind world.DepositKind) (int, bool) {
	for i := range zonalSpecs {
		if zonalSpecs[i].kind == kind {
			return i, true
		}
	}
	return 0, false
}

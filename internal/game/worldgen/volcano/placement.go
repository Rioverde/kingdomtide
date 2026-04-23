package volcano

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
)

// Super-region constants. A volcanic super-region is a fixed 4x4 grid of
// super-chunks (64 * 4 = 256 tiles per side). Poisson-disk sampling runs
// per super-region: the window is large enough to produce good blue-noise
// spacing, small enough to cache cheaply, and the 256-tile side carries
// 3-4 volcanoes on average at the configured min-spacing. The resulting
// density is ~1 volcano per ~4 super-chunks in accepting terrain (~1 per
// 250-350 tiles), so a player walking a short distance from spawn sees
// one inside the viewport without feature-flag gymnastics.
const (
	volcanoMinSpacingTiles = 100
	volcanoPoissonK        = 30
	volcanoBiomeWeightMax  = 1.5
)

// Nothing-up-my-sleeve salts. Each value is a documented 64-bit constant
// distinct from every other worldgen salt already in use. Routed through
// genprim.ToInt64 because the top bit is set — Go rejects these as
// untyped signed literals, so the runtime conversion preserves the bit
// pattern.
var (
	// Random 64-bit nothing-up-my-sleeve pattern.
	seedSaltVolcanoAnchor = genprim.ToInt64(0xca6e7f91b8d1a4e3)
	// SHA-256 initial hash H0 — fractional digits of sqrt(2).
	seedSaltVolcanoState = genprim.ToInt64(0x6a09e667f3bcc908)
	// SHA-256 initial hash H1 — fractional digits of sqrt(3).
	seedSaltVolcanoFootprint = genprim.ToInt64(0xbb67ae8584caa73b)
)

// newVolcanoAnchorRNG builds a PCG seeded from (seed, super-region). Two
// calls with the same inputs produce identical streams; any change in
// either input decorrelates the stream.
func newVolcanoAnchorRNG(seed int64, sr genprim.SuperRegion) *rand.Rand {
	lo := uint64(seed ^ seedSaltVolcanoAnchor)
	hi := genprim.HashSR(sr)
	return rand.New(rand.NewPCG(lo, hi))
}

// pickVolcanoAnchors returns the accepted volcano anchor positions inside
// sr. Bridson's Poisson-disk produces candidate points with
// volcanoMinSpacingTiles minimum spacing; the biome-weight gate plus
// landmark-collision check then trims the candidate set down to the
// anchors worth keeping. Deterministic from (seed, sr).
//
// A super-region yields 0-3 anchors depending on its terrain mix; the
// long-run average is ~2 volcanoes per super-region at the configured
// parameters.
func pickVolcanoAnchors(
	seed int64,
	sr genprim.SuperRegion,
	lm world.LandmarkSource,
	terrain TerrainSampler,
) []geom.Position {
	rng := newVolcanoAnchorRNG(seed, sr)
	minX, minY, side := sr.Bounds()
	candidates := genprim.BridsonSample(rng, minX, minY, side, side, volcanoMinSpacingTiles, volcanoPoissonK)

	out := make([]geom.Position, 0, len(candidates))
	for _, p := range candidates {
		if !acceptVolcanoAnchor(p, lm, terrain, rng) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// acceptVolcanoAnchor runs the biome-weight + landmark-rejection gate on
// a candidate anchor. Water tiles (ocean, deep ocean, lake, river) reject
// unconditionally; land terrain accepts with probability weight/max. The
// rng argument is the same stream bridsonSample consumed — sharing one
// RNG keeps the whole placement pipeline deterministic from a single
// (seed, sr) pair.
func acceptVolcanoAnchor(
	p geom.Position,
	lm world.LandmarkSource,
	terrain TerrainSampler,
	rng *rand.Rand,
) bool {
	tile := terrain.TileAt(p.X, p.Y)
	if genprim.IsWaterOrRiverTile(tile) {
		return false
	}
	weight := volcanoBiomeWeight(tile.Terrain)
	if weight <= 0 {
		return false
	}
	// Acceptance probability = clamp(weight / max, 0, 1). Mountain
	// (weight 3.0) and hills (weight 2.0) both saturate to 1.0 — they
	// always pass the gate. Plains/grassland (weight 0.8) pass 40% of
	// the time; desert/tundra (weight 0.5) pass 25%.
	accept := weight / volcanoBiomeWeightMax
	if accept > 1.0 {
		accept = 1.0
	}
	if rng.Float64() >= accept {
		return false
	}
	if collidesWithLandmark(p, lm) {
		return false
	}
	return true
}

// volcanoBiomeWeight returns the biome-gate weight for a candidate anchor
// tile. Values above zero pass the gate with probability
// clamp(weight/max, 0, 1); zero means reject outright. Weights match the
// plan table with the current volcanoBiomeWeightMax of 2.0:
//
//	mountain (3.0)     → always passes (clamps to 1.0)
//	hills (2.0)        → always passes
//	forest family (1.0) → 50% pass rate
//	plains family (0.8) → 40% pass rate
//	desert/tundra (0.5) → 25% pass rate
//
// Water and beach are excluded (weight 0).
func volcanoBiomeWeight(t world.Terrain) float64 {
	switch t {
	case world.TerrainMountain, world.TerrainSnowyPeak:
		return 3.0
	case world.TerrainHills:
		return 2.0
	case world.TerrainForest, world.TerrainTaiga, world.TerrainJungle:
		return 1.0
	case world.TerrainPlains, world.TerrainGrassland,
		world.TerrainMeadow, world.TerrainSavanna:
		return 0.8
	case world.TerrainDesert, world.TerrainTundra, world.TerrainSnow:
		return 0.5
	default:
		// Beach, ocean, deep ocean, existing volcanic terrains, and any
		// future terrain default to reject.
		return 0
	}
}

// collidesWithLandmark reports whether any landmark in the 3x3 super-chunk
// neighbourhood around p sits on the exact same tile. Landmarks live on
// specific tiles, not footprints, so an exact-match check is enough.
func collidesWithLandmark(p geom.Position, lm world.LandmarkSource) bool {
	if lm == nil {
		return false
	}
	home := geom.WorldToSuperChunk(p.X, p.Y)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			for _, l := range lm.LandmarksIn(sc) {
				if l.Coord.Equal(p) {
					return true
				}
			}
		}
	}
	return false
}

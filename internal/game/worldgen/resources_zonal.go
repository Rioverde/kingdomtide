package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// zonalKinds enumerates every deposit kind placed via the zonal Perlin
// strategy. Declaration order defines the iteration order inside
// zonalDepositAt — kinds that accept overlapping biomes (Timber and
// Game both accept forest) resolve to the first kind whose threshold
// the tile passes. Fertile, Timber, Game is the plan's canonical order
// and matches the "one deposit per tile" tiebreak documented on
// zonalDepositAt.
var zonalKinds = []game.DepositKind{
	game.DepositFertile,
	game.DepositTimber,
	game.DepositGame,
}

// zonalSubSalts carries the per-kind 64-bit salt XOR-ed into the world
// seed before building each zonal noise field. Values are distinct from
// every salt already in use (superchunk, region_source, landmarks,
// volcanoes_placement, seedSaltDepositPerlin from the plan) and routed
// through regionToInt64 because the top bit is set — Go rejects signed
// literals with the top bit set, so the conversion preserves the full
// 64-bit pattern at runtime.
var zonalSubSalts = map[game.DepositKind]int64{
	game.DepositFertile: regionToInt64(0x1a7f15c9e3779b0d),
	game.DepositTimber:  regionToInt64(0x2b3e8f27a6c14d5e),
	game.DepositGame:    regionToInt64(0x3c5d2f59e4a8b2af),
}

// zonalPerlinScale multiplies world-space coords before feeding into
// Eval2Normalized. 0.02 produces zones roughly 50 tiles across — large
// enough that a settlement's region-of-interest sits cleanly inside a
// single zone, small enough that zones aren't monolithic across the
// map. The noise field itself is built with Scale=1 because scaling
// happens here, at call time, rather than inside NewOctaveNoise.
const zonalPerlinScale = 0.02

// zonalNoiseOpts is the fBm configuration for every zonal kind. Three
// octaves give zones a little organic boundary jitter without drowning
// the dominant low-frequency shape. Scale=1 because the caller
// pre-scales world coords via zonalPerlinScale — the noise library
// cannot use a scale < 1 directly, so the outer scaler does the work.
var zonalNoiseOpts = OctaveOpts{
	Octaves:     3,
	Persistence: 0.5,
	Lacunarity:  2.0,
	Scale:       1,
}

// zonalThresholds gate each kind on the noise value in [0, 1]. Values
// strictly above the threshold are "in zone" and produce a deposit;
// values at or below are empty. OpenSimplex's Eval2Normalized output
// clusters around 0.5 with a roughly bell-shaped distribution (median
// ~0.507, tails at ~0.14 and ~0.86), so the thresholds sit close to
// 0.5 rather than the naive "1 - desired fraction" that a uniform
// distribution would suggest. Values were picked from an empirical
// percentile probe (see TestZonalDepositAt_Frequency) so each kind's
// observed in-zone fraction on its valid biomes lands within ±10% of
// the target:
//
//	Fertile ~35% of plains/grassland family (threshold 0.557)
//	Timber  ~40% of forest family          (threshold 0.540)
//	Game    ~38% of forest + grassland     (threshold 0.547)
var zonalThresholds = map[game.DepositKind]float64{
	game.DepositFertile: 0.557,
	game.DepositTimber:  0.540,
	game.DepositGame:    0.547,
}

// zonalMaxAmount carries the starting yield for each zonal deposit.
// Values mirror the plan's constants — tuned for the static-placement
// milestone so future depletion work (M5+) does not require a domain
// migration.
var zonalMaxAmount = map[game.DepositKind]int32{
	game.DepositFertile: 500,
	game.DepositTimber:  800,
	game.DepositGame:    300,
}

// zonalBiomeAccepts reports whether kind can spawn on ter. Fertile
// covers the plains / grassland family; Timber covers the forest
// family (Forest, Taiga, Jungle — the project's three forest biomes);
// Game spans both so a grassland tile can carry Game even when Fertile
// gates it out via threshold. Unknown kinds and terrains fall through
// to false so a future kind without an entry here fails loudly in
// tests rather than silently producing zero deposits.
func zonalBiomeAccepts(kind game.DepositKind, ter game.Terrain) bool {
	switch kind {
	case game.DepositFertile:
		switch ter {
		case game.TerrainPlains,
			game.TerrainGrassland,
			game.TerrainMeadow,
			game.TerrainSavanna:
			return true
		}
	case game.DepositTimber:
		switch ter {
		case game.TerrainForest,
			game.TerrainTaiga,
			game.TerrainJungle:
			return true
		}
	case game.DepositGame:
		switch ter {
		case game.TerrainForest,
			game.TerrainTaiga,
			game.TerrainJungle,
			game.TerrainGrassland,
			game.TerrainMeadow,
			game.TerrainSavanna:
			return true
		}
	}
	return false
}

// zonalDepositAt returns the zonal deposit covering t when the tile
// passes both the biome gate and the Perlin threshold for at least one
// zonal kind, otherwise (Deposit{}, false). Iterates kinds in
// declaration order so overlapping biomes (forest satisfies both
// Timber and Game) resolve as Timber — the "one deposit per tile"
// tiebreak. Noise is sampled only when the biome gate passes, so
// mountain / ocean / desert tiles pay nothing per-kind.
func zonalDepositAt(
	t game.Position,
	ter game.Terrain,
	noises map[game.DepositKind]OctaveNoise,
) (game.Deposit, bool) {
	fx := float64(t.X) * zonalPerlinScale
	fy := float64(t.Y) * zonalPerlinScale
	for _, kind := range zonalKinds {
		if !zonalBiomeAccepts(kind, ter) {
			continue
		}
		noise := noises[kind]
		v := noise.Eval2Normalized(fx, fy)
		if v <= zonalThresholds[kind] {
			continue
		}
		return game.Deposit{
			Position:      t,
			Kind:          kind,
			MaxAmount:     zonalMaxAmount[kind],
			CurrentAmount: zonalMaxAmount[kind],
		}, true
	}
	return game.Deposit{}, false
}

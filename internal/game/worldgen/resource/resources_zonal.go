package resource

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/noise"
)

// zonalKindCount is the number of zonal deposit kinds.
const zonalKindCount = 3

// zonalSpec is one entry in the flat per-kind table the hot zonal
// sampler walks. Grouping kind / salt / threshold / amount into a
// single struct lets zonalDepositAt iterate zonalSpecs with zero map
// lookups on the per-tile path — previously each accepted biome cost
// three map hits (noise, threshold, amount).
type zonalSpec struct {
	kind      world.DepositKind
	subSalt   int64
	threshold float64
	maxAmount int32
}

// zonalSpecs is the flat per-kind table for every zonal deposit kind.
// Iteration order defines the "one deposit per tile" tiebreak:
// overlapping biomes (forest satisfies both Timber and Game) resolve
// to the first kind whose threshold the tile passes. Fertile, Timber,
// Game is the plan's canonical order.
var zonalSpecs = [zonalKindCount]zonalSpec{
	{
		kind:      world.DepositFertile,
		subSalt:   genprim.ToInt64(0x1a7f15c9e3779b0d),
		threshold: 0.557,
		maxAmount: 500,
	},
	{
		kind:      world.DepositTimber,
		subSalt:   genprim.ToInt64(0x2b3e8f27a6c14d5e),
		threshold: 0.540,
		maxAmount: 800,
	},
	{
		kind:      world.DepositGame,
		subSalt:   genprim.ToInt64(0x3c5d2f59e4a8b2af),
		threshold: 0.547,
		maxAmount: 300,
	},
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
var zonalNoiseOpts = noise.OctaveOpts{
	Octaves:     3,
	Persistence: 0.5,
	Lacunarity:  2.0,
	Scale:       1,
}

// zonalBiomeAccepts reports whether kind can spawn on ter. Fertile
// covers the plains / grassland family; Timber covers the forest
// family (Forest, Taiga, Jungle — the project's three forest biomes);
// Game spans both so a grassland tile can carry Game even when Fertile
// gates it out via threshold. Unknown kinds and terrains fall through
// to false so a future kind without an entry here fails loudly in
// tests rather than silently producing zero deposits.
func zonalBiomeAccepts(kind world.DepositKind, ter world.Terrain) bool {
	switch kind {
	case world.DepositFertile:
		switch ter {
		case world.TerrainPlains,
			world.TerrainGrassland,
			world.TerrainMeadow,
			world.TerrainSavanna:
			return true
		}
	case world.DepositTimber:
		switch ter {
		case world.TerrainForest,
			world.TerrainTaiga,
			world.TerrainJungle:
			return true
		}
	case world.DepositGame:
		switch ter {
		case world.TerrainForest,
			world.TerrainTaiga,
			world.TerrainJungle,
			world.TerrainGrassland,
			world.TerrainMeadow,
			world.TerrainSavanna:
			return true
		}
	}
	return false
}

// zonalDepositAt returns the zonal deposit covering t when the tile
// passes both the biome gate and the Perlin threshold for at least one
// zonal kind, otherwise (Deposit{}, false). Iterates zonalSpecs in
// declaration order so overlapping biomes (forest satisfies both
// Timber and Game) resolve as Timber — the "one deposit per tile"
// tiebreak. Noise is sampled only when the biome gate passes, so
// mountain / ocean / desert tiles pay nothing per-kind.
//
// noises is indexed by zonalSpecs slot (0, 1, 2) rather than by
// DepositKind. The fixed-array shape removes three map lookups per
// accepted biome on the per-tile hot path — at hundreds of thousands
// of tiles per super-region the map overhead dominated the noise
// evaluation itself.
func zonalDepositAt(
	t geom.Position,
	ter world.Terrain,
	noises *[zonalKindCount]noise.OctaveNoise,
) (world.Deposit, bool) {
	fx := float64(t.X) * zonalPerlinScale
	fy := float64(t.Y) * zonalPerlinScale
	for i := range zonalSpecs {
		spec := &zonalSpecs[i]
		if !zonalBiomeAccepts(spec.kind, ter) {
			continue
		}
		v := noises[i].Eval2Normalized(fx, fy)
		if v <= spec.threshold {
			continue
		}
		return world.Deposit{
			Position:      t,
			Kind:          spec.kind,
			MaxAmount:     spec.maxAmount,
			CurrentAmount: spec.maxAmount,
		}, true
	}
	return world.Deposit{}, false
}

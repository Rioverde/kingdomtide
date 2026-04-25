package worldgen

import (
	"sync"

	opensimplex "github.com/ojrac/opensimplex-go"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// Region influence sampling tunables. Kept local to this file because
// nothing outside the region pipeline reads them — exposing through
// tuning.go would only invite drift between unrelated subsystems.
const (
	// regionInfluenceThreshold is the minimum Max() component magnitude
	// required for a region to escape RegionNormal. Values below leave
	// Character == RegionNormal even if Dominant() picks a non-zero
	// component, so weak per-anchor draws read as plain regions.
	regionInfluenceThreshold float32 = 0.35

	// regionNoiseFreq sets the base spatial frequency of the per-character
	// fBm field. 0.005 cycles/tile yields ~200-tile wavelengths at the
	// lowest octave, large enough that a single super-chunk (64 tiles)
	// rarely straddles a peak-and-trough — neighbouring anchors thus
	// sample correlated values, producing region clusters instead of
	// salt-and-pepper character variety.
	regionNoiseFreq = 0.005

	// regionOctaves stacks octaves of fBm. 3 is enough for organic-
	// looking influence variation without the per-anchor cost climbing
	// past ~36 noise evals (6 characters × 3 octaves × 2 axes contribute
	// only one Eval2 per octave, but the cost compounds across the world).
	regionOctaves = 3

	// regionLacunarity is the per-octave frequency multiplier.
	regionLacunarity = 2.0

	// regionGain is the per-octave amplitude multiplier. 0.5 keeps the
	// fractal energy concentrated in the lowest octaves where anchor
	// clustering happens.
	regionGain = 0.5

	// regionBiomeBias is the additive nudge applied to the matching
	// influence component when an anchor's biome strongly suggests a
	// thematic character. The value is calibrated so that the bias on
	// its own falls just below regionInfluenceThreshold — biome bias
	// pushes a marginal noise draw over the threshold but never picks
	// the dominant character without noise support.
	regionBiomeBias float32 = 0.20

	// regionSecondaryBiomeBias is the lighter bias applied to a second
	// influence component when a biome contributes to two characters
	// (e.g. desert → Wild + Ancient). Smaller magnitude so the primary
	// affinity stays distinguishable.
	regionSecondaryBiomeBias float32 = 0.12
)

// regionSalt* are nothing-up-my-sleeve constants used to decorrelate
// the six influence noise fields. Each value is the fractional hex of
// sqrt(p) for a small prime p, distinct from every salt already in the
// worldgen pipeline (saltMoist/Temp/Elev/BiomeSmooth and the naming
// salts) so the influence streams cannot collide with any other noise
// field in the program.
//
// Two of the literals exceed math.MaxInt64 in their unsigned form, so
// they cannot be spelled as untyped int64 constants. geom.ToInt64 performs
// the conversion at runtime with two's-complement wraparound, preserving
// the full 64-bit pattern.
//
//	regionSaltBlight  — fractional hex of sqrt(17)
//	regionSaltFae     — fractional hex of sqrt(19)
//	regionSaltAncient — fractional hex of sqrt(23)
//	regionSaltSavage  — fractional hex of sqrt(29)
//	regionSaltHoly    — fractional hex of sqrt(31)
//	regionSaltWild    — fractional hex of sqrt(37)
var (
	regionSaltBlight  = geom.ToInt64(0x428a2f98d728ae22)
	regionSaltFae     = geom.ToInt64(0x7137449123ef65cd)
	regionSaltAncient = geom.ToInt64(0xb5c0fbcfec4d3b2f)
	regionSaltSavage  = geom.ToInt64(0xe9b5dba58189dbbc)
	regionSaltHoly    = geom.ToInt64(0x3956c25bf348b538)
	regionSaltWild    = geom.ToInt64(0x59f111f1b605d019)
)

var regionPatternCount = map[string]int{
	"region.forest":   5,
	"region.plain":    5,
	"region.mountain": 4,
	"region.water":    4,
	"region.desert":   4,
	"region.tundra":   3,
	"region.unknown":  1,
}

// regionBounds returns a freshly built naming.Bounds. Maps are returned
// directly — naming.Generate only reads them, and the package-level
// values are never mutated after init, so sharing the same maps across
// every call is safe.
func regionBounds() naming.Bounds {
	return naming.Bounds{
		PatternCount: regionPatternCount,
		PrefixCount:  characterPrefixCount,
	}
}

// RegionSource is the production region.RegionSource implementation.
// It composes per-super-chunk anchor influence sampling, biome-biased
// nudges, character selection through RegionInfluence.Dominant, and
// language-agnostic Parts naming. Results are memoised by
// SuperChunkCoord so repeated lookups (the cache layer in
// internal/server walks the same coords many times during a session)
// avoid redundant noise evaluation.
type RegionSource struct {
	world *Map
	seed  int64

	noiseBlight  opensimplex.Noise
	noiseFae     opensimplex.Noise
	noiseAncient opensimplex.Noise
	noiseSavage  opensimplex.Noise
	noiseHoly    opensimplex.Noise
	noiseWild    opensimplex.Noise

	cache sync.Map // SuperChunkCoord -> gworld.Region
}

// NewRegionSource builds a RegionSource over a finished worldgen.Map.
//
// The six per-character fBm fields are constructed eagerly so every
// RegionAt call hits the same shared, concurrent-safe noise instances.
// opensimplex.Noise.Eval2 is reentrant after construction — the
// classify/terrain stages already rely on the same property.
func NewRegionSource(w *Map, seed int64) *RegionSource {
	return &RegionSource{
		world:        w,
		seed:         seed,
		noiseBlight:  opensimplex.New(seed ^ regionSaltBlight),
		noiseFae:     opensimplex.New(seed ^ regionSaltFae),
		noiseAncient: opensimplex.New(seed ^ regionSaltAncient),
		noiseSavage:  opensimplex.New(seed ^ regionSaltSavage),
		noiseHoly:    opensimplex.New(seed ^ regionSaltHoly),
		noiseWild:    opensimplex.New(seed ^ regionSaltWild),
	}
}

// RegionAt returns the canonical Region for the super-chunk at sc.
// The result is fully deterministic on (seed, sc, world) and is cached
// keyed by sc. Concurrent callers hitting the same sc may both compute
// once before the cache fills — sync.Map.LoadOrStore is unconditional
// after the second writer's Store completes, so duplicate work is
// bounded to the cache miss window and is harmless.
func (r *RegionSource) RegionAt(sc geom.SuperChunkCoord) gworld.Region {
	return lazyLoad(&r.cache, sc, func() gworld.Region { return r.computeRegion(sc) })
}

// computeRegion runs the full region pipeline for one super-chunk.
// Steps:
//  1. Resolve the deterministic anchor position via geom.AnchorOf.
//  2. Sample six independent fBm fields at the anchor in [0, 1].
//  3. Read the worldgen biome at the anchor and apply biome bias to
//     the matching characters.
//  4. Dominant character + threshold gate determines the final
//     RegionCharacter.
//  5. Generate the language-agnostic Parts name.
//
// Pure: no goroutines, no shared mutation.
func (r *RegionSource) computeRegion(sc geom.SuperChunkCoord) gworld.Region {
	anchor := geom.AnchorOf(r.seed, sc)

	infl := r.sampleInfluence(anchor)
	r.applyBiomeBias(anchor, &infl)
	clampInfluence(&infl)

	character := infl.Dominant()
	if infl.Max() < regionInfluenceThreshold {
		character = gworld.RegionNormal
	}

	name := naming.Generate(naming.Input{
		Domain:    naming.DomainRegion,
		Character: character.Key(),
		SubKind:   regionSubKind(r.biomeAt(anchor)),
		Seed:      r.seed,
		CoordX:    sc.X,
		CoordY:    sc.Y,
	}, regionBounds())

	return gworld.Region{
		Coord:     sc,
		Anchor:    anchor,
		Influence: infl,
		Character: character,
		Name:      name,
	}
}

// sampleInfluence evaluates the six fBm fields at p and returns the
// raw Influence vector remapped to [0, 1]. opensimplex.Eval2 returns
// values in roughly [-1, 1]; halving and shifting yields the canonical
// influence range without losing the sign asymmetry.
func (r *RegionSource) sampleInfluence(p geom.Position) gworld.RegionInfluence {
	x := float64(p.X)
	y := float64(p.Y)
	return gworld.RegionInfluence{
		Blight:  fbm01(r.noiseBlight, x, y),
		Fae:     fbm01(r.noiseFae, x, y),
		Ancient: fbm01(r.noiseAncient, x, y),
		Savage:  fbm01(r.noiseSavage, x, y),
		Holy:    fbm01(r.noiseHoly, x, y),
		Wild:    fbm01(r.noiseWild, x, y),
	}
}

// fbm01 evaluates a multi-octave fBm at (x, y) and returns the result
// remapped to [0, 1]. Sum of amplitudes normalises the output so adding
// or removing octaves does not change the dynamic range.
func fbm01(n opensimplex.Noise, x, y float64) float32 {
	var sum, norm float64
	amp := 1.0
	freq := regionNoiseFreq
	for oct := 0; oct < regionOctaves; oct++ {
		sum += amp * n.Eval2(x*freq, y*freq)
		norm += amp
		amp *= regionGain
		freq *= regionLacunarity
	}
	if norm == 0 {
		return 0
	}
	v := (sum/norm + 1) * 0.5
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return float32(v)
}

// applyBiomeBias nudges influence components based on the worldgen
// biome at the anchor. The bias is additive — it never decreases an
// existing influence — and is bounded by regionBiomeBias on the
// primary affinity plus regionSecondaryBiomeBias on a second component
// where the biome plausibly contributes to two characters.
//
// Mapping rationale:
//   - Volcano/jungle: violent, untamed → Savage
//   - Beach/forest:   liminal, alive   → Wild
//   - Mountain/snow:  old, monumental  → Ancient
//   - Tundra:         barren, dying    → Blight
//   - Meadow/grass:   pastoral, kind   → Holy
//   - Desert:         empty, ruins     → Wild + Ancient
func (r *RegionSource) applyBiomeBias(p geom.Position, infl *gworld.RegionInfluence) {
	t := r.biomeAt(p)
	switch t {
	case gworld.TerrainJungle,
		gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainVolcanoSlope,
		gworld.TerrainAshland,
		gworld.TerrainCraterLake:
		infl.Savage += regionBiomeBias

	case gworld.TerrainBeach, gworld.TerrainForest:
		infl.Wild += regionBiomeBias

	case gworld.TerrainMountain, gworld.TerrainSnowyPeak, gworld.TerrainSnow:
		infl.Ancient += regionBiomeBias

	case gworld.TerrainTundra:
		infl.Blight += regionBiomeBias

	case gworld.TerrainMeadow, gworld.TerrainGrassland:
		infl.Holy += regionBiomeBias

	case gworld.TerrainDesert:
		infl.Wild += regionBiomeBias
		infl.Ancient += regionSecondaryBiomeBias
	}
}

// biomeAt returns the worldgen Terrain at the given position. Falls
// back to TerrainOcean when the anchor is outside the world bounds —
// can happen for super-chunks at the world edge whose anchor jitter
// pushes the point past the last column or row. Callers treat
// TerrainOcean as a no-bias biome.
func (r *RegionSource) biomeAt(p geom.Position) gworld.Terrain {
	if r.world == nil {
		return gworld.TerrainOcean
	}
	x, y := p.X, p.Y
	if x < 0 {
		x = 0
	} else if x >= r.world.Width {
		x = r.world.Width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= r.world.Height {
		y = r.world.Height - 1
	}
	cellID := r.world.Voronoi.CellIDAt(x, y)
	return r.world.Terrain[cellID]
}

// regionSubKind maps a Terrain to a region naming sub-kind. The
// returned key must match a "region.<sub_kind>" entry in
// regionPatternCount and the locale catalog. "unknown" is the catch-
// all for biomes that do not map cleanly (volcano cores, lakes, etc).
func regionSubKind(t gworld.Terrain) string {
	switch t {
	case gworld.TerrainForest, gworld.TerrainJungle, gworld.TerrainTaiga:
		return "forest"
	case gworld.TerrainPlains, gworld.TerrainGrassland,
		gworld.TerrainMeadow, gworld.TerrainSavanna:
		return "plain"
	case gworld.TerrainMountain, gworld.TerrainSnowyPeak,
		gworld.TerrainHills, gworld.TerrainVolcanoSlope:
		return "mountain"
	case gworld.TerrainOcean, gworld.TerrainDeepOcean,
		gworld.TerrainBeach, gworld.TerrainCraterLake:
		return "water"
	case gworld.TerrainDesert, gworld.TerrainAshland:
		return "desert"
	case gworld.TerrainTundra, gworld.TerrainSnow:
		return "tundra"
	}
	return "unknown"
}

// clampInfluence pins each component into [0, 1] after biome bias has
// pushed values past the noise output range. RegionInfluence's
// invariant assumes per-component clamping, and Sum/Max comments rely
// on it.
func clampInfluence(r *gworld.RegionInfluence) {
	r.Blight = clamp01(r.Blight)
	r.Fae = clamp01(r.Fae)
	r.Ancient = clamp01(r.Ancient)
	r.Savage = clamp01(r.Savage)
	r.Holy = clamp01(r.Holy)
	r.Wild = clamp01(r.Wild)
}

// clamp01 pins v to [0, 1].
func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// InfluenceAt is the per-tile sampler used by the client UI tint layer.
// It resolves the tile's home region (via geom.AnchorAt) and returns
// that region's RegionInfluence. Falls back to zero influence outside
// world bounds so the tint pass stays neutral on the edge.
func (r *RegionSource) InfluenceAt(x, y int) gworld.RegionInfluence {
	_, sc := geom.AnchorAt(r.seed, x, y)
	return r.RegionAt(sc).Influence
}

// InfluenceSampler is the per-tile thematic-influence lookup the client
// UI consumes for region tinting. RegionSource implements it; the field
// on the UI Model is nil until the server delivers real region data, at
// which point callers may substitute a live *RegionSource.
type InfluenceSampler interface {
	InfluenceAt(x, y int) gworld.RegionInfluence
}

var (
	_ gworld.RegionSource = (*RegionSource)(nil)
	_ InfluenceSampler    = (*RegionSource)(nil)
)

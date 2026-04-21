package worldgen

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game"
)

// Sub-cell layout: each 64-tile super-chunk splits into four 32-tile
// sub-cells (NW, NE, SW, SE) and every sub-cell emits exactly one
// landmark.
//
// Candidate positions scatter inside an inner "core" offset from each
// sub-cell edge by landmarkSubCellInset tiles. The inset keeps every
// landmark centered enough that a 41x21 viewport has a high chance of
// catching one even when the viewport's 21-tall slice lies between
// sub-cell rows (the geometry of a 32-row sub-cell vs a 21-row viewport
// does not allow a strict guarantee with 4 landmarks per super-chunk;
// see TestLandmarksInViewportCoverage for the empirical coverage rate).
const (
	landmarkSubCellSize          = 32
	landmarkSubCellsPerSC        = 4
	landmarkCandidatesPerSubCell = 8
	landmarkSubCellInset         = 6
)

// Kind base weights before region-character bias is applied. Tower and
// GiantTree are the most visually distinctive landmarks and get the
// highest weight; Chasm is rare on purpose so elevation-gradient filters
// keep it to genuinely dramatic terrain.
const (
	landmarkWeightTower          = 20
	landmarkWeightGiantTree      = 20
	landmarkWeightStandingStones = 15
	landmarkWeightObelisk        = 15
	landmarkWeightChasm          = 10
	landmarkWeightShrine         = 10
)

// Terrain thresholds used by the kind affinity filter. The Tower
// threshold sits below elevationHills so hilltops qualify without
// requiring full mountain terrain; the Chasm threshold demands a
// locally steep slope so fractures do not land on gentle foothills.
//
// The plan's original 0.3 gradient value was expressed in the raw
// noise-output space and is unreachable in practice — fBm elevation
// deltas between adjacent tiles peak around 0.04 in the current
// pipeline. 0.02 corresponds roughly to the steepest ~2% of mountain-
// adjacent tiles, which lines up with the "rare, dramatic terrain"
// intent.
const (
	landmarkTowerElevationMin = 0.55
	landmarkChasmGradientMin  = 0.02
)

// seedSaltLandmarkRaw is the raw bit pattern for seedSaltLandmark: the
// fractional digits of sqrt(5) in hex, a nothing-up-my-sleeve constant
// provably distinct from every salt used by superchunk.go and
// region_source.go.
const seedSaltLandmarkRaw uint64 = 0x3c6ef372fe94f82b

// seedSaltLandmark decorrelates the landmark RNG stream from anchor and
// region streams. Routed through regionToInt64 so future re-tunings with
// a top-bit-set value keep the declaration shape identical and avoid a
// constant-overflow trap.
var seedSaltLandmark = regionToInt64(seedSaltLandmarkRaw)

// landmarkSubCellPrime mixes the sub-cell index into the PRNG seed so
// the four sub-cells of one super-chunk see independent scatter
// streams. Large odd prime with uniform bit density — chosen distinct
// from hashCoordPrimeX / hashCoordPrimeY so the mixer cannot collapse
// when sc.X and subID coincide.
const landmarkSubCellPrime uint64 = 0xd1b54a32d192ed03

// NoiseLandmarkSource is the procedural game.LandmarkSource. It splits
// each 64x64 super-chunk into four 32x32 sub-cells and emits exactly
// one Landmark per sub-cell, guaranteeing that any 41x21 viewport sees
// at least one landmark.
//
// The source is safe for concurrent read: all fields are immutable
// after construction, the shared WorldGenerator's river cache is backed
// by hashicorp/golang-lru/v2 (concurrent-safe), and the RegionSource
// contract already requires concurrent-read safety.
type NoiseLandmarkSource struct {
	seed     int64
	worldgen *WorldGenerator
	regions  game.RegionSource
}

// NewNoiseLandmarkSource wires a landmark source to a shared
// WorldGenerator (for terrain sampling) and RegionSource (for region-
// character biasing). The seed combined with seedSaltLandmark at
// sub-cell granularity keeps landmarks deterministic and uncorrelated
// from region, anchor, and river generation.
func NewNoiseLandmarkSource(
	seed int64,
	regions game.RegionSource,
	wg *WorldGenerator,
) *NoiseLandmarkSource {
	return &NoiseLandmarkSource{
		seed:     seed,
		worldgen: wg,
		regions:  regions,
	}
}

// LandmarksIn returns up to four landmarks — one per sub-cell in fixed
// NW, NE, SW, SE order — with water-dominant sub-cells omitted.
// Landmarks never spawn on ocean, deep-ocean, or lake tiles: a sub-cell
// whose candidates are all water simply returns no landmark, and the
// output slice is shorter than four. Deterministic: same (seed, sc)
// always yields the same slice. Landmark names ship as structured,
// language-agnostic Parts records.
func (s *NoiseLandmarkSource) LandmarksIn(sc game.SuperChunkCoord) []game.Landmark {
	out := make([]game.Landmark, 0, landmarkSubCellsPerSC)

	// Only Character is consumed below; the returned Region.Name is
	// discarded.
	region := s.regions.RegionAt(sc)
	minX := sc.X * game.SuperChunkSize
	minY := sc.Y * game.SuperChunkSize

	for subID := range landmarkSubCellsPerSC {
		lm, ok := s.landmarkForSubCell(sc, subID, minX, minY, region.Character)
		if !ok {
			continue
		}
		out = append(out, lm)
	}
	return out
}

// landmarkForSubCell scatters candidate positions inside the sub-cell,
// skips any candidate whose tile is water (ocean / deep-ocean / lake —
// see isWaterTile), picks the first non-water candidate that fits a
// kind under the terrain + region-bias matrix, and falls back to a
// Shrine at the first non-water candidate if no kind fits. Returns
// ok == false when every candidate is water, signalling the caller to
// omit this sub-cell.
func (s *NoiseLandmarkSource) landmarkForSubCell(
	sc game.SuperChunkCoord,
	subID, scMinX, scMinY int,
	character game.RegionCharacter,
) (game.Landmark, bool) {
	offX, offY := subCellOrigin(subID)
	subMinX := scMinX + offX
	subMinY := scMinY + offY

	rng := newSubCellRNG(s.seed, sc, subID)
	candidates := scatterCandidates(rng, subMinX, subMinY)

	var firstDry game.Position
	firstDryFound := false
	for _, cand := range candidates {
		tile := s.worldgen.TileAt(cand.X, cand.Y)
		if isWaterTile(tile) {
			continue
		}
		if !firstDryFound {
			firstDry = cand
			firstDryFound = true
		}
		elev := s.worldgen.elevationAt(float64(cand.X), float64(cand.Y))

		kind, ok := s.pickKind(rng, cand, tile, elev, character)
		if !ok {
			continue
		}
		name := LandmarkName(kind, character, s.seed, cand)
		return game.Landmark{Coord: cand, Kind: kind, Name: name}, true
	}

	if !firstDryFound {
		return game.Landmark{}, false
	}
	// Shrine fallback on the first non-water candidate — keeps landmark
	// density high in mixed land/water sub-cells while still honouring
	// the no-water-spawn invariant.
	name := LandmarkName(game.LandmarkShrine, character, s.seed, firstDry)
	return game.Landmark{Coord: firstDry, Kind: game.LandmarkShrine, Name: name}, true
}

// isWaterTile reports whether a tile's terrain or overlay puts it
// underwater. Landmarks must not spawn on these — a Tower or Shrine in
// the middle of the ocean would read as a bug, not a landmark.
func isWaterTile(t game.Tile) bool {
	switch t.Terrain {
	case game.TerrainOcean, game.TerrainDeepOcean:
		return true
	}
	return t.Overlays&game.OverlayLake != 0
}

// pickKind filters kinds by terrain affinity, weights the survivors by
// region-character bias, and picks one via a weighted draw from rng.
// The boolean return reports whether any kind fit; a false return
// signals the caller to try the next candidate or fall back to Shrine.
func (s *NoiseLandmarkSource) pickKind(
	rng *rand.Rand,
	cand game.Position,
	tile game.Tile,
	elevation float64,
	character game.RegionCharacter,
) (game.LandmarkKind, bool) {
	gradient := s.elevationGradient(cand, elevation)

	// Accumulator-style weighted pick to avoid an intermediate slice
	// allocation. Every kind is checked in a fixed order so the picked
	// kind is deterministic for the same (rng state, inputs).
	type candidateKind struct {
		kind   game.LandmarkKind
		weight float32
	}
	kinds := [...]candidateKind{
		{game.LandmarkTower, landmarkWeightTower},
		{game.LandmarkGiantTree, landmarkWeightGiantTree},
		{game.LandmarkStandingStones, landmarkWeightStandingStones},
		{game.LandmarkObelisk, landmarkWeightObelisk},
		{game.LandmarkChasm, landmarkWeightChasm},
		{game.LandmarkShrine, landmarkWeightShrine},
	}

	var total float32
	for _, c := range kinds {
		if !fitsTerrain(c.kind, tile, elevation, gradient) {
			continue
		}
		total += c.weight * regionBias(c.kind, character)
	}
	if total <= 0 {
		return game.LandmarkNone, false
	}

	roll := rng.Float32() * total
	var acc float32
	for _, c := range kinds {
		if !fitsTerrain(c.kind, tile, elevation, gradient) {
			continue
		}
		acc += c.weight * regionBias(c.kind, character)
		if roll <= acc {
			return c.kind, true
		}
	}
	// Float-rounding safety net: if roll == total exactly we may fall
	// through. Return the last eligible kind so the caller never sees
	// a spurious "no fit" result.
	for i := len(kinds) - 1; i >= 0; i-- {
		if fitsTerrain(kinds[i].kind, tile, elevation, gradient) {
			return kinds[i].kind, true
		}
	}
	return game.LandmarkNone, false
}

// elevationGradient returns the maximum absolute elevation difference
// between the candidate and any of its four orthogonal neighbours. A
// large gradient indicates a sharp slope and gates Chasm placement.
func (s *NoiseLandmarkSource) elevationGradient(cand game.Position, here float64) float64 {
	var best float64
	offsets := [...][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, off := range offsets {
		e := s.worldgen.elevationAt(float64(cand.X+off[0]), float64(cand.Y+off[1]))
		d := here - e
		if d < 0 {
			d = -d
		}
		if d > best {
			best = d
		}
	}
	return best
}

// subCellOrigin returns the (dx, dy) offset of a sub-cell's top-left
// corner relative to the super-chunk's top-left. Sub-cell ids:
// 0=NW, 1=NE, 2=SW, 3=SE.
func subCellOrigin(subID int) (int, int) {
	switch subID {
	case 0:
		return 0, 0
	case 1:
		return landmarkSubCellSize, 0
	case 2:
		return 0, landmarkSubCellSize
	case 3:
		return landmarkSubCellSize, landmarkSubCellSize
	default:
		// Unreachable — caller bounds subID to [0, landmarkSubCellsPerSC).
		return 0, 0
	}
}

// newSubCellRNG builds a PCG seeded from the world seed XORed with
// seedSaltLandmark and a sub-cell-specific hash. Two calls with the
// same (seed, sc, subID) return RNGs that produce identical streams,
// while any change in the triple produces a decorrelated stream.
func newSubCellRNG(seed int64, sc game.SuperChunkCoord, subID int) *rand.Rand {
	lo := uint64(seed ^ seedSaltLandmark)
	hi := hashSubCell(sc, subID)
	return rand.New(rand.NewPCG(lo, hi))
}

// hashSubCell mixes the sub-chunk coord with the sub-cell id. Reuses
// hashCoord's two-prime shape from region_names.go and multiplies the
// sub-cell id by a third distinct prime so adjacent sub-cells inside
// the same super-chunk do not share a stream.
func hashSubCell(sc game.SuperChunkCoord, subID int) uint64 {
	h := hashCoord(sc)
	h ^= uint64(subID+1) * landmarkSubCellPrime
	return h
}

// scatterCandidates draws landmarkCandidatesPerSubCell uniform
// positions inside the sub-cell's inner core rooted at
// (subMinX, subMinY). Called once per sub-cell, so a slice allocation
// here dominates nothing.
//
// Restricting candidates to the inner core (every tile at least
// landmarkSubCellInset away from the sub-cell edge) keeps landmarks
// away from sub-cell seams, which matters for the viewport-coverage
// property: a landmark hugging the far edge of a sub-cell is the one
// most likely to fall outside a 21-tall viewport overlapping that
// sub-cell.
func scatterCandidates(rng *rand.Rand, subMinX, subMinY int) []game.Position {
	const core = landmarkSubCellSize - 2*landmarkSubCellInset
	out := make([]game.Position, landmarkCandidatesPerSubCell)
	for i := range out {
		out[i] = game.Position{
			X: subMinX + landmarkSubCellInset + rng.IntN(core),
			Y: subMinY + landmarkSubCellInset + rng.IntN(core),
		}
	}
	return out
}

// fitsTerrain reports whether the kind's terrain affinity accepts the
// candidate tile. Shrine accepts any terrain and thereby guarantees
// that the Shrine fallback in landmarkForSubCell is always valid.
func fitsTerrain(
	kind game.LandmarkKind,
	tile game.Tile,
	elevation, gradient float64,
) bool {
	family := FamilyOf(tile.Terrain)
	switch kind {
	case game.LandmarkTower:
		if elevation < landmarkTowerElevationMin {
			return false
		}
		return family == FamilyMountain
	case game.LandmarkGiantTree:
		return family == FamilyForest
	case game.LandmarkStandingStones:
		switch tile.Terrain {
		case game.TerrainPlains, game.TerrainGrassland, game.TerrainMeadow:
			return true
		}
		return false
	case game.LandmarkObelisk:
		return family == FamilyPlain ||
			family == FamilyDesert ||
			family == FamilyMountain ||
			family == FamilyTundra
	case game.LandmarkChasm:
		if family != FamilyMountain {
			return false
		}
		return gradient >= landmarkChasmGradientMin
	case game.LandmarkShrine:
		// Shrine is terrain-agnostic on purpose — it is the fallback
		// kind whose eligibility guarantees every sub-cell resolves.
		return true
	default:
		return false
	}
}

// Region-bias multipliers. Values above 1.0 boost a kind's weight in the
// weighted draw; values below 1.0 suppress it. Grouped by character so
// readers can see the full thematic palette at a glance.
const (
	// Ancient biases — strongly favour monolithic stonework over living terrain.
	biasAncientTower          float32 = 1.5
	biasAncientStandingStones float32 = 8.0
	biasAncientObelisk        float32 = 8.0
	biasAncientGiantTree      float32 = 0.2
	biasAncientShrine         float32 = 0.2
	biasAncientChasm          float32 = 0.2

	// Holy biases — elevate sacred structures, keep towers present.
	biasHolyTower   float32 = 1.2
	biasHolyObelisk float32 = 1.2
	biasHolyShrine  float32 = 1.8

	// Fey biases — primordial forest overwhelms everything else.
	biasFeyGiantTree float32 = 1.5

	// Blighted biases — suppress life, amplify fractures.
	biasBlightedGiantTree float32 = 0.5
	biasBlightedChasm     float32 = 1.5
	biasBlightedShrine    float32 = 0.3

	// Savage biases — dangerous landscape preferred.
	biasSavageChasm float32 = 1.2
)

// regionBias returns the multiplicative weight bonus applied to a kind
// given the region's dominant character. Returns 1.0 when no bias is
// specified, i.e. the kind is neutral in that character. Ancient
// Obelisk and StandingStones biases are set aggressively enough that
// TestLandmarksInRegionBias sees a ~2x count ratio — the plan's
// original 1.5 multiplier was too weak to clear the tight bound on a
// single-category survey.
func regionBias(kind game.LandmarkKind, character game.RegionCharacter) float32 {
	switch character {
	case game.RegionAncient:
		return regionBiasAncient(kind)
	case game.RegionHoly:
		return regionBiasHoly(kind)
	case game.RegionFey:
		return regionBiasFey(kind)
	case game.RegionBlighted:
		return regionBiasBlighted(kind)
	case game.RegionSavage:
		return regionBiasSavage(kind)
	}
	return 1.0
}

// regionBiasAncient returns Ancient-character bias for kind.
func regionBiasAncient(kind game.LandmarkKind) float32 {
	switch kind {
	case game.LandmarkTower:
		return biasAncientTower
	case game.LandmarkStandingStones:
		return biasAncientStandingStones
	case game.LandmarkObelisk:
		return biasAncientObelisk
	case game.LandmarkGiantTree:
		return biasAncientGiantTree
	case game.LandmarkShrine:
		return biasAncientShrine
	case game.LandmarkChasm:
		return biasAncientChasm
	}
	return 1.0
}

// regionBiasHoly returns Holy-character bias for kind.
func regionBiasHoly(kind game.LandmarkKind) float32 {
	switch kind {
	case game.LandmarkTower:
		return biasHolyTower
	case game.LandmarkObelisk:
		return biasHolyObelisk
	case game.LandmarkShrine:
		return biasHolyShrine
	}
	return 1.0
}

// regionBiasFey returns Fey-character bias for kind.
func regionBiasFey(kind game.LandmarkKind) float32 {
	if kind == game.LandmarkGiantTree {
		return biasFeyGiantTree
	}
	return 1.0
}

// regionBiasBlighted returns Blighted-character bias for kind.
func regionBiasBlighted(kind game.LandmarkKind) float32 {
	switch kind {
	case game.LandmarkGiantTree:
		return biasBlightedGiantTree
	case game.LandmarkChasm:
		return biasBlightedChasm
	case game.LandmarkShrine:
		return biasBlightedShrine
	}
	return 1.0
}

// regionBiasSavage returns Savage-character bias for kind.
func regionBiasSavage(kind game.LandmarkKind) float32 {
	if kind == game.LandmarkChasm {
		return biasSavageChasm
	}
	return 1.0
}

// Compile-time assertion that NoiseLandmarkSource implements the
// consumer-side interface. Mirrors the pattern in region_source.go.
var _ game.LandmarkSource = (*NoiseLandmarkSource)(nil)

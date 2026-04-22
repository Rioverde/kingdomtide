package worldgen

import (
	"math"
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game"
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
	volcanoSuperRegionSideSC    = 4
	volcanoSuperRegionSideTiles = volcanoSuperRegionSideSC * game.SuperChunkSize
	volcanoMinSpacingTiles      = 100
	volcanoPoissonK             = 30
	volcanoBiomeWeightMax       = 1.5
)

// Nothing-up-my-sleeve salts. Each value is a documented 64-bit constant
// distinct from every other worldgen salt already in use
// (superchunk.go, region_source.go, landmarks.go). Routed through
// regionToInt64 because the top bit is set — Go rejects these as untyped
// signed literals, so the runtime conversion preserves the bit pattern.
var (
	// Random 64-bit nothing-up-my-sleeve pattern.
	seedSaltVolcanoAnchor = regionToInt64(0xca6e7f91b8d1a4e3)
	// SHA-256 initial hash H0 — fractional digits of sqrt(2).
	seedSaltVolcanoState = regionToInt64(0x6a09e667f3bcc908)
	// SHA-256 initial hash H1 — fractional digits of sqrt(3).
	seedSaltVolcanoFootprint = regionToInt64(0xbb67ae8584caa73b)
)

// volcanoSuperRegionPrime mixes a super-region coord into the Poisson-disk
// seed. Distinct from hashCoordPrimeX, hashCoordPrimeY, and
// landmarkSubCellPrime so an unlucky super-region coord cannot collide
// with a region or sub-cell stream.
const volcanoSuperRegionPrime uint64 = 0x94d049bb133111eb

// superRegion is one 8x8-super-chunk tile of the world plane. Placement
// runs per super-region; super-regions tile the world exactly.
type superRegion struct {
	X, Y int
}

// superRegionOf returns the super-region that contains sc. Uses floor
// division so negative super-chunk coords map to negative super-region
// coords consistently with game.WorldToSuperChunk's floor division.
func superRegionOf(sc game.SuperChunkCoord) superRegion {
	return superRegion{
		X: volcanoFloorDiv(sc.X, volcanoSuperRegionSideSC),
		Y: volcanoFloorDiv(sc.Y, volcanoSuperRegionSideSC),
	}
}

// volcanoFloorDiv returns the mathematical floor of a/b for positive b.
// Local copy of internal/game.floorDiv because that helper is unexported.
// Two lines — cheaper than widening the game package's exported surface.
func volcanoFloorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// bounds returns the inclusive top-left tile coord and the side length of
// the super-region's world-space rectangle. Every volcano anchor returned
// by pickVolcanoAnchors lives inside [minX, minX+side) x [minY, minY+side).
func (sr superRegion) bounds() (minX, minY, side int) {
	side = volcanoSuperRegionSideTiles
	minX = sr.X * side
	minY = sr.Y * side
	return
}

// hashSR mixes a super-region coord into a 64-bit value. Shape mirrors
// hashCoord — two large primes — but uses volcanoSuperRegionPrime on one
// axis so the stream is decorrelated from region-name and sub-cell
// streams.
func hashSR(sr superRegion) uint64 {
	return uint64(int64(sr.X))*hashCoordPrimeX ^
		uint64(int64(sr.Y))*volcanoSuperRegionPrime
}

// newVolcanoAnchorRNG builds a PCG seeded from (seed, super-region). Two
// calls with the same inputs produce identical streams; any change in
// either input decorrelates the stream.
func newVolcanoAnchorRNG(seed int64, sr superRegion) *rand.Rand {
	lo := uint64(seed ^ seedSaltVolcanoAnchor)
	hi := hashSR(sr)
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
	sr superRegion,
	lm game.LandmarkSource,
	wg *WorldGenerator,
) []game.Position {
	rng := newVolcanoAnchorRNG(seed, sr)
	minX, minY, side := sr.bounds()
	candidates := bridsonSample(rng, minX, minY, side, side, volcanoMinSpacingTiles, volcanoPoissonK)

	out := make([]game.Position, 0, len(candidates))
	for _, p := range candidates {
		if !acceptVolcanoAnchor(p, lm, wg, rng) {
			continue
		}
		out = append(out, p)
	}
	return out
}

// bridsonSample returns Poisson-disk samples inside the rectangle
// [minX, minX+width) x [minY, minY+height) with minimum spacing
// minDistance and k candidate attempts per active point. Deterministic
// from rng — two calls with RNGs in the same state produce the same
// slice in the same order.
//
// Uses a uniform-grid accelerator with cell side = minDistance/sqrt(2)
// so each cell holds at most one sample; neighbour checks cover a 5x5
// cell window. Complexity is O(N) in the number of samples.
func bridsonSample(
	rng *rand.Rand,
	minX, minY, width, height int,
	minDistance, k int,
) []game.Position {
	if width <= 0 || height <= 0 || minDistance <= 0 {
		return nil
	}

	cellSize := float64(minDistance) / math.Sqrt2
	cellsX := int(math.Ceil(float64(width)/cellSize)) + 1
	cellsY := int(math.Ceil(float64(height)/cellSize)) + 1

	grid := make([]int, cellsX*cellsY)
	for i := range grid {
		grid[i] = -1
	}
	cellIndex := func(cx, cy int) int { return cy*cellsX + cx }

	samples := make([]game.Position, 0, 8)
	active := make([]int, 0, 8)

	// Seed point: pick a uniform first sample inside the rectangle.
	first := game.Position{
		X: minX + rng.IntN(width),
		Y: minY + rng.IntN(height),
	}
	samples = append(samples, first)
	active = append(active, 0)
	firstCX := int(float64(first.X-minX) / cellSize)
	firstCY := int(float64(first.Y-minY) / cellSize)
	grid[cellIndex(firstCX, firstCY)] = 0

	minDistSq := float64(minDistance) * float64(minDistance)

	for len(active) > 0 {
		// Pick a random active point; swap-remove on exhaustion.
		aIdx := rng.IntN(len(active))
		origin := samples[active[aIdx]]
		found := false

		for range k {
			// Annulus sample in [r, 2r).
			theta := rng.Float64() * 2.0 * math.Pi
			radius := float64(minDistance) * (1.0 + rng.Float64())
			cx := float64(origin.X) + math.Cos(theta)*radius
			cy := float64(origin.Y) + math.Sin(theta)*radius
			px := int(math.Round(cx))
			py := int(math.Round(cy))

			if px < minX || px >= minX+width || py < minY || py >= minY+height {
				continue
			}

			gx := int(float64(px-minX) / cellSize)
			gy := int(float64(py-minY) / cellSize)
			if gx < 0 || gx >= cellsX || gy < 0 || gy >= cellsY {
				continue
			}

			// Reject if any neighbour sample sits too close. A 5x5
			// window is the standard Bridson cover for min-distance=r
			// with cell side r/sqrt(2) — every grid cell holds at most
			// one sample, and no sample outside this window can be
			// closer than minDistance.
			tooClose := false
			for dy := -2; dy <= 2 && !tooClose; dy++ {
				ny := gy + dy
				if ny < 0 || ny >= cellsY {
					continue
				}
				for dx := -2; dx <= 2; dx++ {
					nx := gx + dx
					if nx < 0 || nx >= cellsX {
						continue
					}
					idx := grid[cellIndex(nx, ny)]
					if idx < 0 {
						continue
					}
					sx := samples[idx].X - px
					sy := samples[idx].Y - py
					if float64(sx*sx+sy*sy) < minDistSq {
						tooClose = true
						break
					}
				}
			}
			if tooClose {
				continue
			}

			samples = append(samples, game.Position{X: px, Y: py})
			active = append(active, len(samples)-1)
			grid[cellIndex(gx, gy)] = len(samples) - 1
			found = true
			break
		}

		if !found {
			// Exhaust this active point — swap-remove to keep the slice
			// compact without reallocating.
			active[aIdx] = active[len(active)-1]
			active = active[:len(active)-1]
		}
	}

	return samples
}

// acceptVolcanoAnchor runs the biome-weight + landmark-rejection gate on
// a candidate anchor. Water tiles (ocean, deep ocean, lake, river) reject
// unconditionally; land terrain accepts with probability weight/max. The
// rng argument is the same stream bridsonSample consumed — sharing one
// RNG keeps the whole placement pipeline deterministic from a single
// (seed, sr) pair.
func acceptVolcanoAnchor(
	p game.Position,
	lm game.LandmarkSource,
	wg *WorldGenerator,
	rng *rand.Rand,
) bool {
	tile := wg.TileAt(p.X, p.Y)
	if isWaterOrRiverTile(tile) {
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
func volcanoBiomeWeight(t game.Terrain) float64 {
	switch t {
	case game.TerrainMountain, game.TerrainSnowyPeak:
		return 3.0
	case game.TerrainHills:
		return 2.0
	case game.TerrainForest, game.TerrainTaiga, game.TerrainJungle:
		return 1.0
	case game.TerrainPlains, game.TerrainGrassland,
		game.TerrainMeadow, game.TerrainSavanna:
		return 0.8
	case game.TerrainDesert, game.TerrainTundra, game.TerrainSnow:
		return 0.5
	default:
		// Beach, ocean, deep ocean, existing volcanic terrains, and any
		// future terrain default to reject.
		return 0
	}
}

// isWaterOrRiverTile extends landmarks.go's isWaterTile with
// OverlayRiver: volcanoes must not spawn on rivers either, not just
// standing water. Kept local to volcano placement so landmarks.go is
// untouched.
func isWaterOrRiverTile(t game.Tile) bool {
	switch t.Terrain {
	case game.TerrainOcean, game.TerrainDeepOcean:
		return true
	}
	return t.Overlays&(game.OverlayLake|game.OverlayRiver) != 0
}

// collidesWithLandmark reports whether any landmark in the 3x3 super-chunk
// neighbourhood around p sits on the exact same tile. Landmarks live on
// specific tiles, not footprints, so an exact-match check is enough.
func collidesWithLandmark(p game.Position, lm game.LandmarkSource) bool {
	if lm == nil {
		return false
	}
	home := game.WorldToSuperChunk(p.X, p.Y)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sc := game.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			for _, l := range lm.LandmarksIn(sc) {
				if l.Coord.Equal(p) {
					return true
				}
			}
		}
	}
	return false
}

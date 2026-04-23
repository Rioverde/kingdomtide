// Package genprim holds the primitives shared between worldgen sub-packages:
// deterministic hashing, super-region placement units, water-tile predicates,
// and Poisson-disk sampling. Lives under internal/ so external consumers
// cannot depend on it — these are implementation details of the worldgen
// pipeline.
package genprim

import (
	"math"
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// PrimeX and PrimeY are large odd primes preserved from the original
// worldgen region-name mixer. They remain part of the public API because
// TestHashCoordDistribution exercises the exact mix used by the pre-split
// code. Distribution tests in downstream packages expect these specific
// constants.
const (
	PrimeX uint64 = 0x9e3779b185ebca87
	PrimeY uint64 = 0xc2b2ae3d27d4eb4f
)

// SuperRegionPrime mixes a super-region coord into a 64-bit value. Distinct
// from PrimeY so the super-region hash stays decorrelated from per-tile and
// super-chunk hash streams.
const SuperRegionPrime uint64 = 0x94d049bb133111eb

// ToInt64 reinterprets a uint64 bit pattern as int64. Routed through a
// helper because Go rejects top-bit-set constants as untyped int64 literals;
// the runtime conversion preserves the full 64-bit pattern.
func ToInt64(u uint64) int64 { return int64(u) }

// HashCoord mixes the two components of a SuperChunkCoord into a single
// uint64 suitable for seeding rand.NewPCG's second input.
func HashCoord(sc geom.SuperChunkCoord) uint64 {
	return uint64(int64(sc.X))*PrimeX ^ uint64(int64(sc.Y))*PrimeY
}

// HashPos mixes a tile coord into a 64-bit value. Same shape as HashCoord
// but takes a geom.Position so callers need not convert.
func HashPos(p geom.Position) uint64 {
	return uint64(int64(p.X))*PrimeX ^ uint64(int64(p.Y))*PrimeY
}

// SplitMix64 mixes three 64-bit inputs into a single well-diffused 64-bit
// value using large-prime multiplication and the SplitMix64 finalizer. Used
// for deterministic seeded jittering and state hashing; not a cryptographic
// hash.
func SplitMix64(a, b, c uint64) uint64 {
	x := a*0x9e3779b97f4a7c15 ^ b*0x6c62272e07bb0142 ^ c*0xbf58476d1ce4e5b9
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// SuperRegionSideSC is the side length of a super-region measured in
// super-chunks. Every super-region is a fixed 4x4 block of super-chunks.
const SuperRegionSideSC = 4

// SuperRegionSideTiles is the side length of a super-region measured in
// world tiles. Precomputed so hot paths avoid the multiplication.
const SuperRegionSideTiles = SuperRegionSideSC * geom.SuperChunkSize

// SuperRegion is one SuperRegionSideSC x SuperRegionSideSC block of
// super-chunks. Volcano placement and resource generation both tile the
// world into super-regions, so the type lives in the shared package rather
// than being duplicated.
type SuperRegion struct {
	X, Y int
}

// SuperRegionOf returns the super-region that contains sc. Uses floor
// division so negative super-chunk coords map to negative super-region
// coords consistently with geom.WorldToSuperChunk's floor division.
func SuperRegionOf(sc geom.SuperChunkCoord) SuperRegion {
	return SuperRegion{
		X: FloorDiv(sc.X, SuperRegionSideSC),
		Y: FloorDiv(sc.Y, SuperRegionSideSC),
	}
}

// Bounds returns the inclusive top-left tile coord and the side length of
// the super-region's world-space rectangle. Every anchor generated inside a
// super-region lives in [minX, minX+side) x [minY, minY+side).
func (sr SuperRegion) Bounds() (minX, minY, side int) {
	side = SuperRegionSideTiles
	minX = sr.X * side
	minY = sr.Y * side
	return
}

// HashSR mixes a super-region coord into a 64-bit value. Uses a distinct
// second prime from HashCoord so super-region streams stay decorrelated
// from region-name and sub-cell streams.
func HashSR(sr SuperRegion) uint64 {
	return uint64(int64(sr.X))*PrimeX ^
		uint64(int64(sr.Y))*SuperRegionPrime
}

// FloorDiv returns the mathematical floor of a/b for positive b. Local copy
// of the unexported game.floorDiv so the worldgen sub-packages need not
// widen game's exported surface.
func FloorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// IsWaterOrRiverTile reports whether a tile's terrain or overlay puts it on
// or in water. Ocean and deep-ocean terrain always count; lake and river
// overlays count regardless of the underlying terrain. Placement pipelines
// (volcano anchors, mineral deposits) reject water-covered tiles via this
// predicate.
func IsWaterOrRiverTile(t world.Tile) bool {
	switch t.Terrain {
	case world.TerrainOcean, world.TerrainDeepOcean:
		return true
	}
	return t.Overlays&(world.OverlayLake|world.OverlayRiver) != 0
}

// BridsonSample returns Poisson-disk samples inside the rectangle
// [minX, minX+width) x [minY, minY+height) with minimum spacing
// minDistance and k candidate attempts per active point. Deterministic
// from rng — two calls with RNGs in the same state produce the same
// slice in the same order.
//
// Uses a uniform-grid accelerator with cell side = minDistance/sqrt(2)
// so each cell holds at most one sample; neighbour checks cover a 5x5
// cell window. Complexity is O(N) in the number of samples.
func BridsonSample(
	rng *rand.Rand,
	minX, minY, width, height int,
	minDistance, k int,
) []geom.Position {
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

	samples := make([]geom.Position, 0, 8)
	active := make([]int, 0, 8)

	// Seed point: pick a uniform first sample inside the rectangle.
	first := geom.Position{
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

			samples = append(samples, geom.Position{X: px, Y: py})
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

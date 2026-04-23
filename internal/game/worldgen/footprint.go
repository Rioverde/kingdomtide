package worldgen

import (
	"math/rand/v2"
	"sort"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// zoneSizes holds the inclusive min / max tile counts for each zone of a
// volcano footprint. Active volcanoes carry the largest footprint; Extinct
// volcanoes heal their ashland back into the surrounding biome so their
// size range is (0, 0).
type zoneSizes struct {
	core    [2]int
	slope   [2]int
	ashland [2]int
}

// tierSizes maps lifecycle state to the footprint size ranges for that
// state. Numbers match the plan's "Zone sizes" table:
//
//	Active:  core 2-4,  slope 5-8,  ashland 8-14  (total 15-26)
//	Dormant: core 1-2,  slope 5-8,  ashland 5-10  (total 11-20)
//	Extinct: core 1-2,  slope 4-7,  ashland 0     (total 5-9)
var tierSizes = map[world.VolcanoState]zoneSizes{
	world.VolcanoActive:  {core: [2]int{2, 4}, slope: [2]int{5, 8}, ashland: [2]int{8, 14}},
	world.VolcanoDormant: {core: [2]int{1, 2}, slope: [2]int{5, 8}, ashland: [2]int{5, 10}},
	world.VolcanoExtinct: {core: [2]int{1, 2}, slope: [2]int{4, 7}, ashland: [2]int{0, 0}},
}

// hashPos mixes a tile coord into a 64-bit value. Shape mirrors hashCoord
// (two large primes XOR-mixed) but uses distinct primes so position
// streams stay decorrelated from super-chunk and super-region streams.
func hashPos(p geom.Position) uint64 {
	return uint64(int64(p.X))*hashCoordPrimeX ^
		uint64(int64(p.Y))*hashCoordPrimeY
}

// newFootprintRNG builds a PCG seeded from (seed, anchor) for the
// per-volcano footprint growth stream. The XOR salt keeps the stream
// decorrelated from anchor-selection and state-assignment streams.
func newFootprintRNG(seed int64, anchor geom.Position) *rand.Rand {
	lo := uint64(seed ^ seedSaltVolcanoFootprint)
	hi := hashPos(anchor)
	return rand.New(rand.NewPCG(lo, hi))
}

// randIntInclusive returns a uniform int in [lo, hi]. Returns lo when
// hi <= lo so a zero-width range collapses cleanly (used for Extinct's
// ashland range {0, 0}).
func randIntInclusive(rng *rand.Rand, lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + rng.IntN(hi-lo+1)
}

// footprintNeighbourOffsets lists the four orthogonal neighbours used by
// walkZone. Diagonal moves are not part of 4-connectivity so the
// footprint stays contiguous.
var footprintNeighbourOffsets = [4][2]int{
	{0, -1},
	{0, 1},
	{-1, 0},
	{1, 0},
}

// growFootprint produces the three zone tile slices for the volcano
// anchored at anchor in the given state. Zones are disjoint, 4-connected,
// sorted in (X, Y) order, and never collide with any landmark in
// landmarks. Ashland may NOT overwrite water tiles — a volcano's ash
// ring stops at the coast. Core and slope ignore water (they overwrite
// it) but the anchor biome gate already keeps cores off open water.
//
// Ashland in the Extinct tier is always empty by design — the ring
// healed back to the surrounding biome. All zone slices are allocated
// even when empty so the downstream sync.Once-backed consumer does not
// need a nil-guard on every iteration.
func growFootprint(
	anchor geom.Position,
	state world.VolcanoState,
	seed int64,
	wg *WorldGenerator,
	landmarks []world.Landmark,
) (core, slope, ashland []geom.Position) {
	rng := newFootprintRNG(seed, anchor)
	sizes, ok := tierSizes[state]
	if !ok {
		return nil, nil, nil
	}

	coreCount := randIntInclusive(rng, sizes.core[0], sizes.core[1])
	slopeCount := randIntInclusive(rng, sizes.slope[0], sizes.slope[1])
	ashlandCount := randIntInclusive(rng, sizes.ashland[0], sizes.ashland[1])

	landmarkSet := make(map[geom.Position]struct{}, len(landmarks))
	for _, l := range landmarks {
		landmarkSet[l.Coord] = struct{}{}
	}

	claimed := make(map[geom.Position]struct{})
	// Core grows from the anchor and always contains it. Landmark tiles
	// are rejected but their neighbours may still propagate the walk so
	// the blob can route around a single landmark in the 1-tile
	// neighbourhood.
	core = walkZone(anchor, nil, coreCount, rng, claimed, func(t geom.Position) bool {
		_, blocked := landmarkSet[t]
		return !blocked
	})

	// Slope expands outward from the ENTIRE core boundary, not a single
	// seed tile — seeding the frontier with every core tile makes the
	// slope radiate concentrically around the core rather than extending
	// as a lobe from one side. anchor is passed for signature symmetry
	// but ignored when prevZone is non-empty.
	slope = walkZone(anchor, core, slopeCount, rng, claimed, func(t geom.Position) bool {
		_, blocked := landmarkSet[t]
		return !blocked
	})

	if ashlandCount > 0 {
		// Ashland radiates outward from every slope tile, same pattern:
		// concentric ring rather than single-lobe walk.
		ashland = walkZone(anchor, slope, ashlandCount, rng, claimed, func(t geom.Position) bool {
			if _, blocked := landmarkSet[t]; blocked {
				return false
			}
			tile := wg.TileAt(t.X, t.Y)
			return !isWaterOrRiverTile(tile)
		})
	}

	sortPositions(core)
	sortPositions(slope)
	sortPositions(ashland)
	return
}

// walkZone grows a 4-connected blob of up to count tiles using a
// frontier-based random walk. The walk picks a random tile from the
// frontier, visits each orthogonal neighbour in a shuffled order, and
// adds the first neighbour that is (a) not already claimed by any zone,
// (b) accepted by the caller's accept predicate. Rejected tiles are
// added to claimed so the frontier does not re-consider them on
// subsequent iterations; this guarantees walkZone terminates — every
// iteration either adds the tile to out (progress) or marks it claimed
// (future skips), and at most 4 visits per frontier tile can happen
// before the tile is exhausted.
//
// Two growth modes, selected by prevZone:
//
//   - Core mode (prevZone == nil). The walk starts from seedTile: the
//     tile is added to out (so the core zone always contains the
//     volcano anchor), and the frontier is initialised to [seedTile].
//     If seedTile is already claimed or rejected, the walk returns
//     early with a zero-length slice.
//
//   - Ring mode (prevZone != nil and len > 0). The walk radiates
//     outward from every tile of prevZone in parallel — the frontier
//     is initialised to a COPY of prevZone so subsequent mutations do
//     not touch the caller's slice. prevZone tiles are already claimed
//     by the prior zone and are NOT re-added to the output; they only
//     anchor the first accepted neighbour of this zone. The resulting
//     ring of new tiles is concentric around the prior zone rather
//     than lobed off a single seed tile. seedTile is accepted for
//     signature symmetry but ignored.
//
// Edge case: an empty prevZone (the prior zone failed to grow past a
// single tile) falls back to core mode with seedTile so the current
// zone still has something to radiate from.
//
// All accepted tiles are written into claimed so cross-zone collisions
// cannot happen — slope and ashland walks see the core's tiles already
// marked.
func walkZone(
	seedTile geom.Position,
	prevZone []geom.Position,
	count int,
	rng *rand.Rand,
	claimed map[geom.Position]struct{},
	accept func(geom.Position) bool,
) []geom.Position {
	if count <= 0 {
		return nil
	}
	out := make([]geom.Position, 0, count)

	tryAdd := func(p geom.Position) bool {
		if _, already := claimed[p]; already {
			return false
		}
		if !accept(p) {
			claimed[p] = struct{}{}
			return false
		}
		claimed[p] = struct{}{}
		out = append(out, p)
		return true
	}

	if len(prevZone) == 0 {
		// Core mode: the seed tile itself is the first output.
		prevZone = nil
	}

	var frontier []geom.Position
	if prevZone == nil {
		if _, already := claimed[seedTile]; !already {
			if !tryAdd(seedTile) {
				return out
			}
		}
		frontier = []geom.Position{seedTile}
	} else {
		// Ring mode: copy the prevZone so frontier mutations stay local.
		frontier = append(make([]geom.Position, 0, len(prevZone)), prevZone...)
	}

	for len(out) < count && len(frontier) > 0 {
		idx := rng.IntN(len(frontier))
		cur := frontier[idx]

		// Shuffle neighbour offsets per-step so the walk is rotationally
		// symmetric — otherwise the blob biases toward the first
		// direction in the fixed offset list.
		shuffled := footprintNeighbourOffsets
		rng.Shuffle(len(shuffled), func(i, j int) {
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})

		added := false
		exhausted := true
		for _, off := range shuffled {
			cand := geom.Position{X: cur.X + off[0], Y: cur.Y + off[1]}
			if _, already := claimed[cand]; already {
				continue
			}
			exhausted = false
			if tryAdd(cand) {
				frontier = append(frontier, cand)
				added = true
				break
			}
		}
		if !added && exhausted {
			frontier[idx] = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}
	}
	return out
}

// sortPositions sorts ps in (X, Y) lex order in-place. Used to make zone
// slices iteration-stable across map-based intermediate data.
func sortPositions(ps []geom.Position) {
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].X != ps[j].X {
			return ps[i].X < ps[j].X
		}
		return ps[i].Y < ps[j].Y
	})
}

// terrainForZone returns the Terrain the renderer should paint at a tile
// covered by zone in a volcano of state state. The mapping matches the
// plan's "Biome transformation" table:
//
//	Core, Active   -> TerrainVolcanoCore
//	Core, Dormant  -> TerrainVolcanoCoreDormant
//	Core, Extinct  -> TerrainCraterLake
//	Slope, any     -> TerrainVolcanoSlope
//	Ashland, Active or Dormant -> TerrainAshland
//	Ashland, Extinct -> unreachable (Extinct volcanoes have no ashland)
//	None            -> "" (empty, caller treats as miss)
func terrainForZone(zone world.VolcanoZone, state world.VolcanoState) world.Terrain {
	switch zone {
	case world.VolcanoZoneCore:
		switch state {
		case world.VolcanoActive:
			return world.TerrainVolcanoCore
		case world.VolcanoDormant:
			return world.TerrainVolcanoCoreDormant
		case world.VolcanoExtinct:
			return world.TerrainCraterLake
		}
	case world.VolcanoZoneSlope:
		return world.TerrainVolcanoSlope
	case world.VolcanoZoneAshland:
		// Active and Dormant both paint ashland; Extinct has no ashland
		// tiles so this branch is never reached for Extinct volcanoes.
		return world.TerrainAshland
	}
	return ""
}

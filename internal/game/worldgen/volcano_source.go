package worldgen

import (
	"math/rand/v2"
	"slices"
	"sort"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// volcanoSeedSalt mixes the world seed into a dedicated PRNG stream
// for hotspot picking. Decouples volcano placement from any other
// randomness derived from the same seed so unrelated tuning changes
// (rivers, regions) do not shift volcano positions.
const volcanoSeedSalt int64 = 0x5e2a91c7b3d4f608

// volcanoStateSalt rotates the per-anchor state PRNG so two volcanoes
// at adjacent anchors get statistically independent state rolls
// instead of correlated ones from a shared seed prefix.
const volcanoStateSalt int64 = 0x3b9d7c45a1e08f29

// Hotspot picking constraints. Volcanoes only spawn on land cells with
// elevation above the threshold whose biome reads as a high-relief
// landform — mountains, peaks, or hills. The min-distance gate keeps
// neighbouring anchors from sharing slope/ashland tiles.
const (
	volcanoMinElevation = 0.65
	volcanoMinDistance  = 24
	// hotspotsPerContinent scales the candidate count with world size:
	// each continent budgets up to this many volcanoes before spacing /
	// terrain filters cull the list. ContinentCount * 2 keeps Standard
	// (3 continents) at ≈6 volcanoes, Huge (5) at ≈10.
	hotspotsPerContinent = 2
	// hotspotCandidatePool is how many high-elevation cells we shuffle
	// before applying the spacing filter. Larger pool = better odds of
	// finding well-separated picks even on cramped continents. Capped to
	// avoid pathological work on Gigantic worlds.
	hotspotCandidatePool = 1024
)

// Footprint radii. Core is impassable, slope is rough but passable,
// ashland is the dusted outskirt. Choices match the docstring on
// world.Volcano: core ≈13 tiles, slope ring ≈96, ashland ring ≈220.
const (
	volcanoCoreRadius    = 2
	volcanoSlopeRadius   = 5
	volcanoAshlandRadius = 9
)

// State probability weights, scaled so they sum to 100. Active 30%,
// Dormant 50%, Extinct 20% — extinct volcanoes turn their core into a
// crater lake.
const (
	volcanoActiveWeight  = 30
	volcanoDormantWeight = 50
	volcanoExtinctWeight = 20
	volcanoWeightTotal   = volcanoActiveWeight + volcanoDormantWeight + volcanoExtinctWeight
)

// VolcanoSource is the production world.VolcanoSource implementation.
// It runs once at construction, picks N anchors out of the finished
// terrain, builds each volcano's three concentric rings, and serves
// queries from precomputed lookups: VolcanoAt is O(1) via a per-super-
// chunk slice map; TerrainOverrideAt is O(volcanoes-near-tile) via a
// per-tile zone map keyed on packed coords.
//
// All fields are read-only after construction so the source is safe
// for concurrent reads without synchronisation.
type VolcanoSource struct {
	volcanoes []world.Volcano

	// bySC indexes volcanoes by anchor super-chunk for O(1) VolcanoAt.
	// Multiple volcanoes can share a super-chunk; the slice is shared
	// with the cache layer (volcanoCache) and must not be mutated.
	bySC map[geom.SuperChunkCoord][]world.Volcano

	// terrainAt maps packed (x,y) → terrain override. Built once over
	// every footprint tile so the hot path is a single map read.
	// Memory cost: ≈220 tiles × ≈10 volcanoes ≈ 2.2K entries on
	// Standard — negligible. Packed key keeps it cache-friendly.
	terrainAt map[uint64]world.Terrain
}

// NewVolcanoSource builds a VolcanoSource over a finished worldgen.Map.
//
// Algorithm:
//  1. Walk every cell, keep those that pass the eligibility filter
//     (elevation, biome, non-ocean).
//  2. Shuffle deterministically; greedy-pick anchors with a min-distance
//     spacing check until we hit the size-scaled budget.
//  3. For each anchor, materialise the three concentric Euclidean rings
//     and assign a state from the weighted roll.
//  4. Bucket volcanoes by super-chunk and build the tile→terrain map.
func NewVolcanoSource(w *Map, seed int64) *VolcanoSource {
	src := &VolcanoSource{
		bySC:      make(map[geom.SuperChunkCoord][]world.Volcano),
		terrainAt: make(map[uint64]world.Terrain),
	}
	if w == nil || w.Voronoi == nil || len(w.Voronoi.Cells) == 0 {
		return src
	}

	candidates := collectCandidates(w)
	if len(candidates) == 0 {
		return src
	}

	// Cap the pool first so the shuffle on Gigantic worlds stays bounded.
	if len(candidates) > hotspotCandidatePool {
		candidates = candidates[:hotspotCandidatePool]
	}

	// Independent PRNG stream for hotspot picking. PCG seeded with
	// (seed, seed^salt) matches the convention in voronoi/voronoi.go and
	// rivers.go so the seed mixing is consistent across stages.
	pcg := rand.NewPCG(uint64(seed), uint64(seed)^uint64(volcanoSeedSalt))
	rng := rand.New(pcg)
	rng.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	budget := budgetFor(w)
	picked := pickWithSpacing(candidates, budget, volcanoMinDistance)

	src.volcanoes = make([]world.Volcano, 0, len(picked))
	for _, anchor := range picked {
		v := buildVolcano(w, anchor, seed)
		if len(v.CoreTiles) == 0 {
			continue
		}
		src.volcanoes = append(src.volcanoes, v)
	}

	// Sort once for stable ordering across runs (the shuffle above is
	// already deterministic, but explicit lex sort future-proofs against
	// PCG implementation tweaks and reads naturally in test snapshots).
	sort.Slice(src.volcanoes, func(i, j int) bool {
		a, b := src.volcanoes[i].Anchor, src.volcanoes[j].Anchor
		if a.X != b.X {
			return a.X < b.X
		}
		return a.Y < b.Y
	})

	for i := range src.volcanoes {
		v := src.volcanoes[i]
		sc := geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y)
		src.bySC[sc] = append(src.bySC[sc], v)

		for _, t := range v.CoreTiles {
			src.terrainAt[geom.PackPos(t)] = coreTerrain(v.State)
		}
		for _, t := range v.SlopeTiles {
			src.terrainAt[geom.PackPos(t)] = world.TerrainVolcanoSlope
		}
		for _, t := range v.AshlandTiles {
			// Don't overwrite if a slope/core tile from another volcano
			// already claimed this position — core/slope take priority.
			if _, exists := src.terrainAt[geom.PackPos(t)]; exists {
				continue
			}
			src.terrainAt[geom.PackPos(t)] = world.TerrainAshland
		}
	}

	return src
}

// VolcanoAt returns the slice of volcanoes whose anchor lies in sc.
// Returns nil for super-chunks with no anchors. The slice is shared
// with the cache; callers must not mutate.
func (s *VolcanoSource) VolcanoAt(sc geom.SuperChunkCoord) []world.Volcano {
	return s.bySC[sc]
}

// TerrainOverrideAt reports the volcanic terrain at t, or ("", false)
// when t is not inside any volcano footprint. O(1) map read.
func (s *VolcanoSource) TerrainOverrideAt(t geom.Position) (world.Terrain, bool) {
	if terr, ok := s.terrainAt[geom.PackPos(t)]; ok {
		return terr, true
	}
	return "", false
}

// All returns every placed volcano, sorted by anchor coord. Test-only:
// each call clones the outer slice and every per-volcano tile slice so
// callers can mutate the result without affecting the source's
// precomputed lookups (which the cache layer also references).
func (s *VolcanoSource) All() []world.Volcano {
	out := make([]world.Volcano, len(s.volcanoes))
	for i, v := range s.volcanoes {
		v.CoreTiles = slices.Clone(v.CoreTiles)
		v.SlopeTiles = slices.Clone(v.SlopeTiles)
		v.AshlandTiles = slices.Clone(v.AshlandTiles)
		out[i] = v
	}
	return out
}

// collectCandidates scans every cell once and returns the anchor
// position of every cell that satisfies the eligibility filter.
// Returns positions, not cell IDs, so the picking pass operates on a
// flat slice without re-deriving geometry.
func collectCandidates(w *Map) []geom.Position {
	out := make([]geom.Position, 0, 256)
	for cellID := range w.Voronoi.Cells {
		id := uint32(cellID)
		if !cellEligible(w, id) {
			continue
		}
		c := w.Voronoi.Cells[id]
		anchor := geom.Position{X: int(c.CenterX), Y: int(c.CenterY)}
		// Snap inside grid bounds — Lloyd centroids round to within the
		// raster, but be defensive against future Voronoi tweaks.
		if anchor.X < 0 || anchor.Y < 0 || anchor.X >= w.Width || anchor.Y >= w.Height {
			continue
		}
		out = append(out, anchor)
	}
	return out
}

// cellEligible returns true when cellID is a high-relief land cell
// suitable to host a volcano anchor. Filter:
//   - not ocean
//   - elevation > volcanoMinElevation
//   - terrain ∈ {Mountain, SnowyPeak, Hills}
//
// Lakes are caught by the terrain filter (they're not in the allow set).
func cellEligible(w *Map, cellID uint32) bool {
	if w.IsOcean(cellID) {
		return false
	}
	if int(cellID) >= len(w.Elevation) || w.Elevation[cellID] <= volcanoMinElevation {
		return false
	}
	switch w.Terrain[cellID] {
	case world.TerrainMountain, world.TerrainSnowyPeak, world.TerrainHills:
		return true
	default:
		return false
	}
}

// budgetFor returns the target volcano count for w's size: continent
// count × hotspotsPerContinent. Floor of 2 so even Tiny worlds get a
// volcano or two for tests and demos.
func budgetFor(w *Map) int {
	n := w.Size.ContinentCount() * hotspotsPerContinent
	return max(2, n)
}

// pickWithSpacing greedily walks the shuffled candidate list and
// accepts a position only if its squared distance to every previously
// accepted anchor exceeds minDist². Stops early when the budget is hit.
//
// Squared comparisons avoid the sqrt in the inner loop. Worst-case cost
// is O(budget × len(candidates)); budget is single digits and
// candidates is capped at hotspotCandidatePool, so the loop is cheap.
func pickWithSpacing(candidates []geom.Position, budget, minDist int) []geom.Position {
	if budget <= 0 {
		return nil
	}
	picked := make([]geom.Position, 0, budget)
	minSq := minDist * minDist
	for _, c := range candidates {
		ok := true
		for _, p := range picked {
			if geom.SqDist(c.X, c.Y, p.X, p.Y) < minSq {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		picked = append(picked, c)
		if len(picked) >= budget {
			break
		}
	}
	return picked
}

// buildVolcano materialises the concentric ring tiles around anchor
// and rolls a state. Tiles in ocean cells are filtered out so a
// coastal volcano never paints volcanic terrain over water — the
// resulting footprint will be lopsided but still gameplay-valid.
//
// Distance metric is Euclidean (squared comparison) so the rings read
// as circles rather than diamonds. A core radius of 2 yields a 13-tile
// disc, slope of 5 a 81-tile disc minus core, ashland of 9 a 253-tile
// disc minus the inner two — close to the docstring's "~13 / ~96 / ~220"
// targets after ocean filtering trims the edges.
func buildVolcano(w *Map, anchor geom.Position, seed int64) world.Volcano {
	state := rollState(seed, anchor)

	core := tilesInRadius(w, anchor, 0, volcanoCoreRadius)
	slope := tilesInRadius(w, anchor, volcanoCoreRadius, volcanoSlopeRadius)
	ash := tilesInRadius(w, anchor, volcanoSlopeRadius, volcanoAshlandRadius)

	return world.Volcano{
		Anchor:       anchor,
		State:        state,
		CoreTiles:    core,
		SlopeTiles:   slope,
		AshlandTiles: ash,
	}
}

// tilesInRadius returns every integer tile whose Euclidean distance from
// anchor lies in (innerR, outerR] when innerR > 0, or [0, outerR] when
// innerR == 0. Ocean tiles are dropped. Off-map tiles are dropped.
//
// Output order is row-major (y outer, x inner) so test snapshots stay
// stable, and the slice is sized exactly so we don't carry hidden
// excess capacity into the per-volcano storage.
func tilesInRadius(w *Map, anchor geom.Position, innerR, outerR int) []geom.Position {
	if outerR <= 0 {
		return nil
	}
	innerSq := innerR * innerR
	outerSq := outerR * outerR

	out := make([]geom.Position, 0, (2*outerR+1)*(2*outerR+1))
	for dy := -outerR; dy <= outerR; dy++ {
		for dx := -outerR; dx <= outerR; dx++ {
			d2 := dx*dx + dy*dy
			if d2 > outerSq {
				continue
			}
			if innerR > 0 && d2 <= innerSq {
				continue
			}
			x := anchor.X + dx
			y := anchor.Y + dy
			if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
				continue
			}
			cellID := w.Voronoi.CellIDAt(x, y)
			if w.IsOcean(cellID) {
				continue
			}
			out = append(out, geom.Position{X: x, Y: y})
		}
	}
	return out
}

// rollState returns a deterministic VolcanoState for the given anchor.
// Uses Splitmix64 over (seed, salt, anchor.X, anchor.Y) so the result
// is independent of the picking PRNG and stable under shuffle changes.
func rollState(seed int64, anchor geom.Position) world.VolcanoState {
	mix := geom.MixCoords(seed, volcanoStateSalt, anchor.X, anchor.Y)
	roll := geom.Splitmix64(mix) % volcanoWeightTotal
	switch {
	case roll < volcanoActiveWeight:
		return world.VolcanoActive
	case roll < volcanoActiveWeight+volcanoDormantWeight:
		return world.VolcanoDormant
	default:
		return world.VolcanoExtinct
	}
}

// coreTerrain maps a state to the core-tile terrain. Active and dormant
// cores block movement; extinct cores fill with water (crater lake).
func coreTerrain(s world.VolcanoState) world.Terrain {
	switch s {
	case world.VolcanoActive:
		return world.TerrainVolcanoCore
	case world.VolcanoDormant:
		return world.TerrainVolcanoCoreDormant
	case world.VolcanoExtinct:
		return world.TerrainCraterLake
	default:
		// Unknown state shouldn't reach here — treat as dormant rather
		// than crash. The zero value would otherwise pass through as
		// the empty-string Terrain and break the override contract.
		return world.TerrainVolcanoCoreDormant
	}
}

var _ world.VolcanoSource = (*VolcanoSource)(nil)

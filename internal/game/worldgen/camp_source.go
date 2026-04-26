package worldgen

// CampSource is the production polity.CampSource implementation. Camps
// are pre-historic settler clusters scattered across all super-chunks
// of the world via Bridson Poisson-disk sampling weighted by per-cell
// habitability. Construction rolls a per-SC settlement-willingness
// affinity, draws Bridson Poisson-disk candidates per SC, runs each
// candidate through a 6-gate acceptance chain (water, volcano, landmark,
// impassable peak, habitability+affinity floor, weighted roll), and
// derives Faiths/Population/Footprint for each survivor. Footprint
// random-walk grows a connected 2-3 tile cluster from each anchor.
//
// All maps are allocated once during NewCampSource and never mutated
// thereafter. CampSource is safe for concurrent read.

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/polity"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// campSettlementID derives a deterministic SettlementID from the world seed
// and the camp's anchor position. The same (seed, position) pair always
// produces the same ID, making NewCampSource fully deterministic regardless
// of call order or concurrency.
func campSettlementID(seed int64, anchor geom.Position) polity.SettlementID {
	h := geom.Splitmix64(uint64(seed) ^ geom.PackPos(anchor) ^ 0xDEADBEEFCAFEBABE)
	return polity.SettlementID(int64(h))
}

// CampSource indexes camps by super-chunk for O(1) CampsIn lookup and
// keeps a globally sorted slice for deterministic All() output.
type CampSource struct {
	// bySC indexes camps by their anchor's home super-chunk for O(1)
	// CampsIn lookup. Camps that span an SC boundary via Footprint
	// appear under their Position's SC only — Footprint is a render-tier
	// concern, not a query key.
	bySC map[geom.SuperChunkCoord][]polity.Camp

	// sorted holds every camp in (Y, X) lex order on Position for
	// deterministic All() output.
	sorted []polity.Camp
}

// CampSourceConfig holds the upstream sources required to place camps.
// All non-nil sources are used; nil sources fall back to maximally
// permissive behaviour where possible (e.g. nil VolcanoSource skips
// the volcano gate). Regions and the world map are mandatory.
type CampSourceConfig struct {
	Regions   gworld.RegionSource
	Landmarks gworld.LandmarkSource
	Volcanoes gworld.VolcanoSource
	Deposits  gworld.DepositSource
}

// NewCampSource builds a CampSource over a finished worldgen.Map.
//
// Construction visits every super-chunk in the map's bounds, rolls a
// per-SC settlement-willingness multiplier, draws Bridson Poisson-disk
// candidates inside each SC's bounds, runs the 6-gate acceptance chain,
// and derives Faiths/Population/Footprint for each accepted candidate.
// Footprint is a connected 2-3 tile cluster grown via random-walk from
// each anchor.
func NewCampSource(w *Map, seed int64, cfg CampSourceConfig) *CampSource {
	if cfg.Regions == nil {
		// Regions are required — without them every camp's Region
		// would be RegionNormal which defeats the cultural-clustering
		// purpose. Construction with a nil RegionSource is a
		// programming error.
		panic("newCampSource: cfg.Regions must not be nil")
	}

	bySC := make(map[geom.SuperChunkCoord][]polity.Camp)
	var all []polity.Camp

	// Iterate every super-chunk inside the world bounds. The map
	// dimensions are tile-aligned to the SC grid by worldgen
	// construction, so a simple integer divide gives the SC bounds.
	scWide := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scTall := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize

	for scY := 0; scY < scTall; scY++ {
		for scX := 0; scX < scWide; scX++ {
			sc := geom.SuperChunkCoord{X: scX, Y: scY}
			scCamps := buildSCCamps(w, seed, sc, cfg)
			if len(scCamps) == 0 {
				continue
			}
			bySC[sc] = scCamps
			all = append(all, scCamps...)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		a, b := all[i].Position, all[j].Position
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})

	return &CampSource{
		bySC:   bySC,
		sorted: all,
	}
}

// buildSCCamps performs camp placement for one super-chunk: rolls the
// per-SC settlement-willingness multiplier, draws Bridson candidates
// inside the SC bounds, runs the gate chain + habitability acceptance
// per candidate, and derives Faiths/Population/Footprint for each
// accepted camp. Result is sorted by Position (Y, X) lex order.
func buildSCCamps(
	w *Map,
	seed int64,
	sc geom.SuperChunkCoord,
	cfg CampSourceConfig,
) []polity.Camp {
	region := cfg.Regions.RegionAt(sc).Character

	// Per-SC settlement-willingness roll. Salt distinct from the Bridson
	// and accept streams so all three are pairwise decorrelated.
	affinity := regionAffinity(seed, sc)

	// Bridson candidates inside SC's tile bounds.
	bounds := scBounds(sc, w.Width, w.Height)
	bridsonRng := campBridsonRNG(seed, sc)
	candidates := bridsonSample(bridsonRng, bounds, campMinSpacing, campPoissonK)
	if len(candidates) == 0 {
		return nil
	}

	// Gate chain. Build a landmark-tile set from the current
	// SC AND its 8 neighbours so that footprint random-walks that
	// wander across SC boundaries still respect landmark exclusion.
	// Without neighbour SCs, a walk growing toward an adjacent SC could
	// land on a landmark tile that was only queried for the home SC.
	var landmarkTiles map[geom.Position]struct{}
	if cfg.Landmarks != nil {
		landmarkTiles = make(map[geom.Position]struct{})
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nsc := geom.SuperChunkCoord{X: sc.X + dx, Y: sc.Y + dy}
				for _, lm := range cfg.Landmarks.LandmarksIn(nsc) {
					landmarkTiles[lm.Coord] = struct{}{}
				}
			}
		}
		if len(landmarkTiles) == 0 {
			landmarkTiles = nil
		}
	}

	// claimedTiles tracks every footprint tile claimed by any camp in
	// this SC so that random-walk growth from a later camp cannot
	// overlap tiles already owned by an earlier one. Anchors are
	// registered before growth; footprint tiles are added after.
	claimedTiles := make(map[geom.Position]struct{}, len(candidates)*3)

	acceptRng := campAcceptRNG(seed, sc)
	out := make([]polity.Camp, 0, len(candidates))
	for _, p := range candidates {
		cellID := w.Voronoi.CellIDAt(p.X, p.Y)
		if !acceptCamp(p, w, cellID, cfg.Deposits, cfg.Volcanoes, landmarkTiles, claimedTiles, affinity, acceptRng) {
			continue
		}
		pop := campPop(seed, p)
		footprint := growCampFootprint(p, pop, seed, w, cfg.Volcanoes, landmarkTiles, claimedTiles)
		// Register all footprint tiles in the cross-camp claimed set.
		for _, fp := range footprint {
			claimedTiles[fp] = struct{}{}
		}
		faiths := campFaiths(seed, p, region)
		rulerName := naming.GenerateRulerName(seed, p, region.Key())
		campName := naming.GenerateSettlementName(seed, p, region.Key())
		rulerStream := dice.New(seed^int64(geom.PackPos(p)), dice.Salt(seedSaltCampRuler))
		out = append(out, polity.Camp{
			Settlement: polity.Settlement{
				ID:         campSettlementID(seed, p),
				Tier:       polity.TierCamp,
				Name:       campName,
				Position:   p,
				Footprint:  footprint,
				Region:     region,
				Faiths:     faiths,
				Population: int(pop),
				Founded:    0,
				Ruler:      polity.NewRuler(rulerStream, 0, rulerName),
			},
		})
	}

	if len(out) == 0 {
		return nil
	}

	// Stable Position (Y, X) lex order for deterministic CampsIn output.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].Position, out[j].Position
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return out
}

// campTileCount returns the footprint budget for a given population.
// Pop ≤ campFootprintSmallPopThreshold → 2 tiles; above that → 3 tiles.
// Mirrors KINGDOMS.md §2.6 villageTileCount.
func campTileCount(pop int32) int {
	if pop <= 0 {
		return 1
	}
	if pop <= campFootprintSmallPopThreshold {
		return 2
	}
	return 3
}

// campFootprintAccept reports whether tile p may be added to a camp
// footprint. Rejects: out-of-bounds, water, impassable terrain
// (SnowyPeak, VolcanoCore, VolcanoCoreDormant, CraterLake), any
// volcano override that is core/slope/ashland/crater, and landmark
// tiles. Does NOT re-run the habitability roll — already-grown
// footprint tiles inherit the anchor's acceptance.
func campFootprintAccept(
	p geom.Position,
	w *Map,
	vs gworld.VolcanoSource,
	landmarkTiles map[geom.Position]struct{},
) bool {
	if p.X < 0 || p.X >= w.Width || p.Y < 0 || p.Y >= w.Height {
		return false
	}
	cellID := w.Voronoi.CellIDAt(p.X, p.Y)
	if w.IsOcean(cellID) || w.IsRiver(p.X, p.Y) {
		return false
	}
	switch w.Terrain[cellID] {
	case gworld.TerrainSnowyPeak,
		gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainCraterLake:
		return false
	}
	if vs != nil {
		if override, ok := vs.TerrainOverrideAt(p); ok {
			switch override {
			case gworld.TerrainVolcanoCore,
				gworld.TerrainVolcanoCoreDormant,
				gworld.TerrainVolcanoSlope,
				gworld.TerrainAshland,
				gworld.TerrainCraterLake:
				return false
			}
		}
	}
	if _, isLandmark := landmarkTiles[p]; isLandmark {
		return false
	}
	return true
}

// growCampFootprint returns a connected 2-3 tile footprint for the camp
// at anchor. The algorithm is a frontier-based random walk (§8 of the
// camps plan): starting from anchor, it repeatedly picks a frontier
// tile and tries to expand into one of its four cardinal neighbours.
// Neighbours are shuffled deterministically so same anchor+seed always
// produce the same shape.
//
// claimedTiles is the per-SC cross-camp ownership set; tiles already
// in it are skipped to prevent footprint overlap between camps.
//
// Output is sorted in (Y, X) lex order for deterministic storage.
func growCampFootprint(
	anchor geom.Position,
	pop int32,
	seed int64,
	w *Map,
	vs gworld.VolcanoSource,
	landmarkTiles map[geom.Position]struct{},
	claimedTiles map[geom.Position]struct{},
) []geom.Position {
	budget := campTileCount(pop)
	rng := newPCG(uint64(seed) ^ uint64(seedSaltCampFootprint) ^ geom.PackPos(anchor))

	// local tracks tiles claimed by this camp only; we start with the
	// anchor, which is always included regardless of terrain — it
	// already passed the full gate chain.
	local := map[geom.Position]struct{}{anchor: {}}
	frontier := []geom.Position{anchor}

	for len(local) < budget && len(frontier) > 0 {
		idx := rng.IntN(len(frontier))
		p := frontier[idx]
		grew := false
		for _, n := range fourNeighborsShuffled(p, rng) {
			if _, taken := local[n]; taken {
				continue
			}
			if _, taken := claimedTiles[n]; taken {
				continue
			}
			if !campFootprintAccept(n, w, vs, landmarkTiles) {
				continue
			}
			local[n] = struct{}{}
			frontier = append(frontier, n)
			grew = true
			break
		}
		if !grew {
			// This frontier tile has no usable neighbours; remove it.
			frontier[idx] = frontier[len(frontier)-1]
			frontier = frontier[:len(frontier)-1]
		}
	}

	out := make([]geom.Position, 0, len(local))
	for fp := range local {
		out = append(out, fp)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return out
}

// fourNeighborsShuffled returns the 4 cardinal neighbours of p in a
// seed-dependent order so that the same anchor always grows in the
// same direction for a given seed.
func fourNeighborsShuffled(p geom.Position, rng *rand.Rand) [4]geom.Position {
	n := [4]geom.Position{
		{X: p.X + 1, Y: p.Y},
		{X: p.X - 1, Y: p.Y},
		{X: p.X, Y: p.Y + 1},
		{X: p.X, Y: p.Y - 1},
	}
	rng.Shuffle(len(n), func(i, j int) { n[i], n[j] = n[j], n[i] })
	return n
}

// acceptCamp runs the 6-gate chain on a candidate position. Returns
// true iff the candidate should become a Camp. Gates run in
// cheapest-first order so cheap rejections short-circuit before any
// allocation-heavy work.
//
// Gate 1:   water reject              — ocean / deep-ocean cells
// Gate 1.5: claimed tile reject       — position already in claimedTiles
// Gate 2:   volcano footprint reject  — TerrainOverrideAt ∈ volcanic set
// Gate 3:   landmark tile reject      — position in landmarkTiles set
// Gate 4:   impassable terrain reject — SnowyPeak / VolcanoCore* / CraterLake
// Gate 5:   habitability × affinity ≥ campHabitabilityFloor
// Gate 6:   weighted acceptance roll  — rng.Float32() < clamp(h*affinity, 1)
//
// landmarkTiles may be nil (gate 3 skipped). vs may be nil (gate 2
// skipped). claimedTiles may be nil (gate 1.5 skipped). The water and
// impassable gates always run — they read map state, not optional sources.
func acceptCamp(
	p geom.Position,
	w *Map,
	cellID uint32,
	ds gworld.DepositSource,
	vs gworld.VolcanoSource,
	landmarkTiles map[geom.Position]struct{},
	claimedTiles map[geom.Position]struct{},
	affinity float32,
	rng *rand.Rand,
) bool {
	// Gate 1: water — ocean, lake (TerrainOcean per classify.go convention)
	// and river tiles are uninhabitable. Rivers are tile-level water in
	// riverBits, not cell-level, so cellID-based ocean check alone misses
	// them — a river tile inside a land cell would otherwise pass.
	if w.IsOcean(cellID) || w.IsRiver(p.X, p.Y) {
		return false
	}

	// Gate 1.5: defensive — reject if another camp already claimed this
	// tile. Bridson spacing 8 + footprint ≤ 3 makes this impossible today,
	// but the check survives a future spacing reduction without behaviour
	// change.
	if _, claimed := claimedTiles[p]; claimed {
		return false
	}

	// Gate 2: volcano footprint — hard-reject tiles inside any
	// volcano's core, slope, or ashland rings. Ashland has a non-zero
	// biome score (0.05) so gate 5 might not catch it; hard-reject
	// here is explicit and cheap.
	if vs != nil {
		if t, ok := vs.TerrainOverrideAt(p); ok {
			switch t {
			case gworld.TerrainVolcanoCore,
				gworld.TerrainVolcanoCoreDormant,
				gworld.TerrainVolcanoSlope,
				gworld.TerrainCraterLake,
				gworld.TerrainAshland:
				return false
			}
		}
	}

	// Gate 3: landmark tile — camps cannot occupy a tile that already
	// holds a landmark. The set was built from LandmarksIn(sc) before
	// the candidate loop; nil means no landmark source is wired.
	if landmarkTiles != nil {
		if _, exists := landmarkTiles[p]; exists {
			return false
		}
	}

	// Gate 4: impassable terrain — snowy peaks and crater lakes block
	// movement entirely; volcano cores are handled by gate 2 when a
	// VolcanoSource is wired, but may survive as base terrain when it
	// is not. Reject here regardless.
	terrain := w.Terrain[cellID]
	switch terrain {
	case gworld.TerrainSnowyPeak,
		gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainCraterLake:
		return false
	}

	// Gate 5: habitability floor. Multiply raw score by affinity to
	// apply the per-SC "settlement willingness" roll before the floor
	// comparison so sparse regions stay sparse even on good terrain.
	h := habitabilityAt(p, w, cellID, ds, vs)
	if h <= 0 {
		return false
	}
	score := h * affinity
	if score < campHabitabilityFloor {
		return false
	}

	// Gate 6: weighted acceptance roll, scaled by campRarityMultiplier.
	// The clamp is intentionally on h*affinity (before the multiplier)
	// so the multiplier is a true "rarity dial" — halving it halves
	// the effective acceptance everywhere, not just on average tiles.
	if score > 1.0 {
		score = 1.0
	}
	return rng.Float32() < score*campRarityMultiplier
}

// campAcceptRNG builds the per-SC PCG stream used by the Gate 6
// weighted acceptance roll. Uses seedSaltCampAccept — distinct from
// seedSaltCamp (Bridson) and seedSaltCampRegionAffinity — so the
// three per-SC streams are pairwise decorrelated.
func campAcceptRNG(seed int64, sc geom.SuperChunkCoord) *rand.Rand {
	return newPCG(uint64(seed) ^ uint64(seedSaltCampAccept) ^ scHash(sc))
}

// regionAffinity returns the per-super-chunk settlement-willingness
// multiplier from .omc/plans/camps.md §3. Each SC rolls a uniform value
// in [campRegionAffinityMin, campRegionAffinityMax] once at worldgen
// time. The result multiplies every camp candidate's habitability score
// in that SC during the gate chain acceptance.
func regionAffinity(seed int64, sc geom.SuperChunkCoord) float32 {
	rng := newPCG(uint64(seed) ^ uint64(seedSaltCampRegionAffinity) ^ scHash(sc))
	span := float32(campRegionAffinityMax - campRegionAffinityMin)
	return campRegionAffinityMin + rng.Float32()*span
}

// campBridsonRNG builds the per-SC PCG stream used by the Bridson
// candidate generator. Salt is seedSaltCamp, distinct from every
// other camp-subsystem salt.
func campBridsonRNG(seed int64, sc geom.SuperChunkCoord) *rand.Rand {
	return newPCG(uint64(seed) ^ uint64(seedSaltCamp) ^ scHash(sc))
}

// scBounds returns the tile-coordinate Rect of the super-chunk sc,
// clamped to [0, mapW) × [0, mapH). Edge SCs may have width or
// height < geom.SuperChunkSize when the map dimensions are not
// integer multiples of SuperChunkSize.
func scBounds(sc geom.SuperChunkCoord, mapW, mapH int) geom.Rect {
	x0 := sc.X * geom.SuperChunkSize
	y0 := sc.Y * geom.SuperChunkSize
	x1 := x0 + geom.SuperChunkSize
	y1 := y0 + geom.SuperChunkSize
	if x1 > mapW {
		x1 = mapW
	}
	if y1 > mapH {
		y1 = mapH
	}
	return geom.Rect{MinX: x0, MinY: y0, MaxX: x1, MaxY: y1}
}

// scHash produces a uint64 mix from a super-chunk coord, suitable for
// combining with seed and a salt to derive a per-SC PCG stream. Uses
// geom.Splitmix64 for avalanche quality.
func scHash(sc geom.SuperChunkCoord) uint64 {
	return geom.Splitmix64(uint64(int64(sc.X))*0x9E3779B97F4A7C15 ^ uint64(int64(sc.Y))*0xBF58476D1CE4E5B9)
}

// CampsIn returns every camp whose Position is inside super-chunk sc,
// in stable (Y, X) lex order. Returns nil if no camps land in sc.
func (s *CampSource) CampsIn(sc geom.SuperChunkCoord) []polity.Camp {
	return s.bySC[sc]
}

// All returns every camp in the world in stable (Y, X) lex order.
// Used by diagnostics, dev tools, and determinism tests.
func (s *CampSource) All() []polity.Camp {
	return s.sorted
}

// Compile-time guarantee CampSource implements polity.CampSource.
var _ polity.CampSource = (*CampSource)(nil)

// campFaithByRegion holds the per-RegionCharacter faith weight tables
// from .omc/plans/camps.md §6. Each row defines relative weights;
// pickWeighted normalises at draw time so the entries are ratios, not
// percentages. Heretic minorities (the small entries) emerge naturally
// from the roll and feed religion-diffusion mechanics from year 1.
//
// Row order matches the polity.RegionCharacter enum so the row can be
// indexed by camp.Region directly without a map lookup.
var campFaithByRegion = [...][]weighted[polity.Faith]{
	polity.RegionNormal: {
		{polity.FaithOldGods, 65},
		{polity.FaithSunCovenant, 15},
		{polity.FaithGreenSage, 10},
		{polity.FaithOneOath, 7},
		{polity.FaithStormPact, 3},
	},
	polity.RegionBlighted: {
		{polity.FaithOldGods, 85},
		{polity.FaithSunCovenant, 2},
		{polity.FaithGreenSage, 2},
		{polity.FaithOneOath, 6},
		{polity.FaithStormPact, 5},
	},
	polity.RegionFey: {
		{polity.FaithOldGods, 30},
		{polity.FaithSunCovenant, 5},
		{polity.FaithGreenSage, 55},
		{polity.FaithOneOath, 5},
		{polity.FaithStormPact, 5},
	},
	polity.RegionAncient: {
		{polity.FaithOldGods, 55},
		{polity.FaithSunCovenant, 20},
		{polity.FaithGreenSage, 10},
		{polity.FaithOneOath, 10},
		{polity.FaithStormPact, 5},
	},
	polity.RegionSavage: {
		{polity.FaithOldGods, 30},
		{polity.FaithSunCovenant, 5},
		{polity.FaithGreenSage, 5},
		{polity.FaithOneOath, 45},
		{polity.FaithStormPact, 15},
	},
	polity.RegionHoly: {
		{polity.FaithOldGods, 40},
		{polity.FaithSunCovenant, 45},
		{polity.FaithGreenSage, 5},
		{polity.FaithOneOath, 8},
		{polity.FaithStormPact, 2},
	},
	polity.RegionWild: {
		{polity.FaithOldGods, 40},
		{polity.FaithSunCovenant, 5},
		{polity.FaithGreenSage, 40},
		{polity.FaithOneOath, 5},
		{polity.FaithStormPact, 10},
	},
}

// campFaiths builds a FaithDistribution for a camp at anchor inside region.
// The dominant faith is determined by a weighted roll (campFaithByRegion),
// then seeded at 0.92 majority with the other four faiths at 0.02 each —
// matching the NewFaithDistribution default shares so the diffusion and
// schism mechanics have a live secondary pool from year 1.
func campFaiths(seed int64, anchor geom.Position, region polity.RegionCharacter) polity.FaithDistribution {
	rng := newPCG(uint64(seed) ^ uint64(seedSaltCampFaith) ^ geom.PackPos(anchor))
	dominant := pickWeighted(rng, campFaithByRegion[region], polity.FaithOldGods)

	fd := polity.NewFaithDistribution()
	// Swap dominant faith to majority share; set all others to minority share.
	// NewFaithDistribution seeds OldGods as majority, so only rearrange if a
	// different faith won the roll.
	if dominant != polity.FaithOldGods {
		const maj = 0.92
		const min = 0.02
		for f := polity.Faith(0); f < polity.FaithCount; f++ {
			if f == dominant {
				fd[f] = maj
			} else {
				fd[f] = min
			}
		}
	}
	return fd
}

// campPop draws an initial population from the camp Pareto:
//
//	p = min(campZipfMin * (1-u)^(-1/campZipfAlpha), campMaxPop)
//
// where u is uniform [0, 1). Steeper than the city Pareto (alpha 1.5
// vs 1.0): most camps stay tiny, a few rare ones approach campMaxPop.
func campPop(seed int64, anchor geom.Position) int32 {
	rng := newPCG(uint64(seed) ^ uint64(seedSaltCampPop) ^ geom.PackPos(anchor))
	u := rng.Float64()
	base := campZipfMin * math.Pow(1.0-u, -1.0/campZipfAlpha)
	if base > campMaxPop {
		base = campMaxPop
	}
	return int32(base)
}

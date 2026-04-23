package worldgen

import (
	"sync"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// NoiseVolcanoSource implements world.VolcanoSource on top of a
// WorldGenerator and a LandmarkSource. Placement is per super-region
// (8x8 super-chunks) and cached once generated — a 3x3 neighbourhood of
// super-regions is hit on every TerrainOverrideAt call to catch
// cross-super-region footprint spills.
//
// The source is safe for concurrent read. Per-super-region generation
// runs exactly once under sync.Once; subsequent readers observe the
// filled record without locking. sync.Map fits this access pattern
// (many disjoint keys, mostly-read after first-write, expensive
// per-key generation) better than a plain map + RWMutex.
type NoiseVolcanoSource struct {
	seed      int64
	worldgen  *WorldGenerator
	landmarks world.LandmarkSource

	// cache keys superRegion to *superRegionData. Lazy-filled via
	// sync.Once per entry so concurrent readers collapse to one
	// generation pass.
	cache sync.Map
}

// superRegionData is the generated state for one super-region: the list
// of volcano records keyed by anchor super-chunk, and a flat index from
// any footprint tile (across all three zones, including tiles that spill
// into adjacent super-regions) to its terrain. tileIndex is the hot-path
// data structure consulted by TerrainOverrideAt. hasAnyFootprint is a
// single-bit sentinel that lets hot-path callers skip the map lookup
// entirely when the super-region generated no volcanoes — empty-SR
// queries are the common case for worlds where most super-regions are
// ocean-heavy or landmark-dense.
type superRegionData struct {
	once            sync.Once
	volcanoes       []world.Volcano
	tileIndex       map[geom.Position]world.Terrain
	hasAnyFootprint bool
}

// NewNoiseVolcanoSource wires a volcano source to a WorldGenerator (for
// biome sampling during anchor acceptance and ashland growth) and a
// LandmarkSource (for collision rejection). The landmark source may be
// nil — the gate simply skips the landmark check in that case.
func NewNoiseVolcanoSource(
	seed int64,
	wg *WorldGenerator,
	lm world.LandmarkSource,
) *NoiseVolcanoSource {
	return &NoiseVolcanoSource{
		seed:      seed,
		worldgen:  wg,
		landmarks: lm,
	}
}

// VolcanoAt returns the volcanoes whose anchor sits inside sc. Footprint
// spills into sc from a neighbour's volcano are NOT returned — callers
// that want full coverage must query the 3x3 super-chunk neighbourhood
// themselves, exactly as they do for the landmark source.
//
// Deterministic: same (seed, sc) yields the same slice (including order
// of volcanoes and order of each volcano's zone tiles) on every call.
// Safe for concurrent read.
func (s *NoiseVolcanoSource) VolcanoAt(sc geom.SuperChunkCoord) []world.Volcano {
	sr := superRegionOf(sc)
	data := s.ensureSuperRegion(sr)

	// Filter to volcanoes whose anchor sits in sc. The super-region is
	// 8x8 super-chunks so this is at most ~3-6 anchor checks per call.
	var out []world.Volcano
	for i := range data.volcanoes {
		v := &data.volcanoes[i]
		if geom.WorldToSuperChunk(v.Anchor.X, v.Anchor.Y) == sc {
			out = append(out, *v)
		}
	}
	return out
}

// TerrainOverrideAt returns the volcanic terrain covering tile t, or
// ("", false) when no volcano's footprint includes t. Queries the 3x3
// super-region neighbourhood around t's super-region so footprints that
// spill across super-region boundaries are resolved correctly.
//
// Fast path: 9 map lookups worst case. First hit returns immediately;
// the allocation-free case (tile not inside any footprint) touches only
// the cached per-super-region tileIndex maps.
func (s *NoiseVolcanoSource) TerrainOverrideAt(t geom.Position) (world.Terrain, bool) {
	home := superRegionOf(geom.WorldToSuperChunk(t.X, t.Y))
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sr := superRegion{X: home.X + dx, Y: home.Y + dy}
			data := s.ensureSuperRegion(sr)
			if !data.hasAnyFootprint {
				continue
			}
			if ter, ok := data.tileIndex[t]; ok {
				return ter, true
			}
		}
	}
	return "", false
}

// ensureSuperRegion returns the filled superRegionData for sr, running
// generate exactly once under sync.Once. Concurrent callers collapse to
// a single generation pass; subsequent callers observe the populated
// record immediately.
func (s *NoiseVolcanoSource) ensureSuperRegion(sr superRegion) *superRegionData {
	if v, ok := s.cache.Load(sr); ok {
		data := v.(*superRegionData)
		data.once.Do(func() { data.generate(s, sr) })
		return data
	}
	fresh := &superRegionData{}
	actual, _ := s.cache.LoadOrStore(sr, fresh)
	data := actual.(*superRegionData)
	data.once.Do(func() { data.generate(s, sr) })
	return data
}

// generate runs the full placement pipeline for one super-region:
// anchor selection via Poisson-disk -> per-anchor state assignment ->
// per-anchor footprint growth -> tile-index build. Intended to be
// called exactly once per super-region, under sync.Once.
func (d *superRegionData) generate(s *NoiseVolcanoSource, sr superRegion) {
	anchors := pickVolcanoAnchors(s.seed, sr, s.landmarks, s.worldgen)
	if len(anchors) == 0 {
		d.tileIndex = map[geom.Position]world.Terrain{}
		return
	}

	d.volcanoes = make([]world.Volcano, 0, len(anchors))
	d.tileIndex = make(map[geom.Position]world.Terrain)

	for _, anchor := range anchors {
		state := volcanoState(s.seed, anchor)
		landmarks := landmarksNear(anchor, s.landmarks)
		core, slope, ashland := growFootprint(anchor, state, s.seed, s.worldgen, landmarks)
		if len(core) == 0 {
			// Degenerate footprint — skip rather than emit a malformed
			// volcano record that downstream code would have to special-
			// case.
			continue
		}
		v := world.Volcano{
			Anchor:       anchor,
			State:        state,
			CoreTiles:    core,
			SlopeTiles:   slope,
			AshlandTiles: ashland,
		}
		d.volcanoes = append(d.volcanoes, v)
		for _, p := range core {
			d.tileIndex[p] = terrainForZone(world.VolcanoZoneCore, state)
		}
		for _, p := range slope {
			d.tileIndex[p] = terrainForZone(world.VolcanoZoneSlope, state)
		}
		for _, p := range ashland {
			d.tileIndex[p] = terrainForZone(world.VolcanoZoneAshland, state)
		}
	}
	d.hasAnyFootprint = len(d.tileIndex) > 0
}

// volcanoState returns the deterministic lifecycle state for a volcano
// anchored at p. Uses a stable hash over (seed, anchor, salt) to produce
// a uniform value in [0, 1), mapped to a state via fixed probability
// bands: Active 20%, Dormant 30%, Extinct 50%. The state is a pure
// function of (seed, anchor) — independent of biome, neighbours, or
// other volcanoes.
func volcanoState(seed int64, p geom.Position) world.VolcanoState {
	// Reuse splitMix64 — it already lives in the worldgen package and
	// gives well-diffused 64-bit output from three inputs.
	h := splitMix64(uint64(seed^seedSaltVolcanoState), uint64(int64(p.X)), uint64(int64(p.Y)))
	// Top 24 bits are plenty of entropy for a 3-bucket split.
	u := float64(h>>40) / float64(1<<24)
	switch {
	case u < 0.20:
		return world.VolcanoActive
	case u < 0.50:
		return world.VolcanoDormant
	default:
		return world.VolcanoExtinct
	}
}

// landmarksNear returns the landmarks in the 3x3 super-chunk
// neighbourhood around p. Volcanoes footprint up to ~14 tiles from the
// anchor, so a 1-super-chunk (64-tile) radius covers every collision
// possibility. Returns nil when lm is nil.
func landmarksNear(p geom.Position, lm world.LandmarkSource) []world.Landmark {
	if lm == nil {
		return nil
	}
	home := geom.WorldToSuperChunk(p.X, p.Y)
	var out []world.Landmark
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			out = append(out, lm.LandmarksIn(sc)...)
		}
	}
	return out
}

// Compile-time assertion that NoiseVolcanoSource satisfies the
// consumer-side interface. Mirrors the pattern in region_source.go and
// landmarks.go.
var _ world.VolcanoSource = (*NoiseVolcanoSource)(nil)

// Package resource holds the deterministic deposit placement pipeline
// for the living-world map: zonal (Fertile/Timber/Game), structural
// fish, point-like Poisson-disk kinds (Iron/Stone/Gold/Silver/Gems/Salt),
// and volcanic structural kinds (Obsidian/Sulfur).
//
// The package satisfies world.DepositSource via NoiseDepositSource; the
// worldgen façade exposes a NewNoiseDepositSource constructor so existing
// importers (internal/server, cmd/devtools) keep their stable import path.
package resource

import (
	"cmp"
	"slices"
	"sync"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
	"github.com/Rioverde/gongeons/internal/game/worldgen/noise"
)

// TerrainSampler is the minimum-viable consumer-side interface the
// deposit source needs for per-tile terrain lookups. *worldgen.WorldGenerator
// satisfies it structurally via TileAt(x, y int) world.Tile — the same
// pattern volcano and landmark sub-packages use to avoid a cyclic import
// back to the root worldgen package.
type TerrainSampler interface {
	TileAt(x, y int) world.Tile
}

// Deposit generation shares the volcano super-region granularity
// (`genprim.SuperRegionSideSC`) so cache warm-up amortises across both
// layers — a caller that touches one footprint usually touches the
// other. Referenced directly via `genprim.SuperRegionSideTiles` /
// `sr.Bounds()` rather than through a local alias to avoid documenting
// the coupling twice.

// NoiseDepositSource implements world.DepositSource by generating resource
// deposits deterministically from a world seed. Generation batches at the
// 4x4 SC super-region granularity so the cache aligns with the volcano
// layer; each super-region is produced exactly once under sync.Once and
// cached in a plain map guarded by cacheMu.
//
// The source is safe for concurrent read. Per-super-region generation
// runs exactly once under sync.Once; cacheMu is held only for the map
// insert — once a *depositRegionData is visible, all subsequent reads
// go through sync.Once without taking cacheMu.
type NoiseDepositSource struct {
	seed      int64
	terrain   TerrainSampler
	landmarks world.LandmarkSource
	volcanoes world.VolcanoSource

	cacheMu sync.RWMutex
	cache   map[genprim.SuperRegion]*depositRegionData

	// zonalNoise holds one OctaveNoise per zonal kind. Indexed by
	// zonalSpecs slot (0, 1, 2) rather than by DepositKind — the fixed
	// array removes per-tile map lookups on the zonal placement path.
	zonalNoise [zonalKindCount]noise.OctaveNoise
}

// depositRegionData is the generated state for one super-region.
// Once-per-key generation; cacheMu protects only map insertion.
type depositRegionData struct {
	once     sync.Once
	deposits []world.Deposit
	byTile   map[geom.Position]world.Deposit
}

// NewNoiseDepositSource wires a deposit source to a TerrainSampler (for
// biome sampling), a LandmarkSource (used by point-like collision
// rejection), and a VolcanoSource (used by volcanic structural obsidian
// and sulfur). The landmark and volcano sources may be nil; callers that
// only need zonal + fish can pass nil without special casing.
//
// The returned source pre-allocates the per-kind zonal noise fields
// once so per-tile sampling stays allocation-free.
func NewNoiseDepositSource(
	seed int64,
	terrain TerrainSampler,
	lm world.LandmarkSource,
	vs world.VolcanoSource,
) *NoiseDepositSource {
	s := &NoiseDepositSource{
		seed:      seed,
		terrain:   terrain,
		landmarks: lm,
		volcanoes: vs,
		cache:     make(map[genprim.SuperRegion]*depositRegionData),
	}
	for i := range zonalSpecs {
		s.zonalNoise[i] = noise.NewOctaveNoise(seed^zonalSpecs[i].subSalt, zonalNoiseOpts)
	}
	return s
}

// DepositAt returns the deposit covering tile p, or (Deposit{}, false)
// when none exists. Deterministic and safe for concurrent read.
func (s *NoiseDepositSource) DepositAt(p geom.Position) (world.Deposit, bool) {
	sr := genprim.SuperRegionOf(geom.WorldToSuperChunk(p.X, p.Y))
	data := s.ensureRegion(sr)
	d, ok := data.byTile[p]
	return d, ok
}

// DepositsIn returns every deposit whose Position lies inside rect. The
// returned slice is a fresh allocation; callers may mutate it without
// affecting the source cache. Iteration order is deterministic: deposits
// are yielded super-region by super-region in ascending (X, Y) order,
// and within a super-region in the order the generator produced them.
func (s *NoiseDepositSource) DepositsIn(rect geom.Rect) []world.Deposit {
	if rect.Empty() {
		return nil
	}
	srs := superRegionsIntersecting(rect)
	out := make([]world.Deposit, 0, 32)
	for _, sr := range srs {
		data := s.ensureRegion(sr)
		for _, d := range data.deposits {
			if rect.Contains(d.Position) {
				out = append(out, d)
			}
		}
	}
	return out
}

// DepositsNear returns every deposit within Chebyshev radius of p,
// sorted by distance ascending with ties broken by (X, Y) lex order so
// the result is fully deterministic across calls with the same inputs.
// radius < 0 returns nil; radius == 0 returns the single deposit on p,
// when one exists.
func (s *NoiseDepositSource) DepositsNear(p geom.Position, radius int) []world.Deposit {
	if radius < 0 {
		return nil
	}
	// Rect uses half-open bounds — add one to the inclusive max.
	rect := geom.Rect{
		MinX: p.X - radius, MaxX: p.X + radius + 1,
		MinY: p.Y - radius, MaxY: p.Y + radius + 1,
	}
	in := s.DepositsIn(rect)
	out := in[:0]
	for _, d := range in {
		if chebyshev(d.Position, p) <= radius {
			out = append(out, d)
		}
	}
	slices.SortFunc(out, func(a, b world.Deposit) int {
		da := chebyshev(a.Position, p)
		db := chebyshev(b.Position, p)
		if c := cmp.Compare(da, db); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Position.X, b.Position.X); c != 0 {
			return c
		}
		return cmp.Compare(a.Position.Y, b.Position.Y)
	})
	return out
}

// ensureRegion returns the filled depositRegionData for sr, running
// generate exactly once under sync.Once. Concurrent callers collapse
// to a single generation pass; subsequent callers observe the
// populated record immediately.
func (s *NoiseDepositSource) ensureRegion(sr genprim.SuperRegion) *depositRegionData {
	s.cacheMu.RLock()
	data, ok := s.cache[sr]
	s.cacheMu.RUnlock()
	if !ok {
		s.cacheMu.Lock()
		if data, ok = s.cache[sr]; !ok {
			data = &depositRegionData{}
			s.cache[sr] = data
		}
		s.cacheMu.Unlock()
	}
	data.once.Do(func() { data.generate(s, sr) })
	return data
}

// depositPriority scores a placement candidate. Higher wins when two
// strategies target the same tile. The scheme:
//
//	zonal   = 10
//	fish    = 20
//	point   = 100 + pointMinDistance[kind]  (140 common … 700 rare)
//	obsidian = 2000
//	sulfur  = 3000
//
// Every structural (obsidian/sulfur) band sits above every point band
// so volcanic placement never gets clobbered by a point-kind candidate
// that sneaks past the biome gate; within point, a rarer kind (larger
// minDistance) wins over a denser one; fish wins over zonal; point
// wins over both.
const (
	priZonal    = 10
	priFish     = 20
	priPoint    = 100 // add pointMinDistance[kind] for tiebreak within point
	priObsidian = 2000
	priSulfur   = 3000
)

// placer accumulates candidate deposits per tile and resolves collisions
// by priority at write time. Avoids the O(n) slice-removal pattern that
// would otherwise appear on every overwrite — the flat deposits slice
// is materialised exactly once at the end via emit.
type placer struct {
	byTile   map[geom.Position]world.Deposit
	priority map[geom.Position]int
}

func newPlacer(cap int) *placer {
	return &placer{
		byTile:   make(map[geom.Position]world.Deposit, cap),
		priority: make(map[geom.Position]int, cap),
	}
}

// place stores dep at t when pri is strictly greater than any prior
// write on t. Equal or lower priorities are dropped, preserving the
// "first-wins-at-tier" property callers rely on (e.g. point-kinds of
// equal rarity iterate in pointKinds order, and the first one to land
// on a shared tile keeps it).
func (pl *placer) place(t geom.Position, dep world.Deposit, pri int) {
	if existing, ok := pl.priority[t]; ok && existing >= pri {
		return
	}
	pl.byTile[t] = dep
	pl.priority[t] = pri
}

// emit copies the per-tile winners into a sorted slice for iteration.
// Sorting by (X, Y) lex order keeps downstream iteration stable across
// calls and across independent sources with the same seed — map-range
// order in Go is deliberately randomised per-call.
func (pl *placer) emit() []world.Deposit {
	out := make([]world.Deposit, 0, len(pl.byTile))
	for _, dep := range pl.byTile {
		out = append(out, dep)
	}
	slices.SortFunc(out, func(a, b world.Deposit) int {
		if c := cmp.Compare(a.Position.X, b.Position.X); c != 0 {
			return c
		}
		return cmp.Compare(a.Position.Y, b.Position.Y)
	})
	return out
}

// generate runs the placement pipeline for one super-region: per-tile
// zonal + fish, then point-like Poisson-disk per kind, then structural
// volcanic (obsidian + sulfur) on any volcano slope tiles inside the
// super-region. All candidates funnel through a placer that resolves
// collisions by priority at O(1) per write and emits the final sorted
// deposit slice exactly once — avoiding the quadratic slice-shuffle
// that naive overwrite-and-rebuild would trigger if a future biome-
// gate widening introduced frequent cross-strategy collisions.
func (d *depositRegionData) generate(s *NoiseDepositSource, sr genprim.SuperRegion) {
	minX, minY, side := sr.Bounds()
	pl := newPlacer(side * side / 4)

	for y := minY; y < minY+side; y++ {
		for x := minX; x < minX+side; x++ {
			t := geom.Position{X: x, Y: y}
			// Fetch tile once; both zonal and fish consumers share it to
			// avoid a second TileAt (five noise samples + biome lookup).
			tile := s.terrain.TileAt(x, y)
			if dep, ok := zonalDepositAt(t, tile.Terrain, &s.zonalNoise); ok {
				pl.place(t, dep, priZonal)
			}
			if dep, ok := fishDepositAtTerrain(s.seed, t, tile.Terrain, s.terrain); ok {
				pl.place(t, dep, priFish)
			}
		}
	}

	// Point-like — Poisson-disk per kind on the super-region. Biome gate
	// keeps point-kinds disjoint from zonal (point lives on mountain /
	// hills / desert / beach; zonal lives on plains / forest family), so
	// the only realistic collisions are two point-kinds on the same
	// tile — resolved by pointMinDistance: the rarer kind (larger
	// spacing) lands at a higher priority and overwrites the denser.
	for _, dep := range pointDepositsInRegion(s.seed, sr, s.terrain, s.landmarks, s.volcanoes) {
		pl.place(dep.Position, dep, priPoint+pointMinDistance[dep.Kind])
	}

	// Structural volcanic — obsidian on any slope, sulfur on core-
	// adjacent slope. Iterates only volcanoes whose anchor SC sits inside
	// this super-region, then filters each volcano's SlopeTiles back to
	// the super-region's own tile footprint so slope spills into a
	// neighbour SR don't double-emit — the neighbour SR's own generate
	// picks them up via its 3x3 SC scan inside obsidian/sulfurDepositAt.
	//
	// On a tile eligible for both kinds (core-adjacent slope on an
	// Active or Dormant volcano), sulfur is tried first and wins if it
	// places. If sulfur rejects (dormant roll misses the 50% gate), the
	// tile falls through to obsidian's 70% roll — so a dormant core-rim
	// tile's final odds are 50% sulfur / 35% obsidian / 15% nothing,
	// not a flat 70% obsidian. The priority scheme (priSulfur >
	// priObsidian) makes this explicit regardless of call order.
	if s.volcanoes != nil {
		minSC := geom.WorldToSuperChunk(minX, minY)
		maxSC := geom.WorldToSuperChunk(minX+side-1, minY+side-1)
		for scX := minSC.X; scX <= maxSC.X; scX++ {
			for scY := minSC.Y; scY <= maxSC.Y; scY++ {
				sc := geom.SuperChunkCoord{X: scX, Y: scY}
				for _, v := range s.volcanoes.VolcanoAt(sc) {
					for _, t := range v.SlopeTiles {
						if t.X < minX || t.X >= minX+side || t.Y < minY || t.Y >= minY+side {
							continue
						}
						if dep, ok := sulfurDepositAt(s.seed, t, s.volcanoes); ok {
							pl.place(t, dep, priSulfur)
							continue
						}
						if dep, ok := obsidianDepositAt(s.seed, t, s.volcanoes); ok {
							pl.place(t, dep, priObsidian)
						}
					}
				}
			}
		}
	}

	d.byTile = pl.byTile
	d.deposits = pl.emit()
}

// superRegionsIntersecting returns every super-region whose bounds
// overlap rect. rect is assumed half-open (MinX/MinY inclusive,
// MaxX/MaxY exclusive), matching geom.Rect's convention. Empty rects
// produce an empty slice.
func superRegionsIntersecting(rect geom.Rect) []genprim.SuperRegion {
	if rect.Empty() {
		return nil
	}
	// Map the inclusive corners of rect to super-regions. MaxX/MaxY are
	// exclusive so the last tile that can lie inside is (MaxX-1, MaxY-1).
	minSC := geom.WorldToSuperChunk(rect.MinX, rect.MinY)
	maxSC := geom.WorldToSuperChunk(rect.MaxX-1, rect.MaxY-1)
	minSR := genprim.SuperRegionOf(minSC)
	maxSR := genprim.SuperRegionOf(maxSC)

	out := make([]genprim.SuperRegion, 0, (maxSR.X-minSR.X+1)*(maxSR.Y-minSR.Y+1))
	for y := minSR.Y; y <= maxSR.Y; y++ {
		for x := minSR.X; x <= maxSR.X; x++ {
			out = append(out, genprim.SuperRegion{X: x, Y: y})
		}
	}
	return out
}

// chebyshev returns the Chebyshev (L-infinity) distance between two
// tile positions on the square grid. Used by DepositsNear to match the
// grid's 8-connectivity movement metric.
func chebyshev(a, b geom.Position) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
}

// Compile-time assertion that NoiseDepositSource satisfies the consumer
// interface. Mirrors the same assertion in volcano and region sub-packages
// so interface drift fails at build time.
var _ world.DepositSource = (*NoiseDepositSource)(nil)

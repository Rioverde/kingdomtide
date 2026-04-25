// Package worldgen generates the bounded, mapgen2-style world the
// server and developer tools consume through the world.TileSource
// interface. The pipeline is described in Generate's doc comment.
package worldgen

import (
	"math"
	"time"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/voronoi"
)

// GenStageHook is called after each Generate pipeline stage when
// non-nil. Used by diagnostic tooling to time each pass — production
// code leaves this at its zero value (no-op).
var GenStageHook func(stage string, dur time.Duration)

// stageTime invokes the hook if installed. Helper to keep the
// pipeline body readable.
func stageTime(stage string, t0 time.Time) {
	if GenStageHook != nil {
		GenStageHook(stage, time.Since(t0))
	}
}

// World is the output of Generate. Data is a cell graph: Voronoi
// holds the tile-to-cell rasterisation and cell adjacency; the
// per-cell slices (Elevation, Moisture, Terrain) are indexed by
// cell ID. Land/ocean / coast classifications are not stored — they
// derive from Terrain and the Voronoi graph via IsOcean / IsCoast.
type World struct {
	Size   WorldSize
	Seed   int64
	Width  int
	Height int

	Voronoi *voronoi.Diagram

	Elevation   []float32
	Moisture    []float32
	Temperature []float32
	Terrain     []gworld.Terrain

	// Watershed maps each cell to the corner index it ultimately
	// drains to (its coast outlet). -1 means no path to coast — ocean
	// cells, or endorheic basins. Two cells with the same value belong
	// to the same drainage basin. int32 not int — corner indices fit
	// well inside ±2¹⁵, halving the memory cost on big worlds.
	Watershed []int32

	// riverBits is a packed W*H bitset — bit set if the tile lies on
	// a rasterised river edge. Consumed by TileAt to set
	// OverlayRiver on the returned tile. 8× smaller than the equivalent
	// []bool, which matters at Huge / 10x-Large scales.
	riverBits *bitset
}

// Generate runs the mapgen2 pipeline end-to-end.
//
// Pipeline stages:
//  1. Voronoi + Lloyd's relaxation — uniform blue-noise cells
//  2. Water classification (multi-centre Patel perlin radial)
//  3. Ocean / lake flood-fill from map border
//  4. Elevation — graph distance from coast
//  5. Moisture — graph distance from water, inverted
//  6. Terrain — Whittaker table on (elevation, moisture)
//  7. Hydrology — corner graph, rivers (downslope + Bresenham),
//     watersheds (downslope propagation to coast)
func Generate(seed int64, size WorldSize) *World {
	w, h := size.Dimensions()
	cellCount := cellCountFor(size)

	out := &World{
		Size:   size,
		Seed:   seed,
		Width:  w,
		Height: h,
	}

	cellSize := math.Sqrt(float64(w*h) / float64(cellCount))
	t0 := time.Now()
	// 1 Lloyd iteration is sufficient on top of Bridson-density seed
	// placement. A second pass adds <2% centroid uniformity improvement
	// at ~25ms extra cost per Standard gen — not worth the budget.
	out.Voronoi = voronoi.Generate(seed, w, h, cellCount, 1, cellSize*0.25)
	stageTime("voronoi", t0)

	t0 = time.Now()
	applyNoisyEdges(out, seed)
	stageTime("noisy_edges", t0)

	n := len(out.Voronoi.Cells)
	out.Elevation = make([]float32, n)
	out.Moisture = make([]float32, n)
	out.Temperature = make([]float32, n)
	out.Terrain = make([]gworld.Terrain, n)

	t0 = time.Now()
	isWater := classifyWater(out, seed)
	stageTime("classify_water", t0)

	t0 = time.Now()
	isOcean, isLake := classifyOceanLake(out, isWater)
	stageTime("classify_ocean_lake", t0)

	t0 = time.Now()
	computeElevation(out, isOcean)
	stageTime("elevation", t0)

	t0 = time.Now()
	perturbElevation(out, isOcean, seed)
	stageTime("perturb_elev", t0)

	t0 = time.Now()
	redistributeElevation(out, isOcean)
	stageTime("redistribute_elev", t0)

	t0 = time.Now()
	computeMoisture(out, isWater, seed)
	stageTime("moisture", t0)

	t0 = time.Now()
	computeTemperature(out)
	stageTime("temperature", t0)

	t0 = time.Now()
	assignTerrains(out, isOcean, isLake)
	stageTime("terrains", t0)

	t0 = time.Now()
	smoothBiomeBoundaries(out, seed)
	stageTime("smooth_biomes", t0)

	t0 = time.Now()
	corners := buildCorners(out, isOcean)
	assignDownslope(corners)
	rivers, lakeCorners := computeRivers(out, corners, seed)
	out.riverBits = rivers
	applyRiverLakes(out, corners, lakeCorners, isOcean)
	out.Watershed = computeWatersheds(out, corners, isOcean)
	stageTime("hydrology", t0)

	return out
}

// TileAt implements world.TileSource. Off-grid queries return deep
// ocean so callers can safely probe outside the world boundary.
// Sets OverlayRiver when the tile is on a rasterised river edge.
func (w *World) TileAt(x, y int) gworld.Tile {
	if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return gworld.Tile{Terrain: gworld.TerrainDeepOcean}
	}
	cellID := w.Voronoi.CellIDAt(x, y)
	tile := gworld.Tile{Terrain: w.Terrain[cellID]}
	if w.IsRiver(x, y) {
		tile.Overlays |= gworld.OverlayRiver
	}
	return tile
}

// IsRiver reports whether the tile sits on a river edge. Off-grid
// queries return false.
func (w *World) IsRiver(x, y int) bool {
	if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return false
	}
	if w.riverBits == nil {
		return false
	}
	return w.riverBits.Get(y*w.Width + x)
}

// IsOcean reports whether the cell's terrain is ocean-like.
func (w *World) IsOcean(cellID uint16) bool {
	t := w.Terrain[cellID]
	return t == gworld.TerrainOcean || t == gworld.TerrainDeepOcean
}

// IsCoast reports whether the cell is land with at least one ocean
// neighbour — derived from the Voronoi graph.
func (w *World) IsCoast(cellID uint16) bool {
	if w.IsOcean(cellID) {
		return false
	}
	for _, n := range w.Voronoi.Cells[cellID].Neighbors {
		if w.IsOcean(n) {
			return true
		}
	}
	return false
}

var _ gworld.TileSource = (*World)(nil)

// cellCountFor scales the Voronoi cell count with world area. The
// cellsPerSqrtArea constant (see tuning.go) lands at ~13-tile cells
// on Standard — fine-grained enough that biome transitions feel
// hand-painted on a roguelike tile grid. Cell COUNT scales with
// √area so smaller worlds stay light.
func cellCountFor(size WorldSize) int {
	w, h := size.Dimensions()
	per := math.Sqrt(float64(w*h)) * cellsPerSqrtArea
	count := int(per)
	if count < 200 {
		count = 200
	}
	return count
}

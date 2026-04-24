// Package worldgen generates the bounded, mapgen2-style world the
// server and developer tools consume through the world.TileSource
// interface. The pipeline is described in Generate's doc comment.
package worldgen

import (
	"math"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/voronoi"
)

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

	Elevation []float32
	Moisture  []float32
	Terrain   []gworld.Terrain
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
	out.Voronoi = voronoi.Generate(seed, w, h, cellCount, 2, cellSize*0.25)

	n := len(out.Voronoi.Cells)
	out.Elevation = make([]float32, n)
	out.Moisture = make([]float32, n)
	out.Terrain = make([]gworld.Terrain, n)

	isWater := classifyWater(out, seed)
	isOcean, isLake := classifyOceanLake(out, isWater)
	computeElevation(out, isOcean)
	computeMoisture(out, isWater)
	assignTerrains(out, isOcean, isLake)

	return out
}

// TileAt implements world.TileSource. Off-grid queries return deep
// ocean so callers can safely probe outside the world boundary.
func (w *World) TileAt(x, y int) gworld.Tile {
	if x < 0 || y < 0 || x >= w.Width || y >= w.Height {
		return gworld.Tile{Terrain: gworld.TerrainDeepOcean}
	}
	cellID := w.Voronoi.CellIDAt(x, y)
	return gworld.Tile{Terrain: w.Terrain[cellID]}
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

// cellCountFor scales the Voronoi cell count with world area —
// ~1500 cells for Standard.
func cellCountFor(size WorldSize) int {
	w, h := size.Dimensions()
	per := math.Sqrt(float64(w*h)) * 0.3
	count := int(per)
	if count < 200 {
		count = 200
	}
	return count
}

package worldgen

import (
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// computeElevation walks outward from coast cells (land with at
// least one ocean neighbour) via multi-source BFS through the land
// graph. Hops from coast = elevation rank; normalised to [0, 1].
// Ocean cells stay at 0.
func computeElevation(w *World, isOcean []bool) {
	dist := make([]int, len(w.Voronoi.Cells))
	for i := range dist {
		dist[i] = -1
	}
	queue := make([]uint16, 0, len(w.Voronoi.Cells))
	// Seed BFS with coast cells (land with ocean neighbour) —
	// computed inline so no separate isCoast slice is needed.
	for id, cell := range w.Voronoi.Cells {
		if isOcean[id] {
			continue
		}
		for _, n := range cell.Neighbors {
			if isOcean[n] {
				dist[id] = 0
				queue = append(queue, uint16(id))
				break
			}
		}
	}
	maxDist := 0
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		for _, n := range w.Voronoi.Cells[id].Neighbors {
			if isOcean[n] || dist[n] != -1 {
				continue
			}
			dist[n] = dist[id] + 1
			if dist[n] > maxDist {
				maxDist = dist[n]
			}
			queue = append(queue, n)
		}
	}
	for i, d := range dist {
		if d < 0 || maxDist == 0 {
			w.Elevation[i] = 0
			continue
		}
		w.Elevation[i] = float32(d) / float32(maxDist)
	}
}

// computeMoisture — multi-source BFS from every water cell. Distance
// in hops = dryness; inverted and normalised to [0, 1].
func computeMoisture(w *World, isWater []bool) {
	dist := make([]int, len(w.Voronoi.Cells))
	for i := range dist {
		dist[i] = -1
	}
	queue := make([]uint16, 0, len(w.Voronoi.Cells))
	for id := range isWater {
		if isWater[id] {
			dist[id] = 0
			queue = append(queue, uint16(id))
		}
	}
	maxDist := 0
	for head := 0; head < len(queue); head++ {
		id := queue[head]
		for _, n := range w.Voronoi.Cells[id].Neighbors {
			if dist[n] != -1 {
				continue
			}
			dist[n] = dist[id] + 1
			if dist[n] > maxDist {
				maxDist = dist[n]
			}
			queue = append(queue, n)
		}
	}
	for i, d := range dist {
		if d < 0 || maxDist == 0 {
			w.Moisture[i] = 0
			continue
		}
		w.Moisture[i] = 1 - float32(d)/float32(maxDist)
	}
}

// assignTerrains maps each cell's classification plus (elevation,
// moisture) to a world.Terrain value. Oceans split into deep vs
// shallow based on whether they border any land; lakes collapse to
// shallow ocean; everything else runs through the Whittaker table.
func assignTerrains(w *World, isOcean, isLake []bool) {
	for i := range w.Voronoi.Cells {
		switch {
		case isOcean[i]:
			// Shallow if any neighbour is land; deep otherwise.
			shallow := false
			for _, n := range w.Voronoi.Cells[i].Neighbors {
				if !isOcean[n] && !isLake[n] {
					shallow = true
					break
				}
			}
			if shallow {
				w.Terrain[i] = gworld.TerrainOcean
			} else {
				w.Terrain[i] = gworld.TerrainDeepOcean
			}
		case isLake[i]:
			w.Terrain[i] = gworld.TerrainOcean
		default:
			w.Terrain[i] = whittakerTerrain(w.Elevation[i], w.Moisture[i])
		}
	}
}

// whittakerTerrain picks a game Terrain from (elevation, moisture).
// Bands on elevation crossed with bands on moisture — Amit Patel's
// Whittaker table mapped onto the game's terrain enum.
func whittakerTerrain(elev, moist float32) gworld.Terrain {
	if elev < 0.08 {
		return gworld.TerrainBeach
	}

	if elev < 0.30 {
		switch {
		case moist < 0.16:
			return gworld.TerrainDesert
		case moist < 0.33:
			return gworld.TerrainSavanna
		default:
			return gworld.TerrainJungle
		}
	}

	if elev < 0.60 {
		switch {
		case moist < 0.16:
			return gworld.TerrainDesert
		case moist < 0.33:
			return gworld.TerrainPlains
		case moist < 0.50:
			return gworld.TerrainGrassland
		default:
			return gworld.TerrainForest
		}
	}

	if elev < 0.80 {
		switch {
		case moist < 0.33:
			return gworld.TerrainHills
		case moist < 0.66:
			return gworld.TerrainMeadow
		default:
			return gworld.TerrainTaiga
		}
	}

	switch {
	case moist < 0.20:
		return gworld.TerrainMountain
	case moist < 0.50:
		return gworld.TerrainTundra
	case moist < 0.80:
		return gworld.TerrainSnow
	default:
		return gworld.TerrainSnowyPeak
	}
}

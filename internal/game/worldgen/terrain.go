package worldgen

import (
	"math"
	"sort"

	opensimplex "github.com/ojrac/opensimplex-go"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// saltMoistNoise / saltTempNoise are the fixed salts for the
// moisture-perturbation and temperature-jitter noise fields.
const (
	saltMoistNoise int64 = 0x082efa98ec4eec6a
	saltTempNoise  int64 = 0x38d8c2c4e0a4f7a3
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

// redistributeElevation re-maps land-cell elevations to a uniform
// [0, 1] distribution by sorting by current elevation and reassigning
// to rank/N. Patel's mapgen2 calls this out as a required step:
// raw BFS-distance elevations cluster heavily at low values (most
// cells are 1-2 hops from coast), which leaves the Whittaker biome
// bands so squashed that beach swallows half the lowland and rivers
// can never accumulate volume because their heads sit at impossibly
// low elevations. After redistribution, x% of land cells have
// elevation ≤ x/100 — the assumption every threshold in
// whittakerTerrain and pickRiverHeads was tuned against.
func redistributeElevation(w *World, isOcean []bool) {
	type rank struct {
		id   uint16
		elev float32
	}
	land := make([]rank, 0, len(w.Voronoi.Cells))
	for i, e := range w.Elevation {
		if isOcean[i] {
			continue
		}
		land = append(land, rank{uint16(i), e})
	}
	if len(land) == 0 {
		return
	}
	sort.Slice(land, func(a, b int) bool {
		return land[a].elev < land[b].elev
	})
	n := float32(len(land))
	for r, c := range land {
		w.Elevation[c.id] = float32(r) / n
	}
}

// computeMoisture — multi-source BFS from every water cell. Distance
// in hops = dryness; inverted and normalised to [0, 1]. A final
// noise perturbation breaks the uniform BFS gradient so adjacent
// cells land in different biome bands.
func computeMoisture(w *World, isWater []bool, seed int64) {
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

	// Noise perturbation — shifts each cell by up to ±moistureJitter
	// so the BFS distribution spreads across the Whittaker bands
	// instead of bunching all cells at similar moisture.
	noise := opensimplex.New(seed ^ saltMoistNoise)
	halfH := float64(w.Height) / 2
	for i, d := range dist {
		var v float32
		if d >= 0 && maxDist > 0 {
			v = 1 - float32(d)/float32(maxDist)
		}
		cell := w.Voronoi.Cells[i]
		nx := (cell.CenterX - float64(w.Width)/2) / halfH
		ny := (cell.CenterY - halfH) / halfH
		jitter := float32(noise.Eval2(nx*moistureNoiseFreq, ny*moistureNoiseFreq)) * moistureJitter
		v += jitter
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		w.Moisture[i] = v
	}
}

// computeTemperature derives per-cell temperature from latitude
// (equator warm, poles cold) with an elevation correction (high
// altitudes cool) and light noise to break hard latitude bands.
// Normalised to [0, 1].
func computeTemperature(w *World) {
	noise := opensimplex.New(w.Seed ^ saltTempNoise)
	halfH := float64(w.Height) / 2
	for i, cell := range w.Voronoi.Cells {
		// Latitude factor: 1 at equator (centre), 0 at poles.
		lat := 1 - math.Abs(cell.CenterY-halfH)/halfH

		// Elevation cooling — high peaks are cold even on the equator.
		cooling := float64(w.Elevation[i]) * temperatureElevCooling

		// Small noise jitter so latitude bands are not razor-straight.
		nx := (cell.CenterX - float64(w.Width)/2) / halfH
		ny := cell.CenterY / halfH
		jitter := noise.Eval2(nx*temperatureNoiseFreq, ny*temperatureNoiseFreq) * temperatureJitter

		t := lat - cooling + jitter
		if t < 0 {
			t = 0
		}
		if t > 1 {
			t = 1
		}
		w.Temperature[i] = float32(t)
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
			w.Terrain[i] = whittakerTerrain(w.Elevation[i], w.Moisture[i], w.Temperature[i])
		}
	}
}

// whittakerTerrain picks a game Terrain from (elevation, moisture,
// temperature). Three-dimensional classification: elevation gives
// relief ladders (peak / hill / lowland), temperature splits climate
// zones (polar / temperate / tropical), moisture picks vegetation
// density within each band. Covers all 16 non-volcanic terrains.
//
// All thresholds are named constants (see the const block above) —
// edit them there to rebalance distribution, not the body.
func whittakerTerrain(elev, moist, temp float32) gworld.Terrain {
	// Beach — very low elevation everywhere (coastal sand / tundra
	// shoreline reads the same regardless of temperature).
	if elev < biomeBeachElev {
		return gworld.TerrainBeach
	}

	// High peaks — temperature decides snow-capped vs bare rock.
	if elev > biomePeakElev {
		if temp < biomePeakSnowTemp {
			return gworld.TerrainSnowyPeak
		}
		return gworld.TerrainMountain
	}

	// Upper highlands.
	if elev > biomeHighElev {
		switch {
		case temp < biomeHighSnowTemp:
			return gworld.TerrainSnow
		case temp > biomeHighHotTemp:
			return gworld.TerrainMountain // tropical bare mountains
		case moist > biomeHighTaigaMoist:
			return gworld.TerrainTaiga
		default:
			return gworld.TerrainHills
		}
	}

	// Polar zone (cold climate) — dominates irrespective of elev.
	if temp < biomePolarTemp {
		if moist > biomePolarTaigaMoist {
			return gworld.TerrainTaiga
		}
		return gworld.TerrainTundra
	}

	// Tropical zone (hot climate).
	if temp > biomeTropicTemp {
		switch {
		case moist > biomeTropicJungleMoist:
			return gworld.TerrainJungle
		case moist > biomeTropicSavannaMoist:
			return gworld.TerrainSavanna
		default:
			return gworld.TerrainDesert
		}
	}

	// Temperate highlands (above lowland, below peaks).
	if elev > biomeHighlandElev {
		if moist > biomeHighlandMeadowMoist {
			return gworld.TerrainMeadow
		}
		return gworld.TerrainHills
	}

	// Temperate lowlands.
	switch {
	case moist > biomeForestMoist:
		return gworld.TerrainForest
	case moist > biomeGrasslandMoist:
		return gworld.TerrainGrassland
	case moist > biomePlainsMoist:
		return gworld.TerrainPlains
	default:
		return gworld.TerrainDesert
	}
}

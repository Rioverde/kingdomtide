package worldgen

import (
	"math"
	"sort"

	opensimplex "github.com/ojrac/opensimplex-go"

	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// saltMoistNoise / saltTempNoise / saltElevNoise / saltBiomeSmooth
// are fixed salts that prevent correlated noise fields across pipeline
// stages when they share the same world seed.
const (
	saltMoistNoise  int64 = 0x082efa98ec4eec6a
	saltTempNoise   int64 = 0x38d8c2c4e0a4f7a3
	saltElevNoise   int64 = 0x5d3a8f7b2c4e9a1f
	saltBiomeSmooth int64 = 0x7e2b9f4a6d8c3e51
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
// in hops = dryness; inverted and normalised to [0, 1]. A multi-octave
// fBm perturbation breaks the uniform BFS gradient so adjacent cells
// land in different Whittaker bands. A rain-shadow pass then penalises
// cells sheltered behind high terrain to the west.
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

	// Multi-octave fBm perturbation — 4 octaves spread moisture across
	// the Whittaker bands much more effectively than a single-octave
	// jitter. Each octave doubles frequency and halves amplitude; the
	// sum is normalised before scaling by moistureJitter.
	noise := opensimplex.New(seed ^ saltMoistNoise)
	halfH := float64(w.Height) / 2
	halfW := float64(w.Width) / 2
	for i, d := range dist {
		var v float32
		if d >= 0 && maxDist > 0 {
			v = 1 - float32(d)/float32(maxDist)
		}
		cell := w.Voronoi.Cells[i]
		nx := (cell.CenterX - halfW) / halfH
		ny := (cell.CenterY - halfH) / halfH

		fbm := 0.0
		amp := 1.0
		freq := moistureNoiseFreq
		norm := 0.0
		for oct := 0; oct < moistureOctaves; oct++ {
			fbm += amp * noise.Eval2(nx*freq, ny*freq)
			norm += amp
			amp *= moistureGain
			freq *= moistureLacunarity
		}
		jitter := float32(fbm/norm) * moistureJitter

		v += jitter
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		w.Moisture[i] = v
	}

	// Rain shadow pass — for each land cell, walk up to rainShadowHops
	// westward via the neighbour with the smallest CenterX. If any
	// visited cell has elevation above the threshold (i.e. a mountain
	// blocking the prevailing westerlies), penalise this cell's moisture.
	// Walking the cell graph is O(N·hops) and avoids a spatial hash.
	for i, cell := range w.Voronoi.Cells {
		if isWater[i] {
			continue
		}
		cur := uint16(i)
		for hop := 0; hop < rainShadowHops; hop++ {
			// Pick the neighbour most to the west (smallest CenterX).
			best := uint16(0)
			bestX := math.MaxFloat64
			found := false
			for _, n := range w.Voronoi.Cells[cur].Neighbors {
				nx := w.Voronoi.Cells[n].CenterX
				// Only walk west — stop if no neighbour is further west.
				if nx < cell.CenterX && nx < bestX {
					bestX = nx
					best = n
					found = true
				}
			}
			if !found {
				break
			}
			cur = best
			if w.Elevation[cur] > rainShadowElevThreshold {
				w.Moisture[i] *= rainShadowPenalty
				if w.Moisture[i] < 0 {
					w.Moisture[i] = 0
				}
				break
			}
		}
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

// perturbElevation adds multi-octave fBm noise to land-cell elevations
// BEFORE redistribution. The BFS distance field is monotonically smooth
// (every cell is just "hops from coast"), which means mountains have no
// valleys and plains have no hills. The perturbation re-orders cells in
// the elevation ranking so redistribution produces a more varied result.
// Ocean cells are left at zero — they do not participate in redistribution.
func perturbElevation(w *World, isOcean []bool, seed int64) {
	if w == nil || len(w.Voronoi.Cells) == 0 {
		return
	}
	noise := opensimplex.New(seed ^ saltElevNoise)
	halfH := float64(w.Height) / 2
	halfW := float64(w.Width) / 2
	for i, cell := range w.Voronoi.Cells {
		if isOcean[i] {
			continue
		}
		nx := (cell.CenterX - halfW) / halfH
		ny := (cell.CenterY - halfH) / halfH

		fbm := 0.0
		amp := 1.0
		freq := elevationNoiseFreq
		norm := 0.0
		for oct := 0; oct < elevationOctaves; oct++ {
			fbm += amp * noise.Eval2(nx*freq, ny*freq)
			norm += amp
			amp *= 0.5
			freq *= 2.0
		}
		delta := float32(fbm/norm) * elevationNoiseAmplitude
		v := w.Elevation[i] + delta
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		w.Elevation[i] = v
	}
}

// smoothBiomeBoundaries runs after assignTerrains. For each land cell
// that sits on a biome boundary (at least one neighbour has a different
// terrain), with probability biomeSmoothChance the cell adopts the
// terrain of one of those differently-typed neighbours. The choice is
// deterministic — hashing cellID against the world seed — so the result
// is reproducible without a global RNG state. Only land-to-land swaps
// are performed; ocean and lake cells are never touched.
func smoothBiomeBoundaries(w *World, seed int64) {
	if w == nil || len(w.Voronoi.Cells) == 0 {
		return
	}
	// Work from a snapshot of pre-pass terrains so swaps made earlier
	// in the loop do not influence later cells in the same pass.
	snapshot := make([]gworld.Terrain, len(w.Terrain))
	copy(snapshot, w.Terrain)

	for i, cell := range w.Voronoi.Cells {
		t := snapshot[i]
		if t == gworld.TerrainOcean || t == gworld.TerrainDeepOcean {
			continue
		}

		// Collect neighbours with a different land terrain.
		different := make([]gworld.Terrain, 0, len(cell.Neighbors))
		for _, n := range cell.Neighbors {
			nt := snapshot[n]
			if nt == gworld.TerrainOcean || nt == gworld.TerrainDeepOcean {
				continue
			}
			if nt != t {
				different = append(different, nt)
			}
		}
		if len(different) == 0 {
			continue
		}

		// Deterministic probability check: hash (cellID, seed) → [0,1).
		// Using a simple xorshift mix keeps the hot path allocation-free.
		h := uint64(i)*0x9e3779b97f4a7c15 ^ uint64(seed)*0x6c62272e07bb0142
		h ^= h >> 30
		h *= 0xbf58476d1ce4e5b9
		h ^= h >> 27
		h *= 0x94d049bb133111eb
		h ^= h >> 31
		prob := float64(h>>11) / float64(1<<53)
		if prob >= biomeSmoothChance {
			continue
		}

		// Pick one of the differing-terrain neighbours deterministically.
		pick := int(h>>32) % len(different)
		w.Terrain[i] = different[pick]
	}
}

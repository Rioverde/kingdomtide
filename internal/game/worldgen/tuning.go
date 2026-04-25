// Tuning collects every user-facing knob the worldgen pipeline
// exposes — biome thresholds, noise amplitudes, continent shapes,
// river density, cell density. Edit values here to rebalance
// generation; per-file salts and implementation constants stay close
// to their consumers.
package worldgen

// === Cell density ============================================
//
// cellsPerSqrtArea multiplies √area to derive Voronoi cell count.
// 10.0 lands at ~13-tile cells on Standard — fine-grained enough
// that a single viewport spans many biome transitions.
const cellsPerSqrtArea = 10.0

// === Land/water classification (classify.go) =================
//
// Patel multi-centre radial: isLand = noise > base + slope·length²
// where length is normalised distance from nearest continent centre.
const (
	// continentRadiusFraction multiplies map half-height to give one
	// continent's nominal radius. 0.75 keeps each continent inside
	// the bounding box on the standard 2.5:1 aspect.
	continentRadiusFraction = 0.75
	// continentSpacingFactor sets minimum centre-to-centre distance
	// in units of continentRadius. >2 keeps continents separate;
	// 2.2 leaves a sea channel between every pair.
	continentSpacingFactor = 2.2
	// classifyOctaves stacks octaves of perlin for the land mask.
	// Higher = more fractal coastlines.
	classifyOctaves = 8
	// classifyBaseThreshold raises the noise bar centrally — higher
	// shrinks land area inside continents.
	classifyBaseThreshold = 0.3
	// classifySlopeThreshold pulls land away from continent edges —
	// higher = sharper continent perimeter.
	classifySlopeThreshold = 0.3
)

// === Moisture (terrain.go) ===================================
//
// Moisture starts as the inverse of BFS distance from water; the
// perturbation noise breaks the uniform gradient so adjacent cells
// land in different Whittaker bands.
const (
	moistureNoiseFreq = 3.0  // spatial frequency multiplier
	moistureJitter    = 0.25 // ± amplitude of perturbation
)

// === Temperature (terrain.go) ================================
//
// Temperature = latitude − elev·cooling + noise jitter.
const (
	// temperatureElevCooling — peak (elev=1) loses this much temp.
	// 0.40 keeps tropical peaks bare-mountain instead of snow-capped.
	temperatureElevCooling = 0.40
	temperatureNoiseFreq   = 2.0  // spatial frequency multiplier
	temperatureJitter      = 0.07 // ± amplitude of jitter
)

// === Whittaker biome bands (terrain.go) ======================
//
// After redistributeElevation, land has uniform [0, 1] elevation;
// thresholds are tuned against THAT distribution.
//
// Zone layout in elev space:
//
//	< beachElev               : Beach (~8% land at 0.08)
//	beachElev .. highlandElev : temperate lowlands (forest/grass/plains/desert)
//	highlandElev .. highElev  : temperate hills/meadows
//	highElev .. peakElev      : high zones (mountain/snow/taiga/hills)
//	> peakElev                : peaks (snowy peak / mountain)
const (
	biomeBeachElev    = 0.08
	biomeHighlandElev = 0.65 // lowland → temperate highland
	biomeHighElev     = 0.80 // → high zone
	biomePeakElev     = 0.92 // peaks: ~8% of land instead of 15%

	// Peak/high temperature splits — below these the band turns snowy.
	biomePeakSnowTemp   = 0.45
	biomeHighSnowTemp   = 0.28
	biomeHighHotTemp    = 0.70 // tropical high → bare mountain
	biomeHighTaigaMoist = 0.60

	// Polar climate (cold, dominates lowland regardless of elev).
	biomePolarTemp       = 0.25
	biomePolarTaigaMoist = 0.50

	// Tropical climate.
	biomeTropicTemp         = 0.70
	biomeTropicJungleMoist  = 0.55
	biomeTropicSavannaMoist = 0.25

	// Temperate highland (between lowland and high zones).
	biomeHighlandMeadowMoist = 0.55

	// Temperate lowland moisture bands — looser thresholds give
	// Plains/Desert visibility instead of Forest/Grassland eating
	// every cell.
	biomeForestMoist    = 0.65
	biomeGrasslandMoist = 0.40
	biomePlainsMoist    = 0.20
)

// === Rivers (rivers.go) ======================================
const (
	// River source elevation band per Patel mapgen2. Outside [min,
	// max] candidates are rejected — too low and the chain barely
	// reaches the coast; too high and the source has no useful
	// descent.
	riverHeadElevMin = 0.30
	riverHeadElevMax = 0.90
	// riverHeadFraction is the fraction of corners considered as
	// river heads. 1.5% on Standard gives a few dozen rivers — enough
	// to feel populated, sparse enough to read.
	riverHeadFraction = 0.015
)

// === Noisy edges (noisy_edges.go) ============================
const (
	// noisyEdgesFreq — spatial frequency of the displacement field.
	// ~25-tile period gives gentle large-scale curves.
	noisyEdgesFreq = 0.04
	// noisyEdgesAmplitude — max ± per-axis tile displacement. ±5 is
	// visibly organic; tuned against Lloyd-relaxed cell sizes so the
	// warp stays inside neighbouring-cell territory.
	noisyEdgesAmplitude = 5.0
)

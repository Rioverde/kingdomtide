// Tuning collects every user-facing knob the worldgen pipeline
// exposes — biome thresholds, noise amplitudes, continent shapes,
// river density, cell density. Edit values here to rebalance
// generation; per-file salts and implementation constants stay close
// to their consumers.
package worldgen

// === Cell density ============================================
//
// cellsPerSqrtArea multiplies √area to derive Voronoi cell count.
// 20.0 lands at ~9-tile cells on Standard — small enough that
// individual cell outlines drop below visual perception even at
// medium zoom; combined with the multi-octave noisy-edges warp,
// pentagon shapes disappear entirely.
const cellsPerSqrtArea = 20.0

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
	// classifySlopePower shapes how the threshold rises with distance
	// from a continent centre. 2.0 (quadratic) gives sharp continent
	// edges and zero archipelagos. 1.0 (linear) lets land probability
	// fade gradually so noise pockets become small islands within
	// ~2× continent radius — natural archipelago halos.
	classifySlopePower = 1.0
)

// === Moisture (terrain.go) ===================================
//
// Moisture starts as the inverse of BFS distance from water; the
// perturbation noise breaks the uniform gradient so adjacent cells
// land in different Whittaker bands.
const (
	moistureNoiseFreq = 3.0  // spatial frequency multiplier
	moistureJitter    = 0.25 // ± amplitude of perturbation

	// fBm parameters for multi-octave moisture noise.
	moistureOctaves    = 4   // number of stacked octaves
	moistureLacunarity = 2.0 // frequency doubles each octave
	moistureGain       = 0.5 // amplitude halves each octave
)

// === Rain shadow (terrain.go) ================================
//
// After BFS moisture, cells east of high terrain are penalised.
// The check walks westward via neighbour hops — cheap on the cell
// graph, no spatial hash needed.
const (
	// rainShadowHops is how many westward cell-graph hops to check
	// for blocking terrain. 4 hops ≈ a few cell widths, enough to
	// shadow the immediate lee side of a mountain.
	rainShadowHops = 4
	// rainShadowElevThreshold — cells at or above this normalised
	// elevation (post-redistribute, so ~top 30% of land) cast shadow.
	rainShadowElevThreshold = 0.70
	// rainShadowPenalty multiplies moisture on shadowed cells.
	// 0.55 gives a noticeable desert-rain-shadow without desiccating
	// every continental interior.
	rainShadowPenalty = 0.55
)

// === Elevation perturbation (terrain.go) =====================
//
// fBm noise added to BFS elevation BEFORE redistribution breaks the
// monotonic coast-distance gradient so mountains have valleys and
// plains have hills.
const (
	elevationOctaves       = 4    // stacked octaves
	elevationNoiseFreq     = 1.5  // base spatial frequency
	elevationNoiseAmplitude = 0.15 // relative perturbation magnitude
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

// === Biome boundary smoothing (terrain.go) ===================
//
// A post-assignTerrains pass that randomly swaps boundary cells to
// a neighbour's terrain. Creates DF-like fringe zones where biomes
// blend instead of cutting hard lines.
const (
	// biomeSmoothChance is the probability a boundary land cell
	// adopts a neighbouring biome. 0.25 blends ~25% of edges.
	biomeSmoothChance = 0.25
)

// === Noisy edges (noisy_edges.go) ============================
//
// Multi-octave fBm warp — single-octave produced uniform wave-pattern
// boundaries that still read as polygonal cell shapes. Stacking 4
// octaves spans periods ~25, 10, 4, 1.6 tiles → big organic curves
// PLUS pixel-level jitter that scatters individual tiles across
// biome boundaries.
//
// Amplitude tuned ≈ avg cell side so cells visibly intrude into
// their neighbours, dissolving the pentagon outlines.
//
// Coarse-grid sampling: noise is evaluated on a sparse grid
// (1 sample per noisyEdgesCoarseFactor×noisyEdgesCoarseFactor tile
// patch) and bilinearly interpolated at every tile. fBm is smooth at
// small scales so 8×8 interpolation is visually indistinguishable
// from per-tile sampling while cutting noise evaluations by 64×.
const (
	// noisyEdgesOctaves stacks octaves of displacement noise.
	noisyEdgesOctaves = 4
	// noisyEdgesFreqFactor — base spatial frequency = factor /
	// avgCellSide, so the lowest-octave period spans ~2× cell size
	// regardless of world scale. Without this, Gigantic worlds (cell
	// ~25 tiles) get one wave cycle per cell — not organic. With it,
	// neighbouring cells share correlated wavy boundaries that read
	// as continuous curves across the whole map.
	noisyEdgesFreqFactor = 0.5
	// noisyEdgesLacunarity — frequency multiplier per octave.
	noisyEdgesLacunarity = 2.5
	// noisyEdgesGain — amplitude multiplier per octave. 0.55 keeps
	// high octaves contributing meaningful pixel-level jitter.
	noisyEdgesGain = 0.55
	// noisyEdgesAmplitudeFactor scales the warp amplitude with the
	// average cell side, so polygon-dissolution stays consistent
	// across world sizes (Standard cells ~9 tiles, Gigantic ~25).
	// 1.55× cell side = each cell can intrude into a full neighbour
	// in either direction, dissolving the polygon outline.
	noisyEdgesAmplitudeFactor = 1.55
	// noisyEdgesCoarseOctaves splits the fBm: the LOW octaves (long
	// wavelength, big curves) are baked onto a coarse grid and
	// bilinearly interpolated — cheap and visually lossless. The
	// HIGH octaves (short wavelength, pixel-scale jitter) are
	// evaluated per-tile because bilinear interp would otherwise
	// smear them away and re-expose the underlying cell polygons as
	// blocky outlines. 1 coarse + 3 per-tile maximises the per-tile
	// jitter that destroys pentagon visibility.
	noisyEdgesCoarseOctaves = 1
	// noisyEdgesCoarseFactor — tiles per coarse-grid cell on each
	// axis. Noise is sampled once per (cf×cf) patch and bilinearly
	// interpolated at each tile. 8 gives 64× fewer Eval2 calls with
	// no perceptible quality loss because fBm is spatially smooth
	// at this scale (lowest-octave period is ~25 tiles >> 8).
	noisyEdgesCoarseFactor = 8
)

// === Camps (camp_source.go) ==================================
//
// Camps are the pre-historic settler clusters that seed the
// 200-year fold-forward simulation (KINGDOMS.md §2.6 footprint
// shape; placement is its own algorithm). A camp is a 2-3 tile
// settlement scattered across the world via Bridson Poisson-disk
// sampling, weighted by a per-cell habitability score. Survivors
// of the simulation become Cities or Villages; non-survivors die.
const (
	// campMinSpacing is the minimum tile distance between any two
	// camp anchors (Bridson radius). 8 tiles → ~30-40 candidates
	// per 64×64 super-chunk; balanced between organic density
	// (medieval Europe ~ village every 2-5 km) and per-year
	// simulation cost.
	campMinSpacing = 8
	// campPoissonK is the Bridson rejection-sampling parameter.
	// Standard 30; higher values waste time without changing
	// resulting density meaningfully.
	campPoissonK = 30
	// campZipfMin is the smallest viable initial population.
	// 10 = a single extended family. Below this a 'camp' is
	// indistinguishable from a transient hunting party.
	campZipfMin = 10.0
	// campZipfAlpha shapes the Pareto distribution of initial
	// population. 1.5 is steeper than the city Pareto (1.0): most
	// camps stay tiny, a few rare ones approach campMaxPop.
	campZipfAlpha = 1.5
	// campMaxPop caps the initial population at founding. After 200
	// years of simulation, surviving camps can grow far beyond this —
	// the cap only constrains the seed value.
	campMaxPop = 50.0
	// campFootprintSmallPopThreshold splits the 2-tile and 3-tile
	// footprint budget. Camps with Pop ≤ this get 2 tiles; above
	// it, 3 tiles. Mirrors KINGDOMS.md §2.6 villageTileCount.
	campFootprintSmallPopThreshold int32 = 25
	// campHabitabilityFloor is the minimum cell habitability score for a
	// candidate to be eligible. 0.15 keeps empty regions empty —
	// desert/tundra-only zones may have zero camps, which is correct.
	// Raise the floor to restrict camps to only the best terrain;
	// lower it to allow marginal terrain candidates.
	campHabitabilityFloor = 0.15
	// campRegionAffinityMin and campRegionAffinityMax bracket
	// the per-super-chunk "settlement willingness" roll. Each
	// region rolls a uniform value in [min, max] once at
	// worldgen time; the result multiplies every candidate's
	// habitability score inside that region. Some regions
	// become densely populated, others stay sparse — organic
	// variance independent of biome quality. Range [0.5, 1.5]
	// gives up to 3× density spread between adjacent regions
	// while keeping the lower bound above zero so no region is
	// guaranteed empty just by RNG.
	campRegionAffinityMin = 0.5
	campRegionAffinityMax = 1.5
	// campRarityMultiplier scales the final Gate-6 acceptance roll by
	// a flat multiplier so density becomes a single predictable dial.
	// 0.25 means: even on perfect terrain (Plains, h=0.95, affinity=1.5)
	// each Bridson candidate has only ~25% chance of becoming a camp.
	// Tune up for denser worlds, down for sparser. Per-camp Pop and
	// Faith distributions are unaffected because this multiplier is
	// uniform across terrain quality.
	campRarityMultiplier = 0.25
)

// === Habitability bonuses (habitability.go) ==================
//
// Per-bonus knobs for habitabilityAt. Tuned so a Plains tile next to a
// river with one food deposit nearby lands above 1.0 (clamped), while a
// Tundra tile with no water and no deposits stays well below
// campHabitabilityFloor.
const (
	// campCoastBonus is added when w.IsCoast(cellID) is true. Coastal
	// trade and fishing make even mediocre biomes attractive.
	campCoastBonus = 0.10
	// campRiverBonus is added when w.IsRiver(p.X, p.Y) is true. Rivers
	// supply fresh water and transport — historically the single most
	// important settlement factor.
	campRiverBonus = 0.15
	// campFoodDepositBonus is added per Fertile/Fish/Game deposit within
	// campDepositSearchRadius. Multiple deposits stack but the score is
	// clamped at 1.0.
	campFoodDepositBonus = 0.08
	// campGenericDepositBonus is added per non-food deposit (Iron, Stone,
	// Timber, Salt, Gold, Silver, Gems, Obsidian, Sulfur) within
	// campDepositSearchRadius. Lower than food because raw materials do
	// not directly feed people.
	campGenericDepositBonus = 0.04
	// campDepositSearchRadius is the Chebyshev radius queried by
	// habitabilityAt for nearby deposits. 5 tiles ≈ a half-day's walk in
	// medieval terms.
	campDepositSearchRadius = 5
	// campVolcanoPenaltyRadius is the Chebyshev radius beyond a volcano's
	// footprint that triggers the proximity penalty. Hard-rejected tiles
	// (inside the footprint) are filtered by the gate chain, not here.
	campVolcanoPenaltyRadius = 8
	// campVolcanoPenaltyMult halves the habitability score for tiles
	// within campVolcanoPenaltyRadius of a volcano. Visible volcanic
	// hazard suppresses but does not eliminate settlement (Pompeii,
	// Catania).
	campVolcanoPenaltyMult = 0.4
)

// === Camp seed salts (camp_source.go) ========================
//
// One PCG stream per camp subsystem. Each salt is the fractional
// hex of √(distinct prime) so streams are decorrelated. No
// subsystem reads from another's stream.
const (
	seedSaltCamp               int64 = 0x6e8c4d7a9b2f51c3
	seedSaltCampFaith          int64 = 0x3a5e7b9f2c4d1068
	seedSaltCampPop            int64 = 0x5b9e3f7c2a8d4061
	seedSaltCampFootprint      int64 = 0x7c1b5f3a9e2d8064
	seedSaltCampRegionAffinity int64 = 0x2f8a4c1e7b5d9063
	// seedSaltCampAccept drives the per-candidate weighted acceptance roll
	// (Gate 6). Kept distinct from seedSaltCamp (Bridson stream) so the
	// two PCG streams are decorrelated — identical seed+SC combos hit
	// different PRNG states. Value: fractional hex of √83.
	seedSaltCampAccept int64 = 0x1d4a8c3e9f2b5071
	// seedSaltCampRuler seeds the dice.Stream used to roll the founding
	// ruler's ability scores. Distinct from every other camp salt so
	// ruler rolls are decorrelated from placement, pop, and faith draws.
	// Value: fractional hex of √89.
	seedSaltCampRuler int64 = 0x4f6b8c2e1a3d7059
)

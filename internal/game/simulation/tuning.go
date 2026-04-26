// Tuning collects every user-facing knob the bottom-up settlement
// simulation exposes — birth/death rates, merge geometry, promotion
// thresholds, faith dynamics. Edit values here to retune; per-step
// implementation constants live closer to their consumers.
package simulation

import "github.com/Rioverde/gongeons/internal/game/polity"

const (
	// === Time horizon =====================================
	// simYears is the per-load fold-forward duration. Default is 500
	// years, bumped from the KINGDOMS.md §8 baseline of 200 for richer
	// settlement dynamics (Turchin's secular cycle motivation still
	// holds; 500 years spans ~2.5 full cycles). Edit here to retune.
	simYears = 500

	// === Population dynamics (population.go) ==============
	// simBaseBirthRate / simBaseDeathRate per .omc/plans/simulation.md §4.
	// Net +0.4%/yr in good years; slower, more medieval-realistic pace.
	simBaseBirthRate = 0.040
	simBaseDeathRate = 0.036
	// simFamineMult — death rate multiplier in famine years.
	// ×2.0 produces ~5% excess mortality; milder than the historical
	// Great Famine peak but appropriate for a game that needs diverse
	// settlement outcomes.
	simFamineMult = 2.0
	// simD6FamineProb — Bernoulli probability that any given
	// region-year is a famine year. 0.04 ≈ 1 famine per region per
	// 25 years on average.
	simD6FamineProb = 0.04

	// === Death / abandonment (deaths.go) ==================
	simCampPopAbandonFloor = 2
	simAbandonStreakYears  = 3

	// === Satellite spawning (satellites.go) ===============
	simCampPopCap         = 60
	simHamletPopCap       = 200
	simVillagePopCap      = 1500
	simSatelliteRadiusMin = 8
	simSatelliteRadiusMax = 16
	simSatelliteAttempts  = 12
	// simSatelliteSpawnProb is the per-year Bernoulli probability that an
	// over-cap settlement spawns a splinter Camp instead of just staying
	// full. 0.10 = ~1 splinter per decade for an always-over-cap
	// settlement. Lower for a conservative world; raise for expansionary.
	simSatelliteSpawnProb = 0.10

	// === Merging (merges.go) ==============================
	// simMergeDistTiles matches the Bridson minimum spacing from camps.md
	// so neighbouring original camps that share region + faith are eligible
	// to merge directly. Below 8, only sub-Bridson satellites could ever merge.
	simMergeDistTiles   = 8
	simMergeRatioMax    = 4.0
	simFaithCohesionMin = 0.5
	// simMergeProb is the per-year probability that an eligible pair of
	// settlements actually unites. 0.10 ≈ 50% chance of merging within
	// 7 years for an eligible pair.
	simMergeProb = 0.10

	// === Promotion (promotions.go) ========================
	// simHamletPromotePop is set AT simCampPopCap so camps that reach
	// the population cap and sustain it promote to hamlet after
	// simHamletPromoteSustain years.
	simHamletPromotePop         = 58
	simHamletPromoteSustain     = 3
	simVillagePromotePop        = 195
	simVillagePromoteSustain    = 8
	simVillageExclusivityRadius = 16

	// === Faith dynamics (faith.go) ========================
	// Markov-chain conversion rates per .omc/plans/simulation.md §9.5.
	// c:d = 3:1 yields equilibrium ~75% majority.
	simFaithConformityRate = 0.005
	simFaithDissidenceRate = 0.00167
	simFaithEpsilon        = 1e-4
	// Region-affinity bias — pulls each settlement toward its region's
	// preferred faith over centuries, breaking the homogenization
	// failure mode where every settlement converges on its initial
	// majority. 0.005 produces meaningful drift over 500 years
	// (Fey regions visibly favour GreenSage by mid-simulation).
	simFaithRegionAffinityRate = 0.005

	// === Plague (population.go) ===========================
	// simPlagueProb is the per-region per-year probability of a plague.
	// Plague years multiply death rate by simPlagueMult. Distinct from
	// famine; plagues are rarer but more catastrophic. ~1 plague per
	// region per 200 years on average.
	simPlagueProb = 0.005
	simPlagueMult = 4.0
)

// regionGrowthMod multiplies the net birth-death rate for settlements
// in each region. Matches geographic carrying capacity:
//
//	Holy:     +0.20  // fertile, blessed land
//	Normal:    0.00  // baseline
//	Wild:      0.00
//	Ancient:  -0.10  // older, more developed; smaller marginal growth
//	Fey:      -0.10  // strange terrain, modest yield
//	Savage:   -0.20  // harsh, contested
//	Blighted: -0.40  // poisoned land, struggling
var regionGrowthMod = [polity.RegionCharacterCount]float64{
	polity.RegionNormal:   0.00,
	polity.RegionBlighted: -0.40,
	polity.RegionFey:      -0.10,
	polity.RegionAncient:  -0.10,
	polity.RegionSavage:   -0.20,
	polity.RegionHoly:     +0.20,
	polity.RegionWild:     0.00,
}

// Per-stream PCG seed salts. One per subsystem so streams are
// pairwise decorrelated. Hex values are arbitrary 64-bit constants
// chosen to be pairwise distinct.
const (
	seedSaltSim          int64 = 0x4f8a3c1e7b9f5062
	seedSaltSimPop       int64 = 0x6c2e8a1f3b5d7094
	seedSaltSimDeath     int64 = -7040766074291400586
	seedSaltSimSatellite int64 = 0x2c8e4a3f7b1d9065
	seedSaltSimMerge     int64 = 0x7b3c8e5f2a4d1098
	seedSaltSimFacility  int64 = 0x5d9e3a8c4f2b7061
	seedSaltSimFamine    int64 = 0x3a5c7e9f2b8d4067
	seedSaltSimName      int64 = -8134558268596084632
	// seedSaltSimRuler seeds the dice.Stream for satellite-camp founding
	// ruler ability scores. Distinct from all other sim salts.
	seedSaltSimRuler int64 = 0x4e3f5c7b2a9d4061
	// seedSaltSimFootprint seeds the random-walk used by regrowFootprint
	// at promotion and merge time. Distinct from all other sim salts.
	seedSaltSimFootprint int64 = -4494003504727084986
	// seedSaltSimPlague seeds the per-region plague schedule. Distinct
	// from famine so plague rolls do not correlate with famine years.
	seedSaltSimPlague int64 = 0x1c8b7d4f2e6a3052
	// seedSaltSimRulerSucc seeds the per-(year, settlementID) succession
	// stream used when a Ruler dies of old age and a new ruler is rolled.
	seedSaltSimRulerSucc int64 = 0x6b2a4e8c1f9d3057
)

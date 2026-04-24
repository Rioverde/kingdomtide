package resource

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
)

// densityRoll derives a deterministic [0, 1) value from a 64-bit hash.
// Uses the top 24 bits of h rather than the low 24 because hashPos is
// built from prime multiplications — the avalanche in prime-multiplied
// uint64 values propagates toward the high bits, while the low bits can
// degenerate near (0, 0) (for p = {0, 0} the product collapses to zero
// and XORing with a per-stream salt yields identical low bits across
// every tile). Top-bit extraction matches the pattern already used by
// volcanoState over in the volcano sub-package.
func densityRoll(h uint64) float64 {
	return float64(h>>40) / float64(1<<24)
}

// fishDensityFraction is the fraction of beach-facing-ocean tiles that
// carry a Fish deposit. 0.5 keeps coastlines populated with fishing
// grounds without saturating every tile — empty beach tiles still exist
// as scenery and as future landing sites for coastal landmarks.
const fishDensityFraction = 0.5

// fishMaxAmount is the starting yield for a Fish deposit. Mid-range:
// richer than Game (300) because fisheries sustain larger populations
// historically, but below Timber (800) because a single coast tile
// produces less total biomass than a forested acre.
const fishMaxAmount = 400

// obsidianDensityFraction is the fraction of volcano slope tiles that
// carry an Obsidian deposit. 0.7 keeps obsidian abundant on every
// volcano flank without saturating every tile — the remaining 30%
// stays bare slope for movement cost and future lava-flow visuals.
const obsidianDensityFraction = 0.7

// obsidianMaxAmount is the starting yield for an Obsidian deposit.
// Tuned against the other volcanic / mountain kinds: richer than the
// rarest mountain points (Gold 50, Gems 30) because obsidian flows are
// broadly harvestable, poorer than Stone (1000) because obsidian is a
// specialty material, not a bulk building stone.
const obsidianMaxAmount = 200

// sulfurDormantFraction gates sulfur placement on Dormant volcanoes:
// only 50% of core-adjacent slope tiles carry sulfur around a dormant
// core. Active volcanoes accept unconditionally; Extinct volcanoes
// reject wholesale. Matches the plan's "Active 100% / Dormant 50% /
// Extinct 0%" rule for structural sulfur.
const sulfurDormantFraction = 0.5

// sulfurMaxAmount is the starting yield for a Sulfur deposit. Below
// obsidian (200) because sulfur sits in a much narrower ring — only
// the slope tiles that directly border the core — so the total sulfur
// budget around a volcano stays low by construction.
const sulfurMaxAmount = 150

// seedSaltDepositFish is the top-level salt for the fish selection
// stream. Distinct from every other worldgen salt; the plan's value is
// routed through regionToInt64 because the top bit is set.
var seedSaltDepositFish = genprim.ToInt64(0xce8a5b3f1d9e2c4a)

// fishSubSalt mixes an additional per-stream salt into the hash so
// later structural kinds (obsidian, sulfur) with the same seed root
// cannot correlate with fish placement. Distinct from the root above
// and from every existing salt.
var fishSubSalt = genprim.ToInt64(0x4f7e2c9a6b3d5e81)

// seedSaltDepositVolcanic is the top-level salt for the volcanic
// structural family (obsidian + sulfur). XOR-ing it into the per-kind
// sub-salt keeps the family stream decorrelated from fish and from
// every non-structural deposit stream even when a future kind's
// sub-salt accidentally collides with another structural salt.
// Routed through regionToInt64 because the top bit is set.
var seedSaltDepositVolcanic = genprim.ToInt64(0xdf9b6c4a2e1f3d5b)

// obsidianSubSalt mixes an additional per-stream salt into the hash so
// obsidian selection does not correlate with sulfur, fish, or any
// other structural or point-like stream. Distinct from every existing
// salt across superchunk, region_source, landmarks, volcanoes, and
// the rest of resources.
var obsidianSubSalt = genprim.ToInt64(0x4c8a5b3f1d9e2c4a)

// sulfurSubSalt mixes an additional per-stream salt into the hash so
// sulfur selection does not correlate with obsidian or any other
// stream. Distinct from every existing salt; the top bit is set so
// the literal routes through regionToInt64 like the others.
var sulfurSubSalt = genprim.ToInt64(0x5d9b6c4a2e1f3d5b)

// fishDepositAt returns the Fish deposit on t when t is a beach tile
// directly adjacent to open ocean and the density hash selects it,
// otherwise (Deposit{}, false). A coast tile is restricted to
// TerrainBeach because Beach is the unambiguous coast biome the
// generator emits. Non-beach tiles that happen to touch ocean (plains
// meeting water without a beach step) are not fish candidates — those
// can still carry zonal Fertile or Game via the zonal path instead.
//
// Selection is deterministic from (seed, tile) via hashPos XOR'd with
// the fish salts — same seed and tile always produce the same result.
func fishDepositAt(seed int64, t geom.Position, terrain TerrainSampler) (world.Deposit, bool) {
	tile := terrain.TileAt(t.X, t.Y)
	return fishDepositAtTerrain(seed, t, tile.Terrain, terrain)
}

// fishDepositAtTerrain is the inner hot-path variant used by generate
// when the caller has already fetched the tile. Accepts the terrain
// value directly so TileAt is not called a second time per tile.
func fishDepositAtTerrain(seed int64, t geom.Position, ter world.Terrain, terrain TerrainSampler) (world.Deposit, bool) {
	if ter != world.TerrainBeach {
		return world.Deposit{}, false
	}
	if !beachFacesOpenOcean(t, terrain) {
		return world.Deposit{}, false
	}
	h := genprim.HashPos(t) ^ uint64(seed^seedSaltDepositFish^fishSubSalt)
	if densityRoll(h) > fishDensityFraction {
		return world.Deposit{}, false
	}
	return world.Deposit{
		Position:      t,
		Kind:          world.DepositFish,
		MaxAmount:     fishMaxAmount,
		CurrentAmount: fishMaxAmount,
	}, true
}

// beachFacesOpenOcean reports whether any of t's 8 neighbours is an
// ocean or deep-ocean tile. Rivers and lakes do not qualify — Fish
// deposits are marine, not freshwater. Lake-side beaches and
// river-mouth beaches thus stay fish-free, which matches the plan's
// "open sea" rule.
func beachFacesOpenOcean(t geom.Position, terrain TerrainSampler) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			n := terrain.TileAt(t.X+dx, t.Y+dy)
			if n.Terrain == world.TerrainOcean || n.Terrain == world.TerrainDeepOcean {
				return true
			}
		}
	}
	return false
}

// obsidianDepositAt returns an Obsidian deposit when t sits on a
// volcano slope tile and the density hash selects it, otherwise
// (Deposit{}, false). Placement is state-independent by design — an
// Extinct volcano's slope still carries obsidian from historical lava
// flows; only the core and ashland zones differ by lifecycle state.
//
// Returns (Deposit{}, false) when vs is nil (no volcano source wired,
// e.g. a test source that exercises only zonal or fish paths). The
// lookup scans the 3x3 super-chunk neighbourhood around t's home SC
// because a volcano's slope can spill across super-chunk boundaries
// when the anchor sits near an edge.
//
// Selection is deterministic from (seed, tile) via hashPos XOR-ed with
// the volcanic salts — same seed and tile always produce the same
// result. The top 24 bits of the hash feed a uniform [0, 1) density
// gate against obsidianDensityFraction.
func obsidianDepositAt(seed int64, t geom.Position, vs world.VolcanoSource) (world.Deposit, bool) {
	if vs == nil {
		return world.Deposit{}, false
	}
	home := geom.WorldToSuperChunk(t.X, t.Y)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			for _, v := range vs.VolcanoAt(sc) {
				if v.ZoneAt(t) != world.VolcanoZoneSlope {
					continue
				}
				h := genprim.HashPos(t) ^ uint64(seed^seedSaltDepositVolcanic^obsidianSubSalt)
				if densityRoll(h) > obsidianDensityFraction {
					return world.Deposit{}, false
				}
				return world.Deposit{
					Position:      t,
					Kind:          world.DepositObsidian,
					MaxAmount:     obsidianMaxAmount,
					CurrentAmount: obsidianMaxAmount,
				}, true
			}
		}
	}
	return world.Deposit{}, false
}

// sulfurDepositAt returns a Sulfur deposit when t sits on a slope tile
// 4-adjacent to a core tile of the same volcano, with per-state
// density filtering:
//
//	VolcanoActive   -> every core-adjacent slope tile carries sulfur
//	VolcanoDormant  -> sulfurDormantFraction (50%) of candidates pass
//	VolcanoExtinct  -> never (core is a crater lake, sulfur weathered away)
//
// Returns (Deposit{}, false) when vs is nil, when t is not on a slope,
// when t does not neighbour a core tile of the containing volcano, or
// when the density gate rejects. The 3x3 super-chunk scan mirrors
// obsidianDepositAt so cross-SC volcano footprints resolve correctly.
//
// Selection is deterministic from (seed, tile) — the density hash uses
// the sulfurSubSalt, disjoint from obsidianSubSalt, so a tile eligible
// for both kinds rolls independent densities.
func sulfurDepositAt(seed int64, t geom.Position, vs world.VolcanoSource) (world.Deposit, bool) {
	if vs == nil {
		return world.Deposit{}, false
	}
	home := geom.WorldToSuperChunk(t.X, t.Y)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
			for _, v := range vs.VolcanoAt(sc) {
				if v.ZoneAt(t) != world.VolcanoZoneSlope {
					continue
				}
				if !slopeAdjacentToCore(t, v) {
					continue
				}
				switch v.State {
				case world.VolcanoExtinct:
					return world.Deposit{}, false
				case world.VolcanoDormant:
					h := genprim.HashPos(t) ^ uint64(seed^seedSaltDepositVolcanic^sulfurSubSalt)
					if densityRoll(h) > sulfurDormantFraction {
						return world.Deposit{}, false
					}
				case world.VolcanoActive:
					// unconditional — every core-adjacent slope tile
					// around an active volcano carries sulfur.
				default:
					// Unknown / zero state: reject rather than emit a
					// deposit attached to a malformed volcano record.
					return world.Deposit{}, false
				}
				return world.Deposit{
					Position:      t,
					Kind:          world.DepositSulfur,
					MaxAmount:     sulfurMaxAmount,
					CurrentAmount: sulfurMaxAmount,
				}, true
			}
		}
	}
	return world.Deposit{}, false
}

// slopeAdjacentToCore reports whether t has at least one 4-neighbour
// in v.CoreTiles. Used by sulfur placement to restrict deposits to the
// inner rim of the slope ring rather than the whole slope. Uses a
// linear scan rather than a set build: footprints are small (a few
// dozen tiles) and every call previously allocated a fresh map — at
// ~20 volcanoes × dozens of slope tiles per super-region the alloc
// churn dominated the actual work. Mirrors the ZoneAt idiom on
// world.Volcano next door.
func slopeAdjacentToCore(t geom.Position, v world.Volcano) bool {
	if len(v.CoreTiles) == 0 {
		return false
	}
	for _, c := range v.CoreTiles {
		dx := c.X - t.X
		dy := c.Y - t.Y
		if (dx == 0 && (dy == 1 || dy == -1)) || (dy == 0 && (dx == 1 || dx == -1)) {
			return true
		}
	}
	return false
}

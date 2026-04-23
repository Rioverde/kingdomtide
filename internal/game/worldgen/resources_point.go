package worldgen

import (
	"math/rand/v2"
	"sort"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// pointKinds enumerates every deposit kind placed via Poisson-disk.
// Declaration order governs iteration order inside pointDepositsInRegion;
// iterating common-first (Iron, Stone) and rare-last (Gold, Silver, Gems)
// keeps the output slice easy to reason about when tracing a specific
// seed's output by eye.
var pointKinds = []world.DepositKind{
	world.DepositIron,
	world.DepositStone,
	world.DepositSalt,
	world.DepositGold,
	world.DepositSilver,
	world.DepositGems,
}

// pointSubSalts carries the per-kind 64-bit salt XOR-ed into the world
// seed and hashSR to decorrelate each kind's Poisson-disk stream from
// every other worldgen stream. Values are distinct 64-bit patterns,
// distinct from every other salt already in use (superchunk,
// region_source, landmarks, volcanoes_placement, resources_zonal,
// resources_structural). Routed through regionToInt64 because the top
// bit is set — Go rejects those as untyped signed literals.
var pointSubSalts = map[world.DepositKind]int64{
	world.DepositIron:   regionToInt64(0x4e2a9b3f7d15c80a),
	world.DepositStone:  regionToInt64(0x5f3b8c4e2a1d9b0c),
	world.DepositSalt:   regionToInt64(0x6a4c9d5f3b2e1a0d),
	world.DepositGold:   regionToInt64(0x7b5d2a6c4f3e1b0e),
	world.DepositSilver: regionToInt64(0x8c6e3b7d5a4f2c0f),
	world.DepositGems:   regionToInt64(0x9d7f4c8e6b5a3d10),
}

// seedSaltDepositPoisson is the top-level salt for the point-like family.
// XORing it into the per-kind salt keeps the family stream decorrelated
// from zonal and structural streams even when a future kind's sub-salt
// collides accidentally with a non-point salt.
var seedSaltDepositPoisson = regionToInt64(0xaf6c3e9d1b5a7f28)

// pointMinDistance sets each kind's Poisson-disk minimum spacing in
// tiles. Common kinds are dense (Stone 40), rare kinds sparse
// (Gems 600). At the 256-tile super-region side, these produce roughly
// 40 Stone candidates but often 0 Gems per SR — matching the plan's
// "rare-and-sometimes-absent" semantics.
var pointMinDistance = map[world.DepositKind]int{
	world.DepositStone:  40,
	world.DepositIron:   80,
	world.DepositSalt:   100,
	world.DepositSilver: 200,
	world.DepositGold:   400,
	world.DepositGems:   600,
}

// pointPoissonK is Bridson's candidate count per attempt. 30 is the
// canonical value and aligns with the volcano placement tuning.
const pointPoissonK = 30

// pointMaxAmount sets each kind's yield ceiling at generation time.
// CurrentAmount equals MaxAmount on every placed deposit in the static-
// placement phase; later depletion work reads these ceilings as the
// respawn target.
var pointMaxAmount = map[world.DepositKind]int32{
	world.DepositStone:  1000,
	world.DepositIron:   600,
	world.DepositSalt:   400,
	world.DepositSilver: 100,
	world.DepositGold:   50,
	world.DepositGems:   30,
}

// pointBiomeAccepts reports whether kind can spawn on ter. Mountain-tier
// kinds (Iron, Stone, Silver, Gems) accept Mountain, SnowyPeak, and
// Hills — all three are mountain-tier terrain in the Whittaker grid.
// Gold narrows to Mountain only because gold is historically mined on
// accessible slopes, not glaciated crests. Salt covers Desert and Beach
// (marsh has no project equivalent — see CLAUDE.md adaptation note).
func pointBiomeAccepts(kind world.DepositKind, ter world.Terrain) bool {
	switch kind {
	case world.DepositIron, world.DepositStone, world.DepositSilver, world.DepositGems:
		switch ter {
		case world.TerrainMountain, world.TerrainSnowyPeak, world.TerrainHills:
			return true
		}
	case world.DepositGold:
		if ter == world.TerrainMountain {
			return true
		}
	case world.DepositSalt:
		switch ter {
		case world.TerrainDesert, world.TerrainBeach:
			return true
		}
	}
	return false
}

// tileBlocked reports whether p is forbidden for any point-like deposit
// regardless of kind. Rejection sources:
//
//   - Water / river / lake overlays (inherited from isWaterOrRiverTile).
//   - Landmark tiles in the 3x3 SC neighbourhood — landmarks are one-tile
//     features; collocating a mine would erase the landmark's visual.
//   - Volcano footprint tiles (core/slope/ashland). Volcanic resources
//     are placed structurally in M4, not by Poisson-disk; keep the two
//     strategies non-overlapping.
//
// vs may be nil in tests — volcano rejection is then skipped silently.
// lm may be nil for the same reason. wg must be non-nil.
//
// The volcano check uses TerrainOverrideAt (O(1) per-SR tileIndex hit)
// rather than iterating the 3x3 SC neighbourhood's volcano list and
// scanning each volcano's CoreTiles/SlopeTiles/AshlandTiles — the source
// already maintains that index internally and exposes the cheap path.
func tileBlocked(p geom.Position, wg *WorldGenerator, lm world.LandmarkSource, vs world.VolcanoSource) bool {
	if isWaterOrRiverTile(wg.TileAt(p.X, p.Y)) {
		return true
	}
	if lm != nil {
		home := geom.WorldToSuperChunk(p.X, p.Y)
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				sc := geom.SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
				for _, l := range lm.LandmarksIn(sc) {
					if l.Coord.Equal(p) {
						return true
					}
				}
			}
		}
	}
	if vs != nil {
		if _, inFootprint := vs.TerrainOverrideAt(p); inFootprint {
			return true
		}
	}
	return false
}

// pointDepositsInRegion returns every point-like deposit inside the
// super-region sr. Each kind runs its own Bridson pass seeded from
// (seed, hashSR(sr), pointSubSalts[kind]); candidates are filtered
// through pointBiomeAccepts and tileBlocked. The output slice is sorted
// grouped by kind (Iron first, Gems last) with (X, Y) lex order within
// each kind so iteration order is stable across calls and across
// independent sources with the same seed.
func pointDepositsInRegion(
	seed int64,
	sr superRegion,
	wg *WorldGenerator,
	lm world.LandmarkSource,
	vs world.VolcanoSource,
) []world.Deposit {
	side := volcanoSuperRegionSideTiles
	minX := sr.X * side
	minY := sr.Y * side
	out := make([]world.Deposit, 0, 64)
	for _, kind := range pointKinds {
		lo := uint64(seed ^ seedSaltDepositPoisson ^ pointSubSalts[kind])
		hi := hashSR(sr) ^ uint64(pointSubSalts[kind])
		rng := rand.New(rand.NewPCG(lo, hi))
		candidates := bridsonSample(rng, minX, minY, side, side, pointMinDistance[kind], pointPoissonK)
		for _, p := range candidates {
			if !pointBiomeAccepts(kind, wg.TileAt(p.X, p.Y).Terrain) {
				continue
			}
			if tileBlocked(p, wg, lm, vs) {
				continue
			}
			out = append(out, world.Deposit{
				Position:      p,
				Kind:          kind,
				MaxAmount:     pointMaxAmount[kind],
				CurrentAmount: pointMaxAmount[kind],
			})
		}
	}
	sortPointDeposits(out)
	return out
}

// sortPointDeposits orders point-like deposits by kind ordinal ascending
// then (X, Y) lex order. Kind ordinal first keeps related kinds grouped
// for debug traces; (X, Y) within a kind is the same tiebreak
// DepositsIn applies inside each super-region, so downstream iteration
// stays stable across calls and across independent sources with the
// same seed.
func sortPointDeposits(ds []world.Deposit) {
	sort.Slice(ds, func(i, j int) bool {
		if ds[i].Kind != ds[j].Kind {
			return ds[i].Kind < ds[j].Kind
		}
		if ds[i].Position.X != ds[j].Position.X {
			return ds[i].Position.X < ds[j].Position.X
		}
		return ds[i].Position.Y < ds[j].Position.Y
	})
}

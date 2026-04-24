package resource

import (
	"cmp"
	"math/rand/v2"
	"slices"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen/internal/genprim"
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
	world.DepositIron:   genprim.ToInt64(0x4e2a9b3f7d15c80a),
	world.DepositStone:  genprim.ToInt64(0x5f3b8c4e2a1d9b0c),
	world.DepositSalt:   genprim.ToInt64(0x6a4c9d5f3b2e1a0d),
	world.DepositGold:   genprim.ToInt64(0x7b5d2a6c4f3e1b0e),
	world.DepositSilver: genprim.ToInt64(0x8c6e3b7d5a4f2c0f),
	world.DepositGems:   genprim.ToInt64(0x9d7f4c8e6b5a3d10),
}

// seedSaltDepositPoisson is the top-level salt for the point-like family.
// XORing it into the per-kind salt keeps the family stream decorrelated
// from zonal and structural streams even when a future kind's sub-salt
// collides accidentally with a non-point salt.
var seedSaltDepositPoisson = genprim.ToInt64(0xaf6c3e9d1b5a7f28)

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
//   - Landmark positions in lmSet (pre-built by the caller for the whole
//     super-region; nil means skip the landmark check).
//   - Volcano footprint tiles (core/slope/ashland). Volcanic resources
//     are placed structurally, not by Poisson-disk; keep the two
//     strategies non-overlapping.
//
// vs may be nil in tests — volcano rejection is then skipped silently.
// terrain must be non-nil.
//
// The volcano check uses TerrainOverrideAt (O(1) per-SR tileIndex hit)
// rather than iterating the 3x3 SC neighbourhood's volcano list and
// scanning each volcano's CoreTiles/SlopeTiles/AshlandTiles — the source
// already maintains that index internally and exposes the cheap path.
func tileBlocked(p geom.Position, terrain TerrainSampler, lmSet map[geom.Position]struct{}, vs world.VolcanoSource) bool {
	if genprim.IsWaterOrRiverTile(terrain.TileAt(p.X, p.Y)) {
		return true
	}
	if _, blocked := lmSet[p]; blocked {
		return true
	}
	if vs != nil {
		if _, inFootprint := vs.TerrainOverrideAt(p); inFootprint {
			return true
		}
	}
	return false
}

// landmarkSet builds a position set from every landmark in the 6x6 SC
// area covering the super-region plus its 1-SC border. Any tile inside
// the super-region has its 3x3 SC landmark neighbourhood fully covered
// by this area, so tileBlocked can do a single O(1) map lookup instead
// of calling LandmarksIn nine times per candidate.
// Returns nil when lm is nil.
func landmarkSet(sr genprim.SuperRegion, lm world.LandmarkSource) map[geom.Position]struct{} {
	if lm == nil {
		return nil
	}
	// The SR spans SCs [minSC, minSC+SuperRegionSideSC). Expanding by 1
	// on each side gives the full neighbourhood any tile inside could need.
	minSC := geom.SuperChunkCoord{
		X: sr.X*genprim.SuperRegionSideSC - 1,
		Y: sr.Y*genprim.SuperRegionSideSC - 1,
	}
	extent := genprim.SuperRegionSideSC + 2 // 6 SCs per axis
	set := make(map[geom.Position]struct{})
	for dy := 0; dy < extent; dy++ {
		for dx := 0; dx < extent; dx++ {
			sc := geom.SuperChunkCoord{X: minSC.X + dx, Y: minSC.Y + dy}
			for _, l := range lm.LandmarksIn(sc) {
				set[l.Coord] = struct{}{}
			}
		}
	}
	return set
}

// pointDepositsInRegion returns every point-like deposit inside the
// super-region sr. Each kind runs its own Bridson pass seeded from
// (seed, genprim.HashSR(sr), pointSubSalts[kind]); candidates are filtered
// through pointBiomeAccepts and tileBlocked. The output slice is sorted
// grouped by kind (Iron first, Gems last) with (X, Y) lex order within
// each kind so iteration order is stable across calls and across
// independent sources with the same seed.
func pointDepositsInRegion(
	seed int64,
	sr genprim.SuperRegion,
	terrain TerrainSampler,
	lm world.LandmarkSource,
	vs world.VolcanoSource,
) []world.Deposit {
	side := genprim.SuperRegionSideTiles
	minX := sr.X * side
	minY := sr.Y * side
	lmSet := landmarkSet(sr, lm)
	out := make([]world.Deposit, 0, 64)
	for _, kind := range pointKinds {
		lo := uint64(seed ^ seedSaltDepositPoisson ^ pointSubSalts[kind])
		hi := genprim.HashSR(sr) ^ uint64(pointSubSalts[kind])
		rng := rand.New(rand.NewPCG(lo, hi))
		candidates := genprim.BridsonSample(rng, minX, minY, side, side, pointMinDistance[kind], pointPoissonK)
		for _, p := range candidates {
			if !pointBiomeAccepts(kind, terrain.TileAt(p.X, p.Y).Terrain) {
				continue
			}
			if tileBlocked(p, terrain, lmSet, vs) {
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
	slices.SortFunc(ds, func(a, b world.Deposit) int {
		if c := cmp.Compare(a.Kind, b.Kind); c != 0 {
			return c
		}
		if c := cmp.Compare(a.Position.X, b.Position.X); c != 0 {
			return c
		}
		return cmp.Compare(a.Position.Y, b.Position.Y)
	})
}

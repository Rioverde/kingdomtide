package worldgen

import (
	"sort"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
	gworld "github.com/Rioverde/kingdomtide/internal/game/world"
)

// depositSeedSalt decorrelates the deposit PRNG stream from every other
// worldgen subsystem. Value: fractional hex of sqrt(43).
const depositSeedSalt int64 = 0x4f8b3c9e5a2d706f

// Per-kind MaxAmount caps. Chosen so common resources (timber, game) are
// plentiful and rare ones (gold, gems) are tightly capped — scarcity makes
// them worth contesting.
const (
	depositMaxIron     int32 = 500
	depositMaxStone    int32 = 400
	depositMaxTimber   int32 = 200
	depositMaxFertile  int32 = 300
	depositMaxFish     int32 = 100
	depositMaxGame     int32 = 150
	depositMaxSalt     int32 = 250
	depositMaxGold     int32 = 50
	depositMaxSilver   int32 = 80
	depositMaxGems     int32 = 30
	depositMaxObsidian int32 = 120
	depositMaxSulfur   int32 = 90
)

// Biome-based candidate tables. Each slice sums to a natural weight; the
// draw normalises at roll time so the exact values are relative ratios.
// An explicit "nothing" entry uses DepositNone as a placeholder — when
// the draw lands on it no deposit is placed for that cell.
var (
	depositWeightsMountain = []weighted[gworld.DepositKind]{
		{gworld.DepositIron, 38},
		{gworld.DepositStone, 28},
		{gworld.DepositSilver, 5},
		{gworld.DepositGold, 4},
		{gworld.DepositGems, 2},
		{gworld.DepositNone, 23},
	}
	depositWeightsHills = []weighted[gworld.DepositKind]{
		{gworld.DepositStone, 30},
		{gworld.DepositIron, 15},
		{gworld.DepositSilver, 3},
		{gworld.DepositGold, 2},
		{gworld.DepositGems, 1},
		{gworld.DepositNone, 49},
	}
	// Jungle is the densest biome — highest timber yield. Forest is
	// thinner; Taiga is coniferous and sparser still.
	depositWeightsJungle = []weighted[gworld.DepositKind]{
		{gworld.DepositTimber, 55},
		{gworld.DepositGame, 18},
		{gworld.DepositNone, 27},
	}
	depositWeightsForest = []weighted[gworld.DepositKind]{
		{gworld.DepositTimber, 38},
		{gworld.DepositGame, 17},
		{gworld.DepositNone, 45},
	}
	depositWeightsTaiga = []weighted[gworld.DepositKind]{
		{gworld.DepositTimber, 22},
		{gworld.DepositGame, 22},
		{gworld.DepositNone, 56},
	}
	depositWeightsSnow = []weighted[gworld.DepositKind]{
		{gworld.DepositGame, 15},
		{gworld.DepositNone, 85},
	}
	depositWeightsPlains = []weighted[gworld.DepositKind]{
		{gworld.DepositGame, 15},
		{gworld.DepositFertile, 10},
		{gworld.DepositNone, 75},
	}
	depositWeightsSavanna = []weighted[gworld.DepositKind]{
		{gworld.DepositGame, 20},
		{gworld.DepositNone, 80},
	}
	depositWeightsDesertTundra = []weighted[gworld.DepositKind]{
		{gworld.DepositSalt, 8},
		{gworld.DepositStone, 5},
		{gworld.DepositNone, 87},
	}
	depositWeightsCoast = []weighted[gworld.DepositKind]{
		{gworld.DepositFish, 28},
		{gworld.DepositSalt, 4},
		{gworld.DepositNone, 68},
	}
	depositWeightsVolcanic = []weighted[gworld.DepositKind]{
		{gworld.DepositObsidian, 20},
		{gworld.DepositSulfur, 15},
		{gworld.DepositNone, 65},
	}
)

// depositMaxAmounts maps a DepositKind to its initial MaxAmount. Fixed-
// size lookup so the per-cell fill is a single index into the array instead
// of a switch.
var depositMaxAmounts = [...]int32{
	gworld.DepositNone:     0,
	gworld.DepositIron:     depositMaxIron,
	gworld.DepositStone:    depositMaxStone,
	gworld.DepositTimber:   depositMaxTimber,
	gworld.DepositFertile:  depositMaxFertile,
	gworld.DepositFish:     depositMaxFish,
	gworld.DepositGame:     depositMaxGame,
	gworld.DepositSalt:     depositMaxSalt,
	gworld.DepositGold:     depositMaxGold,
	gworld.DepositSilver:   depositMaxSilver,
	gworld.DepositGems:     depositMaxGems,
	gworld.DepositObsidian: depositMaxObsidian,
	gworld.DepositSulfur:   depositMaxSulfur,
}

// DepositSource is the production world.DepositSource implementation. It
// places one deposit per Voronoi cell using biome-based weighted draws seeded
// deterministically from (worldSeed, cellID). All caches are built once at
// construction so query methods allocate nothing on the hot path.
type DepositSource struct {
	// byPos indexes every placed deposit by packed (X,Y) for O(1) DepositAt.
	byPos map[uint64]gworld.Deposit

	// sorted holds all deposits in (Y, X) lex order for deterministic
	// DepositsIn and DepositsNear results.
	sorted []gworld.Deposit
}

// DepositSourceConfig holds the upstream sources a DepositSource needs.
// Volcanoes may be nil — deposits degrade to biome-only weights when absent.
type DepositSourceConfig struct {
	Volcanoes gworld.VolcanoSource // for volcanic kind eligibility
}

// NewDepositSource builds a DepositSource over a finished worldgen.Map.
// Construction iterates every Voronoi cell once, O(cells), and never runs again.
//
// cfg.Volcanoes is optional — when nil volcanic deposit placement is disabled.
// When non-nil, cells whose center tile falls inside a volcano zone (core,
// slope, ashland, crater lake) use the volcanic weight table (obsidian +
// sulfur) instead of the cell's underlying biome table.
func NewDepositSource(w *Map, seed int64, cfg DepositSourceConfig) *DepositSource {
	volcanoes := cfg.Volcanoes
	byPos := make(map[uint64]gworld.Deposit, len(w.Voronoi.Cells)/4)
	var all []gworld.Deposit

	for cellID := range w.Voronoi.Cells {
		cell := w.Voronoi.Cells[cellID]
		cx, cy := int(cell.CenterX), int(cell.CenterY)
		if cx < 0 || cy < 0 || cx >= w.Width || cy >= w.Height {
			continue
		}
		if w.IsOcean(uint32(cellID)) {
			continue
		}

		terrain := w.Terrain[cellID]
		center := geom.Position{X: cx, Y: cy}
		weights := depositWeightsForCell(terrain, w, uint32(cellID), center, volcanoes)
		if len(weights) == 0 {
			continue
		}

		// Deterministic per-cell PRNG: mix seed, cell ID, and salt.
		state := uint64(seed) ^
			uint64(depositSeedSalt) ^
			(uint64(cellID) * geom.SeedSaltX)
		rng := newPCG(state)

		kind := pickWeighted(rng, weights, gworld.DepositNone)
		if kind == gworld.DepositNone {
			continue
		}

		pos := geom.Position{X: cx, Y: cy}
		maxAmt := depositMaxAmounts[kind]
		d := gworld.Deposit{
			Position:      pos,
			Kind:          kind,
			MaxAmount:     maxAmt,
			CurrentAmount: maxAmt,
			LastRespawn:   0,
		}
		key := geom.PackPos(pos)
		byPos[key] = d
		all = append(all, d)
	}

	// Stable (Y, X) lex order so DepositsIn / DepositsNear are deterministic
	// across calls with identical inputs.
	sort.Slice(all, func(i, j int) bool {
		a, b := all[i].Position, all[j].Position
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})

	return &DepositSource{
		byPos:  byPos,
		sorted: all,
	}
}

// depositWeightsForCell returns the appropriate candidate table for a cell.
// It checks first whether the cell's center tile falls inside a volcano zone
// via the VolcanoSource terrain override — volcanic terrain is a tile-level
// override and cells inside a volcano keep their original biome (Mountain/Hills).
// If a volcanic override is detected the volcanic weight table is returned
// directly. Otherwise the function falls through to the biome-based switch.
// Coastal land cells (adjacent to ocean) are eligible for fish. All
// ocean/deep-ocean cells are pre-filtered by the caller.
func depositWeightsForCell(
	t gworld.Terrain,
	w *Map,
	cellID uint32,
	center geom.Position,
	volcanoes gworld.VolcanoSource,
) []weighted[gworld.DepositKind] {
	if volcanoes != nil {
		if override, ok := volcanoes.TerrainOverrideAt(center); ok {
			switch override {
			case gworld.TerrainVolcanoCore,
				gworld.TerrainVolcanoCoreDormant,
				gworld.TerrainCraterLake,
				gworld.TerrainVolcanoSlope,
				gworld.TerrainAshland:
				return depositWeightsVolcanic
			}
		}
	}
	return depositWeightsByBiome(t, w, cellID)
}

// depositWeightsByBiome returns the biome-appropriate candidate table for a
// cell using the cell-level terrain value. Volcanic terrain entries are kept
// here as a fallback for cases where the VolcanoSource is nil but the cell
// terrain was somehow classified as volcanic at the Voronoi level.
func depositWeightsByBiome(t gworld.Terrain, w *Map, cellID uint32) []weighted[gworld.DepositKind] {
	switch t {
	case gworld.TerrainMountain, gworld.TerrainSnowyPeak:
		return depositWeightsMountain
	case gworld.TerrainHills:
		return depositWeightsHills
	case gworld.TerrainJungle:
		return depositWeightsJungle
	case gworld.TerrainForest:
		return depositWeightsForest
	case gworld.TerrainTaiga:
		return depositWeightsTaiga
	case gworld.TerrainSnow:
		return depositWeightsSnow
	case gworld.TerrainPlains, gworld.TerrainGrassland, gworld.TerrainMeadow:
		return depositWeightsPlains
	case gworld.TerrainSavanna:
		return depositWeightsSavanna
	case gworld.TerrainDesert, gworld.TerrainTundra:
		return depositWeightsDesertTundra
	case gworld.TerrainBeach:
		// Beach cells are coastal by definition.
		return depositWeightsCoast
	case gworld.TerrainVolcanoCore,
		gworld.TerrainVolcanoCoreDormant,
		gworld.TerrainVolcanoSlope,
		gworld.TerrainAshland,
		gworld.TerrainCraterLake:
		return depositWeightsVolcanic
	}
	// Non-ocean cells that are coast-adjacent get fish eligibility.
	if w.IsCoast(cellID) {
		return depositWeightsCoast
	}
	return nil
}


// DepositAt returns the deposit on the exact tile p, or (Deposit{}, false)
// when none exists.
func (s *DepositSource) DepositAt(p geom.Position) (gworld.Deposit, bool) {
	d, ok := s.byPos[geom.PackPos(p)]
	return d, ok
}

// DepositsIn returns every deposit whose Position lies inside rect, in (Y, X)
// lex order.
func (s *DepositSource) DepositsIn(rect geom.Rect) []gworld.Deposit {
	if rect.Empty() {
		return nil
	}
	var out []gworld.Deposit
	for _, d := range s.sorted {
		if rect.Contains(d.Position) {
			out = append(out, d)
		}
	}
	return out
}

// DepositsNear returns every deposit within Chebyshev radius of p, sorted by
// Chebyshev distance ascending; ties break by (Y, X) lex order.
func (s *DepositSource) DepositsNear(p geom.Position, radius int) []gworld.Deposit {
	var out []gworld.Deposit
	for _, d := range s.sorted {
		if geom.ChebyshevDist(d.Position, p) <= radius {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		di := geom.ChebyshevDist(out[i].Position, p)
		dj := geom.ChebyshevDist(out[j].Position, p)
		if di != dj {
			return di < dj
		}
		a, b := out[i].Position, out[j].Position
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return out
}

var _ gworld.DepositSource = (*DepositSource)(nil)

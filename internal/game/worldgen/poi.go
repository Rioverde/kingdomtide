package worldgen

import (
	"sort"

	"github.com/Rioverde/gongeons/internal/game"
)

// POI placement constants. poiCandidatesPerChunk is the number of candidate slots
// evaluated inside each chunk; the generator hashes (cx, cy, slot, seed) to pick a
// deterministic position and kind for every slot. poiMinDistance is the minimum
// axial distance between any two accepted POIs — the greedy filter enforces this
// across the 3x3 neighbourhood so POIs never cluster across chunk borders.
const (
	poiCandidatesPerChunk = 4
	poiMinDistance        = 6
	poiVillageSpawnChance = 0.35
	poiCastleSpawnChance  = 0.18
)

// poiCandidate holds a tentative POI before the min-distance filter is applied.
type poiCandidate struct {
	q, r    int
	kind    game.ObjectKind
	sortKey uint64 // raw hash used for stable ordering in the greedy filter
}

// poiHash mixes (cx, cy, slot, seed) into a single unsigned 64-bit value using the same
// mixing constants as riverSourceHash. The extra "poi" salt (0xdeadbeefcafebabe) ensures
// the hash space is decorrelated from river source hashes even when the inputs coincide.
func poiHash(cx, cy, slot int, seed int64) uint64 {
	const salt uint64 = 0xdeadbeefcafebabe
	a := uint64(cx)*0x9e3779b97f4a7c15 ^
		uint64(cy)*0x6c62272e07bb0142 ^
		uint64(slot)*0xbf58476d1ce4e5b9 ^
		uint64(seed)*0x94d049bb133111eb ^
		salt
	a ^= a >> 30
	a *= 0xbf58476d1ce4e5b9
	a ^= a >> 27
	a *= 0x94d049bb133111eb
	a ^= a >> 31
	return a
}

// hexDistance returns the axial distance between two hex tiles. The standard formula for
// pointy-top axial coordinates is (abs(dq) + abs(dr) + abs(dq+dr)) / 2.
func hexDistance(aq, ar, bq, br int) int {
	dq := aq - bq
	dr := ar - br
	d := abs(dq) + abs(dr) + abs(dq+dr)
	return d / 2
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// villageAllowed reports whether a village may be placed on a tile with the given terrain.
// Villages prefer cultivated / coastal / temperate biomes and are rejected on water,
// extreme elevations, and hostile biomes.
func villageAllowed(t game.Terrain) bool {
	switch t {
	case game.TerrainPlains, game.TerrainGrassland, game.TerrainMeadow, game.TerrainBeach, game.TerrainSavanna:
		return true
	default:
		return false
	}
}

// castleAllowed reports whether a castle may be placed on a tile with the given terrain.
// Castles suit defensible high ground and open land; they are rejected on water,
// deep jungle, swamp-like biomes, and snow-heavy terrain.
func castleAllowed(t game.Terrain) bool {
	switch t {
	case game.TerrainHills, game.TerrainPlains, game.TerrainGrassland, game.TerrainMeadow:
		return true
	default:
		return false
	}
}

// ObjectsInChunk returns the deterministic POI map for chunk cc. The map key is the
// chunk-local [dq, dr] offset (not world coordinates) so callers can write directly
// into chunk.Tiles[dr][dq].Object without an extra coordinate conversion.
//
// Algorithm: scan the 3x3 neighbourhood of cc (cc itself plus its eight neighbours) and
// evaluate poiCandidatesPerChunk candidate slots per chunk. Each slot is hashed to a
// kind and a position. Biome and river checks reject unsuitable tiles. The surviving
// candidates are sorted by their raw hash and filtered greedily: a candidate is kept
// only if it is at least poiMinDistance hex tiles away from every previously accepted
// POI. Only placements whose world coordinates fall inside cc.Bounds() are returned.
func (g *WorldGenerator) ObjectsInChunk(cc ChunkCoord) map[[2]int]game.ObjectKind {
	// poiThresholdResolution is the fixed-point denominator for spawn-chance comparisons.
	// Using the top 16 bits of the hash (range [0, 65536)) avoids the float64 precision
	// loss that arises when multiplying spawn chances by ^uint64(0). The resolution of
	// 65536 steps is more than sufficient for the ~18% castle and ~35% village probabilities.
	// res is a typed float64 variable (not a constant) so the compiler treats the product
	// as a runtime expression and allows truncation to uint64.
	res := float64(1 << 16) // 65536, matches the 16-bit topBits range
	castleThreshold := uint64(res * poiCastleSpawnChance)
	villageThreshold := uint64(res * (poiCastleSpawnChance + poiVillageSpawnChance))

	// Gather all candidates from the 3x3 neighbourhood.
	candidates := make([]poiCandidate, 0, 9*poiCandidatesPerChunk)

	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			ncx := cc.X + dx
			ncy := cc.Y + dy
			ncc := ChunkCoord{X: ncx, Y: ncy}
			minQ, _, minR, _ := ncc.Bounds()

			for slot := range poiCandidatesPerChunk {
				h := poiHash(ncx, ncy, slot, g.seed)

				// Determine kind from the top 16 bits of the hash so the comparison
				// aligns with the fixed-point thresholds computed above.
				topBits := h >> 48
				var kind game.ObjectKind
				switch {
				case topBits < castleThreshold:
					kind = game.ObjectCastle
				case topBits < villageThreshold:
					kind = game.ObjectVillage
				default:
					continue // no POI for this slot
				}

				// Determine position within the chunk from a second derived hash so
				// position and kind are decorrelated. Unsigned modulo before int
				// conversion avoids the signed-overflow corner cases that arise when
				// the bit-split value is cast to int before reducing.
				ph := poiHash(ncx, ncy, slot+1000, g.seed)
				dq := int(ph % ChunkSize)
				dr := int((ph / ChunkSize) % ChunkSize)
				wq := minQ + dq
				wr := minR + dr

				// Fetch the tile to check biome and river suitability.
				tile := g.TileAt(wq, wr)
				if tile.River {
					continue
				}
				switch kind {
				case game.ObjectVillage:
					if !villageAllowed(tile.Terrain) {
						continue
					}
				case game.ObjectCastle:
					if !castleAllowed(tile.Terrain) {
						continue
					}
				}

				candidates = append(candidates, poiCandidate{
					q:       wq,
					r:       wr,
					kind:    kind,
					sortKey: h,
				})
			}
		}
	}

	// Stable sort by hash so the greedy filter is deterministic regardless of
	// iteration order over the neighbourhood.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].sortKey < candidates[j].sortKey
	})

	// Greedy min-distance filter: keep a candidate only if it is far enough from
	// every previously accepted POI.
	accepted := make([]poiCandidate, 0, len(candidates))
	for _, c := range candidates {
		tooClose := false
		for _, a := range accepted {
			if hexDistance(c.q, c.r, a.q, a.r) < poiMinDistance {
				tooClose = true
				break
			}
		}
		if !tooClose {
			accepted = append(accepted, c)
		}
	}

	// Collect placements that fall inside cc's own bounds.
	minQ, maxQ, minR, maxR := cc.Bounds()
	result := make(map[[2]int]game.ObjectKind)
	for _, a := range accepted {
		if a.q >= minQ && a.q < maxQ && a.r >= minR && a.r < maxR {
			dq := a.q - minQ
			dr := a.r - minR
			result[[2]int{dq, dr}] = a.kind
		}
	}
	return result
}

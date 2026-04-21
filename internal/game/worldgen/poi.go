package worldgen

import (
	"sort"

	"github.com/Rioverde/gongeons/internal/game"
)

// POI placement constants. poiCandidatesPerChunk is the number of candidate slots
// evaluated inside each chunk; the generator hashes (cx, cy, slot, seed) to pick a
// deterministic position and kind for every slot. poiMinDistance is the minimum
// grid distance between any two accepted POIs — the greedy filter enforces this
// across the 3x3 neighbourhood so POIs never cluster across chunk borders.
const (
	poiCandidatesPerChunk = 4
	poiMinDistance        = 6
	poiVillageSpawnChance = 0.35
	poiCastleSpawnChance  = 0.18
)

// poiThresholdResolution is the fixed-point denominator for spawn-chance comparisons.
// Using the top 16 bits of the hash (range [0, 65536)) avoids the float64 precision loss
// that arises when multiplying spawn chances by ^uint64(0). The 65536-step resolution is
// more than sufficient for the ~18% castle and ~35% village probabilities.
const poiThresholdResolution = 1 << 16

// poiCandidate holds a tentative POI before the min-distance filter is applied.
type poiCandidate struct {
	x, y    int
	kind    game.StructureKind
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

// hexDistance returns the axial distance between two hex tiles. The formula is a
// holdover from an earlier pointy-top hex prototype; on the current square grid it
// still produces a reasonable "how far apart do these POIs feel" metric, which is
// all the min-distance filter needs.
func hexDistance(ax, ay, bx, by int) int {
	dx := ax - bx
	dy := ay - by
	d := abs(dx) + abs(dy) + abs(dx+dy)
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

// StructuresInChunk returns the deterministic POI map for chunk cc. The map key is the
// chunk-local [dx, dy] offset (not world coordinates) so callers can write directly
// into chunk.Tiles[dy][dx].Structure without an extra coordinate conversion.
//
// Algorithm: scan the 3x3 neighbourhood of cc (cc itself plus its eight neighbours) and
// evaluate poiCandidatesPerChunk candidate slots per chunk. Each slot is hashed to a
// kind and a position. Biome and river checks reject unsuitable tiles. The surviving
// candidates are sorted by their raw hash and filtered greedily: a candidate is kept
// only if it is at least poiMinDistance tiles away from every previously accepted POI.
// Only placements whose world coordinates fall inside cc.Bounds() are returned.
func (g *WorldGenerator) StructuresInChunk(cc ChunkCoord) map[[2]int]game.StructureKind {
	cands := g.poiCandidatesIn(cc)
	kept := filterPOIByDistance(cands)
	return assignPOIKinds(cc, kept)
}

// poiCandidatesIn collects raw POI candidates from the 3x3 chunk neighbourhood around cc.
// For each of the nine chunks it evaluates poiCandidatesPerChunk hashed slots, derives a
// kind from the top 16 bits of the hash, and hashes again to pick a local position. The
// resulting world tile is checked against biome rules; surviving slots become candidates.
// River tiles are intentionally allowed — castles-guarding-rivers and riverside villages
// are thematically desirable, and the renderer's structure > river > terrain precedence
// makes the composition display correctly. Candidates from neighbouring chunks are
// included so the downstream min-distance filter can prevent POIs from clumping across
// chunk borders.
func (g *WorldGenerator) poiCandidatesIn(cc ChunkCoord) []poiCandidate {
	// res is a typed float64 variable (not a constant) so the compiler treats the product
	// as a runtime expression and allows truncation to uint64.
	res := float64(poiThresholdResolution)
	castleThreshold := uint64(res * poiCastleSpawnChance)
	villageThreshold := uint64(res * (poiCastleSpawnChance + poiVillageSpawnChance))

	candidates := make([]poiCandidate, 0, 9*poiCandidatesPerChunk)

	for dcy := -1; dcy <= 1; dcy++ {
		for dcx := -1; dcx <= 1; dcx++ {
			ncx := cc.X + dcx
			ncy := cc.Y + dcy
			ncc := ChunkCoord{X: ncx, Y: ncy}
			minX, _, minY, _ := ncc.Bounds()

			for slot := range poiCandidatesPerChunk {
				h := poiHash(ncx, ncy, slot, g.seed)

				// Determine kind from the top 16 bits of the hash so the comparison
				// aligns with the fixed-point thresholds computed above.
				topBits := h >> 48
				var kind game.StructureKind
				switch {
				case topBits < castleThreshold:
					kind = game.StructureCastle
				case topBits < villageThreshold:
					kind = game.StructureVillage
				default:
					continue // no POI for this slot
				}

				// Determine position within the chunk from a second derived hash so
				// position and kind are decorrelated. Unsigned modulo before int
				// conversion avoids the signed-overflow corner cases that arise when
				// the bit-split value is cast to int before reducing.
				ph := poiHash(ncx, ncy, slot+1000, g.seed)
				dx := int(ph % ChunkSize)
				dy := int((ph / ChunkSize) % ChunkSize)
				wx := minX + dx
				wy := minY + dy

				tile := g.TileAt(wx, wy)
				switch kind {
				case game.StructureVillage:
					if !villageAllowed(tile.Terrain) {
						continue
					}
				case game.StructureCastle:
					if !castleAllowed(tile.Terrain) {
						continue
					}
				}

				candidates = append(candidates, poiCandidate{
					x:       wx,
					y:       wy,
					kind:    kind,
					sortKey: h,
				})
			}
		}
	}

	return candidates
}

// filterPOIByDistance runs the greedy min-distance filter. Candidates are first
// sorted by their raw hash so the pass is deterministic regardless of iteration
// order over the 3x3 neighbourhood, then a candidate is kept only if it is at
// least poiMinDistance away from every already-accepted POI.
func filterPOIByDistance(cands []poiCandidate) []poiCandidate {
	sort.Slice(cands, func(i, j int) bool {
		return cands[i].sortKey < cands[j].sortKey
	})

	accepted := make([]poiCandidate, 0, len(cands))
	for _, c := range cands {
		tooClose := false
		for _, a := range accepted {
			if hexDistance(c.x, c.y, a.x, a.y) < poiMinDistance {
				tooClose = true
				break
			}
		}
		if !tooClose {
			accepted = append(accepted, c)
		}
	}
	return accepted
}

// assignPOIKinds collects the accepted candidates that fall inside cc.Bounds() and
// converts each surviving world coordinate to a chunk-local (dx, dy) offset. The
// returned map key is the local offset so callers can write directly into
// chunk.Tiles[dy][dx].Structure without an extra coordinate conversion.
func assignPOIKinds(cc ChunkCoord, accepted []poiCandidate) map[[2]int]game.StructureKind {
	minX, maxX, minY, maxY := cc.Bounds()
	result := make(map[[2]int]game.StructureKind)
	for _, a := range accepted {
		if a.x >= minX && a.x < maxX && a.y >= minY && a.y < maxY {
			dx := a.x - minX
			dy := a.y - minY
			result[[2]int{dx, dy}] = a.kind
		}
	}
	return result
}

package worldgen

// riverAccumThreshold is the minimum D8 upstream-area count a cell must have to
// be classified as a river. Sparsity gate: a catchment below this size is a wet
// gully, not a river, and the map stays cleaner without salt-and-pepper
// streams.
//
// Calibration: TestRiverDensityRealistic (rivers_test.go) sweeps seeds over a
// 16×16 chunk window and requires river density between 0.3% and 4% of land
// tiles. With the mountain-source gate active the density under a given
// threshold is lower than the old "pure-accum" model, so the threshold is
// tuned to land inside that band.
const riverAccumThreshold int32 = 40

// riverMoistureThreshold gates a mountain cell from becoming a river source
// when the local climate is arid. Deserts do not birth rivers — this keeps
// headwaters off bare rock peaks in the middle of a hot/dry band. Applied
// only at the mountain source cell; downstream moisture does not matter
// because the water carries over from the upstream catchment.
const riverMoistureThreshold = 0.50

// RiverTilesInChunk returns the set of world-space grid coordinates inside cc
// that are river tiles. Deterministic per (seed, cc).
//
// A tile is a river iff its D8 upstream catchment (bounded by the hydrology
// buffer) contains at least one mountain-and-moist cell AND at least
// riverAccumThreshold cells flow through it AND it is neither a lake nor an
// ocean cell. The implementation is a pure scan over the precomputed river
// mask on the chunk's hydrology field — all heavy lifting happened when the
// field was built.
func (g *WorldGenerator) RiverTilesInChunk(cc ChunkCoord) map[[2]int]struct{} {
	f := g.hydrologyFor(cc)
	result := make(map[[2]int]struct{})
	minX, maxX, minY, maxY := cc.Bounds()
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			if f.isRiverAt(x, y) {
				result[[2]int{x, y}] = struct{}{}
			}
		}
	}
	return result
}

// LakeTilesInChunk returns the set of world-space grid coordinates inside cc
// that are lake tiles (raised by Priority-Flood depression filling).
// Deterministic per (seed, cc).
func (g *WorldGenerator) LakeTilesInChunk(cc ChunkCoord) map[[2]int]struct{} {
	f := g.hydrologyFor(cc)
	result := make(map[[2]int]struct{})
	minX, maxX, minY, maxY := cc.Bounds()
	for y := minY; y < maxY; y++ {
		for x := minX; x < maxX; x++ {
			if f.isLakeAt(x, y) {
				result[[2]int{x, y}] = struct{}{}
			}
		}
	}
	return result
}

package worldgen

import "github.com/Rioverde/kingdomtide/internal/game/geom"

// testSeed is the canonical seed shared across all worldgen test files.
// Pinned to 42 so placement, distribution, and snapshot assertions stay
// reproducible across CI runs without per-file constants that can drift.
const testSeed int64 = 42

// iterSuperChunks walks every super-chunk grid cell that intersects w and
// calls fn for each one in row-major order. Used by tests that need to
// sweep the full super-chunk grid without duplicating the iteration logic.
func iterSuperChunks(w *Map, fn func(sc geom.SuperChunkCoord)) {
	maxX := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	maxY := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < maxY; sy++ {
		for sx := 0; sx < maxX; sx++ {
			fn(geom.SuperChunkCoord{X: sx, Y: sy})
		}
	}
}

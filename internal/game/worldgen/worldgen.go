package worldgen

import "github.com/Rioverde/gongeons/internal/game/world"

// NewWorld is the convenience wrapper used by the server boot path. It
// builds an infinite, procedural *world.World seeded from the given
// value — equivalent to world.NewWorld(NewChunkedSource(seed)).
func NewWorld(seed int64) *world.World {
	return world.NewWorld(NewChunkedSource(seed))
}

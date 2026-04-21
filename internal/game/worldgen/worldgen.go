package worldgen

import "github.com/Rioverde/gongeons/internal/game"

// NewWorld is the convenience wrapper used by the server boot path. It
// builds an infinite, procedural *game.World seeded from the given
// value — equivalent to game.NewWorld(NewChunkedSource(seed)).
func NewWorld(seed int64) *game.World {
	return game.NewWorld(NewChunkedSource(seed))
}

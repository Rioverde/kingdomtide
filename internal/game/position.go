package game

// Position is a square-grid coordinate. Origin is the top-left of the world;
// X grows to the right, Y grows downward. Value semantics: all methods return
// new Positions and never mutate the receiver.
type Position struct {
	X, Y int
}

// Add returns the Position offset by (dx, dy). The receiver is not modified.
func (p Position) Add(dx, dy int) Position {
	return Position{X: p.X + dx, Y: p.Y + dy}
}

// Equal reports whether p and other refer to the same coordinate.
func (p Position) Equal(other Position) bool {
	return p.X == other.X && p.Y == other.Y
}

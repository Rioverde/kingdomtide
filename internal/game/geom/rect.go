package game

// Rect is an axis-aligned tile rectangle on the square grid. MinX/MinY
// are inclusive, MaxX/MaxY are exclusive — the same half-open convention
// used for world-space bounds throughout the project. A zero-value Rect
// is empty and Contains returns false for every Position.
type Rect struct {
	MinX, MinY, MaxX, MaxY int
}

// Contains reports whether p lies inside r under the half-open
// convention (MinX/MinY inclusive, MaxX/MaxY exclusive). A negative-width
// or negative-height rect contains nothing.
func (r Rect) Contains(p Position) bool {
	return p.X >= r.MinX && p.X < r.MaxX &&
		p.Y >= r.MinY && p.Y < r.MaxY
}

// Empty reports whether r covers zero tiles. Used by callers that want
// to short-circuit on degenerate inputs before iterating.
func (r Rect) Empty() bool {
	return r.MaxX <= r.MinX || r.MaxY <= r.MinY
}

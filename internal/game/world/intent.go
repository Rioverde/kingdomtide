package world

import "github.com/Rioverde/gongeons/internal/game/stats"

// Intent is the closed sum type of gameplay actions that resolve inside a
// tick rather than immediately. Concrete intents implement the unexported
// isIntent marker so the set is fixed at compile time; the Cost method
// reports how much Energy the intent consumes when it succeeds.
//
// Intent is distinct from Command: Command covers lifecycle operations
// (join, leave) that bypass tick accounting, while Intent covers per-entity
// actions that must wait until the entity has accumulated enough Energy.
type Intent interface {
	isIntent()
	// Cost returns the Energy charged on a successful resolution of this
	// intent. Refund-on-failure is the World's responsibility, not the
	// intent's.
	Cost() int
}

// MoveIntent asks the world to step an entity by (DX, DY) on the next tick
// where the entity has at least Cost Energy available. Exactly one of DX,
// DY is non-zero and its value is in {-1, +1}; diagonal and zero-length
// moves are rejected at resolution time, mirroring MoveCmd's contract.
type MoveIntent struct {
	DX, DY int
}

func (MoveIntent) isIntent() {}

// Cost reports the Energy charged when the move resolves successfully.
// Moves run at the baseline rate; faster or slower actions override this.
func (MoveIntent) Cost() int { return stats.BaseActionCost }

// Compile-time proof that every concrete intent satisfies Intent.
var _ Intent = MoveIntent{}

package world

import "github.com/Rioverde/gongeons/internal/game/entity"

// Mover is the tick-loop's view of any entity that takes turns — the minimum
// shape the Tick scheduler needs to order entities, accumulate energy, and
// dispatch intents. Both *entity.Player and *entity.Monster satisfy this
// interface through methods promoted from their embedded Character and
// DerivedStats fields.
//
// Mover is defined here rather than in the entity package to avoid an import
// cycle: Intent is declared in world, and entity must not import world.
type Mover interface {
	// TickID returns the entity's unique identifier used for tick ordering.
	TickID() string
	// TickSpeed returns the entity's Speed for energy accumulation.
	TickSpeed() int
	// TickInitiative returns the entity's Initiative for within-tick ordering.
	TickInitiative() int
	// TickEnergy returns the entity's current Energy.
	TickEnergy() int
	// SetTickEnergy updates the entity's Energy.
	SetTickEnergy(n int)
	// TickIntent returns the pending intent, or nil when idle.
	TickIntent() any
	// SetTickIntent replaces the pending intent; pass nil to clear.
	SetTickIntent(i any)
}

// Compile-time proof that both concrete entity types satisfy Mover.
var _ Mover = (*entity.Player)(nil)
var _ Mover = (*entity.Monster)(nil)

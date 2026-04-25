package entity

// TickID returns the entity's unique identifier for tick ordering.
// Defined on Character so both Player and Monster inherit it via embedding.
func (c *Character) TickID() string { return c.ID }

// TickSpeed returns the entity's Speed for energy accumulation.
func (d *DerivedStats) TickSpeed() int { return d.Speed }

// TickInitiative returns the entity's Initiative for within-tick ordering.
func (d *DerivedStats) TickInitiative() int { return d.Initiative }

// TickEnergy returns the entity's current Energy.
func (d *DerivedStats) TickEnergy() int { return d.Energy }

// SetTickEnergy updates the entity's Energy.
func (d *DerivedStats) SetTickEnergy(n int) { d.Energy = n }

// TickIntent returns the entity's pending intent value, or nil when idle.
func (d *DerivedStats) TickIntent() any { return d.Intent }

// SetTickIntent replaces the entity's pending intent. Pass nil to clear.
// Stores an untyped nil when i is nil so downstream nil checks evaluate
// correctly (a typed-interface nil would pass the != nil guard).
func (d *DerivedStats) SetTickIntent(i any) { d.Intent = i }

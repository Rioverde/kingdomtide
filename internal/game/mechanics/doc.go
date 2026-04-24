// Package mechanics holds the per-year tick functions that drive the
// living-world simulation. Each function mutates a single City (or, in
// later milestones, a Kingdom) according to one phase of the per-year
// update loop. Functions are pure with respect to their inputs — the
// same City + Stream + year yields the same mutation, which underpins
// the simulation's replay determinism.
//
// Functions take a *dice.Stream for all randomness and never touch
// math/rand directly — every random draw must flow through a
// subsystem-scoped Stream so adding a roll in one subsystem does not
// perturb another subsystem's sequence.
//
// TickCityYear composes the individual steps into the canonical order.
// Callers (the WorldGenerator at Pass-2 time, the per-year tick loop at
// runtime) should prefer TickCityYear over invoking the steps
// directly.
package mechanics

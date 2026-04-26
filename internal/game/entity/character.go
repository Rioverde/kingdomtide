package entity

import (
	"github.com/Rioverde/kingdomtide/internal/game/geom"
	"github.com/Rioverde/kingdomtide/internal/game/stats"
)

// Character is the shared base of every combat-capable entity in the
// game. It carries identity (ID, Name, Position), the D&D 5e-style raw
// ability scores (Stats), and the computed tick-state (DerivedStats —
// HP pool, Equipment, Speed, Intent). Player and Monster embed Character
// so every method defined here (BaseDamage) and every method inherited
// from the embedded DerivedStats (TakeDamage, IsAlive, Equip) is
// available on both without duplication.
//
// Keep Character thin — only fields and methods that are genuinely
// universal to "a named combat-capable being" belong here. Anything
// Player-only (session id, input latency) or Monster-only (challenge
// rating, AI archetype, loot table) lives on the wrapping type.
type Character struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Position geom.Position   `json:"position"`
	Stats    stats.CoreStats `json:"stats"`

	DerivedStats
}

// BaseDamage returns the character's current outgoing weapon damage.
// Derived from CoreStats so runtime stat changes (buffs, level-up) flow
// through automatically; used by Attack via the Combatant interface.
func (c *Character) BaseDamage() int {
	return c.Stats.BaseDamage()
}

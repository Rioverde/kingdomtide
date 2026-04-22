package game

// Combatant is the interface satisfied by any entity that can participate
// in combat: it can take damage, report liveness (via Occupant), and
// surface its current outgoing base damage.
type Combatant interface {
	Occupant
	TakeDamage(damage int)
	BaseDamage() int
}

// Occupant is the minimal interface for anything that can stand on a tile
// and be checked for liveness.
type Occupant interface {
	IsAlive() bool
}

// Attack resolves one attack from attacker against defender. A random
// body-part slot is selected, the attacker's base damage is scaled by
// that slot's multiplier, and the result is applied via TakeDamage.
func Attack(attacker, defender Combatant) {
	slot := attackedSlot()
	damage := calculateDamage(attacker.BaseDamage(), slot)
	defender.TakeDamage(damage)
}

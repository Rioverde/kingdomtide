package game

// Combatant is the interface satisfied by any entity that can participate in
// combat: it can take damage, report liveness, and expose its current stats.
type Combatant interface {
	TakeDamage(damage int)
	Occupant
	GetStats() Stats
}

// Occupant is the minimal interface for anything that can stand on a tile and
// be checked for liveness.
type Occupant interface {
	IsAlive() bool
}

// Attack resolves one attack from attacker against defender. A random body-part
// slot is selected, the attacker's BaseDamage is scaled by that slot's
// multiplier, and the result is applied via TakeDamage.
func Attack(attacker, defender Combatant) {
	attackerStats := attacker.GetStats()
	slot := attackedSlot()
	damage := calculateDamage(attackerStats.BaseDamage, slot)
	defender.TakeDamage(damage)
}

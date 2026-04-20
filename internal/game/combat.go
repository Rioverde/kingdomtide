package game

type Combatant interface {
	// TakeDamage applies damage to the combatant, reducing their health accordingly.
	TakeDamage(damage int)
	// IsAlive checks if the combatant is still alive (i.e., has health greater than zero).
	IsAlive() bool
	// GetStats returns the current stats of the combatant, including health and mana.
	GetStats() Stats
}

func Attack(attacker, defender Combatant) {
	// Get the attacker's stats to determine the damage output.
	attackerStats := attacker.GetStats()
	// Check on which slot the attack is being made.
	slot := attackedSlot()
	// For simplicity, we'll use the attacker's strength as the damage output.
	damage := calculateDamage(attackerStats.BaseDamage, slot)
	// Apply damage to the defender using the TakeDamage method defined in the Combatant interface.
	defender.TakeDamage(damage)
}

package combat

import "github.com/Rioverde/gongeons/internal/game/entity"

// Combatant is the interface satisfied by any entity that can participate
// in combat: it can take damage, report liveness (via entity.Occupant),
// and surface its current outgoing base damage.
type Combatant interface {
	entity.Occupant
	TakeDamage(damage int)
	BaseDamage() int
}

// Attack resolves one attack from attacker against defender. A random
// body-part slot is selected, the attacker's base damage is scaled by
// that slot's multiplier, and the result is applied via TakeDamage.
func Attack(attacker, defender Combatant) {
	slot := attackedSlot()
	damage := calculateDamage(attacker.BaseDamage(), slot)
	defender.TakeDamage(damage)
}

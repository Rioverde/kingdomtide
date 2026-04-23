package combat

import (
	"math/rand"

	"github.com/Rioverde/gongeons/internal/game/entity"
)

// Slots for equipping armor pieces. Ordered so attackedSlot picks an
// index into a stable slice — tests that stub the RNG see a predictable
// mapping.
var slots = []entity.Slot{entity.SlotHead, entity.SlotBody, entity.SlotLegs}

// calculateDamage applies the body-part damage multiplier to a base damage
// value. Unknown slots pass through unmodified rather than panicking —
// robust handling keeps combat composable with future slot additions.
func calculateDamage(baseDamage int, slot entity.Slot) int {
	switch slot {
	case entity.SlotHead:
		return int(float64(baseDamage) * entity.HeadDamageMultiplier)
	case entity.SlotBody:
		return int(float64(baseDamage) * entity.BodyDamageMultiplier)
	case entity.SlotLegs:
		return int(float64(baseDamage) * entity.LegsDamageMultiplier)
	default:
		return baseDamage
	}
}

// attackedSlot returns a random equipment slot for the incoming attack.
// Placeholder for future targeting logic (player-chosen or AI-chosen
// body part); uses math/rand because combat resolution is cosmetic, not
// security-sensitive.
func attackedSlot() entity.Slot {
	return slots[rand.Intn(len(slots))]
}

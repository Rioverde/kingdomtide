package game

import "math/rand"

// Slots for equipping armor pieces. Ordered so attackedSlot picks an
// index into a stable slice — tests that stub the RNG see a predictable
// mapping.
var slots = []Slot{SlotHead, SlotBody, SlotLegs}

// calculateDamage applies the body-part damage multiplier to a base damage
// value. Unknown slots pass through unmodified rather than panicking —
// robust handling keeps combat composable with future slot additions.
func calculateDamage(baseDamage int, slot Slot) int {
	switch slot {
	case SlotHead:
		return int(float64(baseDamage) * HeadDamageMultiplier)
	case SlotBody:
		return int(float64(baseDamage) * BodyDamageMultiplier)
	case SlotLegs:
		return int(float64(baseDamage) * LegsDamageMultiplier)
	default:
		return baseDamage
	}
}

// attackedSlot returns a random equipment slot for the incoming attack.
// Placeholder for future targeting logic (player-chosen or AI-chosen
// body part); uses math/rand because combat resolution is cosmetic, not
// security-sensitive.
func attackedSlot() Slot {
	return slots[rand.Intn(len(slots))]
}

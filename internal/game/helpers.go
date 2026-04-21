package game

import "math/rand"

// Scaling constants used when deriving Health and Mana from core stats.
const (
	BaseHealthPerStrength   = 10
	BaseHealthPerDexterity  = 5
	BaseManaPerIntelligence = 10
)

// Slots for equipping armor pieces.
var slots = []Slot{SlotHead, SlotBody, SlotLegs}

func calculateHealth(strength, dexterity int) int {
	return strength*BaseHealthPerStrength + dexterity*BaseHealthPerDexterity
}

func calculateMana(intelligence int) int {
	return intelligence * BaseManaPerIntelligence
}

// calculateBaseDamage calculates the base damage based on the attacker's strength.
func calculateBaseDamage(strength int) int {
	return strength * 2
}

// calculateDamage applies the damage multiplier based on the hit slot and returns the final damage to be applied to the player's health.
func calculateDamage(baseDamage int, slot Slot) int {
	switch slot {
	case SlotHead:
		return int(float64(baseDamage) * HeadDamageMultiplier)
	case SlotBody:
		return int(float64(baseDamage) * BodyDamageMultiplier)
	case SlotLegs:
		return int(float64(baseDamage) * LegsDamageMultiplier)
	default:
		return baseDamage // No multiplier for unknown slots
	}
}

// attackedSlot returns a random slot for the attack. This is a placeholder function and can be replaced with more complex logic to allow players to choose the target slot.
func attackedSlot() Slot {
	return slots[rand.Intn(len(slots))]
}

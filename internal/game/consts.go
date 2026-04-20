package game

type Slot string

const (
	// Game constants
	SlotHead Slot = "head"
	SlotBody Slot = "body"
	SlotLegs Slot = "legs"

	// Damage multipliers for different body parts
	HeadDamageMultiplier = 2.0
	BodyDamageMultiplier = 1.0
	LegsDamageMultiplier = 0.5
	numberOfSlots        = 3
)

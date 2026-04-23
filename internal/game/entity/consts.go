package entity

// Slot identifies an equipment slot on a combatant's body (head, body, legs).
type Slot string

const (
	// Equipment slots.
	SlotHead Slot = "head"
	SlotBody Slot = "body"
	SlotLegs Slot = "legs"

	// NumberOfSlots is the fixed count of equipment slots a combatant has
	// (head, body, legs). Exported so other packages can pre-size maps
	// keyed by Slot without duplicating the constant.
	NumberOfSlots = 3

	// Damage multipliers for different body parts.
	HeadDamageMultiplier = 2.0
	BodyDamageMultiplier = 1.0
	LegsDamageMultiplier = 0.5
)

// Occupant is the minimal interface for anything that can stand on a tile
// and be checked for liveness. Concrete implementations (Player, Monster)
// satisfy it structurally; tile consumers (world.Tile.Occupant) hold a
// value of this interface type.
type Occupant interface {
	IsAlive() bool
}

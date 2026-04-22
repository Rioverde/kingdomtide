package game

import (
	"errors"
)

// Ensure that Player implements the Combatant interface at compile time.
var _ Combatant = (*Player)(nil)

// Player is the in-world representation of a human controller.
//
// Stats is the D&D 5e-style six-ability distribution sent by the client
// on join; derived pools (MaxHP, HP, Mana) and tick fields (Speed,
// Initiative) are seeded from it at construction time so the Player value
// is self-consistent the moment NewPlayer returns.
//
// Speed, Energy, Initiative and Intent drive the tick-based turn
// resolution system: Speed is Energy accumulated per World.Tick, Energy
// is the current pool, Initiative is the within-tick tiebreaker, and
// Intent is the single-slot pending action (nil when idle).
type Player struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Position  Position        `json:"position"`
	Stats     CoreStats       `json:"stats"`
	Equipment map[Slot]*Armor `json:"equipment,omitempty"`

	MaxHP int `json:"max_hp"`
	HP    int `json:"hp"`
	Mana  int `json:"mana"`

	Speed      int    `json:"speed"`
	Energy     int    `json:"energy"`
	Initiative int    `json:"initiative"`
	Intent     Intent `json:"-"`
}

// Armor represents the armor equipped by the player, which can provide
// additional defense on a per-slot basis.
type Armor struct {
	Name        string `json:"name"`
	Defense     int    `json:"defense"`
	Description string `json:"description,omitempty"`
}

// TakeDamage applies incoming damage to the player, passing it through
// each equipped armor piece in turn. A dead player (HP <= 0) is a no-op;
// damage that survives all absorbers is subtracted from HP, clamped at
// zero.
func (p *Player) TakeDamage(damage int) {
	if !p.IsAlive() {
		return
	}
	for _, armor := range p.Equipment {
		if damage <= 0 {
			break
		}
		if armor != nil {
			damage = armor.Reduce(damage)
		}
	}
	p.HP -= damage
	if p.HP < 0 {
		p.HP = 0
	}
}

// Reduce returns the damage left over after this armor piece absorbs its
// share. Never returns a negative value.
func (a *Armor) Reduce(damage int) int {
	return max(0, damage-a.Defense)
}

// IsAlive reports whether the player's HP is above zero.
func (p *Player) IsAlive() bool {
	return p.HP > 0
}

// BaseDamage returns the player's current outgoing weapon damage, used by
// Attack via the Combatant interface. Derived from CoreStats so runtime
// stat changes (future buffs, level-up) flow through automatically.
func (p *Player) BaseDamage() int {
	return p.Stats.BaseDamage()
}

// Equip puts armor into the named slot, replacing anything that was
// previously there.
func (p *Player) Equip(slot Slot, armor *Armor) {
	p.Equipment[slot] = armor
}

// NewPlayer creates a Player with the given id, name, core stat
// distribution, and spawn position. Returns an error when id or name is
// empty. Stats are assumed to have been validated by the caller (typical
// path: NewStatsPointBuy on the join frame); MaxHP/HP/Mana/Speed/
// Initiative/Energy are derived from the stats so the returned Player is
// immediately tick-ready.
func NewPlayer(id, name string, stats CoreStats, pos Position) (*Player, error) {
	if id == "" {
		return nil, errors.New("invalid player ID")
	}
	if name == "" {
		return nil, errors.New("invalid player name")
	}
	maxHP := stats.MaxHP()
	return &Player{
		ID:         id,
		Name:       name,
		Position:   pos,
		Stats:      stats,
		Equipment:  make(map[Slot]*Armor, numberOfSlots),
		MaxHP:      maxHP,
		HP:         maxHP,
		Mana:       stats.Mana(),
		Speed:      stats.DerivedSpeed(),
		Energy:     baseActionCost,
		Initiative: stats.DerivedInitiative(),
	}, nil
}

package game

import "errors"

// Ensure that Monster implements the Combatant interface at compile time.
var _ Combatant = (*Monster)(nil)

// Monster mirrors Player for non-player entities. The same D&D 5e six-
// ability CoreStats drives HP, damage, Speed, and Initiative so AI-
// controlled and player-controlled entities resolve through a single tick
// pipeline without special casing. Monsters without a registry entry are
// constructed with DefaultCoreStats (all 10s — a neutral baseline).
type Monster struct {
	ID    string    `json:"id"`
	Name  string    `json:"name"`
	Stats CoreStats `json:"stats"`

	MaxHP int `json:"max_hp"`
	HP    int `json:"hp"`
	Mana  int `json:"mana"`

	Speed      int    `json:"speed"`
	Energy     int    `json:"energy"`
	Initiative int    `json:"initiative"`
	Intent     Intent `json:"-"`
}

// TakeDamage reduces HP by the given amount, clamping at zero. Monster
// armor mechanics are not modelled yet; when added they will mirror
// Player.TakeDamage's slot-aware absorption loop.
func (m *Monster) TakeDamage(damage int) {
	if damage <= 0 {
		return
	}
	m.HP -= damage
	if m.HP < 0 {
		m.HP = 0
	}
}

// BaseDamage returns the monster's current outgoing weapon damage, mirror
// of Player.BaseDamage. Derived from CoreStats so buffs/debuffs flow
// through.
func (m *Monster) BaseDamage() int {
	return m.Stats.BaseDamage()
}

// IsAlive reports whether the monster's HP is above zero.
func (m *Monster) IsAlive() bool {
	return m.HP > 0
}

// NewMonster creates a Monster with the given id, name, and stat
// distribution. Returns an error when id or name is empty. Pass
// DefaultCoreStats for registry-less placeholder monsters.
func NewMonster(id, name string, stats CoreStats) (*Monster, error) {
	if id == "" {
		return nil, errors.New("invalid monster ID")
	}
	if name == "" {
		return nil, errors.New("invalid monster name")
	}
	maxHP := stats.MaxHP()
	return &Monster{
		ID:         id,
		Name:       name,
		Stats:      stats,
		MaxHP:      maxHP,
		HP:         maxHP,
		Mana:       stats.Mana(),
		Speed:      stats.DerivedSpeed(),
		Initiative: stats.DerivedInitiative(),
	}, nil
}

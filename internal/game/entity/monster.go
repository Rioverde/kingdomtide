package entity

import (
	"errors"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// Monster mirrors Player for non-player entities. The same D&D 5e six-
// ability CoreStats drives HP, damage, Speed, and Initiative so AI-
// controlled and player-controlled entities resolve through a single tick
// pipeline without special casing. Monsters without a registry entry are
// constructed with DefaultCoreStats (all 10s — a neutral baseline).
//
// Position is the monster's current world coordinate. Admin insertion
// via AddMonster places the monster at the origin; future AI paths
// will place monsters via AddMonsterAt at spawn time. Move intents
// resolved inside Tick update Position atomically with world-side
// occupancy bookkeeping. Intent is typed as any while the concrete
// Intent interface still lives in package game; a follow-up step of
// the ongoing package split retypes this field once the interface
// lands in internal/game/action.
type Monster struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Stats stats.CoreStats `json:"stats"`

	MaxHP int `json:"max_hp"`
	HP    int `json:"hp"`
	Mana  int `json:"mana"`

	Speed      int           `json:"speed"`
	Energy     int           `json:"energy"`
	Initiative int           `json:"initiative"`
	Position   geom.Position `json:"position"`
	Intent     any           `json:"-"`
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
func NewMonster(id, name string, cs stats.CoreStats) (*Monster, error) {
	if id == "" {
		return nil, errors.New("invalid monster ID")
	}
	if name == "" {
		return nil, errors.New("invalid monster name")
	}
	maxHP := cs.MaxHP()
	return &Monster{
		ID:         id,
		Name:       name,
		Stats:      cs,
		MaxHP:      maxHP,
		HP:         maxHP,
		Mana:       cs.Mana(),
		Speed:      cs.DerivedSpeed(),
		Initiative: cs.DerivedInitiative(),
	}, nil
}

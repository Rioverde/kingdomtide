package entity

import (
	"errors"

	"github.com/Rioverde/kingdomtide/internal/game/stats"
)

// Monster is an AI-controlled entity. It wraps the shared Character
// base with no additional fields today; future Monster-only state
// (challenge rating, loot table, AI archetype) will accrue here as the
// bestiary grows. All combat behaviour (TakeDamage, BaseDamage,
// IsAlive, Equip) is inherited through Character's embedded
// DerivedStats so monsters resolve through the same tick pipeline as
// players.
type Monster struct {
	Character
}

// NewMonster creates a Monster with the given id, name, and stat
// distribution. Returns an error when id or name is empty. Pass
// DefaultCoreStats for registry-less placeholder monsters. DerivedStats
// is seeded from the stats so the returned Monster is immediately
// tick-ready.
func NewMonster(id, name string, cs stats.CoreStats) (*Monster, error) {
	if id == "" {
		return nil, errors.New("invalid monster ID")
	}
	if name == "" {
		return nil, errors.New("invalid monster name")
	}
	maxHP := cs.MaxHP()
	maxMana := cs.Mana()
	return &Monster{
		Character: Character{
			ID:    id,
			Name:  name,
			Stats: cs,
			DerivedStats: DerivedStats{
				Equipment:  make(map[Slot]*Armor, NumberOfSlots),
				MaxHP:      maxHP,
				HP:         maxHP,
				MaxMana:    maxMana,
				Mana:       maxMana,
				Speed:      cs.DerivedSpeed(),
				Initiative: cs.DerivedInitiative(),
			},
		},
	}, nil
}

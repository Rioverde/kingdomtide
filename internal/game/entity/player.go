package entity

import (
	"errors"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
	"github.com/Rioverde/kingdomtide/internal/game/stats"
)

// Player is a human-controlled entity. It wraps the shared Character
// base with no additional fields today; future Player-only state
// (session handle, input latency, visibility of UI hints) will accrue
// here as the game grows. All combat behaviour (TakeDamage, BaseDamage,
// IsAlive, Equip) is inherited through Character's embedded DerivedStats
// so the tick pipeline treats Player and Monster identically.
type Player struct {
	Character
}

// NewPlayer creates a Player with the given id, name, core stat
// distribution, and spawn position. Returns an error when id or name is
// empty. Stats are assumed to have been validated by the caller (typical
// path: NewStatsPointBuy on the join frame); DerivedStats is seeded so
// the returned Player is immediately tick-ready.
func NewPlayer(id, name string, cs stats.CoreStats, pos geom.Position) (*Player, error) {
	if id == "" {
		return nil, errors.New("invalid player ID")
	}
	if name == "" {
		return nil, errors.New("invalid player name")
	}
	maxHP := cs.MaxHP()
	maxMana := cs.Mana()
	return &Player{
		Character: Character{
			ID:       id,
			Name:     name,
			Position: pos,
			Stats:    cs,
			DerivedStats: DerivedStats{
				Equipment:  make(map[Slot]*Armor, NumberOfSlots),
				MaxHP:      maxHP,
				HP:         maxHP,
				MaxMana:    maxMana,
				Mana:       maxMana,
				Speed:      cs.DerivedSpeed(),
				Energy:     stats.BaseActionCost,
				Initiative: cs.DerivedInitiative(),
			},
		},
	}, nil
}

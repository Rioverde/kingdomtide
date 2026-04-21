package game

import (
	"errors"
)

// Ensure that Player implements the Combatant interface at compile time.
var _ Combatant = (*Player)(nil)

// Player represents a player in the game with an ID, name, and stats.
type Player struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Stats     *Stats          `json:"stats"`
	Equipment map[Slot]*Armor `json:"equipment,omitempty"`
}

// Armor represents the armor equipped by the player, which can provide additional defense.
type Armor struct {
	Name        string `json:"name"`
	Defense     int    `json:"defense"`
	Description string `json:"description,omitempty"`
}

// TakeDamage applies damage to the player's health, considering the defense provided by equipped armor pieces.
func (p *Player) TakeDamage(damage int) {
	// If the player is already at or below zero health, stop processing damage.
	if !p.IsAlive() {
		return
	}
	// Calculate the total defense provided by all equipped armor pieces.
	for _, armor := range p.Equipment {
		if damage <= 0 {
			break // No more damage to apply, exit the loop.
		}
		if armor != nil {
			// Pass the damage through each armor piece; Reduce returns the damage left after this piece absorbs its share.
			damage = armor.Reduce(damage)
		}
	}
	// Apply the remaining damage to the player's health.
	p.Stats.applyDamage(damage)
}

// Reduce calculates the effective damage after applying the armor's defense and ensures that the damage does not go below zero.
func (a *Armor) Reduce(damage int) int {
	return max(0, damage-a.Defense)
}

// IsAlive reports whether the player's Health is above zero.
func (p *Player) IsAlive() bool {
	return p.Stats.Health > 0
}

// GetStats returns a copy of the player's current Stats.
func (p *Player) GetStats() Stats {
	return *p.Stats
}

// Equip allows the player to equip an armor piece in a specified slot, replacing any existing armor in that slot.
func (p *Player) Equip(slot Slot, armor *Armor) {
	p.Equipment[slot] = armor
}

// NewPlayer creates a Player with the given id, name, and core stats.
// Returns an error when id or name is empty, or any core stat is negative.
func NewPlayer(id, name string, strength, dexterity, intelligence int) (*Player, error) {
	if id == "" {
		return nil, errors.New("invalid player ID")
	}
	if name == "" {
		return nil, errors.New("invalid player name")
	}
	if strength < 0 || dexterity < 0 || intelligence < 0 {
		return nil, errors.New("core stats cannot be negative")
	}
	return &Player{
		ID:        id,
		Name:      name,
		Stats:     NewStats(strength, dexterity, intelligence),
		Equipment: make(map[Slot]*Armor, numberOfSlots),
	}, nil
}

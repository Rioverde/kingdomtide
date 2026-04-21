package game

import "errors"

// Ensure that Monster implements the Combatant interface at compile time.
var _ Combatant = (*Monster)(nil)

// Monster represents a monster in the game with an ID, name, and stats.
type Monster struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Stats *Stats `json:"stats"`
}

// TakeDamage applies damage to the monster's health, ensuring that health does not drop below zero.
func (m *Monster) TakeDamage(damage int) {
	m.Stats.applyDamage(damage)
}

// NewMonster creates a Monster with the given id, name, and core stats.
// Returns an error when id or name is empty, or any core stat is negative.
func NewMonster(id, name string, strength, dexterity, intelligence int) (*Monster, error) {
	if id == "" {
		return nil, errors.New("invalid monster ID")
	}
	if name == "" {
		return nil, errors.New("invalid monster name")
	}
	if strength < 0 || dexterity < 0 || intelligence < 0 {
		return nil, errors.New("core stats cannot be negative")
	}
	return &Monster{
		ID:    id,
		Name:  name,
		Stats: NewStats(strength, dexterity, intelligence),
	}, nil
}

// IsAlive checks if the monster is still alive (i.e., has health greater than zero).
func (m *Monster) IsAlive() bool {
	return m.Stats.Health > 0
}

// GetStats returns the current stats of the monster, including health and mana.
func (m *Monster) GetStats() Stats {
	return *m.Stats
}

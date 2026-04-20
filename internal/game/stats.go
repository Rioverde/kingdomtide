package game

// CoreStats represents the core attributes of a player that directly influence their derived stats.
type CoreStats struct {
	Strength     int
	Dexterity    int
	Intelligence int
}

// DerivedStats represents the stats that are calculated based on the core stats.
type DerivedStats struct {
	Health     int
	MaxHealth  int
	Mana       int
	BaseDamage int
}

// Stats combines both core and derived stats for a player.
type Stats struct {
	CoreStats
	DerivedStats
}

// NewStats creates a new Stats struct based on the provided core stats and calculates the derived stats.
func NewStats(strength, dexterity, intelligence int) *Stats {
	// Create the core stats struct with the provided values.
	core := CoreStats{
		Strength:     strength,
		Dexterity:    dexterity,
		Intelligence: intelligence,
	}
	// Calculate the derived stats based on the core stats and return the complete Stats struct.
	return &Stats{
		CoreStats:    core,
		DerivedStats: core.CalculateDerivedStats(),
	}
}

// CalculateDerivedStats calculates the derived stats (Health, MaxHealth, Mana) based on the core stats.
func (c *CoreStats) CalculateDerivedStats() DerivedStats {
	// Calculate health and mana based on the core stats using the defined constants.
	health := calculateHealth(c.Strength, c.Dexterity)
	// For simplicity, we assume that MaxHealth is the same as Health at the start. You can modify this logic as needed.
	mana := calculateMana(c.Intelligence)
	// BaseDamage can be calculated based on strength or other core stats as needed. For now, we'll set it to a default value.
	baseDamage := calculateBaseDamage(c.Strength)
	// Return the calculated derived stats.
	return DerivedStats{
		Health:     health,
		MaxHealth:  health,
		Mana:       mana,
		BaseDamage: baseDamage,
	}
}

func (s *Stats) applyDamage(damage int) {
	s.Health -= damage
	if s.Health < 0 {
		s.Health = 0
	}
}

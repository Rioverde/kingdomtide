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

// NewStats creates a Stats value from the three core attributes, computing all
// derived fields (Health, MaxHealth, Mana, BaseDamage) immediately.
func NewStats(strength, dexterity, intelligence int) *Stats {
	core := CoreStats{
		Strength:     strength,
		Dexterity:    dexterity,
		Intelligence: intelligence,
	}
	return &Stats{
		CoreStats:    core,
		DerivedStats: core.CalculateDerivedStats(),
	}
}

// CalculateDerivedStats computes Health, MaxHealth, Mana, and BaseDamage from
// the receiver's core attributes. MaxHealth equals Health at creation time.
func (c *CoreStats) CalculateDerivedStats() DerivedStats {
	health := calculateHealth(c.Strength, c.Dexterity)
	mana := calculateMana(c.Intelligence)
	baseDamage := calculateBaseDamage(c.Strength)
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

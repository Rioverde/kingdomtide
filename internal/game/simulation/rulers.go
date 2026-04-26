package simulation

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// tickRulers advances every settlement's ruler by one year. When a
// ruler's lifespan is exhausted (currentYear >= BirthYear +
// LifeExpectancy), the death is recorded on the outgoing Ruler and a
// fresh Ruler is rolled via the standard naming pipeline + dice.Stream.
//
// Determinism: the succession dice.Stream is keyed on
// (seed ^ packed-anchor, seedSaltSimRulerSucc ^ year ^ id) so two runs
// with the same seed produce identical successions. Iteration order
// uses the cached sortedSettlementIDs slice from population.go to keep
// the log line ordering stable.
//
// This step runs AFTER tickPopulation but BEFORE tickDeaths so that a
// camp that lost its ruler this year still goes through the
// abandonment streak with a freshly-rolled successor — the death of a
// ruler does not preserve the camp by itself.
func (s *state) tickRulers(year int) {
	for _, id := range s.sortedSettlementIDs() {
		set := s.settlements[id].Base()
		if !set.Ruler.Alive() {
			// Already dead and not yet succeeded — should not normally
			// happen because succession runs in the same tick the
			// ruler dies. Defensive guard.
			continue
		}
		expectedDeath := set.Ruler.BirthYear + set.Ruler.LifeExpectancy()
		if year < expectedDeath {
			continue
		}
		s.succeedRuler(set, id, year)
	}
}

// succeedRuler marks the current ruler as dead at year and installs a
// fresh successor. Naming uses the same Markov pipeline as initial
// camp founding, keyed on the settlement's anchor + region. The
// dice.Stream is salted with seedSaltSimRulerSucc XORed with year and
// settlement ID so two consecutive successions in the same settlement
// produce different ability scores.
func (s *state) succeedRuler(set *polity.Settlement, id polity.SettlementID, year int) {
	old := set.Ruler
	oldAge := year - old.BirthYear
	oldTitle := rulerTitle(set.Tier)

	old.DeathYear = year

	successorName := generateRulerName(s.seed, set.Position, set.Region)
	stream := dice.New(
		s.seed^int64(geom.PackPos(set.Position)),
		dice.Salt(uint64(seedSaltSimRulerSucc)^uint64(year)^uint64(id)),
	)
	set.Ruler = polity.NewRuler(stream, year, successorName)

	newTitle := rulerTitle(set.Tier)
	s.log.emit(year, "ruler-succeeded",
		fmt.Sprintf("'%s' — %s '%s' died (age %d); succeeded by %s '%s'",
			set.Name, oldTitle, old.Name, oldAge, newTitle, set.Ruler.Name))
}

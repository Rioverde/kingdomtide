package simulation

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// tickDeaths abandons camps whose population stayed below
// simCampPopAbandonFloor for simAbandonStreakYears consecutive years.
// Hamlets and Villages are immune in this simulation tier — once
// promoted they are stable. Future phases (cities/kingdoms) can add
// catastrophic-collapse mechanics.
func (s *state) tickDeaths(year int) {
	var dead []polity.SettlementID
	for _, id := range s.sortedSettlementIDs() {
		place := s.settlements[id]
		set := place.Base()
		if set.Tier != polity.TierCamp {
			s.abandonStreak[id] = 0
			continue
		}
		if set.Population >= simCampPopAbandonFloor {
			s.abandonStreak[id] = 0
			continue
		}
		s.abandonStreak[id]++
		if s.abandonStreak[id] >= simAbandonStreakYears {
			dead = append(dead, id)
		}
	}
	for _, id := range dead {
		s.killSettlement(id, year, "abandoned — sustained low population")
	}
}

// killSettlement removes the settlement from the live map and emits a
// camp-died log event.
func (s *state) killSettlement(id polity.SettlementID, year int, reason string) {
	if set, ok := s.settlements[id]; ok {
		b := set.Base()
		s.log.emit(year, "camp-died",
			fmt.Sprintf("'%s' %s abandoned in '%s' — %s",
				b.Name, describeRuler(b), b.Region, reason))
	}
	delete(s.settlements, id)
	delete(s.abandonStreak, id)
	s.dirty = true
}

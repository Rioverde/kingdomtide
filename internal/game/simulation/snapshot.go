package simulation

import (
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Snapshot is a frozen copy of simulation state at one tick boundary.
// The dev-tool sim-explorer plays the simulation back by stepping
// through the snapshot list.
type Snapshot struct {
	Year     int
	Camps    []polity.Camp
	Hamlets  []polity.Hamlet
	Villages []polity.Village
}

// snapshot captures a value-typed copy of every live settlement at year.
// Returns a Snapshot that the caller can append to its per-year history.
// Allocations: O(settlement count) per call — acceptable for the
// simulation's ~800 settlements / 500 years.
func (s *state) snapshot(year int) Snapshot {
	out := Snapshot{Year: year}
	for _, id := range s.sortedSettlementIDs() {
		switch p := s.settlements[id].(type) {
		case *polity.Camp:
			out.Camps = append(out.Camps, *p) // value copy
		case *polity.Hamlet:
			out.Hamlets = append(out.Hamlets, *p)
		case *polity.Village:
			out.Villages = append(out.Villages, *p)
		}
	}
	return out
}

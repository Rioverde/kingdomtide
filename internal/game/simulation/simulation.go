package simulation

import (
	"fmt"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Result is the output of Run. Contains the final state ready to be
// adapted to a polity.SettlementSource (Phase 8) plus the per-tick
// snapshot history (for the dev tool playback).
type Result struct {
	settlements map[polity.SettlementID]polity.Place
	snapshots   []Snapshot
}

// Run executes the bottom-up settlement simulation for the configured
// number of years (default simYears=500). Inputs:
//   - seed: the world seed
//   - src:  the worldgen-produced camp seed source
//   - opts: optional WithLogger / WithYears / WithSnapshotEvery
//
// Determinism contract: same (seed, camp set, options) yields the
// same final state and the same snapshot list every run.
func Run(seed int64, src polity.CampSource, opts ...Option) *Result {
	cfg := defaultRunConfig()
	for _, o := range opts {
		o(&cfg)
	}

	st := newState(seed, cfg.years)
	st.log = newLogger(cfg.logger)

	camps := src.All()
	for i := range camps {
		c := camps[i] // value copy
		st.settlements[c.ID] = &c
	}
	st.dirty = true

	st.rollAnnualSchedules(cfg.years)

	st.log.emit(0, "sim-init", initLogDetails(len(camps)))

	var snapshots []Snapshot
	if cfg.snapshotEvery > 0 {
		snapshots = make([]Snapshot, 0, cfg.years/cfg.snapshotEvery+1)
	}

	for year := 0; year < cfg.years; year++ {
		// Refresh the cached ID list once per year so all per-tick steps
		// share one sorted slice instead of recomputing it ~6× per tick.
		st.refreshSortedIDs()

		st.tickPopulation(year)
		st.tickRulers(year)
		st.tickDeaths(year)
		st.tickSatellites(year)
		st.tickMerges(year)
		st.tickPromotions(year)
		st.tickFaithConversion(year)

		if cfg.snapshotEvery > 0 && year%cfg.snapshotEvery == 0 {
			snapshots = append(snapshots, st.snapshot(year))
		}
	}

	st.log.emit(cfg.years-1, "sim-end", endLogDetails(st))

	return &Result{
		settlements: st.settlements,
		snapshots:   snapshots,
	}
}

// initLogDetails formats the sim-init log line.
func initLogDetails(campCount int) string {
	return fmt.Sprintf("camps loaded: %d, regions seeded with famine schedule", campCount)
}

// endLogDetails formats the sim-end log line — counts surviving
// settlements by tier.
func endLogDetails(st *state) string {
	var camps, hamlets, villages int
	for _, p := range st.settlements {
		switch p.(type) {
		case *polity.Camp:
			camps++
		case *polity.Hamlet:
			hamlets++
		case *polity.Village:
			villages++
		}
	}
	return fmt.Sprintf("%d villages, %d hamlets, %d camps surviving", villages, hamlets, camps)
}

// Snapshots returns the per-tick state captures collected during Run.
// Empty if WithSnapshotEvery(0) was passed.
func (r *Result) Snapshots() []Snapshot { return r.snapshots }

// Settlements returns the final settlement map produced by Run.
// Used by the Phase 8 SettlementSource adapter.
func (r *Result) Settlements() map[polity.SettlementID]polity.Place { return r.settlements }

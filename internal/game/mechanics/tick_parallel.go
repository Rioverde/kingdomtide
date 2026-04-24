package mechanics

import (
	"fmt"
	"runtime"
	"sync"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// parallelTickThreshold below which TickCitiesYear falls back to a
// serial loop. Goroutine spawn + WaitGroup sync cost dominates the
// per-city body (~360 ns/city on M1 Max) for tiny batches, so below
// this count the serial path is strictly faster.
const parallelTickThreshold = 64

// TickCitiesYear advances every city in cities by a single simulated
// year, fanning work out across runtime.NumCPU() chunked workers.
// Each city must own its own *dice.Stream — streams are not shared
// and never mutate another city's state.
//
// Determinism: per-city mutations depend only on (city, stream, year)
// and never read any other city's state, so scheduling order does
// not affect the final values. The function is a drop-in replacement
// for a serial for-loop calling TickCityYear for every index.
//
// Preconditions:
//   - len(cities) == len(streams) (panics on mismatch — indicates a
//     call-site bug that would silently desynchronise determinism)
//   - every city in cities has its own private stream (guaranteed by
//     the world-manager seeding from (cityID, SaltKingdomYear))
//
// Safe for city-tick phase only. TickKingdomYear / TickLeagueYear /
// ApplyInterPolityEventsYear / ApplyMulkCycleYear read across the
// full city slice and MUST stay serial, after TickCitiesYear returns.
//
// Implementation: chunked workers rather than per-city goroutines.
// Each worker drains a contiguous slice range, eliminating the
// closure allocation and scheduler back-pressure an errgroup.Go
// incurs per call. Per-tick cost is ~360 ns — cheap enough that
// per-city dispatch overhead would dominate any fan-out win.
func TickCitiesYear(cities []*polity.City, streams []*dice.Stream, year int) {
	if len(cities) != len(streams) {
		panic(fmt.Sprintf("mechanics: TickCitiesYear len mismatch cities=%d streams=%d",
			len(cities), len(streams)))
	}
	n := len(cities)
	if n == 0 {
		return
	}
	if n < parallelTickThreshold {
		for i := range cities {
			TickCityYear(cities[i], streams[i], year)
		}
		return
	}

	workers := min(runtime.NumCPU(), n)
	chunk := (n + workers - 1) / workers

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		start := w * chunk
		end := min(start+chunk, n)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi; i++ {
				TickCityYear(cities[i], streams[i], year)
			}
		}(start, end)
	}
	wg.Wait()
}

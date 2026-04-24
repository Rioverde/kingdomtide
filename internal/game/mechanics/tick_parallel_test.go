package mechanics

import (
	"reflect"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// seededWorld mints n cities + per-city streams with deterministic
// per-index seeds. Used by all tick_parallel tests so the setups are
// identical across runs.
func seededWorld(n int) ([]*polity.City, []*dice.Stream) {
	cities := make([]*polity.City, n)
	streams := make([]*dice.Stream, n)
	for i := 0; i < n; i++ {
		ruler := polity.NewRuler(dice.New(int64(i+1), dice.SaltKingdomYear), 1270)
		c := polity.NewCity("City", geom.Position{X: i, Y: 0}, 1200, ruler)
		c.Population = 5000
		c.Wealth = 5000
		c.Army = 100
		c.Happiness = 70
		c.TaxRate = polity.TaxNormal
		c.Deposits = []polity.Deposit{
			{Kind: polity.DepositGold, RemainingYield: 0.9},
			{Kind: polity.DepositIron, RemainingYield: 0.9},
		}
		cities[i] = c
		streams[i] = dice.New(int64(i+1), dice.SaltKingdomYear)
	}
	return cities, streams
}

// TestTickCitiesYear_EmptyInput verifies the zero-city case is a
// no-op — no deadlock, no crash, no worker spawn. This is the guard
// for the common "no cities yet" startup path.
func TestTickCitiesYear_EmptyInput(t *testing.T) {
	TickCitiesYear(nil, nil, 1500)
	TickCitiesYear([]*polity.City{}, []*dice.Stream{}, 1500)
}

// TestTickCitiesYear_Determinism verifies byte-identical output
// between the parallel fan-out and an equivalent serial loop. Per-
// city state after 10 years must be reflect.DeepEqual. Uses 128
// cities — well above parallelTickThreshold so the chunked-worker
// path runs and the determinism claim is exercised under actual
// concurrent execution (not the below-threshold serial fallback).
func TestTickCitiesYear_Determinism(t *testing.T) {
	const (
		n         = 128
		startYear = 1300
		years     = 10
	)

	serialCities, serialStreams := seededWorld(n)
	for year := startYear; year < startYear+years; year++ {
		for i := range serialCities {
			TickCityYear(serialCities[i], serialStreams[i], year)
		}
	}

	parallelCities, parallelStreams := seededWorld(n)
	for year := startYear; year < startYear+years; year++ {
		TickCitiesYear(parallelCities, parallelStreams, year)
	}

	for i := range serialCities {
		if !reflect.DeepEqual(serialCities[i], parallelCities[i]) {
			t.Errorf("city %d diverged between serial and parallel\n  serial  =%+v\n  parallel=%+v",
				i, *serialCities[i], *parallelCities[i])
		}
	}
}

// TestTickCitiesYear_DeterminismBelowThreshold exercises the serial
// fast path (n < parallelTickThreshold) so we know both branches
// agree. Same contract as the determinism test at n=4.
func TestTickCitiesYear_DeterminismBelowThreshold(t *testing.T) {
	const (
		n         = 4
		startYear = 1300
		years     = 5
	)

	serialCities, serialStreams := seededWorld(n)
	for year := startYear; year < startYear+years; year++ {
		for i := range serialCities {
			TickCityYear(serialCities[i], serialStreams[i], year)
		}
	}

	parallelCities, parallelStreams := seededWorld(n)
	for year := startYear; year < startYear+years; year++ {
		TickCitiesYear(parallelCities, parallelStreams, year)
	}

	for i := range serialCities {
		if !reflect.DeepEqual(serialCities[i], parallelCities[i]) {
			t.Errorf("city %d diverged on small-batch serial path", i)
		}
	}
}

// TestTickCitiesYear_RaceFree fans 256 cities × 100 years through
// the parallel path. Designed to run under `go test -race`; the race
// detector must report zero violations because every city owns a
// private *dice.Stream and TickCityYear mutates only that city.
// 256 > parallelTickThreshold guarantees the chunked-worker branch.
func TestTickCitiesYear_RaceFree(t *testing.T) {
	if testing.Short() {
		t.Skip("short: 256 cities × 100 years is a race-detector stress test")
	}
	const (
		n         = 256
		startYear = 1300
		years     = 100
	)
	cities, streams := seededWorld(n)
	for year := startYear; year < startYear+years; year++ {
		TickCitiesYear(cities, streams, year)
	}
}

// TestTickCitiesYear_MismatchedLengths_Panics guards against silent
// desync — a call site passing city[i] with stream[j] would corrupt
// determinism across the world. Fail loud, fail early.
func TestTickCitiesYear_MismatchedLengths_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on length mismatch, got nil")
		}
	}()
	cities, streams := seededWorld(4)
	TickCitiesYear(cities, streams[:3], 1500)
}

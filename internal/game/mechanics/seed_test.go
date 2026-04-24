package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
)

// TestSeedPopulationZipf_Determinism verifies two streams with the
// same (seed, salt) produce identical population seeds — replay
// contract.
func TestSeedPopulationZipf_Determinism(t *testing.T) {
	a := dice.New(42, dice.SaltCityPopulation)
	b := dice.New(42, dice.SaltCityPopulation)
	for rank := 0; rank < 20; rank++ {
		if x, y := SeedPopulationZipf(a, rank), SeedPopulationZipf(b, rank); x != y {
			t.Fatalf("rank=%d: %d != %d", rank, x, y)
		}
	}
}

// TestSeedPopulationZipf_InRange verifies every seeded population
// lands inside the §2a viability interval [80, 40 000].
func TestSeedPopulationZipf_InRange(t *testing.T) {
	stream := dice.New(42, dice.SaltCityPopulation)
	for rank := 0; rank < 100; rank++ {
		pop := SeedPopulationZipf(stream, rank)
		if pop < 80 || pop > 40000 {
			t.Errorf("rank=%d: pop=%d out of [80, 40000]", rank, pop)
		}
	}
}

// TestSeedPopulationZipf_DescendingByRank verifies larger ranks
// produce smaller populations — the whole point of a Zipf curve. The
// check is "on average decreasing" across the first twenty ranks, not
// strict monotone (jitter can flip adjacent ranks).
func TestSeedPopulationZipf_DescendingByRank(t *testing.T) {
	stream := dice.New(42, dice.SaltCityPopulation)
	var first, last int
	for rank := 0; rank < 20; rank++ {
		pop := SeedPopulationZipf(stream, rank)
		if rank == 0 {
			first = pop
		}
		last = pop
	}
	if first <= last {
		t.Errorf("rank 0 (%d) should exceed rank 19 (%d) on average", first, last)
	}
}

// TestSeedAge_InRange verifies every age lands in the §2b uniform
// interval [10, 1500].
func TestSeedAge_InRange(t *testing.T) {
	stream := dice.New(42, dice.SaltKingdomYear)
	for i := 0; i < 1000; i++ {
		age := SeedAge(stream)
		if age < ageSeedMin || age > ageSeedMax {
			t.Errorf("age=%d out of [%d, %d]", age, ageSeedMin, ageSeedMax)
		}
	}
}

// TestSeedWealth_JitterRange verifies wealth lands inside the ±25 %
// envelope around the base × population, which is what §2c promises.
func TestSeedWealth_JitterRange(t *testing.T) {
	stream := dice.New(42, dice.SaltCityPopulation)
	const pop = 1000
	// base = 1000 × 0.5 = 500. Envelope = [375, 625] for ±25 %.
	const minAccept = 375
	const maxAccept = 625
	for i := 0; i < 200; i++ {
		w := SeedWealth(stream, pop)
		if w < minAccept || w > maxAccept {
			t.Errorf("wealth=%d out of [%d, %d]", w, minAccept, maxAccept)
		}
	}
}

// TestSeedWealth_Determinism verifies two streams at same seed
// produce identical wealth seeds.
func TestSeedWealth_Determinism(t *testing.T) {
	a := dice.New(42, dice.SaltCityPopulation)
	b := dice.New(42, dice.SaltCityPopulation)
	for pop := 100; pop < 5000; pop += 100 {
		if x, y := SeedWealth(a, pop), SeedWealth(b, pop); x != y {
			t.Fatalf("pop=%d: %d != %d", pop, x, y)
		}
	}
}

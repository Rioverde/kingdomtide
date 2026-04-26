package simulation

import (
	"math/rand/v2"

	"github.com/Rioverde/gongeons/internal/game/geom"
)

// newSimRng builds a deterministic PCG stream from
// (seed, salt, extra). Splitmix64 mixes the inputs so that any two
// distinct (salt, extra) tuples produce decorrelated streams,
// avoiding the failure mode where seed XOR salt XOR year happens to
// equal another stream's seed.
func newSimRng(seed, salt int64, extra uint64) *rand.Rand {
	state := geom.Splitmix64(uint64(seed) ^ uint64(salt) ^ extra*0x9E3779B97F4A7C15)
	return rand.New(rand.NewPCG(state, state^0x6c2e8a1f3b5d7094))
}

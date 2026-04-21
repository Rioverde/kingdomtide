//go:build race

package worldgen

// init flips raceEnabled when the test binary is built with -race. Paired with
// bench_test.go's default raceEnabled=false so the performance budget test can
// self-skip without calling runtime internals.
func init() { raceEnabled = true }

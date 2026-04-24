package worldgen

// WorldID is the stable identifier of one generated world. In the
// single-world present it is trivially equal to WorldParams.Seed; the
// distinction exists so the future multi-world registry (see
// .omc/plans/multi-world-future.md once written) can address worlds by
// a namespace-scoped id that is not confused with the seed used to
// generate them.
//
// Keeping the type separate today has two costs — a name and an
// explicit cast at boundaries — and one payoff: the day a player sails
// off the edge of world A into world B, every call site that needs to
// distinguish "which world" already speaks WorldID, not int64-seed.
// Nothing has to be renamed.
type WorldID int64

// WorldIDFromSeed is the default derivation used by the single-world
// client: one seed, one id, same number. Every consumer that needs to
// persist a world to disk or name a cache file should route its int64
// seed through this helper so the eventual multi-world cutover is a
// one-line change at the registry, not a grep through the codebase.
func WorldIDFromSeed(seed int64) WorldID { return WorldID(seed) }

// Int64 returns the underlying int64. Exposed for binary encoding and
// for cases where an id is logged or hashed alongside other 64-bit
// values.
func (id WorldID) Int64() int64 { return int64(id) }

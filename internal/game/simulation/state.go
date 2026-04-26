package simulation

import (
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// state is the per-run simulation state, mutated through the tick
// loop. Built once by Run(...), discarded when finalize returns.
type state struct {
	seed int64

	// log emits structured event lines. nil writer is a no-op.
	log *logger

	// settlements tracks every live settlement by ID. Multiple tier
	// types share one map via the Place interface so the tick loop
	// can iterate uniformly. Key insertion order is irrelevant
	// because every iteration sorts by ID before mutation.
	settlements map[polity.SettlementID]polity.Place

	// abandonStreak tracks consecutive low-pop years per settlement.
	// Used by deaths.go to decide when a camp is abandoned.
	abandonStreak map[polity.SettlementID]int

	// promoteSustain tracks consecutive years a settlement has been
	// over its promotion threshold. Reset to 0 on a year below threshold.
	promoteSustain map[polity.SettlementID]int

	// regionFamine[region][year] is true if that region suffered a
	// famine that year. Pre-rolled at init so per-settlement
	// pop updates can read it without their own RNG.
	regionFamine [polity.RegionCharacterCount][]bool

	// regionPlague[region][year] is true if that region suffered a
	// plague that year. Independent stream from famine so the two
	// can co-occur (rare but devastating). Pre-rolled at init.
	regionPlague [polity.RegionCharacterCount][]bool

	// plagueLogged tracks region-year pairs already announced via the
	// "plague" log event so each plague year emits exactly one line per
	// region. Cleared per year via the (region, year) composite key.
	plagueLogged map[uint64]struct{}

	// faithEmerged records which faith indices have already crossed the
	// emergence threshold (simFaithEmergePct) for each settlement.
	// Prevents re-emitting faith-emerged on micro-fluctuations.
	faithEmerged map[polity.SettlementID][polity.FaithCount]bool

	// cachedSortedIDs is rebuilt at the start of every year by Run and
	// after any settlement insert/delete that flips dirty=true. All tick
	// steps read this slice directly to avoid recomputing the sort
	// per-step (called ~6×/tick previously).
	cachedSortedIDs []polity.SettlementID
	dirty           bool
}

// newState constructs an empty state. Callers populate it via
// methods on state.
func newState(seed int64, years int) *state {
	famine := [polity.RegionCharacterCount][]bool{}
	plague := [polity.RegionCharacterCount][]bool{}
	for r := 0; r < int(polity.RegionCharacterCount); r++ {
		famine[r] = make([]bool, years)
		plague[r] = make([]bool, years)
	}
	return &state{
		seed:           seed,
		log:            newLogger(nil),
		settlements:    make(map[polity.SettlementID]polity.Place),
		abandonStreak:  make(map[polity.SettlementID]int),
		promoteSustain: make(map[polity.SettlementID]int),
		regionFamine:   famine,
		regionPlague:   plague,
		plagueLogged:   make(map[uint64]struct{}),
		faithEmerged:   make(map[polity.SettlementID][polity.FaithCount]bool),
		dirty:          true,
	}
}

package polity

// Faith enumerates the religions a city's population can follow. A
// city tracks a distribution over all faiths in FaithDistribution
// rather than a single Faith value; the majority faith drives UI
// display and the schism four-gate model.
type Faith uint8

const (
	// FaithOldGods is the default ancestral pantheon — the starting
	// faith of every new city. Declines over time as newer religions
	// spread along trade and migration routes.
	FaithOldGods Faith = iota
	// FaithSunCovenant is a solar-monotheist faith favored by
	// agrarian majorities and well-established cities.
	FaithSunCovenant
	// FaithGreenSage is an animist / nature-oriented tradition; its
	// adherents boost Innovation under the tech model.
	FaithGreenSage
	// FaithOneOath is a martial monotheism popular in military
	// strongholds and frontier cities.
	FaithOneOath
	// FaithStormPact is a turbulent sea-and-storm pantheon with
	// strong coastal / maritime presence.
	FaithStormPact
)

// faithCount is the number of defined Faith enum values. Sized to
// keep FaithDistribution a fixed-length array.
const faithCount = 5

// FaithCount is the exported count of defined Faith enum values.
// Mechanics packages use it as a compile-time constant so range
// bounds and minority counts are computed without reflection or len().
const FaithCount = faithCount

// String returns the English name of the faith.
// Dev-only — player-visible text via client i18n catalog.
func (f Faith) String() string {
	switch f {
	case FaithOldGods:
		return "OldGods"
	case FaithSunCovenant:
		return "SunCovenant"
	case FaithGreenSage:
		return "GreenSage"
	case FaithOneOath:
		return "OneOath"
	case FaithStormPact:
		return "StormPact"
	default:
		return "UnknownFaith"
	}
}

// allFaithsList is the ordered list of every defined Faith value,
// pre-allocated once at package init. AllFaiths returns this shared
// slice to every caller so per-tick iteration does not allocate a
// fresh header each time.
var allFaithsList = []Faith{
	FaithOldGods,
	FaithSunCovenant,
	FaithGreenSage,
	FaithOneOath,
	FaithStormPact,
}

// AllFaiths returns the ordered list of every defined Faith value.
// Helper for iteration. The returned slice is shared — callers MUST
// NOT mutate it.
func AllFaiths() []Faith { return allFaithsList }

// SchismEvent records a successful schism that split a city's majority
// faith. Kept on City.FaithHistory so downstream code (UI,
// analytics, future variant-faith type) can enumerate past schisms.
type SchismEvent struct {
	Year             int   `json:"year"`
	OriginalMajority Faith `json:"original_majority"`
	NewSecondary     Faith `json:"new_secondary"`
}

// FaithDistribution is the per-city population-fraction vector for
// each defined faith. Stored as a fixed-size array indexed by the
// Faith enum ordinal so no map allocation is needed, iteration is
// ordered without an AllFaiths bounce, and hot-path callers allocate
// nothing. The sum of entries should equal 1.0 within floating
// tolerance; invoke Normalize after any mutation that can break the
// invariant.
type FaithDistribution [faithCount]float64

// Default faith seed shares. OldGods sits at 0.92 as the dominant
// majority with the other four faiths at 0.02 each (sum = 1.0). A
// pure single-faith start starves the diffusion and schism mechanics
// because the schism gates never see a secondary candidate — seeding
// tiny minorities keeps the subsystem live from day one.
const (
	defaultFaithMajorityShare = 0.92
	defaultFaithMinorityShare = 0.02
)

// NewFaithDistribution constructs the default pre-conversion
// distribution. Default distribution seeds OldGods as 0.92 majority
// with the other four faiths at 0.02 each, so the diffusion /
// schism mechanics have something to move — a pure single-faith
// distribution starves the system. Sum equals 1.0 exactly by
// construction.
func NewFaithDistribution() FaithDistribution {
	var fd FaithDistribution
	fd[FaithOldGods] = defaultFaithMajorityShare
	fd[FaithSunCovenant] = defaultFaithMinorityShare
	fd[FaithGreenSage] = defaultFaithMinorityShare
	fd[FaithOneOath] = defaultFaithMinorityShare
	fd[FaithStormPact] = defaultFaithMinorityShare
	return fd
}

// IsZero reports whether every entry in the distribution is exactly
// zero. A zero-value FaithDistribution (e.g. from polity.City{}
// without NewFaithDistribution) means "faiths not yet initialised";
// call sites use this flag to short-circuit mechanics that would
// otherwise divide by zero or normalize a no-op vector.
func (fd FaithDistribution) IsZero() bool {
	for i := 0; i < faithCount; i++ {
		if fd[i] != 0 {
			return false
		}
	}
	return true
}

// Majority returns the faith with the highest share in the
// distribution. Ties break toward the lower Faith ordinal, which
// keeps the result stable across ticks when two faiths briefly
// equalize.
func (fd FaithDistribution) Majority() Faith {
	best := FaithOldGods
	for f := Faith(1); f < faithCount; f++ {
		if fd[f] > fd[best] {
			best = f
		}
	}
	return best
}

// Normalize rescales every entry so the total sums to 1.0. If the
// current sum is zero (or within floating noise of zero) the
// distribution falls back to 100% FaithOldGods to keep the invariant
// alive without introducing NaNs. Mutates the receiver in place — the
// pointer receiver lets callers see the change on the array they own.
func (fd *FaithDistribution) Normalize() {
	var total float64
	for i := 0; i < faithCount; i++ {
		if fd[i] < 0 {
			fd[i] = 0
		}
		total += fd[i]
	}
	if total <= 1e-12 {
		for i := 0; i < faithCount; i++ {
			fd[i] = 0
		}
		fd[FaithOldGods] = 1.0
		return
	}
	for i := 0; i < faithCount; i++ {
		fd[i] /= total
	}
}

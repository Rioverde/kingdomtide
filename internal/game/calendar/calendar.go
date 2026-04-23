package calendar

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
)

// Month enumerates the twelve calendar months in real-world Gregorian
// order. Values are 1-indexed so MonthZero is the "not set" sentinel
// and every valid month is MonthJanuary..MonthDecember; this mirrors
// time.Month from the standard library so the API reads intuitively.
// Append-only — the iota value participates in wire encoding.
type Month uint8

const (
	MonthZero Month = iota
	MonthJanuary
	MonthFebruary
	MonthMarch
	MonthApril
	MonthMay
	MonthJune
	MonthJuly
	MonthAugust
	MonthSeptember
	MonthOctober
	MonthNovember
	MonthDecember
)

var monthNames = [...]string{
	MonthZero:      "",
	MonthJanuary:   "january",
	MonthFebruary:  "february",
	MonthMarch:     "march",
	MonthApril:     "april",
	MonthMay:       "may",
	MonthJune:      "june",
	MonthJuly:      "july",
	MonthAugust:    "august",
	MonthSeptember: "september",
	MonthOctober:   "october",
	MonthNovember:  "november",
	MonthDecember:  "december",
}

// Key returns the lowercase identifier used for locale catalog lookups
// (e.g. "october"). Out-of-range values return the empty string so
// debug output on a corrupt value remains usable.
func (m Month) Key() string {
	if int(m) >= len(monthNames) {
		return ""
	}
	return monthNames[m]
}

// String implements fmt.Stringer by delegating to Key.
func (m Month) String() string { return m.Key() }

// Season enumerates the four calendar seasons. Derived from Month via
// SeasonOf; never stored independently on a GameTime the derivation
// didn't produce. Order starts from Winter so the northern-hemisphere
// "year-start = deep-winter" convention is explicit.
type Season uint8

const (
	SeasonWinter Season = iota
	SeasonSpring
	SeasonSummer
	SeasonAutumn
)

var seasonNames = [...]string{
	SeasonWinter: "winter",
	SeasonSpring: "spring",
	SeasonSummer: "summer",
	SeasonAutumn: "autumn",
}

// Key returns the lowercase identifier used for locale catalog lookups.
// Out-of-range values return the empty string.
func (s Season) Key() string {
	if int(s) >= len(seasonNames) {
		return ""
	}
	return seasonNames[s]
}

// String implements fmt.Stringer by delegating to Key.
func (s Season) String() string { return s.Key() }

// SeasonOf returns the season that contains m. Northern-hemisphere
// convention: Winter = Dec+Jan+Feb, Spring = Mar+Apr+May, Summer =
// Jun+Jul+Aug, Autumn = Sep+Oct+Nov. Out-of-range values fall through
// to Winter as a safe default so a corrupted Month never panics here.
func SeasonOf(m Month) Season {
	switch m {
	case MonthDecember, MonthJanuary, MonthFebruary:
		return SeasonWinter
	case MonthMarch, MonthApril, MonthMay:
		return SeasonSpring
	case MonthJune, MonthJuly, MonthAugust:
		return SeasonSummer
	case MonthSeptember, MonthOctober, MonthNovember:
		return SeasonAutumn
	}
	return SeasonWinter
}

// GameTime is the decoded calendar position at a specific tick. All
// fields are derived from a Calendar + currentTick pair; they are never
// stored authoritatively on an entity or tile. DayOfMonth is 1-indexed
// to match real-world calendar dates ("1 October") so the UI can show
// the value directly without an off-by-one adjustment.
//
// The zero value GameTime{} has Month = MonthZero and is the documented
// invalid state — a value returned by Calendar.Derive always carries a
// valid Month in [MonthJanuary, MonthDecember].
type GameTime struct {
	Year       int32
	Month      Month
	DayOfMonth int32
	TickOfDay  int32
	Season     Season
}

// Calendar holds the config that maps a tick counter to a GameTime.
// Instances are immutable after construction; callers can share one
// Calendar across goroutines without locking.
//
// ticksPerDay / daysPerMonth / monthsPerYear must be strictly positive;
// NewCalendar panics otherwise so misconfiguration fails fast rather
// than producing nonsense derivations downstream.
//
// epochTickOffset shifts the derivation so currentTick == 0 does not
// necessarily mean "1 January, Year 0". A per-world seed hash jitters
// the offset so two servers running on different seeds don't both call
// today Year 0; purely cosmetic, does not affect gameplay tuning.
type Calendar struct {
	ticksPerDay     int64
	daysPerMonth    int64
	monthsPerYear   int64
	epochTickOffset int64
}

// NewCalendar builds a Calendar with the given cadence and epoch
// offset. Panics if any of the cadence values is non-positive.
func NewCalendar(ticksPerDay, daysPerMonth, monthsPerYear, epochOffset int64) Calendar {
	if ticksPerDay <= 0 || daysPerMonth <= 0 || monthsPerYear <= 0 {
		panic("calendar cadence must be positive")
	}
	return Calendar{
		ticksPerDay:     ticksPerDay,
		daysPerMonth:    daysPerMonth,
		monthsPerYear:   monthsPerYear,
		epochTickOffset: epochOffset,
	}
}

// TicksPerDay returns the number of ticks in one in-game day.
func (c Calendar) TicksPerDay() int64 { return c.ticksPerDay }

// TicksPerMonth returns the number of ticks in one in-game month.
// Matches the granularity at which MonthChangedEvent fires.
func (c Calendar) TicksPerMonth() int64 { return c.ticksPerDay * c.daysPerMonth }

// TicksPerYear returns the number of ticks in one in-game year.
// Matches the granularity at which YearStartedEvent fires.
func (c Calendar) TicksPerYear() int64 {
	return c.ticksPerDay * c.daysPerMonth * c.monthsPerYear
}

// DaysPerMonth returns the configured day count per month.
func (c Calendar) DaysPerMonth() int64 { return c.daysPerMonth }

// MonthsPerYear returns the configured month count per year.
func (c Calendar) MonthsPerYear() int64 { return c.monthsPerYear }

// EpochTickOffset returns the configured epoch offset; zero in tests
// so derivations are easy to read, a seed-derived jitter in production.
func (c Calendar) EpochTickOffset() int64 { return c.epochTickOffset }

// Derive returns the GameTime corresponding to currentTick. Pure
// function of (c, currentTick); deterministic, allocation-free.
//
// Signed arithmetic throughout: a currentTick smaller than the epoch
// offset produces a negative Year, which is the desired behaviour when
// a caller asks "what did the world look like N years before game
// start." Python-style floor division keeps the mapping monotonic
// across negative inputs — Go's native `/` truncates toward zero and
// would introduce a discontinuity at tick 0.
//
// DayOfMonth and Month are 1-indexed for display parity with real-world
// calendar conventions; internal modular arithmetic stays 0-indexed.
func (c Calendar) Derive(currentTick int64) GameTime {
	// Zero-value Calendar guard. NewCalendar panics on non-positive
	// cadence, but a direct `var c Calendar` constructor call bypasses
	// that — return the zero GameTime so callers that accidentally hold
	// a default-valued Calendar never trip a divide-by-zero.
	if c.ticksPerDay == 0 {
		return GameTime{}
	}
	adj := currentTick + c.epochTickOffset
	tickOfDay := floorMod64(adj, c.ticksPerDay)
	totalDays := floorDiv64(adj, c.ticksPerDay)

	dayIdx := floorMod64(totalDays, c.daysPerMonth)
	totalMonths := floorDiv64(totalDays, c.daysPerMonth)

	monthIdx := floorMod64(totalMonths, c.monthsPerYear)
	year := floorDiv64(totalMonths, c.monthsPerYear)

	month := Month(monthIdx + 1)
	return GameTime{
		Year:       int32(year),
		Month:      month,
		DayOfMonth: int32(dayIdx) + 1,
		TickOfDay:  int32(tickOfDay),
		Season:     SeasonOf(month),
	}
}

// DefaultCalendarConfig is the production cadence. Values target ~2
// hours of wall-clock play per in-game year at the project's 10 Hz
// tick rate: 1 minute per day, 10 minutes per month, 12 months per
// year. See `.omc/plans/calendar.md` "Tuning" for the rationale and
// comparison against genre peers.
var DefaultCalendarConfig = struct {
	TicksPerDay   int64
	DaysPerMonth  int64
	MonthsPerYear int64
}{
	TicksPerDay:   600,
	DaysPerMonth:  10,
	MonthsPerYear: 12,
}

// DefaultEpochOffset returns a seed-derived offset in the first 500
// years so two servers with different seeds don't both claim today is
// "1 January, Year 0." Purely cosmetic. Uses splitmix64 on the seed
// for a well-diffused mix — tests that want readable derivations pass
// 0 directly via NewCalendar.
func DefaultEpochOffset(seed int64) int64 {
	h := geom.Splitmix64(uint64(seed) ^ calendarEpochSalt)
	ticksPerYear := DefaultCalendarConfig.TicksPerDay *
		DefaultCalendarConfig.DaysPerMonth *
		DefaultCalendarConfig.MonthsPerYear
	const maxYearsJitter = 500
	return int64(h % uint64(maxYearsJitter*ticksPerYear))
}

// calendarEpochSalt decorrelates the epoch-offset hash stream from the
// worldgen salts (superchunk anchors, region character noise, landmark
// sub-cells, volcano anchors, resource kinds). Distinct 64-bit pattern
// — nothing-up-my-sleeve fractional digits of sqrt(5), mirroring the
// style used in region_source.go. Routed through toInt64-style uint64
// literal so the top-bit pattern survives.
const calendarEpochSalt uint64 = 0x3c6ef372fe94f82b

// floorDiv64 returns the mathematical floor of a/b for positive b. Go
// truncates toward zero; this adjusts the result when a is negative and
// does not divide evenly, so the day/month/year boundaries stay
// monotonic across tick == 0.
func floorDiv64(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

// floorMod64 returns a modulo b with the result in [0, b) for positive
// b, matching Python's % operator. Go's `%` can return negative values
// for negative a, which would produce out-of-range Month indices.
func floorMod64(a, b int64) int64 {
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		r += b
	}
	return r
}

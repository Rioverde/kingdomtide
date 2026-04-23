package game

import "testing"

func TestMonth_Key(t *testing.T) {
	cases := []struct {
		m    Month
		want string
	}{
		{MonthZero, ""},
		{MonthJanuary, "january"},
		{MonthJune, "june"},
		{MonthOctober, "october"},
		{MonthDecember, "december"},
		{Month(99), ""},
	}
	for _, c := range cases {
		if got := c.m.Key(); got != c.want {
			t.Errorf("Month(%d).Key() = %q, want %q", c.m, got, c.want)
		}
		if got := c.m.String(); got != c.want {
			t.Errorf("Month(%d).String() = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestSeason_Key(t *testing.T) {
	cases := []struct {
		s    Season
		want string
	}{
		{SeasonWinter, "winter"},
		{SeasonSpring, "spring"},
		{SeasonSummer, "summer"},
		{SeasonAutumn, "autumn"},
		{Season(99), ""},
	}
	for _, c := range cases {
		if got := c.s.Key(); got != c.want {
			t.Errorf("Season(%d).Key() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestSeasonOf(t *testing.T) {
	cases := []struct {
		m    Month
		want Season
	}{
		{MonthDecember, SeasonWinter},
		{MonthJanuary, SeasonWinter},
		{MonthFebruary, SeasonWinter},
		{MonthMarch, SeasonSpring},
		{MonthApril, SeasonSpring},
		{MonthMay, SeasonSpring},
		{MonthJune, SeasonSummer},
		{MonthJuly, SeasonSummer},
		{MonthAugust, SeasonSummer},
		{MonthSeptember, SeasonAutumn},
		{MonthOctober, SeasonAutumn},
		{MonthNovember, SeasonAutumn},
		{MonthZero, SeasonWinter}, // fallback
		{Month(99), SeasonWinter}, // fallback
	}
	for _, c := range cases {
		if got := SeasonOf(c.m); got != c.want {
			t.Errorf("SeasonOf(%s) = %s, want %s", c.m, got, c.want)
		}
	}
}

func TestNewCalendar_PanicsOnNonPositive(t *testing.T) {
	cases := []struct{ tpd, dpm, mpy int64 }{
		{0, 10, 12},
		{600, 0, 12},
		{600, 10, 0},
		{-1, 10, 12},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("NewCalendar(%d, %d, %d) did not panic", c.tpd, c.dpm, c.mpy)
				}
			}()
			_ = NewCalendar(c.tpd, c.dpm, c.mpy, 0)
		}()
	}
}

func TestCalendar_Derive_Zero(t *testing.T) {
	// Small cadence so boundaries land at easy-to-read ticks.
	cal := NewCalendar(10, 5, 4, 0) // 10 t/day, 5 d/month, 4 m/year = 200 ticks/year

	got := cal.Derive(0)
	want := GameTime{
		Year:       0,
		Month:      MonthJanuary,
		DayOfMonth: 1,
		TickOfDay:  0,
		Season:     SeasonWinter,
	}
	if got != want {
		t.Fatalf("Derive(0) = %+v, want %+v", got, want)
	}
}

func TestCalendar_Derive_Boundaries(t *testing.T) {
	cal := NewCalendar(10, 5, 4, 0) // 10 t/day, 5 d/month, 4 m/year

	cases := []struct {
		tick int64
		want GameTime
	}{
		{0, GameTime{Year: 0, Month: MonthJanuary, DayOfMonth: 1, TickOfDay: 0, Season: SeasonWinter}},
		{9, GameTime{Year: 0, Month: MonthJanuary, DayOfMonth: 1, TickOfDay: 9, Season: SeasonWinter}},
		// Day 2 starts at tick 10 (ticksPerDay = 10)
		{10, GameTime{Year: 0, Month: MonthJanuary, DayOfMonth: 2, TickOfDay: 0, Season: SeasonWinter}},
		// Month boundary: day 6 = month 2 day 1 (daysPerMonth = 5)
		{50, GameTime{Year: 0, Month: MonthFebruary, DayOfMonth: 1, TickOfDay: 0, Season: SeasonWinter}},
		// Quarter-year boundary: month 4 day 1 (monthsPerYear = 4)
		{150, GameTime{Year: 0, Month: MonthApril, DayOfMonth: 1, TickOfDay: 0, Season: SeasonSpring}},
		// Year boundary: tick 200 rolls into year 1, month January
		{200, GameTime{Year: 1, Month: MonthJanuary, DayOfMonth: 1, TickOfDay: 0, Season: SeasonWinter}},
		// Mid-year-2 sample: tick 450 = day 45 = month 10 (9 whole) = year 2 month 2 day 1
		{450, GameTime{Year: 2, Month: MonthFebruary, DayOfMonth: 1, TickOfDay: 0, Season: SeasonWinter}},
	}
	for _, c := range cases {
		got := cal.Derive(c.tick)
		if got != c.want {
			t.Errorf("Derive(%d) = %+v, want %+v", c.tick, got, c.want)
		}
	}
}

func TestCalendar_Derive_NegativeMonotonic(t *testing.T) {
	// Negative tick (prehistory) must produce monotonic negative years
	// without a discontinuity at 0 — Go's truncating division would
	// spike here.
	cal := NewCalendar(10, 5, 4, 0)

	ticks := []int64{-1, -10, -50, -200, -201, -399, -400, -401}
	prev := cal.Derive(ticks[0])
	for _, t1 := range ticks[1:] {
		cur := cal.Derive(t1)
		// Either same year or earlier year than prev; never greater.
		if cur.Year > prev.Year {
			t.Errorf("Derive(%d).Year = %d, not monotonic vs earlier tick", t1, cur.Year)
		}
		prev = cur
	}

	// Specific spot check: tick -1 is the last tick of year -1.
	got := cal.Derive(-1)
	want := GameTime{Year: -1, Month: MonthApril, DayOfMonth: 5, TickOfDay: 9, Season: SeasonSpring}
	if got != want {
		t.Errorf("Derive(-1) = %+v, want %+v", got, want)
	}
}

func TestCalendar_TicksPerYear(t *testing.T) {
	cal := NewCalendar(
		DefaultCalendarConfig.TicksPerDay,
		DefaultCalendarConfig.DaysPerMonth,
		DefaultCalendarConfig.MonthsPerYear,
		0,
	)
	if got, want := cal.TicksPerYear(), int64(600*10*12); got != want {
		t.Errorf("TicksPerYear() = %d, want %d", got, want)
	}
	if got, want := cal.TicksPerMonth(), int64(600*10); got != want {
		t.Errorf("TicksPerMonth() = %d, want %d", got, want)
	}
	if got, want := cal.TicksPerDay(), int64(600); got != want {
		t.Errorf("TicksPerDay() = %d, want %d", got, want)
	}
}

func TestCalendar_PropertyYearAdvance(t *testing.T) {
	// Derive(t + TicksPerYear) should be exactly one year later with
	// identical month/day/tickOfDay.
	cal := NewCalendar(10, 5, 4, 0)
	tpy := cal.TicksPerYear()
	for _, t1 := range []int64{0, 37, 113, 199, -17} {
		a := cal.Derive(t1)
		b := cal.Derive(t1 + tpy)
		if b.Year != a.Year+1 {
			t.Errorf("Derive(%d+tpy).Year = %d, want %d", t1, b.Year, a.Year+1)
		}
		if b.Month != a.Month || b.DayOfMonth != a.DayOfMonth || b.TickOfDay != a.TickOfDay {
			t.Errorf("Derive(%d+tpy) month/day/tick drifted: %+v vs %+v", t1, b, a)
		}
	}
}

func TestDefaultEpochOffset_Deterministic(t *testing.T) {
	// Same seed → same offset across independent calls.
	first := DefaultEpochOffset(42)
	second := DefaultEpochOffset(42)
	if first != second {
		t.Errorf("DefaultEpochOffset(42) non-deterministic: %d vs %d", first, second)
	}
	// Different seeds → different offsets (probabilistic; pick two
	// distant seeds so a chance collision is negligible).
	a := DefaultEpochOffset(1)
	b := DefaultEpochOffset(0xffff)
	if a == b {
		t.Errorf("DefaultEpochOffset(1) == DefaultEpochOffset(0xffff) = %d (unlikely collision)", a)
	}
}

func TestDefaultEpochOffset_WithinRange(t *testing.T) {
	tpy := int64(600 * 10 * 12)
	for _, seed := range []int64{0, 1, 42, 1 << 40, -1} {
		off := DefaultEpochOffset(seed)
		if off < 0 {
			t.Errorf("DefaultEpochOffset(%d) = %d, expected non-negative", seed, off)
		}
		if off >= 500*tpy {
			t.Errorf("DefaultEpochOffset(%d) = %d, exceeds 500-year jitter range", seed, off)
		}
	}
}

func TestFloorDiv64_FloorMod64(t *testing.T) {
	cases := []struct {
		a, b, wantDiv, wantMod int64
	}{
		{10, 3, 3, 1},
		{-10, 3, -4, 2},
		{-1, 3, -1, 2},
		{0, 3, 0, 0},
		{-3, 3, -1, 0},
	}
	for _, c := range cases {
		if d := floorDiv64(c.a, c.b); d != c.wantDiv {
			t.Errorf("floorDiv64(%d, %d) = %d, want %d", c.a, c.b, d, c.wantDiv)
		}
		if m := floorMod64(c.a, c.b); m != c.wantMod {
			t.Errorf("floorMod64(%d, %d) = %d, want %d", c.a, c.b, m, c.wantMod)
		}
	}
}

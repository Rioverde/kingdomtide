package simulation

import (
	"fmt"
	"io"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

// logger writes structured event lines to an io.Writer in the format
// documented in .omc/plans/simulation.md §11. Lines are columnar
// `[year +NNN] event-type details`. Writer can be nil — emit becomes
// a no-op (useful for tests that don't care about logs).
type logger struct {
	w io.Writer
}

// newLogger wraps w; nil w is acceptable.
func newLogger(w io.Writer) *logger {
	return &logger{w: w}
}

// emit writes one event line. event is the dash-separated kind
// (e.g. "camp-died"). details is the human-readable payload.
func (l *logger) emit(year int, event, details string) {
	if l == nil || l.w == nil {
		return
	}
	fmt.Fprintf(l.w, "[year +%03d] %-15s %s\n", year, event, details)
}

// rulerTitle returns the appropriate title prefix per tier so log
// lines read naturally: elder for Camp, chieftain for Hamlet/Village.
func rulerTitle(tier polity.SettlementTier) string {
	switch tier {
	case polity.TierCamp:
		return "elder"
	case polity.TierHamlet, polity.TierVillage:
		return "chieftain"
	}
	return "leader"
}

// describeRuler formats `under <title> '<name>'` for log readability.
// Empty ruler name renders as `under <title> '(unnamed)'`.
func describeRuler(s *polity.Settlement) string {
	name := s.Ruler.Name
	if name == "" {
		name = "(unnamed)"
	}
	return fmt.Sprintf("under %s '%s'", rulerTitle(s.Tier), name)
}

// Option configures Run. See WithLogger, WithYears, WithSnapshotEvery.
type Option func(*runConfig)

type runConfig struct {
	logger        io.Writer
	years         int
	snapshotEvery int
}

func defaultRunConfig() runConfig {
	return runConfig{years: simYears, snapshotEvery: 1}
}

// WithLogger attaches w as the structured-event log sink. Passing nil
// disables logging without error.
func WithLogger(w io.Writer) Option {
	return func(c *runConfig) { c.logger = w }
}

// WithYears overrides the default simulation horizon (simYears).
func WithYears(years int) Option {
	return func(c *runConfig) { c.years = years }
}

// WithSnapshotEvery sets the snapshot cadence: a Snapshot is captured
// every n simulated years. n=1 captures every year; n=0 disables snapshots.
func WithSnapshotEvery(n int) Option {
	return func(c *runConfig) { c.snapshotEvery = n }
}

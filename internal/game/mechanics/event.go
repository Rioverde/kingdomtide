package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Event is the common envelope for every ruler life event and
// natural disaster. Concrete event tables populate slices of Event
// values and feed them through the per-year dispatch in
// mechanics.applyEventTable. EligibleFn filters cities that qualify;
// ModifierFn computes the actual DC from city state; ApplyFn mutates
// city on success.
type Event struct {
	// Name is the English identifier for the event — used in
	// logs and the event ledger. Player-visible localization
	// happens at the client.
	Name string
	// DC is the base difficulty class of the event's d20 check.
	// Actual DC rolled against is DC + ModifierFn(city).
	DC int
	// Natural is true for natural disasters and false for ruler life
	// events. The flag chooses which cascade-cap bucket this event
	// consumes on fire.
	Natural bool
	// EligibleFn filters which cities can fire this event. Nil
	// means every city is eligible.
	EligibleFn func(city *polity.City) bool
	// ModifierFn returns a DC adjustment derived from city state
	// (stats, factions, faith, etc.). Nil means no adjustment.
	ModifierFn func(city *polity.City) int
	// ApplyFn mutates the city when the event fires. Nil means
	// the event has no mechanical effect (narrative only).
	// currentYear carries the simulation year into effect closures
	// so handlers that stamp a timeline value (e.g. assassination
	// DeathYear) can record the actual year rather than derive one
	// from ruler BirthYear + constant.
	ApplyFn func(city *polity.City, stream *dice.Stream, currentYear int)
}

const (
	// CascadeCapNonNatural caps non-natural (ruler life) events at
	// 2 firings per city per year. Prevents runaway compounding and
	// keeps the narrative legible.
	CascadeCapNonNatural = 2
	// CascadeCapNatural caps natural-disaster events at 1 firing
	// per city per year.
	CascadeCapNatural = 1
)

// applyEventTable rolls every event in the table against the
// provided city, honoring the cascade caps above. Helper for
// life-event and disaster tick functions. Fires events in table
// order; each firing consumes a slot in the appropriate cap bucket.
// currentYear threads the simulation year through to each fired
// ApplyFn; handlers that ignore the year simply drop it. Returns
// the total number of events that fired so callers that gate on
// "anything fired this tick" (e.g. the natural-disaster cooldown
// stamp) can branch without re-inspecting city state.
func applyEventTable(
	city *polity.City,
	stream *dice.Stream,
	table []Event,
	currentYear int,
) int {
	nonNatFired := 0
	natFired := 0
	for _, e := range table {
		if e.Natural && natFired >= CascadeCapNatural {
			continue
		}
		if !e.Natural && nonNatFired >= CascadeCapNonNatural {
			continue
		}
		if e.EligibleFn != nil && !e.EligibleFn(city) {
			continue
		}
		dc := e.DC
		if e.ModifierFn != nil {
			dc += e.ModifierFn(city)
		}
		if stream.D20() < dc {
			continue
		}
		if e.ApplyFn != nil {
			e.ApplyFn(city, stream, currentYear)
		}
		if e.Natural {
			natFired++
		} else {
			nonNatFired++
		}
	}
	return nonNatFired + natFired
}

// tick.go implements World.Tick and the supporting machinery for
// turn-resolution: entity ordering, intent dispatch, and energy bookkeeping.
//
// Ordering invariant: within every call to Tick, entities are visited in the
// canonical order (Initiative desc, Speed desc, ID asc). This order is
// computed once per tick from a fresh sorted slice and never changes mid-tick,
// so the sequence of events is fully deterministic given the same starting
// state. Tests that care about ordering must set distinct Initiative or Speed
// values, or rely on lexicographic ID comparison as the final tiebreaker.
//
// One-action cap: each entity executes AT MOST ONE intent per Tick call,
// regardless of how much Energy it has accumulated. Surplus Energy carries
// forward to subsequent ticks. The cap prevents a "burst" scenario where a
// server that missed several ticks would, on recovery, let fast entities take
// a run of actions in a single tick — a bad experience and hard to reason
// about in tests.

package game

import "sort"

// tickEntity is the narrow interface Tick needs from any entity that
// participates in turn resolution. Both *Player and *Monster satisfy it;
// the interface is intentionally small — only the fields Tick actually
// reads or writes are exposed here.
type tickEntity interface {
	tickID() string
	tickSpeed() int
	tickInitiative() int
	tickEnergy() int
	setTickEnergy(int)
	tickIntent() Intent
	setTickIntent(Intent)
}

// tickID returns the player's unique identifier for tick ordering.
func (p *Player) tickID() string { return p.ID }

// tickSpeed returns the player's Speed for Energy accumulation.
func (p *Player) tickSpeed() int { return p.Speed }

// tickInitiative returns the player's Initiative for within-tick ordering.
func (p *Player) tickInitiative() int { return p.Initiative }

// tickEnergy returns the player's current Energy.
func (p *Player) tickEnergy() int { return p.Energy }

// setTickEnergy updates the player's Energy.
func (p *Player) setTickEnergy(e int) { p.Energy = e }

// tickIntent returns the player's pending Intent, or nil when idle.
func (p *Player) tickIntent() Intent { return p.Intent }

// setTickIntent replaces the player's pending Intent. Pass nil to clear.
func (p *Player) setTickIntent(i Intent) { p.Intent = i }

// tickID returns the monster's unique identifier for tick ordering.
func (m *Monster) tickID() string { return m.ID }

// tickSpeed returns the monster's Speed for Energy accumulation.
func (m *Monster) tickSpeed() int { return m.Speed }

// tickInitiative returns the monster's Initiative for within-tick ordering.
func (m *Monster) tickInitiative() int { return m.Initiative }

// tickEnergy returns the monster's current Energy.
func (m *Monster) tickEnergy() int { return m.Energy }

// setTickEnergy updates the monster's Energy.
func (m *Monster) setTickEnergy(e int) { m.Energy = e }

// tickIntent returns the monster's pending Intent, or nil when idle.
func (m *Monster) tickIntent() Intent { return m.Intent }

// setTickIntent replaces the monster's pending Intent. Pass nil to clear.
func (m *Monster) setTickIntent(i Intent) { m.Intent = i }

// mcalcMove returns the energy gain for an entity with the given speed this
// tick, using NetHack-style probabilistic rounding. The fractional part of
// speed/BaseActionCost is handled stochastically: an entity with speed 9
// contributes a guaranteed 0 (since 9 < BaseActionCost and 9%12==9) plus a
// full BaseActionCost bonus with probability 9/12. Over many ticks the mean
// gain equals speed exactly, but the timing is unpredictable — kiting by
// counting "every Nth tick" no longer works. Multiples of BaseActionCost
// (12, 24, …) have zero leftover and are entirely deterministic.
func (w *World) mcalcMove(speed int) int {
	mmove := speed
	leftover := mmove % BaseActionCost
	mmove -= leftover
	if leftover > 0 && w.rng.IntN(BaseActionCost) < leftover {
		mmove += BaseActionCost
	}
	return mmove
}

// Tick advances the world one simulation step.
//
// Every entity (players and monsters) accumulates Speed into Energy.
// Entities are visited in a deterministic order — (Initiative desc, Speed
// desc, ID asc) — so two worlds with identical state produce identical
// event streams. An entity whose Intent is nil or whose Energy has not yet
// reached the intent's Cost simply accumulates and is skipped this tick.
//
// Each visited entity performs AT MOST ONE action per tick: even if Energy
// is several times the Cost, the surplus carries over. This caps "burst"
// behaviour after a lagged tick so a stalled server cannot resolve into a
// flurry of actions on recovery.
//
// On failure (destination blocked, invalid step, ...) the intent slot is
// cleared but Energy is NOT deducted — refund semantics let the entity
// retry a different direction on the very next input. The failure is
// reported via IntentFailedEvent carrying a stable locale-catalog reason
// key; the client renders the player-facing text.
func (w *World) Tick() []Event {
	// Capture the calendar state BEFORE this tick advances the counter,
	// so boundary-change detection compares the new tick against the
	// previous one. When no calendar is wired (ticksPerDay == 0), both
	// samples return the zero-value GameTime and the comparison emits
	// no calendar events — preserving the pre-calendar tick semantics
	// for tests that construct a World without WithCalendar.
	hasCalendar := w.calendar.ticksPerDay != 0
	var prevTime GameTime
	if hasCalendar {
		prevTime = w.calendar.Derive(w.currentTick)
	}
	w.currentTick++

	entities := w.orderedEntities()
	events := make([]Event, 0, len(entities))
	for _, e := range entities {
		// Accumulate and cap in one step. Clamping to BaseActionCost
		// handles two symptoms of unbounded growth: (1) idle sessions,
		// where Energy otherwise grows at Speed × tick-rate forever
		// (e.g. ~140/sec at SpeedNormal, 10 Hz) and surfaces as nonsense
		// like "1056/12" in the stats panel; (2) fast entities
		// (Speed > BaseActionCost) resolving a held key — each successful
		// action deducts Cost, but the next tick's +Speed exceeds it, so
		// Energy drifts up by (Speed - Cost) per tick. The
		// one-action-per-tick cap already forbids cashing in that surplus,
		// so the cap is lossless: "ready" equals exactly one action's
		// worth and the UI progress bar saturates cleanly at Cost.
		e.setTickEnergy(min(e.tickEnergy()+w.mcalcMove(e.tickSpeed()), BaseActionCost))
		intent := e.tickIntent()
		if intent == nil {
			continue
		}
		cost := intent.Cost()
		if e.tickEnergy() < cost {
			continue
		}

		evs, ok := w.resolveIntent(e, intent)
		if !ok {
			events = append(events, evs...)
			e.setTickIntent(nil)
			continue
		}
		events = append(events, evs...)
		e.setTickEnergy(e.tickEnergy() - cost)
		e.setTickIntent(nil)
	}

	// Calendar boundary events — emitted AFTER entity resolution so
	// subscribers see "the tick where month flipped" alongside any
	// movement/intent events from the same tick. Emit order is Month →
	// Season → Year (finest granularity first) so a consumer listening
	// only to YearStartedEvent still sees the annual rollover.
	if hasCalendar {
		curTime := w.calendar.Derive(w.currentTick)
		if curTime.Month != prevTime.Month {
			events = append(events, MonthChangedEvent{
				Month:  curTime.Month,
				Year:   curTime.Year,
				AtTick: w.currentTick,
			})
		}
		if curTime.Season != prevTime.Season {
			events = append(events, SeasonChangedEvent{
				Season: curTime.Season,
				Year:   curTime.Year,
				AtTick: w.currentTick,
			})
		}
		if curTime.Year != prevTime.Year {
			events = append(events, YearStartedEvent{
				Year:   curTime.Year,
				AtTick: w.currentTick,
			})
		}
	}
	return events
}

// EnqueueIntent stores intent as the single-slot pending action for the
// player with the given id. Any previously pending intent on the same
// player is replaced — callers (a UI sending a fresh MoveIntent) do not
// have to check or clear the previous one. Returns ErrPlayerNotFound when
// the player is not currently in the world.
func (w *World) EnqueueIntent(playerID string, intent Intent) error {
	p, ok := w.players[playerID]
	if !ok {
		return ErrPlayerNotFound
	}
	p.Intent = intent
	return nil
}

// orderedEntities returns all players and monsters in the canonical tick
// order: Initiative descending, then Speed descending, then ID ascending.
// The result is a fresh slice — Tick mutates pointer targets (Energy,
// Intent) through the tickEntity interface but never the backing maps.
func (w *World) orderedEntities() []tickEntity {
	out := make([]tickEntity, 0, len(w.players)+len(w.monsters))
	for _, p := range w.players {
		out = append(out, p)
	}
	for _, m := range w.monsters {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.tickInitiative() != b.tickInitiative() {
			return a.tickInitiative() > b.tickInitiative()
		}
		if a.tickSpeed() != b.tickSpeed() {
			return a.tickSpeed() > b.tickSpeed()
		}
		return a.tickID() < b.tickID()
	})
	return out
}

// resolveIntent dispatches an intent to the appropriate domain handler
// and returns the events and success flag. On success (ok == true) the
// world has been mutated and events carries the observable transitions.
// On failure (ok == false) the world is unchanged and events carries a
// single IntentFailedEvent the caller forwards to subscribers.
func (w *World) resolveIntent(e tickEntity, i Intent) ([]Event, bool) {
	switch v := i.(type) {
	case MoveIntent:
		switch entity := e.(type) {
		case *Player:
			return w.applyMoveIntent(entity, v)
		case *Monster:
			return w.applyMonsterMoveIntent(entity, v)
		}
	}
	return []Event{IntentFailedEvent{
		EntityID: e.tickID(),
		Reason:   ReasonIntentMoveInvalid,
	}}, false
}

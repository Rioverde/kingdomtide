package event

import (
	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/geom"
)

// Event is the closed sum type of domain transitions produced by
// ApplyCommand. Events are past-tense facts: a consumer replaying them onto
// an empty world should arrive at the same state as the producer.
type Event interface {
	isEvent()
}

// PlayerJoinedEvent reports a new player entering the world at Position.
type PlayerJoinedEvent struct {
	PlayerID string
	Name     string
	Position geom.Position
}

// PlayerLeftEvent reports a player leaving the world.
type PlayerLeftEvent struct {
	PlayerID string
}

// EntityMovedEvent reports an entity moving from From to To. EntityID is
// deliberately neutral so the same event shape serves monsters once combat
// arrives in later phases.
type EntityMovedEvent struct {
	EntityID string        `json:"entity_id"`
	From     geom.Position `json:"from"`
	To       geom.Position `json:"to"`
}

// Canonical IntentFailedEvent reason codes. The domain emits the key, the
// client resolves it against its locale catalog — the server never ships
// player-facing text. Keep these in lockstep with the locale.KeyErrorIntent*
// constants in internal/ui/locale/keys.go.
const (
	// ReasonIntentMoveBlocked means the destination tile is not passable
	// terrain or is occupied at tick-resolution time.
	ReasonIntentMoveBlocked = "error.intent.move_blocked"

	// ReasonIntentMoveInvalid means the MoveIntent had an illegal shape
	// (zero step, diagonal, or out-of-range delta) by the time the tick
	// tried to resolve it.
	ReasonIntentMoveInvalid = "error.intent.move_invalid"
)

// IntentFailedEvent reports that an entity's pending intent could not be
// resolved at Tick time — destination blocked, target already dead, etc.
// Energy is NOT deducted (refund semantics): the intent slot is cleared so
// the entity becomes idle and its next input is free. Reason is a stable
// locale catalog key (e.g. "error.intent.move_blocked"); the client
// renders the player-facing text via its i18n bundle so the server stays
// language-agnostic.
type IntentFailedEvent struct {
	EntityID string `json:"entity_id"`
	Reason   string `json:"reason"`
}

// MonthChangedEvent fires on the tick where the calendar-derived Month
// differs from the previous tick's Month. The finest-grained calendar
// event; subscribers that want monthly cycling (per-month weather
// rolls, NPC memory decay) wire here.
type MonthChangedEvent struct {
	Month  calendar.Month `json:"month"`
	Year   int32          `json:"year"`
	AtTick int64          `json:"at_tick"`
}

// SeasonChangedEvent fires on the tick where the derived Season differs
// from the previous tick's Season — the four month boundaries per year
// where the season bucket crosses (Feb→Mar, May→Jun, Aug→Sep, Nov→Dec).
// Always accompanied by a MonthChangedEvent on the same tick.
type SeasonChangedEvent struct {
	Season calendar.Season `json:"season"`
	Year   int32           `json:"year"`
	AtTick int64           `json:"at_tick"`
}

// YearStartedEvent fires on the tick where the derived Year advances
// past the previous tick's Year — i.e. the first tick of January.
// Accompanied by MonthChangedEvent and SeasonChangedEvent on the same
// tick; emit order is Month → Season → Year (finest granularity first)
// so a consumer that listens only to YearStartedEvent still sees the
// annual rollover.
type YearStartedEvent struct {
	Year   int32 `json:"year"`
	AtTick int64 `json:"at_tick"`
}

// TimeTickEvent broadcasts the server's authoritative calendar state so
// every client can keep its date HUD in sync without waiting for a full
// snapshot. Fired once every N server ticks (currently every 10 — once
// per wall-clock second at 10 Hz). Carries both the raw tick counter
// and the derived GameTime so the client has zero calendar math to do.
type TimeTickEvent struct {
	CurrentTick int64             `json:"current_tick"`
	GameTime    calendar.GameTime `json:"game_time"`
	AtTick      int64             `json:"at_tick"`
}

func (PlayerJoinedEvent) isEvent()  {}
func (PlayerLeftEvent) isEvent()    {}
func (EntityMovedEvent) isEvent()   {}
func (IntentFailedEvent) isEvent()  {}
func (MonthChangedEvent) isEvent()  {}
func (SeasonChangedEvent) isEvent() {}
func (YearStartedEvent) isEvent()   {}
func (TimeTickEvent) isEvent()      {}

// Compile-time proofs that every concrete event satisfies Event.
var (
	_ Event = PlayerJoinedEvent{}
	_ Event = PlayerLeftEvent{}
	_ Event = EntityMovedEvent{}
	_ Event = IntentFailedEvent{}
	_ Event = MonthChangedEvent{}
	_ Event = SeasonChangedEvent{}
	_ Event = YearStartedEvent{}
	_ Event = TimeTickEvent{}
)

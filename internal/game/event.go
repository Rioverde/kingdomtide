package game

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
	Position Position
}

// PlayerLeftEvent reports a player leaving the world.
type PlayerLeftEvent struct {
	PlayerID string
}

// EntityMovedEvent reports an entity moving from From to To. EntityID is
// deliberately neutral so the same event shape serves monsters once combat
// arrives in later phases.
type EntityMovedEvent struct {
	EntityID string   `json:"entity_id"`
	From     Position `json:"from"`
	To       Position `json:"to"`
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

func (PlayerJoinedEvent) isEvent() {}
func (PlayerLeftEvent) isEvent()   {}
func (EntityMovedEvent) isEvent()  {}
func (IntentFailedEvent) isEvent() {}

// Compile-time proofs that every concrete event satisfies Event.
var (
	_ Event = PlayerJoinedEvent{}
	_ Event = PlayerLeftEvent{}
	_ Event = EntityMovedEvent{}
	_ Event = IntentFailedEvent{}
)

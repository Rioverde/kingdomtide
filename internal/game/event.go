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
	EntityID string
	From, To Position
}

func (PlayerJoinedEvent) isEvent() {}
func (PlayerLeftEvent) isEvent()   {}
func (EntityMovedEvent) isEvent()  {}

// Compile-time proofs that every concrete event satisfies Event.
var (
	_ Event = PlayerJoinedEvent{}
	_ Event = PlayerLeftEvent{}
	_ Event = EntityMovedEvent{}
)

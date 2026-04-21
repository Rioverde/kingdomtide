package game

// Command is the closed sum type of domain intents. Concrete command types
// live in this file and implement the unexported isCommand marker so the set
// of commands is fixed at compile time: no external package can widen it.
type Command interface {
	isCommand()
}

// JoinCmd asks the world to admit a new player. PlayerID is assigned by the
// caller (the server mints a UUID; tests pass any non-empty string) and Name
// is the human-readable label. The domain picks the spawn tile.
type JoinCmd struct {
	PlayerID string
	Name     string
}

// MoveCmd asks the world to move a player one step. Exactly one of DX, DY is
// non-zero and its value is in {-1, +1}; diagonal and zero-length moves are
// rejected by ApplyCommand.
type MoveCmd struct {
	PlayerID string
	DX, DY   int
}

// LeaveCmd asks the world to remove a player, freeing the tile they occupy.
type LeaveCmd struct {
	PlayerID string
}

func (JoinCmd) isCommand()  {}
func (MoveCmd) isCommand()  {}
func (LeaveCmd) isCommand() {}

// Compile-time proofs that every concrete command satisfies Command.
var (
	_ Command = JoinCmd{}
	_ Command = MoveCmd{}
	_ Command = LeaveCmd{}
)

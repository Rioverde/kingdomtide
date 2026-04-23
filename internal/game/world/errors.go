package world

import "errors"

// Sentinel errors returned by ApplyCommand and constructors in this package.
// Callers compare with errors.Is, not string matching. Messages are lowercase
// noun phrases with no trailing punctuation so they compose into log lines.
var (
	// ErrUnknownCommand indicates ApplyCommand received a Command whose concrete
	// type is not handled by the domain.
	ErrUnknownCommand = errors.New("unknown command")

	// ErrPlayerNotFound indicates a command referenced a player ID that is not
	// currently in the world.
	ErrPlayerNotFound = errors.New("player not found")

	// ErrPlayerExists indicates a JoinCmd reused an ID that is already joined.
	ErrPlayerExists = errors.New("player already exists")

	// ErrInvalidMove indicates the shape of a MoveCmd violates the
	// four-directional rule: exactly one of DX/DY is non-zero and its value is
	// in {-1, +1}.
	ErrInvalidMove = errors.New("invalid move")

	// ErrBlocked indicates a movement destination is out of bounds, impassable
	// terrain, or occupied by another entity.
	ErrBlocked = errors.New("destination blocked")

	// ErrInvalidPlayer indicates a Player-construction input failed validation
	// (empty ID or empty Name).
	ErrInvalidPlayer = errors.New("invalid player")

	// ErrNoSpawn indicates a JoinCmd could not find any passable, unoccupied
	// tile to place the new player on.
	ErrNoSpawn = errors.New("no spawn available")
)

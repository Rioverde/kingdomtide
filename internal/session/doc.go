// Package session delivers the Bubble Tea UI over SSH using
// charmbracelet/wish. Every SSH connection opens its own Bubble Tea
// Model wired to a shared server.Service instance — the same service
// that backs the gRPC transport — so SSH and gRPC players inhabit the
// same world and observe the same tick-driven event stream.
//
// The entry point is Handler, which returns the bubbletea middleware
// function Wish expects. Per-session state (subscription, playerID,
// teardown goroutine) is owned by the Bubble Tea Model itself; the
// handler only bridges the ssh.Session into Model construction.
package session

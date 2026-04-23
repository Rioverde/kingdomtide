// Package world holds the pure, in-memory, transport-free rules and state for
// gongeons. It is deliberately transport-free: no I/O, no wall clock, no
// randomness beyond world generation.
//
// # Tick model
//
// World state advances through two distinct entry points that must never be
// mixed:
//
// Lifecycle operations (join, leave) apply immediately and synchronously via
// [World.ApplyCommand]. They are admin-level transitions: a player either
// exists in the world or does not. No energy, no ordering, no tick required.
//
// Gameplay operations (move, and future attack/cast) travel through a
// two-phase pipeline:
//
//  1. [World.EnqueueIntent] stores the intent in the player's single-slot
//     pending field, replacing any unresolved prior intent. The call returns
//     immediately without mutating world position or broadcasting any event.
//
//  2. [World.Tick] advances the simulation one discrete step. Every entity
//     accumulates Energy proportional to its Speed, then — in a deterministic
//     order — executes its pending intent if Energy has reached the action
//     cost. Tick returns the batch of events produced; the server broadcasts
//     them after releasing its mutex.
//
// The server drives the tick loop with a [time.Ticker] at a fixed cadence
// (100 ms by default). Tick is a pure function: identical world state plus
// identical intent sequence produces identical event output, which makes
// determinism tests and replay straightforward.
package world

// Package server is the gRPC implementation of gongeons.v1.GameService.
//
// It owns a single *world.World guarded by a mutex, fans server-authoritative
// events out to every connected stream via Hub, and translates between proto
// messages and domain Commands/Events. The package has no knowledge of the
// terminal UI — clients can be anything that speaks the .proto contract.
package server

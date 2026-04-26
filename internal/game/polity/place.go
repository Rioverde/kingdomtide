package polity

import "github.com/Rioverde/gongeons/internal/game/geom"

// Place is implemented by every concrete settlement tier (Camp, Hamlet,
// Village). It provides a uniform handle for heterogeneous queries — callers
// that only need identity and demographics go through Place rather than
// switching on the concrete type.
type Place interface {
	Base() *Settlement
}

// SettlementSource is the consumer-side interface for querying the living
// settlement layer produced by the fold-forward simulation. Callers
// that need all places in a super-chunk use PlacesIn; tier-specific accessors
// give direct access to the concrete types without a type assertion.
//
// Implementations must be deterministic: the same SuperChunkCoord yields the
// same slice every call (including order), and must be safe for concurrent
// read.
type SettlementSource interface {
	PlacesIn(sc geom.SuperChunkCoord) []Place
	AllCamps() []Camp
	AllHamlets() []Hamlet
	AllVillages() []Village
}

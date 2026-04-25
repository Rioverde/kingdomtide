package world

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Camp is a pre-historic settler cluster — the seed input to the 200-year
// fold-forward simulation. Survivors of the simulation become Cities or
// Villages; non-survivors vanish. Camps are a settlement type alongside
// City and Village, occupying 2-3 connected tiles (small, per
// KINGDOMS.md §2.6 footprint shape). Each Camp inherits a
// RegionCharacter from its Anchor tile's super-chunk and receives a
// Faith via region-weighted deterministic roll. Population ranges from
// 10-50; all camps are founded at simulation start (year 0).
type Camp struct {
	Anchor    geom.Position
	Footprint []geom.Position
	Region    RegionCharacter
	Faith     polity.Faith
	Pop       int32
	// BornYear is the founding year. Camps are generated at year 0 (the
	// simulation start). The fold-forward simulation mutates this on
	// re-foundings and computes age as currentYear - BornYear.
	BornYear int32
}

// CampSource is the consumer-side interface the World delegates to when
// reporting camps inside a super-chunk or across the entire world. The
// interface lives in this package because World consumes it — per Go
// interface-design guidance, interfaces belong at the consumer.
// Implementations live outside (e.g. worldgen.CampSource).
//
// Implementations must be deterministic: the same SuperChunkCoord
// yields the same []Camp every call (including order), and must be
// safe for concurrent read. Returning nil or an empty slice is the
// correct way to signal "no camps in this super-chunk". All() returns
// the full sorted list for diagnostics, dev tools, and tests.
type CampSource interface {
	CampsIn(sc geom.SuperChunkCoord) []Camp
	All() []Camp
}

package worldgen

import (
	"fmt"
	"strings"
)

// WorldSize picks one of five fixed world-dimension presets. Stored as
// uint8 so it fits a wire byte when the server-client handshake starts
// echoing the choice; add new values at the end.
type WorldSize uint8

// World-size enum values. Order is stable — the iota participates in any
// future wire encoding, and reordering shifts every preset-indexed
// lookup downstream.
const (
	WorldSizeTiny     WorldSize = iota // 512x512  — tests, quick iteration
	WorldSizeSmall                     // 1024x1024 — compact
	WorldSizeStandard                  // 2048x2048 — recommended default
	WorldSizeLarge                     // 3072x3072 — spacious
	WorldSizeHuge                      // 4096x4096 — epic
)

// sizePreset carries the full configuration for one world size. Grouping
// width/height/plate-count/erosion-iterations/label/est-kingdoms/gen-seconds
// into a struct keeps AllSizes iteration cheap and adds a single row per
// new preset.
type sizePreset struct {
	width, height     int
	plateCount        int
	erosionIterations int
	label             string
	expectedKingdoms  int
	genSecondsM1Max   int
}

// sizePresets is the indexed-by-enum preset table. New sizes append to
// both this array and AllSizes.
var sizePresets = [...]sizePreset{
	WorldSizeTiny:     {512, 512, 12, 50, "Tiny", 4, 5},
	WorldSizeSmall:    {1024, 1024, 24, 80, "Small", 10, 15},
	WorldSizeStandard: {2048, 2048, 48, 100, "Standard", 20, 45},
	WorldSizeLarge:    {3072, 3072, 72, 100, "Large", 30, 120},
	WorldSizeHuge:     {4096, 4096, 96, 100, "Huge", 40, 300},
}

// Dimensions returns (width, height) in tiles for this size.
func (s WorldSize) Dimensions() (int, int) {
	p := sizePresets[s]
	return p.width, p.height
}

// PlateCount returns the suggested tectonic-plate count for this size.
// Tuned so each plate averages ~22k tiles regardless of world area.
func (s WorldSize) PlateCount() int { return sizePresets[s].plateCount }

// ErosionIterations returns the hydraulic-erosion iteration budget for
// this size. Smaller worlds iterate fewer times because the carving
// signal saturates faster on a smaller grid.
func (s WorldSize) ErosionIterations() int { return sizePresets[s].erosionIterations }

// Label returns the human-readable name ("Tiny", "Standard", ...).
func (s WorldSize) Label() string { return sizePresets[s].label }

// ExpectedKingdoms returns the approximate kingdom count a mature
// history simulation produces on this size. Consumed by menu UIs so
// players can pick the density they want.
func (s WorldSize) ExpectedKingdoms() int { return sizePresets[s].expectedKingdoms }

// EstimatedGenSeconds returns the ballpark wall-clock generation time
// on an M1 Max. Used by menu UIs to set expectations before a user
// commits to a large world. Real numbers live in benchstat once the
// pipeline is populated.
func (s WorldSize) EstimatedGenSeconds() int { return sizePresets[s].genSecondsM1Max }

// String implements fmt.Stringer by returning the label. Matches what
// menu rendering and structured logging want.
func (s WorldSize) String() string { return s.Label() }

// AllSizes returns every world-size value in enum order. Callers that
// want to iterate the menu or run coverage tests use this rather than
// hard-coding the set.
func AllSizes() []WorldSize {
	return []WorldSize{
		WorldSizeTiny, WorldSizeSmall, WorldSizeStandard, WorldSizeLarge, WorldSizeHuge,
	}
}

// ParseWorldSize maps a case-insensitive label ("tiny", "Standard", "") to
// a WorldSize. Empty string defaults to WorldSizeStandard so downstream
// code can treat an unset config field as "user accepted the default".
func ParseWorldSize(s string) (WorldSize, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "tiny":
		return WorldSizeTiny, nil
	case "small":
		return WorldSizeSmall, nil
	case "standard", "":
		return WorldSizeStandard, nil
	case "large":
		return WorldSizeLarge, nil
	case "huge":
		return WorldSizeHuge, nil
	}
	return 0, fmt.Errorf("unknown world size %q (tiny/small/standard/large/huge)", s)
}

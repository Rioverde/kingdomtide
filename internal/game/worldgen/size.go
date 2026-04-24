package worldgen

import (
	"fmt"
	"strings"
)

// WorldSize picks one of five fixed world-dimension presets. Stored
// as uint8 so it fits a wire byte when the server-client handshake
// starts echoing the choice; add new values at the end.
type WorldSize uint8

const (
	WorldSizeTiny     WorldSize = iota // 640x256  — tests, quick iteration
	WorldSizeSmall                     // 1280x512 — compact
	WorldSizeStandard                  // 2560x1024 — recommended default
	WorldSizeLarge                     // 3840x1536 — spacious
	WorldSizeHuge                      // 5120x2048 — epic
)

// sizePreset carries the configuration for one world size.
type sizePreset struct {
	width, height    int
	continentCount   int
	label            string
	expectedKingdoms int
	genSecondsM1Max  int
}

// sizePresets is the indexed-by-enum preset table. Dimensions use a
// 2.5:1 aspect ratio so the world reads as a planet strip rather
// than a square island.
var sizePresets = [...]sizePreset{
	WorldSizeTiny:     {640, 256, 2, "Tiny", 4, 3},
	WorldSizeSmall:    {1280, 512, 2, "Small", 10, 8},
	WorldSizeStandard: {2560, 1024, 3, "Standard", 20, 25},
	WorldSizeLarge:    {3840, 1536, 4, "Large", 30, 70},
	WorldSizeHuge:     {5120, 2048, 5, "Huge", 40, 180},
}

// Dimensions returns (width, height) in tiles for this size.
func (s WorldSize) Dimensions() (int, int) {
	p := sizePresets[s]
	return p.width, p.height
}

// ContinentCount returns the number of major continents the
// generator places for this size.
func (s WorldSize) ContinentCount() int { return sizePresets[s].continentCount }

// Label returns the human-readable name ("Tiny", "Standard", ...).
func (s WorldSize) Label() string { return sizePresets[s].label }

// ExpectedKingdoms returns the approximate kingdom count a mature
// history simulation produces on this size.
func (s WorldSize) ExpectedKingdoms() int { return sizePresets[s].expectedKingdoms }

// EstimatedGenSeconds returns the ballpark wall-clock generation
// time on an M1 Max — used by menu UIs to set expectations.
func (s WorldSize) EstimatedGenSeconds() int { return sizePresets[s].genSecondsM1Max }

// String implements fmt.Stringer by returning the label.
func (s WorldSize) String() string { return s.Label() }

// AllSizes returns every world-size value in enum order.
func AllSizes() []WorldSize {
	return []WorldSize{
		WorldSizeTiny, WorldSizeSmall, WorldSizeStandard, WorldSizeLarge, WorldSizeHuge,
	}
}

// ParseWorldSize maps a case-insensitive label ("tiny", "Standard")
// to a WorldSize. Empty string defaults to WorldSizeStandard.
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

package worldgen

import (
	"fmt"
	"strings"
)

// ContinentPreset picks one of five landmass-layout presets. Decoupled
// from WorldSize so a player can mix "Huge archipelago" with "Tiny
// pangaea" — every combination is valid and produces a coherent world.
type ContinentPreset uint8

// Continent-preset values. Order is stable; the iota value participates
// in any future wire encoding. Append new presets at the end.
const (
	ContinentPangaea    ContinentPreset = iota // 1 supercontinent, minimal ocean
	ContinentTwo                                 // 2 large continents, one major sea
	ContinentTrinity                             // 3 continents, recommended default
	ContinentQuartet                             // 4 smaller continents
	ContinentArchipelago                         // 6+ islands scattered across ocean
)

// continentPreset carries the tuning values the tectonics + sea-level
// stages consume for one layout. Grouping count/sea-fraction/noise-scale
// into a struct keeps the generator hot path map-free.
type continentPreset struct {
	count           int     // number of continent attractor seeds placed
	seaFraction     float32 // target fraction of tiles below sea level
	noiseScale      float64 // overrides the demo-stage noise scale for visual preview
	edgeBias        float32 // how strongly elevation drops near the world edges
	label           string
	description     string
}

// continentPresets is the indexed-by-enum tuning table. Adding a preset
// means appending one row here and one entry in AllContinentPresets.
var continentPresets = [...]continentPreset{
	ContinentPangaea: {
		count:       1,
		seaFraction: 0.30,
		noiseScale:  384.0,
		edgeBias:    0.35,
		label:       "Pangaea",
		description: "1 supercontinent",
	},
	ContinentTwo: {
		count:       2,
		seaFraction: 0.40,
		noiseScale:  288.0,
		edgeBias:    0.30,
		label:       "Two",
		description: "2 continents",
	},
	ContinentTrinity: {
		count:       3,
		seaFraction: 0.42,
		noiseScale:  256.0,
		edgeBias:    0.28,
		label:       "Trinity",
		description: "3 continents",
	},
	ContinentQuartet: {
		count:       4,
		seaFraction: 0.45,
		noiseScale:  224.0,
		edgeBias:    0.25,
		label:       "Quartet",
		description: "4 continents",
	},
	ContinentArchipelago: {
		count:       7,
		seaFraction: 0.60,
		noiseScale:  160.0,
		edgeBias:    0.20,
		label:       "Archipelago",
		description: "many islands",
	},
}

// Count returns the number of continent attractor seeds the tectonics
// stage will place. Consumed by the real pipeline once it lands; until
// then the demo generator uses it only for display.
func (c ContinentPreset) Count() int { return continentPresets[c].count }

// SeaFraction returns the target fraction of tiles below sea level. The
// sealevel stage binary-searches the elevation histogram to land within
// 2% of this value.
func (c ContinentPreset) SeaFraction() float32 { return continentPresets[c].seaFraction }

// NoiseScale returns the elevation-noise base wavelength tuned for this
// layout. Larger scale -> larger continents (Pangaea); smaller scale ->
// more breakup (Archipelago). Consumed by the demo generator today and
// by the real orogeny-noise pass later.
func (c ContinentPreset) NoiseScale() float64 { return continentPresets[c].noiseScale }

// EdgeBias returns the strength of the "pull elevation down near the
// edges" falloff. Higher values keep continents well inside the world
// bounds; lower values let land touch the edges. Anchored to [0, 1].
func (c ContinentPreset) EdgeBias() float32 { return continentPresets[c].edgeBias }

// Label returns the human-readable name ("Pangaea", "Trinity", ...).
func (c ContinentPreset) Label() string { return continentPresets[c].label }

// Description returns a short subtitle suitable for menu UIs.
func (c ContinentPreset) Description() string { return continentPresets[c].description }

// String implements fmt.Stringer.
func (c ContinentPreset) String() string { return c.Label() }

// AllContinentPresets returns every preset in enum order. Used by the
// explorer menu and coverage-style tests so a new preset is exercised
// automatically.
func AllContinentPresets() []ContinentPreset {
	return []ContinentPreset{
		ContinentPangaea, ContinentTwo, ContinentTrinity,
		ContinentQuartet, ContinentArchipelago,
	}
}

// ParseContinentPreset maps a case-insensitive label to a preset.
// Empty string defaults to ContinentTrinity so "--continents=" is
// treated as "accept the recommended default".
func ParseContinentPreset(s string) (ContinentPreset, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "pangaea":
		return ContinentPangaea, nil
	case "two":
		return ContinentTwo, nil
	case "trinity", "":
		return ContinentTrinity, nil
	case "quartet":
		return ContinentQuartet, nil
	case "archipelago":
		return ContinentArchipelago, nil
	}
	return 0, fmt.Errorf("unknown continent preset %q (pangaea/two/trinity/quartet/archipelago)", s)
}

package game

import "fmt"

// cell* constants are single-character codes for the hand-crafted mock
// layout. Keeping the layout rune-based makes the map editable as a block
// of string literals — what you see is what you get.
const (
	cellMountain  = 'M'
	cellOcean     = 'O'
	cellBeach     = 'B'
	cellForest    = 'F'
	cellGrassland = 'G'
	cellHills     = 'H'
	cellMeadow    = 'E'
	cellPlains    = 'P'
)

// mockLayout is the deterministic demo map used by NewMockWorld. Rows must
// all be the same length; paintLayout validates and panics on mismatch. Keep
// it tweakable by hand — this file is the map editor until a procedural
// generator replaces it.
var mockLayout = []string{
	"MMMMMMMMMMMMMMMMMMMM",
	"MFFPPPPGGGGPPPHHHPPM",
	"MFFFPPPGGGGPPPHHHPPM",
	"MFPPPBBBPPPPPPPHPPPM",
	"MPPPBOOOBPPPPPPPPPPM",
	"MPPPBOOOBPPPPPPPPPPM",
	"MPPPPBBBPPPPPEEEEPPM",
	"MPPPPPPPPPPPPEEEEPPM",
	"MPPPPPPPPPPPPPPPPPPM",
	"MMMMMMMMMMMMMMMMMMMM",
}

// terrainForCell maps a layout rune to its domain Terrain. Unknown runes
// resolve to TerrainPlains — a gentler failure mode than panicking when
// somebody mistypes the map.
func terrainForCell(r rune) Terrain {
	switch r {
	case cellMountain:
		return TerrainMountain
	case cellOcean:
		return TerrainOcean
	case cellBeach:
		return TerrainBeach
	case cellForest:
		return TerrainForest
	case cellGrassland:
		return TerrainGrassland
	case cellHills:
		return TerrainHills
	case cellMeadow:
		return TerrainMeadow
	case cellPlains:
		return TerrainPlains
	default:
		return TerrainPlains
	}
}

// NewMockWorld returns a deterministic, hand-crafted demo world. It is the
// temporary stand-in for a future procedural generator — same signature, so
// the server bootstrap won't change the day procgen lands.
func NewMockWorld() *World {
	w, err := newWorldFromLayout(mockLayout)
	if err != nil {
		// The layout is a source-code literal; a mismatch is a programmer
		// error caught at startup, not runtime data corruption.
		panic(fmt.Sprintf("game: bad mock layout: %v", err))
	}
	return w
}

// newWorldFromLayout builds a World whose dimensions are inferred from the
// layout itself (rows = height, rune count of first row = width). All rows
// must be the same rune-length; else a descriptive error is returned so the
// caller can panic with context.
func newWorldFromLayout(layout []string) (*World, error) {
	if len(layout) == 0 {
		return nil, fmt.Errorf("empty layout")
	}
	height := len(layout)
	width := len([]rune(layout[0]))
	if width == 0 {
		return nil, fmt.Errorf("first row has zero width")
	}
	w := NewWorld(width, height)
	if err := paintLayout(w, layout); err != nil {
		return nil, err
	}
	return w, nil
}

// paintLayout writes the terrain indicated by each rune in layout onto w.
// Every row must be exactly w.Width() runes long; every column count must
// equal w.Height().
func paintLayout(w *World, layout []string) error {
	if len(layout) != w.Height() {
		return fmt.Errorf("layout has %d rows, world height is %d", len(layout), w.Height())
	}
	for y, row := range layout {
		runes := []rune(row)
		if len(runes) != w.Width() {
			return fmt.Errorf("row %d has %d runes, world width is %d", y, len(runes), w.Width())
		}
		for x, r := range runes {
			w.SetTerrain(Position{X: x, Y: y}, terrainForCell(r))
		}
	}
	return nil
}

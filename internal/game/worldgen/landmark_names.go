package worldgen

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
)

// landmarkBounds caps PrefixIndex and PatternIndex draws to the number
// of catalog entries present for each landmark kind and region
// character. Both locales carry the same counts (enforced by the
// locale-coverage tests) so a single Bounds record suffices for every
// language. Pattern keys follow the "<domain>.<sub_kind>" shape that
// naming.Generate expects — here the sub_kind is the LandmarkKind.Key()
// value.
var landmarkBounds = naming.Bounds{
	PatternCount: map[string]int{
		"landmark.tower":           3,
		"landmark.giant_tree":      2,
		"landmark.standing_stones": 2,
		"landmark.obelisk":         2,
		"landmark.chasm":           3,
		"landmark.shrine":          3,
	},
	PrefixCount: map[string]int{
		"normal":   5,
		"blighted": 5,
		"fey":      5,
		"ancient":  5,
		"savage":   5,
		"holy":     5,
		"wild":     5,
	},
}

// LandmarkBounds exposes landmarkBounds for the catalog-coverage tests
// in other packages. The returned value is a snapshot; callers must
// not mutate the underlying maps.
func LandmarkBounds() naming.Bounds {
	return landmarkBounds
}

// LandmarkName produces a deterministic structured name for a
// landmark. Same (kind, character, seed, coord) inputs always return
// the same Parts. The returned Parts is stored on world.Landmark and
// composed into a display string by the client via the locale catalog
// under "landmark.name.*" and "landmark.prefix.*" keys.
func LandmarkName(
	kind world.LandmarkKind,
	character world.RegionCharacter,
	seed int64,
	coord geom.Position,
) naming.Parts {
	return naming.Generate(
		naming.Input{
			Domain:    naming.DomainLandmark,
			Character: character.Key(),
			SubKind:   kind.Key(),
			Seed:      seed,
			CoordX:    coord.X,
			CoordY:    coord.Y,
		},
		landmarkBounds,
	)
}

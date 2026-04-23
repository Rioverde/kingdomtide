package ui

import (
	"fmt"
	"math"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/world"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

const (
	// approachRadius is the Chebyshev distance at which the "you approach"
	// log line fires. Chebyshev distance matches 8-directional movement so
	// diagonal approach is treated equally to orthogonal.
	approachRadius = 3

	// approachExitRadius is the Chebyshev distance the player must move
	// beyond before the approach fires again for the same landmark. This
	// debounce ring prevents repeated messages while the player lingers
	// near a landmark.
	approachExitRadius = 5
)

// detectLandmarkApproach scans visible tiles for the nearest landmark within
// approachRadius of the player's current position. On first approach (coord
// differs from m.approached.coord, or m.approached.outside is true) it emits
// a localized event-log line. Uses Chebyshev distance (max of |dx|, |dy|) to
// match 8-neighbor movement.
func (m *Model) detectLandmarkApproach() {
	self, ok := m.selfPlayer()
	if !ok || len(m.tiles) == 0 {
		return
	}

	var nearest *pb.Landmark
	var nearestCoord geom.Position
	nearestDist := math.MaxInt

	for idx, tile := range m.tiles {
		if tile == nil {
			continue
		}
		lm := tile.GetLandmark()
		if lm == nil || lm.GetKind() == pb.LandmarkKind_LANDMARK_KIND_NONE {
			continue
		}
		// Reconstruct world coord from tile index using the viewport origin.
		tx := m.origin.X + (idx % m.width)
		ty := m.origin.Y + (idx / m.width)
		d := chebyshev(self.Pos, geom.Position{X: tx, Y: ty})
		if d < nearestDist {
			nearestDist = d
			nearest = lm
			nearestCoord = geom.Position{X: tx, Y: ty}
		}
	}

	// Player is outside the exit ring: arm the next approach so a later
	// re-entry fires the message again.
	if nearestDist > approachExitRadius {
		m.approached.outside = true
		return
	}

	// Within approach radius: fire if this is a new landmark or re-armed.
	if nearest != nil && nearestDist <= approachRadius {
		isNew := !m.approached.valid || m.approached.coord != nearestCoord
		if isNew || m.approached.outside {
			m.emitApproachLog(nearest)
			m.approached = approachedLandmark{
				coord:   nearestCoord,
				valid:   true,
				outside: false,
			}
		}
	}
}

// emitApproachLog appends a localized approach event-log line for lm.
// The wire LandmarkKind casts directly into world.LandmarkKind because the
// proto enum mirrors the domain enum bit-for-bit (same order, zero is
// None in both). This keeps a single source of truth for the string key:
// world.LandmarkKind.Key().
func (m *Model) emitApproachLog(lm *pb.Landmark) {
	kind := world.LandmarkKind(lm.GetKind())
	kindKey := kind.Key()
	if kindKey == "" {
		return
	}
	name := composeName(naming.DomainLandmark, lm.GetName(), m.lang)
	msg := locale.Tr(m.lang, locale.LandmarkApproachKey(kindKey), "Name", name)
	m.appendLogDefault(fmt.Sprintf("%s %s", LogBullet, msg))
}

// chebyshev returns the Chebyshev distance between two positions, which is
// max(|dx|, |dy|). This matches movement in all 8 directions equally.
func chebyshev(a, b geom.Position) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

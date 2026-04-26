package simulation

import (
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/naming"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// generateRulerName returns a procedural personal name for a ruler.
// Delegates to the canonical naming.GenerateRulerName pipeline.
// Deterministic on (seed, anchor, region).
func generateRulerName(seed int64, anchor geom.Position, region polity.RegionCharacter) string {
	return naming.GenerateRulerName(seed, anchor, region.Key())
}

// generateSettlementName returns a procedural place name for a settlement.
// Delegates to the canonical naming.GenerateSettlementName pipeline.
// Deterministic on (seed, anchor, region).
func generateSettlementName(seed int64, anchor geom.Position, region polity.RegionCharacter) string {
	return naming.GenerateSettlementName(seed, anchor, region.Key())
}

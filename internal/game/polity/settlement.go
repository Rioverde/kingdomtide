package polity

import "github.com/Rioverde/gongeons/internal/game/geom"

// SettlementID is a stable identity assigned at settlement creation.
// Used in the simulation log and dev tool to track a settlement across
// merges (the surviving settlement keeps its ID; absorbed ones are retired).
type SettlementID int64

// SettlementTier identifies the rung of the settlement ladder a settlement
// currently occupies. Tier transitions are emergent — the simulation promotes
// settlements based on population and neighbourhood rules; no caller assigns
// Tier directly.
type SettlementTier uint8

const (
	TierCamp SettlementTier = iota
	TierHamlet
	TierVillage
	TierCity
)

// Settlement is the shared identity and demographic base of any permanent
// human habitation. Holds the fields every place has regardless of political
// status: a name, a world position, a founding year, and a headcount. City,
// Demesne, Camp, Hamlet, and Village embed it via composition so each gets
// these fields (and the Age method) without duplication. The Ruler carries
// the ruler's name in its Name field.
type Settlement struct {
	Name       string        `json:"name"`
	Position   geom.Position `json:"position"`
	Founded    int           `json:"founded"`    // in-game year of founding
	Population int           `json:"population"` // [0, 40 000]

	ID        SettlementID      `json:"id"`
	Tier      SettlementTier    `json:"tier"`
	Footprint []geom.Position   `json:"footprint,omitempty"`
	Region    RegionCharacter   `json:"region"`
	Faiths    FaithDistribution `json:"faiths"`
	Ruler     Ruler             `json:"ruler"`
}

// Age returns the settlement's age in years relative to the simulation's
// current year. Computed from Founded rather than stored so the value
// stays correct as the world clock advances without per-tick bookkeeping.
func (s Settlement) Age(currentYear int) int {
	return currentYear - s.Founded
}

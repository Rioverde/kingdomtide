package polity

import "github.com/Rioverde/gongeons/internal/game/geom"

// Demesne is a minor agricultural / production node feeding food and
// taxes into its parent City. Has no independent political mechanics
// — no Ruler, no Army, no rank system. When the parent City falls,
// the Demesne is absorbed by whichever polity inherits the
// territory; the ParentCityID field tracks the current owner so
// re-parenting is a single write.
type Demesne struct {
	Settlement

	// ParentCityID is the ID of the City this Demesne feeds. Stored as
	// a string rather than a pointer to avoid ownership cycles and keep
	// JSON / ledger serialization trivial.
	ParentCityID string `json:"parent_city_id"`
}

// NewDemesne constructs a Demesne anchored at pos, feeding the given
// parent City. Returns a pointer because Demesne mutates over time —
// Population shifts with harvests, ParentCityID flips when an owning
// City falls — and value semantics would drop those updates. Population
// defaults to zero; the mechanics layer seeds it from surrounding tile
// fertility at worldgen time.
func NewDemesne(name string, pos geom.Position, founded int, parentCityID string) *Demesne {
	return &Demesne{
		Settlement: Settlement{
			Name:     name,
			Position: pos,
			Founded:  founded,
		},
		ParentCityID: parentCityID,
	}
}

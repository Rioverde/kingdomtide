package polity

// Hamlet is a small permanent settlement that emerges when one or more camps
// merge or grow beyond the camp population threshold. Retains the list of
// absorbed camp IDs so the simulation log can trace provenance.
type Hamlet struct {
	Settlement

	// AbsorbedCampIDs lists every camp that was merged into this hamlet,
	// in the order they were absorbed. Used by the simulation log for
	// provenance tracing and by the dev tool timeline scrubber.
	AbsorbedCampIDs []SettlementID `json:"absorbed_camp_ids,omitempty"`
}

// Base returns a pointer to the embedded Settlement, satisfying the Place
// interface.
func (h *Hamlet) Base() *Settlement { return &h.Settlement }

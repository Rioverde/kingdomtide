package polity

// Village is a mature settlement promoted from a hamlet once population
// and neighbourhood conditions are met. Retains the list of absorbed
// hamlet IDs for simulation log provenance.
type Village struct {
	Settlement

	// AbsorbedHamletIDs lists every hamlet that was merged into this village,
	// in the order they were absorbed. Used by the simulation log for
	// provenance tracing and by the dev tool timeline scrubber.
	AbsorbedHamletIDs []SettlementID `json:"absorbed_hamlet_ids,omitempty"`
}

// Base returns a pointer to the embedded Settlement, satisfying the Place
// interface.
func (v *Village) Base() *Settlement { return &v.Settlement }

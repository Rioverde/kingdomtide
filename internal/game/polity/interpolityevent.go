package polity

// InterPolityEventKind enumerates cross-kingdom events per §5e.
// Fired every year by an orchestrator that can see neighbor kingdoms.
// Each event is a value-typed record of what happened so event-log
// consumers can replay / render without inspecting state.
type InterPolityEventKind uint8

const (
	InterPolityRaid InterPolityEventKind = iota
	InterPolitySiege
	InterPolityTradeCompact
	InterPolityAlliance
	InterPolityEspionage
	InterPolityMissionary
	InterPolityTributeDemand
)

// String returns the English name of the event — dev-only; player-
// visible text via client i18n catalog.
func (k InterPolityEventKind) String() string {
	switch k {
	case InterPolityRaid:
		return "Raid"
	case InterPolitySiege:
		return "Siege"
	case InterPolityTradeCompact:
		return "TradeCompact"
	case InterPolityAlliance:
		return "Alliance"
	case InterPolityEspionage:
		return "Espionage"
	case InterPolityMissionary:
		return "Missionary"
	case InterPolityTributeDemand:
		return "TributeDemand"
	default:
		return "UnknownInterPolityEventKind"
	}
}

// InterPolityEvent captures one completed cross-kingdom event: who
// fired it, who received it, which year, which kind. AggressorID and
// TargetID reference Kingdom.ID values; Outcome is one of "success",
// "failure", or "resisted".
type InterPolityEvent struct {
	Kind        InterPolityEventKind `json:"kind"`
	Year        int                  `json:"year"`
	AggressorID string               `json:"aggressor_id"`
	TargetID    string               `json:"target_id"`
	Outcome     string               `json:"outcome"`
}

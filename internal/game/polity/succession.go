package polity

// SuccessionLaw enumerates the six legal succession patterns a
// kingdom can follow. Distributed roughly across cultures — each
// culture's founding defines which law applies. Picking a new
// ruler uses the law plus the current kin-group / faction state.
type SuccessionLaw uint8

const (
	// SuccessionPrimogeniture passes the crown to the eldest child.
	// Typical of Feudal cultures.
	SuccessionPrimogeniture SuccessionLaw = iota
	// SuccessionUltimogeniture passes to the youngest. Steppe.
	SuccessionUltimogeniture
	// SuccessionTanistry elects from the ruling kin group. Celtic.
	SuccessionTanistry
	// SuccessionElective is a faction-weighted vote. Republican.
	SuccessionElective
	// SuccessionDesignated lets the current ruler name the heir.
	// Imperial.
	SuccessionDesignated
	// SuccessionSalic is male-line only primogeniture. Northern Feudal.
	SuccessionSalic
)

// String returns the English dev name for the law — i18n via client.
func (l SuccessionLaw) String() string {
	switch l {
	case SuccessionPrimogeniture:
		return "Primogeniture"
	case SuccessionUltimogeniture:
		return "Ultimogeniture"
	case SuccessionTanistry:
		return "Tanistry"
	case SuccessionElective:
		return "Elective"
	case SuccessionDesignated:
		return "Designated"
	case SuccessionSalic:
		return "Salic"
	default:
		return "UnknownSuccessionLaw"
	}
}

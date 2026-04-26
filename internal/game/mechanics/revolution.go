package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Revolution constants. The happiness ceiling gates eligibility;
// the D20 DC gates firing. Only natural 20s (the current MVP
// threshold) convert eligible cities into actual revolts, which
// matches the roughly 5 % / yr revolt rate for unhappy medieval
// cities observed in historical sources.
const (
	// revolutionHappinessCeiling is the maximum happiness at which a
	// revolt can fire; happy cities never revolt.
	revolutionHappinessCeiling = 60
	// revolutionDC is the D20 roll target. Meeting or exceeding it
	// (natural 20 with no CHA modifier) triggers the revolt.
	revolutionDC = 20
	// revolutionHappinessReset is the post-revolt happiness baseline
	// — a brand-new ruler carries the honeymoon goodwill of overthrow.
	revolutionHappinessReset = 55
	// religionMismatchGrievance is the raw grievance score added when
	// the ruler's faith differs from the city's majority faith.
	// Crossing it lets the revolution fire even for happy cities.
	religionMismatchGrievance = 10
	// religionGrievanceScale maps the minority-status fraction into a
	// 0-20 grievance score. A ruler whose faith holds 0 % of the city
	// scores 20; a ruler who matches the majority scores 0.
	religionGrievanceScale = 20
)

// religionGrievance approximates how much the city population resents
// the ruler's faith. Returns 0 when the ruler's faith is the majority,
// scaling up to religionGrievanceScale when the ruler's faith has zero
// adherents. The grievance feeds the revolution religion-mismatch
// bypass — a high enough score lifts the happiness ceiling so even a
// content city can revolt over a hated faith.
func religionGrievance(city *polity.City) int {
	if city.Faiths.IsZero() {
		return 0
	}
	share := city.Faiths[city.Ruler.Faith]
	return int((1.0 - share) * religionGrievanceScale)
}

// ApplyRevolutionCheckYear rolls the annual revolt check. Sets
// city.RevolutionThisYear at the top of each call (so the flag only
// stays true for the one year a revolt actually fired). If the D20
// gate opens (stream.D20() ≥ revolutionDC) the Ruler is replaced by
// a faction-sponsored new one drawn from the stream, and Happiness
// is reset to the post-revolt baseline.
//
// The happiness ceiling is bypassed when the ruler's faith differs
// from the city's majority faith and the derived religion grievance
// exceeds religionMismatchGrievance — a content city still topples
// a ruler it sees as heretical.
func ApplyRevolutionCheckYear(city *polity.City, stream *dice.Stream, currentYear int) {
	city.RevolutionThisYear = false

	mismatchBypass := false
	if city.Ruler.Faith != city.Faiths.Majority() &&
		religionGrievance(city) > religionMismatchGrievance {
		mismatchBypass = true
	}
	if !mismatchBypass && city.Happiness > revolutionHappinessCeiling {
		return
	}
	effectiveDC := revolutionDC - techRevolutionDCReduction(city)
	if stream.D20() < effectiveDC {
		return
	}

	city.Ruler = polity.NewRuler(stream, currentYear, "")
	city.Happiness = revolutionHappinessReset
	city.RevolutionThisYear = true
}

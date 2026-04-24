package polity

// DecreeKind enumerates the concrete decree types a ruler can issue.
// Each kind has its own trigger eligibility, execution effect, and
// backlash-on-failure pattern. Adding a new decree means adding a new
// enum value plus wiring a constructor in the mechanics.decrees table.
type DecreeKind uint8

const (
	// DecreeRaiseTax bumps the city's tax rate up one tier.
	DecreeRaiseTax DecreeKind = iota
	// DecreeLowerTax drops the tax rate down one tier.
	DecreeLowerTax
	// DecreeRaiseArmy recruits a burst of soldiers beyond baseline.
	DecreeRaiseArmy
	// DecreeBuildFortification adds a durable happiness + army bonus.
	DecreeBuildFortification
	// DecreeFundTradePost adds a trade-score bonus for several years.
	DecreeFundTradePost
	// DecreeCommissionMonument adds a long-lasting happiness bonus —
	// civic pride from a great public work.
	DecreeCommissionMonument

	// Additional decrees — round out the MVP set. Each reflects a
	// distinct political lever a ruler can pull once per year.
	DecreeDeclareStateReligion // promotes ruler's faith to majority
	DecreeInquisition          // suppresses schism + minority faiths
	DecreeTolerationEdict      // reverses Inquisition effect
	DecreeAppointSteward       // reduces decree DC for a decade
	DecreeExpelFaction         // drops a faction's influence to zero
)

// String returns the dev-only English name of the decree kind.
// Player-visible text flows through the client i18n catalog.
func (k DecreeKind) String() string {
	switch k {
	case DecreeRaiseTax:
		return "RaiseTax"
	case DecreeLowerTax:
		return "LowerTax"
	case DecreeRaiseArmy:
		return "RaiseArmy"
	case DecreeBuildFortification:
		return "BuildFortification"
	case DecreeFundTradePost:
		return "FundTradePost"
	case DecreeCommissionMonument:
		return "CommissionMonument"
	case DecreeDeclareStateReligion:
		return "DeclareStateReligion"
	case DecreeInquisition:
		return "Inquisition"
	case DecreeTolerationEdict:
		return "TolerationEdict"
	case DecreeAppointSteward:
		return "AppointSteward"
	case DecreeExpelFaction:
		return "ExpelFaction"
	default:
		return "UnknownDecreeKind"
	}
}

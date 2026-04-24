package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

const (
	// decreeTriggerDC is the D20 target for a ruler to initiate a
	// decree attempt this year. Only about 10 % of years carry a
	// decree attempt, keeping the cascade tractable.
	decreeTriggerDC = 19
	// decreeExecutionDC is the D20 + stat-mod target for the decree to
	// actually succeed. Failure applies a backlash penalty.
	decreeExecutionDC = 15

	// Fortification bonus — army rise + happiness boost, durable.
	fortificationArmyBoost      = 50
	fortificationHappinessBoost = 5
	fortificationDecayYears     = 30

	// Trade-post bonus — trade-score rise, moderate duration.
	tradePostTradeBoost = 15
	tradePostDecayYears = 12

	// Monument — happiness bonus lasting a generation.
	monumentHappinessBoost = 8
	monumentDecayYears     = 40

	// Raise-army immediate burst.
	raiseArmyBurst = 100

	// Decree backlash — happiness hit when execution fails.
	decreeBacklashHappiness  = -8
	decreeBacklashDecayYears = 5

	// State Religion — shifts Faiths so the ruler's faith becomes
	// majority (minimum 0.6 share).
	stateReligionMajorityFloor = 0.6

	// Inquisition — adds a mod that prevents schism for N years by
	// pushing minority faiths down. Records via HistoricalMod kind
	// Happiness (schism-gate suppression lives in religion.go).
	inquisitionHappinessHit = -5
	inquisitionDecayYears   = 10

	// Toleration Edict — positive happiness mod, cancels Inquisition.
	tolerationHappinessBonus = 6
	tolerationDecayYears     = 15

	// Appoint Steward — adds a decadal DC reduction stored as a
	// HistoricalMod of an "AdminEfficiency" kind (reuse Wealth mod
	// for MVP simplicity — admin speeds up wealth accumulation).
	stewardWealthBonus = 50
	stewardDecayYears  = 10

	// Expel Faction — targets the faction whose influence is
	// currently HIGHEST (most disruptive to the ruler's agenda).
	expelFactionReduction = 0.4
)

// decreeChoice picks which decree kind the ruler attempts this year.
// Driven by a D20 bucket so the pick is deterministic but varied
// across the eleven MVP decree kinds. Avoids RaiseTax when tax is
// already Brutal (illegal) and LowerTax when it is already Low.
func decreeChoice(city *polity.City, s *dice.Stream) polity.DecreeKind {
	roll := s.D20()
	switch {
	case roll <= 2:
		if city.TaxRate != polity.TaxBrutal {
			return polity.DecreeRaiseTax
		}
		return polity.DecreeBuildFortification
	case roll <= 4:
		if city.TaxRate != polity.TaxLow {
			return polity.DecreeLowerTax
		}
		return polity.DecreeFundTradePost
	case roll <= 6:
		return polity.DecreeRaiseArmy
	case roll <= 8:
		return polity.DecreeBuildFortification
	case roll <= 10:
		return polity.DecreeFundTradePost
	case roll <= 12:
		return polity.DecreeCommissionMonument
	case roll <= 14:
		return polity.DecreeDeclareStateReligion
	case roll <= 16:
		return polity.DecreeInquisition
	case roll <= 17:
		return polity.DecreeTolerationEdict
	case roll <= 18:
		return polity.DecreeAppointSteward
	default:
		return polity.DecreeExpelFaction
	}
}

// ApplyDecreeYear runs the ruler's annual decree attempt. Uses a
// two-roll D20 pattern — first a trigger roll to decide whether the
// ruler attempts at all, then an execution roll modified by Charisma
// that determines success or backlash. Failed decrees queue a
// happiness-penalty HistoricalMod so the public remembers.
func ApplyDecreeYear(city *polity.City, stream *dice.Stream, currentYear int) {
	if stream.D20() < decreeTriggerDC {
		return
	}

	kind := decreeChoice(city, stream)
	chaMod := stats.Modifier(city.Ruler.Stats.Charisma)

	effectiveDC := decreeExecutionDC - techDecreeDCReduction(city)
	if stream.D20()+chaMod < effectiveDC {
		city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
			Kind:        polity.HistoricalModHappiness,
			Magnitude:   decreeBacklashHappiness,
			YearApplied: currentYear,
			DecayYears:  decreeBacklashDecayYears,
		})
		return
	}

	applyDecreeEffect(city, kind, currentYear)
}

// applyDecreeEffect dispatches to the per-kind mutation. Each branch
// is intentionally small; larger decrees (religion, war) arrive in
// later milestones and get their own branches.
//
// Durable-happiness decrees (Monument, Fortification) skip queuing a
// new mod when an active mod of equal or larger magnitude is already
// on the queue. This keeps a successful-decree pump from overrunning
// the tax-driven mood malus during long horizons — the civic benefit
// of a monument is "one at a time" by design, not "stack forever".
func applyDecreeEffect(city *polity.City, kind polity.DecreeKind, currentYear int) {
	switch kind {
	case polity.DecreeRaiseTax:
		city.TaxRate = raiseTaxTier(city.TaxRate)
	case polity.DecreeLowerTax:
		city.TaxRate = lowerTaxTier(city.TaxRate)
	case polity.DecreeRaiseArmy:
		city.Army += raiseArmyBurst
	case polity.DecreeBuildFortification:
		BuildFortification(city, currentYear)
		city.Army += fortificationArmyBoost
		city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
			Kind:        polity.HistoricalModHappiness,
			Magnitude:   fortificationHappinessBoost,
			YearApplied: currentYear,
			DecayYears:  fortificationDecayYears,
		})
	case polity.DecreeFundTradePost:
		city.TradeScore = min(100, city.TradeScore+tradePostTradeBoost)
	case polity.DecreeCommissionMonument:
		if !hasActiveHappinessMod(city, monumentHappinessBoost, currentYear) {
			city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   monumentHappinessBoost,
				YearApplied: currentYear,
				DecayYears:  monumentDecayYears,
			})
		}
	case polity.DecreeDeclareStateReligion:
		declareStateReligion(city, currentYear)
	case polity.DecreeInquisition:
		startInquisition(city, currentYear)
	case polity.DecreeTolerationEdict:
		startToleration(city, currentYear)
	case polity.DecreeAppointSteward:
		appointSteward(city, currentYear)
	case polity.DecreeExpelFaction:
		expelDominantFaction(city, currentYear)
	}
}

// declareStateReligion forces the ruler's faith to hold at least
// stateReligionMajorityFloor of the distribution; remaining shares are
// divided equally among the other present faiths. Re-normalizes to
// preserve the sum-to-1 invariant.
func declareStateReligion(city *polity.City, currentYear int) {
	_ = currentYear
	if city.Faiths.IsZero() {
		return
	}
	rf := city.Ruler.Faith
	city.Faiths[rf] = stateReligionMajorityFloor
	remaining := 1.0 - stateReligionMajorityFloor
	others := len(polity.AllFaiths()) - 1
	if others > 0 {
		share := remaining / float64(others)
		for _, f := range polity.AllFaiths() {
			if f != rf {
				city.Faiths[f] = share
			}
		}
	}
	city.Faiths.Normalize()
}

// startInquisition queues a happiness penalty for the inquisition
// duration; the schism-gate suppression interplay lives in religion.go.
func startInquisition(city *polity.City, currentYear int) {
	city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
		Kind:        polity.HistoricalModHappiness,
		Magnitude:   inquisitionHappinessHit,
		YearApplied: currentYear,
		DecayYears:  inquisitionDecayYears,
	})
}

// startToleration queues a happiness bonus that canonically reverses
// the Inquisition — the two-mod queue carries both contributions
// simultaneously so the recrystallize step prunes them independently.
func startToleration(city *polity.City, currentYear int) {
	city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
		Kind:        polity.HistoricalModHappiness,
		Magnitude:   tolerationHappinessBonus,
		YearApplied: currentYear,
		DecayYears:  tolerationDecayYears,
	})
}

// appointSteward queues a wealth bonus representing faster wealth
// accumulation under a competent steward — the MVP surrogate for a
// dedicated administrative-efficiency mod kind.
func appointSteward(city *polity.City, currentYear int) {
	city.HistoricalMods = append(city.HistoricalMods, polity.HistoricalMod{
		Kind:        polity.HistoricalModWealth,
		Magnitude:   stewardWealthBonus,
		YearApplied: currentYear,
		DecayYears:  stewardDecayYears,
	})
}

// expelDominantFaction finds the highest-influence faction and
// reduces it by expelFactionReduction (clamped at zero). Ties break
// toward the lower Faction ordinal for determinism.
func expelDominantFaction(city *polity.City, currentYear int) {
	_ = currentYear
	best := polity.FactionMerchants
	bestV := city.Factions.Get(best)
	for f := polity.FactionMerchants; f <= polity.FactionCriminals; f++ {
		if v := city.Factions.Get(f); v > bestV {
			best, bestV = f, v
		}
	}
	city.Factions.Set(best, max(0, bestV-expelFactionReduction))
}

// hasActiveHappinessMod reports whether an active Happiness mod of the
// given magnitude (or greater) is already queued. Used to suppress
// stacking of same-source positive decree mods so a run of lucky
// execution rolls cannot pump happiness arbitrarily high.
func hasActiveHappinessMod(city *polity.City, magnitude, currentYear int) bool {
	for _, m := range city.HistoricalMods {
		if m.Kind == polity.HistoricalModHappiness &&
			m.Magnitude >= magnitude &&
			m.Active(currentYear) {
			return true
		}
	}
	return false
}

// raiseTaxTier steps the tax rate up one tier, clamped at TaxBrutal.
// Tax values are a fixed enum so the step is modeled as a switch
// rather than arithmetic on the underlying uint8.
func raiseTaxTier(r polity.TaxRate) polity.TaxRate {
	switch r {
	case polity.TaxLow:
		return polity.TaxNormal
	case polity.TaxNormal:
		return polity.TaxHigh
	case polity.TaxHigh:
		return polity.TaxBrutal
	default:
		return r
	}
}

// lowerTaxTier steps the tax rate down one tier, clamped at TaxLow.
// Symmetric companion to raiseTaxTier.
func lowerTaxTier(r polity.TaxRate) polity.TaxRate {
	switch r {
	case polity.TaxBrutal:
		return polity.TaxHigh
	case polity.TaxHigh:
		return polity.TaxNormal
	case polity.TaxNormal:
		return polity.TaxLow
	default:
		return r
	}
}

package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// DCs for each inter-polity event. Tuned so raids / missionary
// attempts fire more often than sieges / tribute demands.
const (
	interDCRaid          = 14
	interDCSiege         = 19
	interDCTradeCompact  = 15
	interDCAlliance      = 17
	interDCEspionage     = 16
	interDCMissionary    = 13
	interDCTributeDemand = 18
)

// Effect magnitudes for each inter-polity event. Named constants so
// tuning is centralized and tests can reference the same values.
const (
	raidWealthDrain        = 200
	raidHappinessPenalty   = -5
	raidHappinessDecayYrs  = 5
	siegeArmyLoss          = 100
	siegeEffectiveRankDC   = 15
	tradeCompactTradeBonus = 10
	tradeCompactCeiling    = 100
	allianceHappinessBonus = 5
	allianceDecayYrs       = 10
	espionageWealthSteal   = 150
	espionageArmyReduce    = 50
	missionaryFaithShift   = 0.05
	tributeDemandRate      = 0.15
)

// maxInterPolityActionsPerYear caps diplomatic actions per kingdom per
// year. Matches the cascade rule applied to per-city events so one
// kingdom cannot dominate the simulation by rolling every event on
// every tick.
const maxInterPolityActionsPerYear = 2

// interPolityHistoryCap is the rolling-window size for InterPolityHistory.
// Older entries are trimmed so the slice never grows without bound.
const interPolityHistoryCap = 100

// InterPolityContext is the minimal view a world manager must pass
// to the inter-polity tick: a kingdom, its neighbors, and the city
// registry. The orchestrator decides neighbor topology — this
// function only consumes it.
type InterPolityContext struct {
	Origin    *polity.Kingdom
	Neighbors []*polity.Kingdom
	Cities    map[string]*polity.City
	Stream    *dice.Stream
	Year      int
}

// interPolityKindOrder is the fixed rolling order for the seven §5e
// events. Declared once at package init so ApplyInterPolityEventsYear
// does not allocate a fresh slice on every call.
var interPolityKindOrder = []polity.InterPolityEventKind{
	polity.InterPolityRaid,
	polity.InterPolitySiege,
	polity.InterPolityTradeCompact,
	polity.InterPolityAlliance,
	polity.InterPolityEspionage,
	polity.InterPolityMissionary,
	polity.InterPolityTributeDemand,
}

// ApplyInterPolityEventsYear rolls each of the seven §5e events for
// the origin kingdom against a randomly-selected neighbor. Respects
// the same cascade caps as per-city events: at most
// maxInterPolityActionsPerYear diplomatic actions per year per
// kingdom. Records completed events on the origin kingdom's
// InterPolityHistory field.
func ApplyInterPolityEventsYear(ctx InterPolityContext) {
	if ctx.Origin == nil || !ctx.Origin.Alive() || len(ctx.Neighbors) == 0 {
		return
	}
	fired := 0
	for _, kind := range interPolityKindOrder {
		if fired >= maxInterPolityActionsPerYear {
			break
		}
		if applyOneInterPolityEvent(ctx, kind) {
			fired++
		}
	}
}

// applyOneInterPolityEvent selects a random living neighbor, rolls a
// D20 + ruler CHA modifier against the kind's DC, and either applies
// the effect (success) or records the attempt (failure). Returns true
// when an action was attempted — used for the per-year cap.
func applyOneInterPolityEvent(ctx InterPolityContext, kind polity.InterPolityEventKind) bool {
	// Int63 gives a 63-bit uniform integer; modulo bias over a small
	// neighbor count (≤ ~20) is on the order of 2^-58 — negligible.
	target := ctx.Neighbors[int(ctx.Stream.Int63()%int64(len(ctx.Neighbors)))]
	if target == nil || target == ctx.Origin || !target.Alive() {
		return false
	}

	chaMod := stats.Modifier(ctx.Origin.CurrentRuler.Stats.Charisma)
	dc := interDCForKind(kind)

	if ctx.Stream.D20()+chaMod < dc {
		record(ctx.Origin, kind, target, ctx.Year, "failure")
		return true
	}

	switch kind {
	case polity.InterPolityRaid:
		applyRaid(ctx, target)
	case polity.InterPolitySiege:
		applySiege(ctx, target)
	case polity.InterPolityTradeCompact:
		applyTradeCompact(ctx, target)
	case polity.InterPolityAlliance:
		applyAlliance(ctx, target)
	case polity.InterPolityEspionage:
		applyEspionage(ctx, target)
	case polity.InterPolityMissionary:
		applyMissionary(ctx, target)
	case polity.InterPolityTributeDemand:
		applyTributeDemand(ctx, target)
	}
	record(ctx.Origin, kind, target, ctx.Year, "success")
	return true
}

// interDCForKind maps each event kind to its tuned DC. Keeping the
// lookup in a dedicated helper isolates tuning from the dispatch
// switch and keeps the hot path allocation-free.
func interDCForKind(k polity.InterPolityEventKind) int {
	switch k {
	case polity.InterPolityRaid:
		return interDCRaid
	case polity.InterPolitySiege:
		return interDCSiege
	case polity.InterPolityTradeCompact:
		return interDCTradeCompact
	case polity.InterPolityAlliance:
		return interDCAlliance
	case polity.InterPolityEspionage:
		return interDCEspionage
	case polity.InterPolityMissionary:
		return interDCMissionary
	case polity.InterPolityTributeDemand:
		return interDCTributeDemand
	}
	return 20
}

// applyRaid drains wealth from every city in the target kingdom and
// queues a happiness penalty that decays over raidHappinessDecayYrs.
func applyRaid(ctx InterPolityContext, target *polity.Kingdom) {
	for _, id := range target.CityIDs {
		c, ok := ctx.Cities[id]
		if !ok || c == nil {
			continue
		}
		c.Wealth = max(0, c.Wealth-raidWealthDrain)
		c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
			Kind:        polity.HistoricalModHappiness,
			Magnitude:   raidHappinessPenalty,
			YearApplied: ctx.Year,
			DecayYears:  raidHappinessDecayYrs,
		})
	}
}

// applySiege strikes the target's capital: reduces its army and, on a
// secondary DC check, demotes its EffectiveRank one step toward
// RankIndependent.
func applySiege(ctx InterPolityContext, target *polity.Kingdom) {
	if len(target.CityIDs) == 0 {
		return
	}
	victim, ok := ctx.Cities[target.CityIDs[0]]
	if !ok || victim == nil {
		return
	}
	victim.Army = max(0, victim.Army-siegeArmyLoss)
	if ctx.Stream.D20() >= siegeEffectiveRankDC {
		if victim.EffectiveRank > polity.RankIndependent {
			victim.EffectiveRank = polity.EffectiveRank(int(victim.EffectiveRank) - 1)
		}
	}
}

// applyTradeCompact boosts TradeScore in every city of both kingdoms,
// capped at tradeCompactCeiling.
func applyTradeCompact(ctx InterPolityContext, target *polity.Kingdom) {
	bump := func(k *polity.Kingdom) {
		for _, id := range k.CityIDs {
			c, ok := ctx.Cities[id]
			if !ok || c == nil {
				continue
			}
			c.TradeScore = min(tradeCompactCeiling, c.TradeScore+tradeCompactTradeBonus)
		}
	}
	bump(ctx.Origin)
	bump(target)
}

// applyAlliance queues a decaying happiness bonus on every city of
// both kingdoms. The decay window is longer than a raid's so alliances
// feel stabilizing compared to raids.
func applyAlliance(ctx InterPolityContext, target *polity.Kingdom) {
	addHappyMod := func(k *polity.Kingdom) {
		for _, id := range k.CityIDs {
			c, ok := ctx.Cities[id]
			if !ok || c == nil {
				continue
			}
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   allianceHappinessBonus,
				YearApplied: ctx.Year,
				DecayYears:  allianceDecayYrs,
			})
		}
	}
	addHappyMod(ctx.Origin)
	addHappyMod(target)
}

// applyEspionage steals wealth from the target's capital into the
// origin's capital and weakens the target's garrison. If either side
// has no cities, the attempt is silently dropped.
func applyEspionage(ctx InterPolityContext, target *polity.Kingdom) {
	if len(target.CityIDs) == 0 || len(ctx.Origin.CityIDs) == 0 {
		return
	}
	from, okF := ctx.Cities[target.CityIDs[0]]
	to, okT := ctx.Cities[ctx.Origin.CityIDs[0]]
	if !okF || !okT || from == nil || to == nil {
		return
	}
	steal := min(from.Wealth, espionageWealthSteal)
	from.Wealth -= steal
	to.Wealth += steal
	from.Army = max(0, from.Army-espionageArmyReduce)
}

// applyMissionary shifts every target-kingdom city's faith
// distribution toward the origin ruler's personal faith, renormalizing
// so the distribution invariant holds.
func applyMissionary(ctx InterPolityContext, target *polity.Kingdom) {
	if len(ctx.Origin.CityIDs) == 0 || len(target.CityIDs) == 0 {
		return
	}
	originFaith := ctx.Origin.CurrentRuler.Faith
	for _, id := range target.CityIDs {
		c, ok := ctx.Cities[id]
		if !ok || c == nil || c.Faiths.IsZero() {
			continue
		}
		c.Faiths[originFaith] += missionaryFaithShift
		c.Faiths.Normalize()
	}
}

// applyTributeDemand moves tributeDemandRate of the target's capital
// wealth to the origin's capital. If either capital is missing or
// wealth is non-positive the demand drops silently.
func applyTributeDemand(ctx InterPolityContext, target *polity.Kingdom) {
	if len(target.CityIDs) == 0 || len(ctx.Origin.CityIDs) == 0 {
		return
	}
	from, okF := ctx.Cities[target.CityIDs[0]]
	to, okT := ctx.Cities[ctx.Origin.CityIDs[0]]
	if !okF || !okT || from == nil || to == nil || from.Wealth <= 0 {
		return
	}
	demand := int(float64(from.Wealth) * tributeDemandRate)
	from.Wealth -= demand
	to.Wealth += demand
}

// record appends a completed inter-polity event to the aggressor's
// history and trims to the rolling interPolityHistoryCap so the slice
// never grows without bound. Centralizes the struct literal so every
// dispatcher uses identical field wiring.
func record(k *polity.Kingdom, kind polity.InterPolityEventKind,
	target *polity.Kingdom, year int, outcome string) {
	k.InterPolityHistory = append(k.InterPolityHistory, polity.InterPolityEvent{
		Kind:        kind,
		Year:        year,
		AggressorID: k.ID,
		TargetID:    target.ID,
		Outcome:     outcome,
	})
	if len(k.InterPolityHistory) > interPolityHistoryCap {
		trim := len(k.InterPolityHistory) - interPolityHistoryCap
		k.InterPolityHistory = k.InterPolityHistory[trim:]
	}
}

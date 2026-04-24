package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// Event DCs — chosen to give roughly 10–15 % firing rate on a
// neutral D20 before eligibility filtering. Scandal and Vision are
// lower-DC so they fire more often; Assassination and Succession
// Crisis stay rare at DC 17–18.
const (
	lifeEventDCTournament     = 12
	lifeEventDCAssassination  = 17
	lifeEventDCGreatCouncil   = 13
	lifeEventDCRoyalMarriage  = 15
	lifeEventDCSuccession     = 18
	lifeEventDCScandal        = 11
	lifeEventDCVision         = 11
	lifeEventDCHeroicCampaign = 14
)

// Effect magnitudes for life events. Centralised so balancing the
// cascade reads at a glance.
const (
	tournamentCharismaGain    = 1
	tournamentWealthCost      = 50
	tournamentEligibleWealth  = 100
	assassinationSaveDC       = 15
	greatCouncilRapportDelta  = 0.1
	royalMarriageGoodwill     = 5
	royalMarriageMinCharisma  = 12
	successionCrisisHappiness = -10
	scandalCharismaLoss       = 1
	scandalHappinessPenalty   = -5
	visionWisdomBoost         = 1
	visionWisdomPenalty       = 1
	visionFavorableDieMin     = 4
	heroicStrengthGain        = 1
	heroicCharismaGain        = 1
	heroicArmyLossDivisor     = 4
	heroicCampaignMinArmy     = 20
	statMin                   = 3
	statMax                   = 20
)

// Decay windows for the historical mods queued by life events.
// Chosen to reflect how long each event's civic aftershock lingers
// before fading from public memory. Permanent effects (stat changes,
// ruler death) stay as direct mutations — decay windows apply only
// to reversible civic deltas (Happiness, Wealth, Army).
const (
	// tournamentHappinessBonus is the short-lived civic goodwill from
	// a public festival. Stat gains on the ruler stay permanent; the
	// happiness kick fades over 3 years once the crowd goes home.
	tournamentHappinessBonus = 3
	tournamentDecayYears     = 3

	// greatCouncilHappinessBonus is the goodwill from the council
	// convening. The underlying faction rapport mutation is
	// permanent; only the happiness ripple decays.
	greatCouncilHappinessBonus = 4
	greatCouncilDecayYears     = 8

	// royalMarriageDecayYears is how long the dynastic-alliance
	// goodwill bump lasts.
	royalMarriageDecayYears = 15

	// successionCrisisDecayYears is how long a disputed heir erodes
	// civic mood before the new line is accepted.
	successionCrisisDecayYears = 5

	// scandalDecayYears is how long the rumor-driven mood dent
	// persists. The ruler's CHA loss is permanent; only the public
	// mood ripple decays.
	scandalDecayYears = 8

	// heroicArmyLossDecayYears is how long the campaign's manpower
	// shortfall lingers before conscription fills the gap. Ruler
	// stat gains (STR, CHA) are permanent; only the army delta
	// decays so the city recovers its standing force.
	heroicArmyLossDecayYears = 10
)

// lifeEvents is the ordered life-event table. Each entry is an
// Event the envelope dispatcher rolls during a year. Non-natural
// flag means cascade caps come from the non-natural bucket (max 2
// firings per city per year).
var lifeEvents = []Event{
	tournamentEvent(),
	assassinationEvent(),
	greatCouncilEvent(),
	royalMarriageEvent(),
	successionCrisisEvent(),
	scandalEvent(),
	visionMadnessEvent(),
	heroicCampaignEvent(),
}

// ApplyRulerLifeEventsYear rolls the annual ruler life-event table
// against the city. Up to two non-natural events may fire per year
// under the non-natural cascade cap. Eligibility filters on the
// event itself keep low-wealth or low-army cities from rolling
// irrelevant events. currentYear threads through so handlers that
// stamp a timeline value (assassination DeathYear) can record the
// actual year rather than derive one from ruler BirthYear.
func ApplyRulerLifeEventsYear(city *polity.City, stream *dice.Stream, currentYear int) {
	_ = applyEventTable(city, stream, lifeEvents, currentYear)
}

// tournamentEvent stages a public tournament — the ruler gains
// Charisma at the cost of treasury funding. Requires minimum wealth
// so bankrupt cities cannot host festivities.
//
// Permanent mutations: ruler CHA gain, treasury Wealth cost.
// Decaying mod: short-lived civic happiness bump from the festival.
func tournamentEvent() Event {
	return Event{
		Name:    "Tournament",
		DC:      lifeEventDCTournament,
		Natural: false,
		EligibleFn: func(c *polity.City) bool {
			return c.Wealth > tournamentEligibleWealth
		},
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.Ruler.Stats.Charisma = min(statMax,
				c.Ruler.Stats.Charisma+tournamentCharismaGain)
			c.Wealth -= tournamentWealthCost
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   tournamentHappinessBonus,
				YearApplied: currentYear,
				DecayYears:  tournamentDecayYears,
			})
		},
	}
}

// assassinationEvent rolls a CON save for the ruler against
// assassinationSaveDC. Failure stamps DeathYear with the actual
// current simulation year — the ruler-rotation system treats any
// non-zero DeathYear as a vacancy. Eligibility gates on low
// happiness so content cities are spared the plot.
func assassinationEvent() Event {
	return Event{
		Name:    "Assassination Attempt",
		DC:      lifeEventDCAssassination,
		Natural: false,
		EligibleFn: func(c *polity.City) bool { return c.Happiness < 50 },
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			save := s.D20() + stats.Modifier(c.Ruler.Stats.Constitution)
			if save < assassinationSaveDC {
				c.Ruler.DeathYear = currentYear
			}
		},
	}
}

// greatCouncilEvent convenes the aristocracy — one faction gains
// standing. Target faction picked uniformly over the three
// legitimate blocs (Merchants, Military, Mages) via a D20 bucket.
// Great Council cannot elevate Criminals — political legitimacy
// does not flow from organized crime.
//
// Permanent mutation: faction rapport delta. Decaying mod: civic
// happiness ripple from the council's visible governance.
func greatCouncilEvent() Event {
	return Event{
		Name:    "Great Council",
		DC:      lifeEventDCGreatCouncil,
		Natural: false,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			target := polity.Faction((s.D20() - 1) / 7)
			if target > polity.FactionMages {
				target = polity.FactionMages
			}
			c.Factions.Add(target, greatCouncilRapportDelta)
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   greatCouncilHappinessBonus,
				YearApplied: currentYear,
				DecayYears:  greatCouncilDecayYears,
			})
		},
	}
}

// royalMarriageEvent forges a dynastic alliance. No alliance system
// yet, so the effect manifests as a happiness bump — public-goodwill
// proxy until the diplomatic layer lands. Queued as a decaying mod
// so the honeymoon glow fades as the alliance becomes routine.
func royalMarriageEvent() Event {
	return Event{
		Name:    "Royal Marriage",
		DC:      lifeEventDCRoyalMarriage,
		Natural: false,
		EligibleFn: func(c *polity.City) bool {
			return c.Ruler.Stats.Charisma >= royalMarriageMinCharisma
		},
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   royalMarriageGoodwill,
				YearApplied: currentYear,
				DecayYears:  royalMarriageDecayYears,
			})
		},
	}
}

// successionCrisisEvent represents a disputed heir. Drains happiness
// — when succession rules ship this also flips a "crisis active"
// latch that gates decrees. Queued as a decaying mod so the
// legitimacy crisis eventually resolves once the new line is
// accepted.
func successionCrisisEvent() Event {
	return Event{
		Name:    "Succession Crisis",
		DC:      lifeEventDCSuccession,
		Natural: false,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   successionCrisisHappiness,
				YearApplied: currentYear,
				DecayYears:  successionCrisisDecayYears,
			})
		},
	}
}

// scandalEvent costs the ruler Charisma and dents public mood. Low
// DC so minor scandals ripple through the timeline frequently.
//
// Permanent mutation: ruler CHA loss. Decaying mod: the public
// mood dent fades as the rumor loses its teeth.
func scandalEvent() Event {
	return Event{
		Name:    "Scandal",
		DC:      lifeEventDCScandal,
		Natural: false,
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.Ruler.Stats.Charisma = max(statMin,
				c.Ruler.Stats.Charisma-scandalCharismaLoss)
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModHappiness,
				Magnitude:   scandalHappinessPenalty,
				YearApplied: currentYear,
				DecayYears:  scandalDecayYears,
			})
		},
	}
}

// visionMadnessEvent resolves on a D6 — ≥4 is a prophetic vision
// (Wisdom gain), <4 is the onset of madness (Wisdom loss). Captures
// the "vision or madness" ambivalence in a single entry.
func visionMadnessEvent() Event {
	return Event{
		Name:    "Vision or Madness",
		DC:      lifeEventDCVision,
		Natural: false,
		ApplyFn: func(c *polity.City, s *dice.Stream, _ int) {
			if s.D6() >= visionFavorableDieMin {
				c.Ruler.Stats.Wisdom = min(statMax,
					c.Ruler.Stats.Wisdom+visionWisdomBoost)
				return
			}
			c.Ruler.Stats.Wisdom = max(statMin,
				c.Ruler.Stats.Wisdom-visionWisdomPenalty)
		},
	}
}

// heroicCampaignEvent is a successful military adventure — ruler
// gains STR and CHA but loses a quarter of the army. Requires a
// standing army to campaign with.
//
// Permanent mutations: ruler STR / CHA gains, standing-army
// headcount reduction (simulating dead soldiers). The decaying
// mod layers an additional "empty barracks" drag on the army
// size while the city rebuilds; it fades as conscription refills
// the ranks.
func heroicCampaignEvent() Event {
	return Event{
		Name:    "Heroic Campaign",
		DC:      lifeEventDCHeroicCampaign,
		Natural: false,
		EligibleFn: func(c *polity.City) bool {
			return c.Army > heroicCampaignMinArmy
		},
		ApplyFn: func(c *polity.City, s *dice.Stream, currentYear int) {
			c.Ruler.Stats.Strength = min(statMax,
				c.Ruler.Stats.Strength+heroicStrengthGain)
			c.Ruler.Stats.Charisma = min(statMax,
				c.Ruler.Stats.Charisma+heroicCharismaGain)
			loss := c.Army / heroicArmyLossDivisor
			c.Army = max(0, c.Army-loss)
			c.HistoricalMods = append(c.HistoricalMods, polity.HistoricalMod{
				Kind:        polity.HistoricalModArmy,
				Magnitude:   -loss,
				YearApplied: currentYear,
				DecayYears:  heroicArmyLossDecayYears,
			})
		},
	}
}

package mechanics

import (
	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// Per-subsystem cadences. Each subsystem runs only every N years —
// the natural update rate of the mechanic it drives. Cities retain
// smooth per-year behaviour for hot paths (food, economy, army,
// population, rank, happiness) while slow-moving cultural /
// political signals update at an interval that roughly matches
// their real-world timescale.
const (
	// factionDriftCadence — factions shift over seasons, not days.
	factionDriftCadence = 2

	// religionDiffusionCadence — faith distributions shift slowly;
	// a trade route moves majority share a percent or two across 2-3
	// years, so ticking every year overshoots by 3×.
	religionDiffusionCadence = 3

	// greatPeopleCadence — great-person births are already sparse
	// (1 %/yr); run the check every other year so the expected rate
	// becomes 0.5 %/yr which still produces ~1-2 per century.
	greatPeopleCadence = 1

	// techCadence — innovation fire rate is high; ticking every year
	// is fine to preserve the feel that "scholars are always scribbling"
	techCadence = 1

	// disasterCadence — natural disasters already have their own
	// 10-year cooldown inside ApplyNaturalDisastersYear. Yearly
	// eligibility roll is fine.
	disasterCadence = 1

	// mulkCadence — cultural assimilation is decades-scale. Running
	// every 5 years keeps the qualitative feel (cities slowly drift
	// to conqueror culture) while saving 80 % of the CPU.
	MulkCadence = 5

	// recrystallizeCadence — HistoricalMod decay happens yearly to
	// stay semantically correct (mods must expire on their declared
	// year). Do NOT tier this.
)

// TickCityYear advances one city by a single simulated year. Runs a
// fixed sequence of subsystem updates; unimplemented features are
// noted inline as TODO so the placement is ready when they land.
// Steps are grouped into six logical phases:
//
//  1. Harvest — this year's food balance and soil fatigue, the
//     foundation every downstream balance formula reads.
//  2. Economic baseline — wealth, happiness, and army recompute
//     themselves from current raw state. These recomputes are full
//     assignments, not deltas, so events that mutate Happiness must
//     run AFTER this phase or their writes get overwritten.
//  3. Events — technology, great people, faction drift, religion,
//     ruler life events, natural disasters. These all stack on top
//     of the baselines written in phase 2; their happiness / army /
//     stat mutations persist because the baselines already ran.
//  4. Depletion & growth — mineral drain, population growth, rank
//     auto-derive. Reads the post-event army and economy state.
//  5. Revolution — D20 vs DC 20 gated on the post-event Happiness
//     so disaster-driven mood collapses can actually trigger revolt.
//  6. Composites — prosperity and trade recompute as derived
//     summaries of the year-end raw state.
//
// Determinism: every random draw flows through stream; identical
// (city, stream, year) inputs produce a bit-identical mutation.
// Ordering inside the function is load-bearing — earlier steps
// write fields that later steps read; see individual function docs
// for the dependency graph.
//
// Dependency note: mineral depletion scales with Army size as a
// labor proxy, so it must run AFTER ApplyArmyYear. Placed in
// phase 4 once the phase-2 army baseline is fresh.
//
// Tiered cadence: slow-moving cultural / political subsystems gate
// on currentYear % N == 0 so they run at their natural timescale
// rather than every year. Hot-path subsystems (food, economy, army,
// population, rank, happiness) always run at cadence 1.
func TickCityYear(city *polity.City, stream *dice.Stream, currentYear int) {
	// Early skip: cities below viability AND without a live ruler
	// cannot produce meaningful mutation this year — they represent
	// a ghost town awaiting resettlement or absorption by a neighbor.
	// Clamping to popMin keeps downstream invariants true without
	// running every subsystem for zero state.
	if city.Population < popMin && !city.Ruler.Alive() {
		city.Population = popMin
		return
	}

	// Harvest — roll D6 variance onto FoodBalance, apply any
	// prior-year soil-fatigue penalty.
	ApplyFoodYear(city, stream)
	// Soil fatigue — accumulate or recover based on this year's
	// food balance.
	ApplySoilFatigueYear(city)

	// Wealth update — tax income − army upkeep. Baseline recompute
	// before events so event Wealth deltas persist. currentYear is
	// required so active Wealth HistoricalMods get folded in.
	ApplyEconomicYear(city, currentYear)
	// Happiness update — base + food delta + tax delta + charisma +
	// active Happiness HistoricalMod sum. Full assignment, so runs
	// before events; any event happiness delta (scandal, plague,
	// flood, etc.) queued this year will not appear in today's
	// Happiness but in next year's recompute — by which point
	// Recrystallize has pruned expired mods.
	ApplyHappinessYear(city, currentYear)
	// Army standing — baseline 2 % population, attrition on deficit.
	// Recalculates yearly; placed after economy so attrition reads
	// fresh Wealth.
	ApplyArmyYear(city)

	// Technology — grow Innovation and unlock crossed thresholds.
	// Runs before great people so a newly-arrived Scholar does not
	// double-count on their arrival year.
	if currentYear%techCadence == 0 {
		ApplyTechnologyYear(city, stream)
	}

	// Great people — birth / expiry of the one hosted notable.
	if currentYear%greatPeopleCadence == 0 {
		ApplyGreatPeopleYear(city, stream, currentYear)
	}

	// Faction drift — stochastic ±0.05/yr per bloc, with a peacetime
	// penalty on the Military faction until the war system lands.
	// Cadence 2: factions shift over seasons, not days.
	if currentYear%factionDriftCadence == 0 {
		ApplyFactionDriftYear(city, stream)
	}

	// Religion diffusion — majority-faith self-diffusion pulse with
	// the four-gate schism check. Cadence 3: faith distributions
	// shift slowly; yearly ticking overshoots the real timescale.
	if currentYear%religionDiffusionCadence == 0 {
		ApplyReligionDiffusionYear(city, stream, currentYear)
	}

	// Ruler life events — eight-event table with non-natural cascade
	// cap 2/year. Reads current Ruler, Wealth, Happiness, Army state
	// written earlier in the tick; writes happiness deltas that
	// persist because the baseline already ran. DC-gating makes
	// these rare; cadence 1 preserves yearly eligibility.
	ApplyRulerLifeEventsYear(city, stream, currentYear)

	// Natural disasters — six-disaster table with natural cascade cap
	// 1/year and its own 10-year internal cooldown. Cadence 1 keeps
	// yearly eligibility rolls intact.
	if currentYear%disasterCadence == 0 {
		ApplyNaturalDisastersYear(city, stream, currentYear)
	}

	// Decrees — ruler rolls an annual initiative (DC 19), then an
	// execution roll (DC 15 + CHA) on success the effect applies; on
	// failure a happiness backlash is queued as a HistoricalMod.
	// DC-gating already makes these rare; cadence 1 preserves feel.
	ApplyDecreeYear(city, stream, currentYear)

	// Mineral depletion — reads Army as a labor proxy, so it must
	// run AFTER ApplyArmyYear even though its canonical ordering
	// places it earlier in the year.
	ApplyMineralDepletionYear(city)

	// Growth phase — must come after food, happiness, economy, and
	// army so the logistic growth sees consistent current-year state.
	ApplyPopulationYear(city)

	// BaseRank auto-derive — placed here so rank sees this year's new
	// pop. EffectiveRank (the dominance output) writes later once the
	// kingdom block is implemented.
	ApplyRankYear(city, currentYear)

	// TODO: Kingdom dominance block (every 10 years) — asabiya,
	// dominance compute, rank assignment, tribute collection, collapse
	// check. Requires the Kingdom aggregate.

	// Revolution check — D20 vs DC 20, gated on happiness. Runs last
	// among state mutations so it reads the post-event happiness
	// including any plague / scandal / succession penalties.
	ApplyRevolutionCheckYear(city, stream, currentYear)

	// Recrystallize — prune expired historical mods so the queue
	// stays bounded. Runs after every step that queues mods (life
	// events, disasters) so this year's fresh mods are preserved;
	// runs before the composite finalisers so expired entries no
	// longer pollute next year's inputs. Always cadence 1 — mods
	// must expire on their declared year.
	ApplyRecrystallizeYear(city, currentYear)

	// Finalisers that synthesise year-end derived values from the
	// freshly-written inputs above.
	ApplyProsperityYear(city, currentYear)
	ApplyTradeYear(city)
}

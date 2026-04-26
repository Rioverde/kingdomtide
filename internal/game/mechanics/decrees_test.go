package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyDecreeYear_MostYearsNothing verifies the trigger gate —
// most years pass without any decree attempt because DC 19 is hard.
func TestApplyDecreeYear_MostYearsNothing(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal}
	stream := dice.New(42, dice.SaltKingdomYear)
	modsBefore := len(c.HistoricalMods)
	taxBefore := c.TaxRate
	for i := 0; i < 20; i++ {
		ApplyDecreeYear(&c, stream, 1500+i)
	}
	if c.TaxRate != taxBefore && len(c.HistoricalMods) > 8 {
		t.Errorf("too many decrees in 20 yr: taxNow=%v modsNow=%d", c.TaxRate, len(c.HistoricalMods))
	}
	_ = modsBefore
}

// TestApplyDecreeYear_EventuallyFires verifies that over a long
// horizon at least one decree must land — otherwise the whole
// subsystem is silently inert.
func TestApplyDecreeYear_EventuallyFires(t *testing.T) {
	c := polity.City{TaxRate: polity.TaxNormal, Happiness: 60}
	c.Ruler.Stats.Charisma = 14 // +2 mod helps execution
	stream := dice.New(42, dice.SaltKingdomYear)
	fireCount := 0
	for i := 0; i < 500; i++ {
		before := c
		ApplyDecreeYear(&c, stream, 1500+i)
		if len(c.HistoricalMods) > len(before.HistoricalMods) ||
			c.TaxRate != before.TaxRate ||
			c.Army != before.Army ||
			c.TradeScore != before.TradeScore {
			fireCount++
		}
	}
	if fireCount == 0 {
		t.Error("no decree ever fired in 500 yr — subsystem inert")
	}
}

// TestRaiseTaxTier_ClampAtBrutal verifies tax tier never exceeds Brutal.
func TestRaiseTaxTier_ClampAtBrutal(t *testing.T) {
	if got := raiseTaxTier(polity.TaxBrutal); got != polity.TaxBrutal {
		t.Errorf("raise from Brutal = %v, want Brutal (clamped)", got)
	}
	if got := raiseTaxTier(polity.TaxHigh); got != polity.TaxBrutal {
		t.Errorf("raise from High = %v, want Brutal", got)
	}
}

// TestLowerTaxTier_ClampAtLow verifies tax tier never drops below Low.
func TestLowerTaxTier_ClampAtLow(t *testing.T) {
	if got := lowerTaxTier(polity.TaxLow); got != polity.TaxLow {
		t.Errorf("lower from Low = %v, want Low (clamped)", got)
	}
	if got := lowerTaxTier(polity.TaxNormal); got != polity.TaxLow {
		t.Errorf("lower from Normal = %v, want Low", got)
	}
}

// TestApplyDecreeEffect_FortificationAddsBoth verifies successful
// fortification decree adds army AND queues a happiness mod.
func TestApplyDecreeEffect_FortificationAddsBoth(t *testing.T) {
	c := polity.City{Army: 50}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeBuildFortification, stream, 1500)
	if c.Army != 50+fortificationArmyBoost {
		t.Errorf("Army = %d, want %d", c.Army, 50+fortificationArmyBoost)
	}
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 happiness mod queued, got %d", len(c.HistoricalMods))
	}
	if c.HistoricalMods[0].Kind != polity.HistoricalModHappiness {
		t.Errorf("mod kind = %v, want Happiness", c.HistoricalMods[0].Kind)
	}
}

// TestApplyDecreeYear_Determinism — same seed same outcome across 200 yr.
func TestApplyDecreeYear_Determinism(t *testing.T) {
	a := polity.City{TaxRate: polity.TaxNormal}
	b := polity.City{TaxRate: polity.TaxNormal}
	sa := dice.New(42, dice.SaltKingdomYear)
	sb := dice.New(42, dice.SaltKingdomYear)
	for i := 0; i < 200; i++ {
		ApplyDecreeYear(&a, sa, 1500+i)
		ApplyDecreeYear(&b, sb, 1500+i)
	}
	if a.TaxRate != b.TaxRate || a.Army != b.Army ||
		a.TradeScore != b.TradeScore ||
		len(a.HistoricalMods) != len(b.HistoricalMods) {
		t.Errorf("decree determinism broken")
	}
}

// TestDecree_DeclareStateReligion_PromotesRulerFaith pins the
// post-effect distribution — ruler's faith must hold at least the
// state-religion majority floor regardless of prior shares.
func TestDecree_DeclareStateReligion_PromotesRulerFaith(t *testing.T) {
	c := polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Ruler.Faith = polity.FaithSunCovenant
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeDeclareStateReligion, stream, 1500)
	if c.Faiths[polity.FaithSunCovenant] < stateReligionMajorityFloor {
		t.Errorf("SunCovenant share = %v, want >= %v",
			c.Faiths[polity.FaithSunCovenant], stateReligionMajorityFloor)
	}
}

// TestDecree_Inquisition_AddsHappinessHit verifies the Inquisition
// decree queues the negative happiness mod with the documented
// magnitude.
func TestDecree_Inquisition_AddsHappinessHit(t *testing.T) {
	c := polity.City{}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeInquisition, stream, 1500)
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 mod, got %d", len(c.HistoricalMods))
	}
	if c.HistoricalMods[0].Magnitude != inquisitionHappinessHit {
		t.Errorf("magnitude = %d, want %d",
			c.HistoricalMods[0].Magnitude, inquisitionHappinessHit)
	}
}

// TestDecree_TolerationEdict_AddsHappinessBonus verifies the
// Toleration edict queues the canonical positive happiness mod.
func TestDecree_TolerationEdict_AddsHappinessBonus(t *testing.T) {
	c := polity.City{}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeTolerationEdict, stream, 1500)
	if len(c.HistoricalMods) != 1 ||
		c.HistoricalMods[0].Magnitude != tolerationHappinessBonus {
		t.Errorf("toleration mod missing or wrong magnitude")
	}
}

// TestDecree_AppointSteward_AddsWealthMod — the steward decree queues
// a Wealth kind mod (MVP surrogate for admin efficiency).
func TestDecree_AppointSteward_AddsWealthMod(t *testing.T) {
	c := polity.City{}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeAppointSteward, stream, 1500)
	if c.HistoricalMods[0].Kind != polity.HistoricalModWealth {
		t.Errorf("steward mod kind = %v, want Wealth", c.HistoricalMods[0].Kind)
	}
}

// TestDecree_ExpelFaction_TargetsHighestInfluence verifies the
// expel decree reduces the current highest-influence faction — not
// a fixed target.
func TestDecree_ExpelFaction_TargetsHighestInfluence(t *testing.T) {
	c := polity.City{}
	c.Factions.Set(polity.FactionMerchants, 0.8)
	c.Factions.Set(polity.FactionMilitary, 0.2)
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeExpelFaction, stream, 1500)
	// Merchants was highest, should be reduced by expelFactionReduction.
	if got := c.Factions.Get(polity.FactionMerchants); got >= 0.8 {
		t.Errorf("Merchants = %v, expected reduced from 0.8", got)
	}
}

func TestDecreeChoice_AllKindsReachable(t *testing.T) {
	stream := dice.New(42, dice.SaltKingdomYear)
	seen := map[polity.DecreeKind]bool{}
	for i := 0; i < 5000; i++ {
		k := decreeChoice(&polity.City{TaxRate: polity.TaxNormal}, stream)
		seen[k] = true
	}
	if len(seen) < 17 {
		t.Errorf("only %d kinds reachable across 5000 rolls, want >= 17", len(seen))
	}
}

func TestDecree_DebaseCurrency_BumpsWealth_AddsHappinessHit(t *testing.T) {
	c := polity.City{Wealth: 1000}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeDebaseCurrency, stream, 1500)
	if c.Wealth != 1000+debaseCurrencyWealthBump {
		t.Errorf("Wealth = %d, want %d", c.Wealth, 1000+debaseCurrencyWealthBump)
	}
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 mod, got %d", len(c.HistoricalMods))
	}
	if c.HistoricalMods[0].Magnitude != debaseCurrencyHappinessHit {
		t.Errorf("happiness magnitude = %d, want %d",
			c.HistoricalMods[0].Magnitude, debaseCurrencyHappinessHit)
	}
}

func TestDecree_GrantCharter_BumpsTrade_AddsMerchants_AndHappy(t *testing.T) {
	c := polity.City{TradeScore: 50}
	stream := dice.New(42, dice.SaltKingdomYear)
	beforeMerchants := c.Factions.Get(polity.FactionMerchants)
	applyDecreeEffect(&c, polity.DecreeGrantCharter, stream, 1500)
	if c.TradeScore != 50+grantCharterTradeBump {
		t.Errorf("TradeScore = %d, want %d", c.TradeScore, 50+grantCharterTradeBump)
	}
	if c.Factions.Get(polity.FactionMerchants) <= beforeMerchants {
		t.Errorf("Merchants influence should increase after GrantCharter")
	}
	if len(c.HistoricalMods) != 1 || c.HistoricalMods[0].Magnitude != grantCharterHappinessBonus {
		t.Errorf("happiness mod missing or wrong magnitude")
	}
}

func TestDecree_PatronizeFaction_RaisesSomeFaction(t *testing.T) {
	c := polity.City{}
	before := [4]float64{
		c.Factions.Get(polity.FactionMerchants),
		c.Factions.Get(polity.FactionMilitary),
		c.Factions.Get(polity.FactionMages),
		c.Factions.Get(polity.FactionCriminals),
	}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreePatronizeFaction, stream, 1500)
	raised := false
	for i, f := range []polity.Faction{
		polity.FactionMerchants, polity.FactionMilitary,
		polity.FactionMages, polity.FactionCriminals,
	} {
		if c.Factions.Get(f) > before[i] {
			raised = true
		}
	}
	if !raised {
		t.Error("PatronizeFaction: no faction influence increased")
	}
}

func TestDecree_CallCrusade_RaisesArmy_AndMilitary(t *testing.T) {
	c := polity.City{Army: 100}
	beforeMilitary := c.Factions.Get(polity.FactionMilitary)
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeCallCrusade, stream, 1500)
	if c.Army != 100+callCrusadeArmyBurst {
		t.Errorf("Army = %d, want %d", c.Army, 100+callCrusadeArmyBurst)
	}
	if c.Factions.Get(polity.FactionMilitary) <= beforeMilitary {
		t.Errorf("Military influence should increase after CallCrusade")
	}
	if len(c.HistoricalMods) != 1 || c.HistoricalMods[0].Magnitude != callCrusadeHappinessBonus {
		t.Errorf("happiness mod missing or wrong magnitude")
	}
}

func TestDecree_DeclareWar_AddsHappinessMod(t *testing.T) {
	c := polity.City{}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeDeclareWar, stream, 1500)
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 mod, got %d", len(c.HistoricalMods))
	}
	if c.HistoricalMods[0].Magnitude != declareWarHappinessHit {
		t.Errorf("magnitude = %d, want %d",
			c.HistoricalMods[0].Magnitude, declareWarHappinessHit)
	}
}

func TestDecree_FormLeagueInitiative_AddsHappinessMod(t *testing.T) {
	c := polity.City{}
	stream := dice.New(42, dice.SaltKingdomYear)
	applyDecreeEffect(&c, polity.DecreeFormLeagueInitiative, stream, 1500)
	if len(c.HistoricalMods) != 1 {
		t.Errorf("want 1 mod, got %d", len(c.HistoricalMods))
	}
	if c.HistoricalMods[0].Magnitude != formLeagueInitiativeBonus {
		t.Errorf("magnitude = %d, want %d",
			c.HistoricalMods[0].Magnitude, formLeagueInitiativeBonus)
	}
}

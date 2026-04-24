package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

const testSeed int64 = 42

func newTestStream() *dice.Stream {
	return dice.New(testSeed, dice.SaltKingdomYear)
}

func newTestCity(id string) *polity.City {
	return polity.NewCity(id, geom.Position{}, 1, polity.Ruler{})
}

func TestAttemptFormLeague_HighDCFailsOften(t *testing.T) {
	stream := newTestStream()
	failures := 0
	total := 100
	for i := range total {
		l := AttemptFormLeague(10, stream, "l", "TestLeague", "a", "b", i)
		if l == nil {
			failures++
		}
	}
	// With CHA 10 (modifier 0) against DC 22, we expect most rolls to fail.
	// A D20 only reaches 22 on a natural 20 with modifier 0 — roughly 5%.
	if failures < 80 {
		t.Errorf("expected most formation attempts to fail (DC 22, no CHA bonus); got %d failures out of %d", failures, total)
	}
}

func TestAttemptFormLeague_CharismaHelpsSucceed(t *testing.T) {
	// CHA 20 gives modifier +5; D20+5 meets DC 22 on a roll of 17+, ~20% chance.
	// Over 200 rolls we expect significantly more successes than CHA 10.
	streamLow := dice.New(testSeed, dice.SaltKingdomYear)
	streamHigh := dice.New(testSeed+1, dice.SaltKingdomYear)

	lowSuccesses := 0
	highSuccesses := 0
	total := 200
	for i := range total {
		if AttemptFormLeague(10, streamLow, "l", "L", "a", "b", i) != nil {
			lowSuccesses++
		}
		if AttemptFormLeague(20, streamHigh, "l", "L", "a", "b", i) != nil {
			highSuccesses++
		}
	}
	if highSuccesses <= lowSuccesses {
		t.Errorf("high CHA ruler should succeed more often: low=%d high=%d", lowSuccesses, highSuccesses)
	}
}

func TestAddMember_RespectsMaxMembers(t *testing.T) {
	l := polity.NewLeague("l1", "TestLeague", "city0", "city1", 1)
	for i := 2; i < leagueMaxMembers; i++ {
		id := string(rune('a' + i))
		if !AddMember(l, id) {
			t.Fatalf("expected AddMember to succeed for member %d", i+1)
		}
	}
	if len(l.MemberCityIDs) != leagueMaxMembers {
		t.Fatalf("expected %d members, got %d", leagueMaxMembers, len(l.MemberCityIDs))
	}
	// One more must fail.
	if AddMember(l, "overflow") {
		t.Error("expected AddMember to reject member beyond cap")
	}
}

func TestAddMember_NoDuplicates(t *testing.T) {
	l := polity.NewLeague("l1", "TestLeague", "city0", "city1", 1)
	if AddMember(l, "city0") {
		t.Error("expected AddMember to reject duplicate city ID")
	}
	if len(l.MemberCityIDs) != 2 {
		t.Errorf("expected 2 members after duplicate rejection, got %d", len(l.MemberCityIDs))
	}
}

func TestAddMember_InitializesTrust(t *testing.T) {
	l := polity.NewLeague("l1", "TestLeague", "city0", "city1", 1)
	added := AddMember(l, "city2")
	if !added {
		t.Fatal("expected AddMember to succeed")
	}
	// city2 should have trust entries with both existing members.
	key01 := "city0|city2"
	key12 := "city1|city2"
	if _, ok := l.Trust[key01]; !ok {
		t.Errorf("missing trust key %q after AddMember", key01)
	}
	if _, ok := l.Trust[key12]; !ok {
		t.Errorf("missing trust key %q after AddMember", key12)
	}
}

func TestTickLeagueYear_DriftEvolvesTrust(t *testing.T) {
	l := polity.NewLeague("l1", "TestLeague", "a", "b", 1)
	initial := l.Trust["a|b"]
	cities := map[string]*polity.City{
		"a": newTestCity("a"),
		"b": newTestCity("b"),
	}
	stream := newTestStream()
	TickLeagueYear(l, cities, stream, 2)

	after := l.Trust["a|b"]
	if after == initial {
		t.Error("expected trust to drift after TickLeagueYear; value unchanged")
	}
	if after < 0 || after > 1 {
		t.Errorf("trust out of [0,1] bounds: %f", after)
	}
}

func TestTickLeagueYear_LowTrustMemberLeaves(t *testing.T) {
	// Use a three-member league where a and b distrust each other AND
	// have low trust with c, so their average trust falls below leagueTrustMinToStay.
	// Call purgeLowTrustMembers directly to avoid stochastic drift noise from driftTrust.
	l := polity.NewLeague("l1", "TestLeague", "a", "b", 1)
	_ = AddMember(l, "c")

	// a and b distrust each other and both distrust c — average for each is 0.05.
	l.Trust["a|b"] = 0.05
	l.Trust["a|c"] = 0.05
	l.Trust["b|c"] = 0.05

	cities := map[string]*polity.City{
		"a": newTestCity("a"),
		"b": newTestCity("b"),
		"c": newTestCity("c"),
	}

	purgeLowTrustMembers(l, cities, 2)

	// At least one of a or b should have been purged due to low average trust.
	memberSet := make(map[string]bool, len(l.MemberCityIDs))
	for _, id := range l.MemberCityIDs {
		memberSet[id] = true
	}
	if memberSet["a"] && memberSet["b"] {
		t.Error("expected at least one of 'a' or 'b' to leave the league due to low trust")
	}
}

func TestTickLeagueYear_EmptyLeagueDissolves(t *testing.T) {
	// Start with two members whose trust is critically low so they get purged,
	// leaving the league with <2 members and triggering dissolution.
	l := polity.NewLeague("l1", "TestLeague", "a", "b", 1)
	l.Trust["a|b"] = 0.0

	cities := map[string]*polity.City{
		"a": newTestCity("a"),
		"b": newTestCity("b"),
	}
	stream := newTestStream()
	TickLeagueYear(l, cities, stream, 5)

	if l.Alive() {
		t.Error("expected league with no qualifying members to dissolve")
	}
	if l.Dissolved == 0 {
		t.Error("expected Dissolved year to be set")
	}
}

func TestTickLeagueYear_MembersGetTradeBonus(t *testing.T) {
	l := polity.NewLeague("l1", "TestLeague", "a", "b", 1)
	// Set trust high enough that members stay.
	l.Trust["a|b"] = 0.9

	cityA := newTestCity("a")
	cityB := newTestCity("b")
	cityA.TradeScore = 10
	cityB.TradeScore = 20

	cities := map[string]*polity.City{
		"a": cityA,
		"b": cityB,
	}
	stream := newTestStream()
	TickLeagueYear(l, cities, stream, 2)

	if cityA.TradeScore < 10+leagueTradeBonus {
		t.Errorf("city a: expected TradeScore >= %d, got %d", 10+leagueTradeBonus, cityA.TradeScore)
	}
	if cityB.TradeScore < 20+leagueTradeBonus {
		t.Errorf("city b: expected TradeScore >= %d, got %d", 20+leagueTradeBonus, cityB.TradeScore)
	}
}

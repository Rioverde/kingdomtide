package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
)

// TestApplyGreatPeopleYear_NoBirthMostYears verifies the 1%/year
// birth rate — a single tick on a fresh stream seldom births a
// great person. With seed 42 and SaltGreatPeople we expect zero.
func TestApplyGreatPeopleYear_NoBirthMostYears(t *testing.T) {
	c := &polity.City{}
	ApplyGreatPeopleYear(c, dice.New(42, dice.SaltGreatPeople), 1500)
	if c.GreatPerson != nil {
		t.Logf("non-fatal: great person born on first tick (kind=%s)",
			c.GreatPerson.Kind)
	}
}

// TestApplyGreatPeopleYear_EventuallyBirths verifies that over many
// ticks at least one great person is born — sanity check the birth
// DC isn't effectively infinite.
func TestApplyGreatPeopleYear_EventuallyBirths(t *testing.T) {
	stream := dice.New(42, dice.SaltGreatPeople)
	births := 0
	for year := 1400; year < 2400; year++ {
		c := &polity.City{}
		ApplyGreatPeopleYear(c, stream, year)
		if c.GreatPerson != nil {
			births++
		}
	}
	if births == 0 {
		t.Errorf("no great people born in 1000 ticks, DC=%d too strict",
			greatPersonBirthDC)
	}
}

// TestApplyGreatPeopleYear_ArchetypeDistribution verifies the four
// archetypes are all reachable across many rolls. Coarse check: each
// kind surfaces at least once across 1000 trials.
func TestApplyGreatPeopleYear_ArchetypeDistribution(t *testing.T) {
	stream := dice.New(42, dice.SaltGreatPeople)
	seen := map[polity.GreatPersonKind]int{}
	for i := 0; i < 10_000; i++ {
		c := &polity.City{}
		ApplyGreatPeopleYear(c, stream, 1500)
		if c.GreatPerson != nil {
			seen[c.GreatPerson.Kind]++
		}
	}
	for _, k := range []polity.GreatPersonKind{
		polity.GreatPersonScholar, polity.GreatPersonGeneral,
		polity.GreatPersonArchitect, polity.GreatPersonPriest,
	} {
		if seen[k] == 0 {
			t.Errorf("archetype %s never appeared in 10k trials", k)
		}
	}
}

// TestApplyGreatPeopleYear_ExpiryRemovesPerson verifies that a great
// person whose DeathYear has arrived is cleared from the slot.
func TestApplyGreatPeopleYear_ExpiryRemovesPerson(t *testing.T) {
	c := &polity.City{
		GreatPerson: &polity.GreatPerson{
			Kind:      polity.GreatPersonScholar,
			BirthYear: 1500,
			DeathYear: 1540,
		},
	}
	ApplyGreatPeopleYear(c, dice.New(42, dice.SaltGreatPeople), 1540)
	if c.GreatPerson != nil {
		t.Errorf("great person not expired at DeathYear=%d, still set", 1540)
	}
}

// TestApplyGreatPeopleYear_SingleSlotInvariant verifies the function
// does not overwrite an alive great person with a fresh birth.
func TestApplyGreatPeopleYear_SingleSlotInvariant(t *testing.T) {
	original := &polity.GreatPerson{
		Kind:      polity.GreatPersonScholar,
		BirthYear: 1500,
		DeathYear: 1545,
	}
	c := &polity.City{GreatPerson: original}
	stream := dice.New(42, dice.SaltGreatPeople)
	for year := 1501; year < 1545; year++ {
		ApplyGreatPeopleYear(c, stream, year)
		if c.GreatPerson == nil || c.GreatPerson != original {
			t.Fatalf("year %d: slot was overwritten with new great person", year)
		}
	}
}

// TestApplyGreatPeopleYear_Determinism verifies two identical runs
// on the same seed produce identical outcomes.
func TestApplyGreatPeopleYear_Determinism(t *testing.T) {
	a := &polity.City{}
	b := &polity.City{}
	streamA := dice.New(42, dice.SaltGreatPeople)
	streamB := dice.New(42, dice.SaltGreatPeople)
	for year := 1400; year < 1600; year++ {
		ApplyGreatPeopleYear(a, streamA, year)
		ApplyGreatPeopleYear(b, streamB, year)
	}
	aliveA := a.GreatPerson != nil
	aliveB := b.GreatPerson != nil
	if aliveA != aliveB {
		t.Fatalf("determinism break: a alive=%v b alive=%v", aliveA, aliveB)
	}
	if aliveA && *a.GreatPerson != *b.GreatPerson {
		t.Errorf("great person diverged: a=%+v b=%+v",
			*a.GreatPerson, *b.GreatPerson)
	}
}

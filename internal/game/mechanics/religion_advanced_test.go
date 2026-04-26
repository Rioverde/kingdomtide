package mechanics

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/dice"
	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// mismatchCity builds a happy city with a ruler whose personal faith
// holds zero share against a SunCovenant majority. religionGrievance
// returns religionGrievanceScale (20) — well above the bypass
// threshold (10) — so the revolution check's happiness ceiling is
// lifted. Happiness sits at 80 (above the 60 ceiling) so any revolt
// that fires must come from the mismatch bypass.
func mismatchCity() *polity.City {
	c := &polity.City{
		Happiness: 80,
		Settlement: polity.Settlement{
			Faiths: polity.NewFaithDistribution(),
			Ruler: polity.Ruler{
				Stats: stats.CoreStats{
					Strength: 10, Dexterity: 10, Constitution: 10,
					Intelligence: 10, Wisdom: 10, Charisma: 10,
				},
				Faith: polity.FaithOneOath,
			},
		},
	}
	// Crowd the ruler's faith to zero; SunCovenant becomes majority.
	c.Faiths[polity.FaithOldGods] = 0
	c.Faiths[polity.FaithSunCovenant] = 1.0
	c.Faiths[polity.FaithGreenSage] = 0
	c.Faiths[polity.FaithOneOath] = 0
	c.Faiths[polity.FaithStormPact] = 0
	c.Faiths.Normalize()
	return c
}

// TestRevolution_ReligionMismatchBypassesHappinessCeiling verifies a
// happy city whose ruler follows a minority faith can still revolt
// because religion grievance lifts the ceiling gate. Over 500 rolls
// the natural-20 firing rate should produce at least one revolt.
func TestRevolution_ReligionMismatchBypassesHappinessCeiling(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	for year := 0; year < 500; year++ {
		c := mismatchCity()
		ApplyRevolutionCheckYear(c, stream, year)
		if c.RevolutionThisYear {
			return // pass on first mismatch-driven revolt
		}
	}
	t.Fatal("500 years of religion-mismatch produced no revolts — bypass wiring broken")
}

// TestRevolution_RulerSameFaithNoBypass verifies that when the
// ruler's faith matches the city's majority, the happiness ceiling
// still applies even at mood 80 — no bypass, no revolt.
func TestRevolution_RulerSameFaithNoBypass(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	for year := 0; year < 200; year++ {
		c := &polity.City{
			Happiness: 80,
			Settlement: polity.Settlement{
				Faiths: polity.NewFaithDistribution(),
				Ruler: polity.Ruler{
					Stats: stats.CoreStats{Charisma: 10},
					Faith: polity.FaithOldGods,
				},
			},
		}
		ApplyRevolutionCheckYear(c, stream, year)
		if c.RevolutionThisYear {
			t.Fatalf("year %d: same-faith happy city revolted", year)
		}
	}
}

// TestRevolution_HighHappinessNoMismatchNoBypass verifies the
// belt-and-suspenders case: high happiness, faiths nil, ruler
// zero-value — the faith check short-circuits (no mismatch) and the
// ceiling blocks the revolt.
func TestRevolution_HighHappinessNoMismatchNoBypass(t *testing.T) {
	stream := dice.New(42, dice.SaltRevolutions)
	c := &polity.City{Happiness: 100}
	for year := 0; year < 200; year++ {
		ApplyRevolutionCheckYear(c, stream, year)
		if c.RevolutionThisYear {
			t.Fatalf("year %d: ceiling-protected city revolted", year)
		}
	}
}

// TestReligionGrievance_ScalesWithMinorityStatus verifies grievance
// climbs from 0 (ruler matches majority) up to religionGrievanceScale
// (ruler's faith has zero adherents). Tests the monotonic scaling the
// bypass condition relies on.
func TestReligionGrievance_ScalesWithMinorityStatus(t *testing.T) {
	cases := []struct {
		name  string
		share float64
		want  int
	}{
		{"full-majority", 1.0, 0},
		{"half-and-half", 0.5, 10},
		{"tenth-share", 0.1, 18},
		{"zero-share", 0.0, religionGrievanceScale},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &polity.City{
				Settlement: polity.Settlement{
					Faiths: polity.NewFaithDistribution(),
					Ruler:  polity.Ruler{Faith: polity.FaithOneOath},
				},
			}
			for _, f := range polity.AllFaiths() {
				c.Faiths[f] = 0
			}
			c.Faiths[polity.FaithOneOath] = tc.share
			c.Faiths[polity.FaithOldGods] = 1.0 - tc.share
			got := religionGrievance(c)
			if got != tc.want {
				t.Errorf("share=%v: grievance=%d, want %d",
					tc.share, got, tc.want)
			}
		})
	}
}

// TestSchism_RecordsInHistory verifies a successful schism appends a
// SchismEvent to city.FaithHistory with the current year and the
// original majority / new secondary captured. Re-uses the four-gate
// setup from religion_test.go's SchismFires case.
func TestSchism_RecordsInHistory(t *testing.T) {
	c := &polity.City{Settlement: polity.Settlement{Faiths: polity.NewFaithDistribution()}}
	c.Faiths[polity.FaithOldGods] = 0.55
	c.Faiths[polity.FaithSunCovenant] = 0.45
	c.Faiths[polity.FaithGreenSage] = 0
	c.Faiths[polity.FaithOneOath] = 0
	c.Faiths[polity.FaithStormPact] = 0
	c.Innovation = 50

	const schismYear = 1340
	ApplyReligionDiffusionYear(c, dice.New(42, dice.SaltReligion), schismYear)

	if len(c.FaithHistory) != 1 {
		t.Fatalf("FaithHistory length = %d, want 1", len(c.FaithHistory))
	}
	ev := c.FaithHistory[0]
	if ev.Year != schismYear {
		t.Errorf("SchismEvent.Year = %d, want %d", ev.Year, schismYear)
	}
	if ev.OriginalMajority != polity.FaithOldGods {
		t.Errorf("SchismEvent.OriginalMajority = %v, want OldGods",
			ev.OriginalMajority)
	}
	if ev.NewSecondary != polity.FaithSunCovenant {
		t.Errorf("SchismEvent.NewSecondary = %v, want SunCovenant",
			ev.NewSecondary)
	}
}

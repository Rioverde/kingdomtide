package simulation

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
)

func TestLogger_FormatsYearAndEvent(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf)
	l.emit(42, "camp-died", "test details")
	got := buf.String()
	if !strings.HasPrefix(got, "[year +042] camp-died") {
		t.Errorf("unexpected log line: %q", got)
	}
	if !strings.HasSuffix(got, "test details\n") {
		t.Errorf("expected trailing details + newline, got %q", got)
	}
}

func TestLogger_NilWriterIsNoOp(t *testing.T) {
	l := newLogger(nil)
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("expected no panic on nil writer, got %v", r)
		}
	}()
	l.emit(0, "x", "y")
}

func TestLogger_NilLoggerIsNoOp(t *testing.T) {
	var l *logger // nil pointer
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("expected no panic on nil logger, got %v", r)
		}
	}()
	l.emit(0, "x", "y")
}

func TestRulerTitle_Tiers(t *testing.T) {
	cases := []struct {
		tier  polity.SettlementTier
		title string
	}{
		{polity.TierCamp, "elder"},
		{polity.TierHamlet, "chieftain"},
		{polity.TierVillage, "chieftain"},
	}
	for _, tc := range cases {
		got := rulerTitle(tc.tier)
		if got != tc.title {
			t.Errorf("rulerTitle(%v) = %q, want %q", tc.tier, got, tc.title)
		}
	}
}

func TestDescribeRuler_WithName(t *testing.T) {
	s := &polity.Settlement{
		Tier:  polity.TierCamp,
		Ruler: polity.Ruler{Name: "Korvin"},
	}
	got := describeRuler(s)
	want := "under elder 'Korvin'"
	if got != want {
		t.Errorf("describeRuler = %q, want %q", got, want)
	}
}

func TestDescribeRuler_EmptyName(t *testing.T) {
	s := &polity.Settlement{
		Tier:  polity.TierHamlet,
		Ruler: polity.Ruler{Name: ""},
	}
	got := describeRuler(s)
	want := "under chieftain '(unnamed)'"
	if got != want {
		t.Errorf("describeRuler = %q, want %q", got, want)
	}
}

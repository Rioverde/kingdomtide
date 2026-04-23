package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/stats"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// newCreationModel returns a Model parked in phaseCharacterCreation with
// the standard baseline (all stats at PointBuyMin, cursor on STR). Tests
// start from this state so they don't have to repeat the name-entry dance
// for every scenario.
func newCreationModel(t *testing.T) *Model {
	t.Helper()
	m := New(context.Background(), "localhost:50051")
	m.nameInput.SetValue("alice")
	m.resetStatsForCreation()
	m.setPhase(phaseCharacterCreation)
	return m
}

// TestPhaseTransitionEnterNameToCreation asserts that a valid name plus
// Enter advances the state machine into phaseCharacterCreation and seeds
// the stats array at the Point Buy baseline.
func TestPhaseTransitionEnterNameToCreation(t *testing.T) {
	t.Parallel()
	m := New(context.Background(), "localhost:50051")
	for _, r := range "alice" {
		model, _ := m.Update(keyRunes(r))
		m = model.(*Model)
	}
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)

	if m.phase != phaseCharacterCreation {
		t.Fatalf("phase = %d, want phaseCharacterCreation", m.phase)
	}
	for i, s := range m.stats {
		if s != stats.PointBuyMin {
			t.Errorf("stats[%d] = %d, want %d", i, s, stats.PointBuyMin)
		}
	}
	if m.selectedStat != 0 {
		t.Errorf("selectedStat = %d, want 0", m.selectedStat)
	}
}

// TestPhaseTransitionCreationToConnecting verifies that Enter with a
// valid 27-point distribution leaves phaseCharacterCreation for
// phaseConnecting and returns a non-nil Cmd (the dial batch).
func TestPhaseTransitionCreationToConnecting(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)

	// Standard array: 15,14,13,12,10,8 -> cost 9+7+5+4+2+0 = 27.
	m.stats = [statsCount]int{15, 14, 13, 12, 10, 8}

	model, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = model.(*Model)
	if m.phase != phaseConnecting {
		t.Fatalf("phase = %d, want phaseConnecting", m.phase)
	}
	if cmd == nil {
		t.Fatalf("expected a Cmd from valid Enter")
	}
	if m.statsError != "" {
		t.Errorf("statsError = %q, want empty on success", m.statsError)
	}
}

// TestPhaseTransitionCreationBack verifies Esc returns to phaseEnterName
// and resets the stats distribution so the next entry starts fresh.
func TestPhaseTransitionCreationBack(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	m.stats = [statsCount]int{15, 14, 13, 12, 10, 8}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = model.(*Model)
	if m.phase != phaseEnterName {
		t.Fatalf("phase = %d, want phaseEnterName", m.phase)
	}
	for i, s := range m.stats {
		if s != stats.PointBuyMin {
			t.Errorf("stats[%d] = %d after Esc reset, want %d", i, s, stats.PointBuyMin)
		}
	}
}

// TestStatIncrement verifies that pressing the increase key on the
// selected stat raises it by one and lowers the remaining budget by
// the matching Point Buy cost delta.
func TestStatIncrement(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	// Cursor starts on STR (index 0).
	if got, want := m.pointBuyRemaining(), stats.PointBuyBudget; got != want {
		t.Fatalf("initial remaining = %d, want %d", got, want)
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = model.(*Model)
	if m.stats[statIdxStrength] != stats.PointBuyMin+1 {
		t.Errorf("stats[STR] = %d, want %d", m.stats[statIdxStrength], stats.PointBuyMin+1)
	}
	// Cost of 9 is 1, cost of 8 is 0: remaining drops by 1.
	if got, want := m.pointBuyRemaining(), stats.PointBuyBudget-1; got != want {
		t.Errorf("remaining = %d, want %d", got, want)
	}
}

// TestStatDecrement verifies that pressing the decrease key on an
// above-floor stat lowers it by one.
func TestStatDecrement(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	m.stats[statIdxStrength] = 10

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = model.(*Model)
	if m.stats[statIdxStrength] != 9 {
		t.Errorf("stats[STR] = %d, want 9", m.stats[statIdxStrength])
	}
}

// TestStatBudgetGuard verifies that the increment attempt silently fails
// when the remaining Point Buy budget is exactly zero — the stat must
// not move and the model state must stay consistent.
func TestStatBudgetGuard(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	// 15,15,15,8,8,8 -> 9+9+9+0+0+0 = 27, remaining 0.
	m.stats = [statsCount]int{15, 15, 15, 8, 8, 8}
	m.selectedStat = statIdxIntelligence // currently at 8

	if got := m.pointBuyRemaining(); got != 0 {
		t.Fatalf("remaining = %d, want 0", got)
	}

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = model.(*Model)
	if m.stats[statIdxIntelligence] != 8 {
		t.Errorf("stats[INT] = %d, want 8 (guarded)", m.stats[statIdxIntelligence])
	}
}

// TestStatRangeGuard verifies increments stop at PointBuyMax even if
// the budget still allows more cost.
func TestStatRangeGuard(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	m.stats = [statsCount]int{15, 8, 8, 8, 8, 8} // STR already maxed
	m.selectedStat = statIdxStrength

	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = model.(*Model)
	if m.stats[statIdxStrength] != stats.PointBuyMax {
		t.Errorf("stats[STR] = %d, want %d (capped)",
			m.stats[statIdxStrength], stats.PointBuyMax)
	}
}

// TestValidateBudget sweeps a set of distributions around the budget
// edge and asserts that only exact-27 passes, with the rest surfacing
// a localized error and no phase change.
func TestValidateBudget(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name         string
		dist         [statsCount]int
		wantPhase    phase
		wantErrEmpty bool
	}{
		{
			name:         "standard_array_27",
			dist:         [statsCount]int{15, 14, 13, 12, 10, 8},
			wantPhase:    phaseConnecting,
			wantErrEmpty: true,
		},
		{
			name:         "all_eights_0_points",
			dist:         [statsCount]int{8, 8, 8, 8, 8, 8},
			wantPhase:    phaseCharacterCreation,
			wantErrEmpty: false,
		},
		{
			name:         "above_budget_30",
			dist:         [statsCount]int{15, 15, 15, 11, 8, 8},
			wantPhase:    phaseCharacterCreation,
			wantErrEmpty: false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newCreationModel(t)
			m.stats = tc.dist
			model, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = model.(*Model)
			if m.phase != tc.wantPhase {
				t.Errorf("phase = %d, want %d", m.phase, tc.wantPhase)
			}
			if tc.wantErrEmpty && m.statsError != "" {
				t.Errorf("statsError = %q, want empty", m.statsError)
			}
			if !tc.wantErrEmpty && m.statsError == "" {
				t.Errorf("statsError empty, want non-empty localized message")
			}
		})
	}
}

// TestSelectStatWrapAround verifies the cursor wraps cleanly across the
// top and bottom of the stat list.
func TestSelectStatWrapAround(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	// Up from STR wraps to CHA (last index).
	model, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = model.(*Model)
	if m.selectedStat != statIdxCharisma {
		t.Errorf("selectedStat after wrap-up = %d, want %d", m.selectedStat, statIdxCharisma)
	}
	// Down from CHA wraps back to STR.
	model, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = model.(*Model)
	if m.selectedStat != statIdxStrength {
		t.Errorf("selectedStat after wrap-down = %d, want %d", m.selectedStat, statIdxStrength)
	}
}

// TestRenderCharacterCreation_EnAndRu renders the screen at both
// supported locales and asserts localized labels are present. The
// centered-box helper requires non-zero terminal dimensions before it
// calls lipgloss.Place.
func TestRenderCharacterCreation_EnAndRu(t *testing.T) {
	t.Parallel()
	cases := []struct {
		lang       string
		wantHeader string
		wantStr    string
	}{
		{"en", "Character Creation", "STR"},
		{"ru", "Создание персонажа", "СИЛ"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.lang, func(t *testing.T) {
			t.Parallel()
			m := newCreationModel(t)
			m.lang = tc.lang
			m.termWidth = 80
			m.termHeight = 24
			out := m.viewCharacterCreation()
			if !strings.Contains(out, tc.wantHeader) {
				t.Errorf("[%s] output missing header %q", tc.lang, tc.wantHeader)
			}
			if !strings.Contains(out, tc.wantStr) {
				t.Errorf("[%s] output missing STR label %q", tc.lang, tc.wantStr)
			}
			// All six stats should render.
			for _, key := range creationStatKeys {
				want := locale.Tr(tc.lang, key)
				if !strings.Contains(out, want) {
					t.Errorf("[%s] output missing stat label %q", tc.lang, want)
				}
			}
		})
	}
}

// TestRenderShowsLocalizedError asserts the error line is rendered when
// statsError is non-empty.
func TestRenderShowsLocalizedError(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	m.termWidth = 80
	m.termHeight = 24
	m.statsError = locale.Tr(m.lang, locale.KeyCreationErrorBudget)

	out := m.viewCharacterCreation()
	if !strings.Contains(out, m.statsError) {
		t.Errorf("output missing error line %q", m.statsError)
	}
}

// TestIncrementCostDelta verifies the non-linear Point Buy cost delta:
// stepping from 13 (cost 5) to 14 (cost 7) must debit 2 points, not 1.
// This is the bug a naive +1 / -1 implementation would silently produce.
func TestIncrementCostDelta(t *testing.T) {
	t.Parallel()
	m := newCreationModel(t)
	m.stats[statIdxStrength] = 13
	before := m.pointBuyRemaining()
	m.tryIncreaseStat()
	after := m.pointBuyRemaining()
	if delta := before - after; delta != 2 {
		t.Errorf("13->14 cost delta = %d, want 2", delta)
	}
}

package world

import (
	"testing"

	"github.com/Rioverde/gongeons/internal/game/entity"
	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/stats"
)

// TestPlayerJoinWithStats verifies that NewPlayer hydrates every derived
// field from the supplied CoreStats so the returned Player is
// tick-ready without further mutation.
func TestPlayerJoinWithStats(t *testing.T) {
	cs, err := stats.NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	pos := geom.Position{X: 3, Y: 7}
	p, err := entity.NewPlayer("p1", "Alice", *cs, pos)
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}
	if p.ID != "p1" || p.Name != "Alice" {
		t.Errorf("id/name: %+v", p)
	}
	if p.Position != pos {
		t.Errorf("Position = %+v, want %+v", p.Position, pos)
	}
	if p.Stats != *cs {
		t.Errorf("Stats = %+v, want %+v", p.Stats, *cs)
	}
	if p.MaxHP != cs.MaxHP() || p.HP != cs.MaxHP() {
		t.Errorf("HP/MaxHP = %d/%d, want %d/%d", p.HP, p.MaxHP, cs.MaxHP(), cs.MaxHP())
	}
	if p.Mana != cs.Mana() {
		t.Errorf("Mana = %d, want %d", p.Mana, cs.Mana())
	}
	if p.Speed != cs.DerivedSpeed() {
		t.Errorf("Speed = %d, want %d", p.Speed, cs.DerivedSpeed())
	}
	if p.Initiative != cs.DerivedInitiative() {
		t.Errorf("Initiative = %d, want %d", p.Initiative, cs.DerivedInitiative())
	}
	if p.Energy != stats.BaseActionCost {
		t.Errorf("Energy = %d, want %d", p.Energy, stats.BaseActionCost)
	}
	if p.Intent != nil {
		t.Errorf("Intent = %v, want nil", p.Intent)
	}
}

// TestNewPlayerValidation keeps the constructor's basic empty-id /
// empty-name rejection behaviour covered after the signature change.
func TestNewPlayerValidation(t *testing.T) {
	if _, err := entity.NewPlayer("", "Alice", stats.DefaultCoreStats(), geom.Position{}); err == nil {
		t.Error("empty id: want error, got nil")
	}
	if _, err := entity.NewPlayer("p1", "", stats.DefaultCoreStats(), geom.Position{}); err == nil {
		t.Error("empty name: want error, got nil")
	}
}

// TestNewMonsterDerivations mirrors TestPlayerJoinWithStats for the
// monster constructor.
func TestNewMonsterDerivations(t *testing.T) {
	m, err := entity.NewMonster("m1", "Rat", stats.DefaultCoreStats())
	if err != nil {
		t.Fatalf("NewMonster: %v", err)
	}
	if m.MaxHP != 10 || m.HP != 10 {
		t.Errorf("HP/MaxHP = %d/%d, want 10/10", m.HP, m.MaxHP)
	}
	if m.Mana != 5 {
		t.Errorf("Mana = %d, want 5", m.Mana)
	}
	if m.Speed != stats.SpeedNormal {
		t.Errorf("Speed = %d, want %d", m.Speed, stats.SpeedNormal)
	}
	if m.Initiative != 0 {
		t.Errorf("Initiative = %d, want 0", m.Initiative)
	}
}

// TestApplyJoinUsesStats end-to-ends a JoinCmd carrying validated stats
// and asserts the world-stored Player reflects every derived field —
// proof that the stats travel through ApplyCommand intact.
func TestApplyJoinUsesStats(t *testing.T) {
	cs, err := stats.NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	w := newTestWorld(testTiles{})
	_, err = w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice", Stats: *cs})
	if err != nil {
		t.Fatalf("ApplyCommand: %v", err)
	}
	p, ok := w.PlayerByID("p1")
	if !ok {
		t.Fatalf("player p1 missing after join")
	}
	if p.Stats != *cs {
		t.Errorf("Stats = %+v, want %+v", p.Stats, *cs)
	}
	if p.MaxHP != cs.MaxHP() {
		t.Errorf("MaxHP = %d, want %d", p.MaxHP, cs.MaxHP())
	}
	if p.Speed != cs.DerivedSpeed() {
		t.Errorf("Speed = %d, want %d", p.Speed, cs.DerivedSpeed())
	}
}

// TestApplyJoinDefaultsStatsWhenOmitted asserts the applyJoin fallback
// path: a bare JoinCmd without Stats gets the neutral baseline so
// pre-stats callers and domain tests keep working.
func TestApplyJoinDefaultsStatsWhenOmitted(t *testing.T) {
	w := newTestWorld(testTiles{})
	_, err := w.ApplyCommand(JoinCmd{PlayerID: "p1", Name: "Alice"})
	if err != nil {
		t.Fatalf("ApplyCommand: %v", err)
	}
	p, _ := w.PlayerByID("p1")
	if p.Stats != stats.DefaultCoreStats() {
		t.Errorf("Stats = %+v, want DefaultCoreStats", p.Stats)
	}
	if p.MaxHP != 10 {
		t.Errorf("MaxHP = %d, want 10", p.MaxHP)
	}
}

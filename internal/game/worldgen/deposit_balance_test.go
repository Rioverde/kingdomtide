//go:build diag
// +build diag

package worldgen

import (
	"sort"
	"testing"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
	gworld "github.com/Rioverde/kingdomtide/internal/game/world"
)

// TestDepositBalance dumps the per-kind deposit distribution on a Standard
// world so a human can eyeball balance (no asserts — diagnostic only).
func TestDepositBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("Standard-world deposit balance dump")
	}
	w := Generate(42, WorldSizeStandard)
	volcanoes := NewVolcanoSource(w, 42)
	deposits := NewDepositSource(w, 42, DepositSourceConfig{Volcanoes: volcanoes})

	rect := geom.Rect{MinX: 0, MinY: 0, MaxX: w.Width, MaxY: w.Height}
	all := deposits.DepositsIn(rect)
	counts := map[gworld.DepositKind]int{}
	for _, d := range all {
		counts[d.Kind]++
	}

	type row struct {
		kind  gworld.DepositKind
		count int
	}
	rows := make([]row, 0, len(counts))
	for k, c := range counts {
		rows = append(rows, row{k, c})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })

	t.Logf("=== DEPOSIT BALANCE (seed=42 Standard, %d deposits) ===", len(all))
	for _, r := range rows {
		barLen := r.count * 40 / len(all)
		bar := ""
		for i := 0; i < barLen; i++ {
			bar += "█"
		}
		pct := 100.0 * float64(r.count) / float64(len(all))
		t.Logf("  %-12s %5d  %5.1f%%  %s", r.kind, r.count, pct, bar)
	}
}

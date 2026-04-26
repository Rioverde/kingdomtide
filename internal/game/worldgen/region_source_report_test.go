//go:build diag
// +build diag

package worldgen

import (
	"fmt"
	"sort"
	"testing"

	"github.com/Rioverde/kingdomtide/internal/game/geom"
	gworld "github.com/Rioverde/kingdomtide/internal/game/world"
)

// TestRegionSource_Report is a -v reporting helper for the human-readable
// summary required by the orchestrator brief. It is not an assertion test
// — failures are silent — and is gated behind a manual run flag so it
// never costs CI time.
func TestRegionSource_Report(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w := Generate(42, WorldSizeStandard)
	src := NewRegionSource(w, 42)

	counts := make(map[gworld.RegionCharacter]int)
	var samples []gworld.Region
	maxX := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	maxY := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	for sy := 0; sy < maxY; sy++ {
		for sx := 0; sx < maxX; sx++ {
			r := src.RegionAt(geom.SuperChunkCoord{X: sx, Y: sy})
			counts[r.Character]++
			if len(samples) < 3 && r.Character != gworld.RegionNormal {
				samples = append(samples, r)
			}
		}
	}

	keys := make([]gworld.RegionCharacter, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	t.Logf("seed=42 Standard distinct=%d", len(counts))
	for _, k := range keys {
		t.Logf("  %s: %d", k, counts[k])
	}
	for i, r := range samples {
		t.Logf("sample %d: sc=%+v char=%s sub=%s body_seed=%d format=%s prefix_idx=%d pattern_idx=%d",
			i, r.Coord, r.Character, r.Name.SubKind, r.Name.BodySeed,
			fmt.Sprint(r.Name.Format), r.Name.PrefixIndex, r.Name.PatternIndex)
	}
}

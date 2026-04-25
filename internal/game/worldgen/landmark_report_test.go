//go:build diag
// +build diag

package worldgen

import (
	"sort"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// TestReportLandmarks logs the Standard seed=42 distribution. Useful
// for the Phase 2 report; gated behind -short like the heavier suites.
func TestReportLandmarks(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~3s")
	}
	w, regions := buildLandmarkTestWorld(t)
	src := NewLandmarkSource(w, landmarkSampleSeed, LandmarkSourceConfig{Regions: regions})

	totals := map[gworld.LandmarkKind]int{}
	all := []gworld.Landmark{}
	type pair struct {
		sc geom.SuperChunkCoord
		lm gworld.Landmark
	}
	var samples []pair
	sweepLandmarkSuperChunks(w, src, func(sc geom.SuperChunkCoord, lms []gworld.Landmark) {
		for _, lm := range lms {
			totals[lm.Kind]++
			all = append(all, lm)
			samples = append(samples, pair{sc, lm})
		}
	})
	t.Logf("Standard seed=42 total landmarks=%d", len(all))
	keys := []gworld.LandmarkKind{
		gworld.LandmarkTower,
		gworld.LandmarkGiantTree,
		gworld.LandmarkStandingStones,
		gworld.LandmarkObelisk,
		gworld.LandmarkChasm,
		gworld.LandmarkShrine,
	}
	for _, k := range keys {
		t.Logf("  kind=%s count=%d", k, totals[k])
	}
	sort.Slice(samples, func(i, j int) bool {
		a, b := samples[i].lm.Coord, samples[j].lm.Coord
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	if len(samples) >= 3 {
		for i, idx := range []int{0, len(samples) / 2, len(samples) - 1} {
			s := samples[idx]
			region := regions.RegionAt(s.sc)
			t.Logf("sample[%d] coord=%v kind=%s region=%s",
				i, s.lm.Coord, s.lm.Kind, region.Character)
		}
	}
}

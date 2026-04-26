//go:build diag
// +build diag

package worldgen

import (
	"fmt"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/geom"
	"github.com/Rioverde/gongeons/internal/game/polity"
	gworld "github.com/Rioverde/gongeons/internal/game/world"
)

// TestCampSource_Report logs a human-readable tabular summary of the
// Standard seed=42 camp distribution. It is not an assertion test — no
// t.Error calls are made — and is gated behind the "diag" build tag so
// it never costs CI time. Run with:
//
//	go test -tags diag -v -run TestCampSource_Report ./internal/game/worldgen/
func TestCampSource_Report(t *testing.T) {
	if testing.Short() {
		t.Skip("short — Standard world generation costs ~400ms")
	}

	const seed = int64(42)
	w := Generate(seed, WorldSizeStandard)
	regions := NewRegionSource(w, seed)
	volcanoes := NewVolcanoSource(w, seed)
	landmarks := NewLandmarkSource(w, seed, LandmarkSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
	})
	deposits := NewDepositSource(w, seed, DepositSourceConfig{
		Volcanoes: volcanoes,
	})
	src := NewCampSource(w, seed, CampSourceConfig{
		Regions:   regions,
		Volcanoes: volcanoes,
		Landmarks: landmarks,
		Deposits:  deposits,
	})

	camps := src.All()
	total := len(camps)

	scWide := (w.Width + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	scTall := (w.Height + geom.SuperChunkSize - 1) / geom.SuperChunkSize
	totalSC := scWide * scTall

	t.Logf("=== Camp Report — Standard seed=%d ===", seed)
	t.Logf("Total camps: %d  |  SCs: %d  |  Mean camps/SC: %.2f",
		total, totalSC, float64(total)/float64(totalSC))

	// --- Per-region camp count and faith distribution --------------------
	type faithRow [polity.FaithCount]int
	regionCounts := [7]int{}
	faithByRegion := [7]faithRow{}
	for _, c := range camps {
		r := int(c.Region)
		if r < 7 {
			regionCounts[r]++
			faithByRegion[r][c.Faiths.Majority()]++
		}
	}
	regionNames := [7]string{"Normal", "Blighted", "Fey", "Ancient", "Savage", "Holy", "Wild"}
	faithNames := []polity.Faith{
		polity.FaithOldGods,
		polity.FaithSunCovenant,
		polity.FaithGreenSage,
		polity.FaithOneOath,
		polity.FaithStormPact,
	}

	t.Logf("")
	t.Logf("%-10s  %6s  %9s  %11s  %9s  %9s  %10s",
		"Region", "Camps", "OldGods%", "SunCovenant%", "GreenSage%", "OneOath%", "StormPact%")
	t.Logf("%s", "----------  ------  ---------  ------------  ----------  ---------  ----------")
	for r := 0; r < 7; r++ {
		n := regionCounts[r]
		if n == 0 {
			t.Logf("%-10s  %6d  —", regionNames[r], 0)
			continue
		}
		pct := func(f polity.Faith) string {
			return fmt.Sprintf("%.1f%%", float64(faithByRegion[r][f])/float64(n)*100)
		}
		t.Logf("%-10s  %6d  %9s  %12s  %10s  %9s  %10s",
			regionNames[r], n,
			pct(faithNames[0]), pct(faithNames[1]), pct(faithNames[2]),
			pct(faithNames[3]), pct(faithNames[4]))
	}

	// --- Pop histogram ---------------------------------------------------
	popBuckets := [4]int{} // [10-19], [20-29], [30-39], [40-50]
	for _, c := range camps {
		switch {
		case c.Population >= 10 && c.Population <= 19:
			popBuckets[0]++
		case c.Population >= 20 && c.Population <= 29:
			popBuckets[1]++
		case c.Population >= 30 && c.Population <= 39:
			popBuckets[2]++
		case c.Population >= 40 && c.Population <= 50:
			popBuckets[3]++
		}
	}
	popLabels := [4]string{"10-19", "20-29", "30-39", "40-50"}
	t.Logf("")
	t.Logf("Pop histogram:")
	for i, n := range popBuckets {
		t.Logf("  %5s: %5d  (%.1f%%)", popLabels[i], n, float64(n)/float64(total)*100)
	}

	// --- BornYear histogram ----------------------------------------------
	bornBuckets := [4]int{} // [-200,-150), [-150,-100), [-100,-50), [-50,0)
	for _, c := range camps {
		switch {
		case c.Founded >= -200 && c.Founded < -150:
			bornBuckets[0]++
		case c.Founded >= -150 && c.Founded < -100:
			bornBuckets[1]++
		case c.Founded >= -100 && c.Founded < -50:
			bornBuckets[2]++
		case c.Founded >= -50 && c.Founded < 0:
			bornBuckets[3]++
		}
	}
	bornLabels := [4]string{"[-200,-150)", "[-150,-100)", "[-100,-50 )", "[-50,  0  )"}
	t.Logf("")
	t.Logf("BornYear histogram:")
	for i, n := range bornBuckets {
		t.Logf("  %12s: %5d  (%.1f%%)", bornLabels[i], n, float64(n)/float64(total)*100)
	}

	// --- Footprint size histogram ----------------------------------------
	fpBuckets := [3]int{} // size 1, 2, 3
	for _, c := range camps {
		n := len(c.Footprint)
		if n >= 1 && n <= 3 {
			fpBuckets[n-1]++
		}
	}
	t.Logf("")
	t.Logf("Footprint size histogram:")
	for size := 1; size <= 3; size++ {
		n := fpBuckets[size-1]
		t.Logf("  size %d: %5d  (%.1f%%)", size, n, float64(n)/float64(total)*100)
	}

	// --- Per-region SC camp count stats ----------------------------------
	scCounts := make(map[gworld.RegionCharacter][]int)
	for sy := 0; sy < scTall; sy++ {
		for sx := 0; sx < scWide; sx++ {
			sc := geom.SuperChunkCoord{X: sx, Y: sy}
			region := regions.RegionAt(sc).Character
			cnt := len(src.CampsIn(sc))
			scCounts[region] = append(scCounts[region], cnt)
		}
	}
	t.Logf("")
	t.Logf("Per-region SC camp stats:")
	for r := 0; r < 7; r++ {
		char := gworld.RegionCharacter(r)
		counts := scCounts[char]
		if len(counts) == 0 {
			continue
		}
		var sum int
		maxC := 0
		for _, v := range counts {
			sum += v
			if v > maxC {
				maxC = v
			}
		}
		mean := float64(sum) / float64(len(counts))
		t.Logf("  %-10s: %3d SCs, mean %.1f camps/SC, max %d",
			regionNames[r], len(counts), mean, maxC)
	}
}

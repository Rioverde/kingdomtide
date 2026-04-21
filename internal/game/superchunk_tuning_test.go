//go:build tuning

// run with: go test -tags=tuning -run TestJitterTuning -v ./internal/game/
package game

import (
	"fmt"
	"math"
	"testing"
)

// TestJitterTuning exhaustively samples a 1024×1024 tile window for two jitter
// configurations and reports the actual region area distribution. This is an
// informational test — it always passes. Run with:
//
//	go test -tags=tuning -run TestJitterTuning -v ./internal/game/
//
// Results are printed to stdout and should be pasted into
// .omc/plans/phase-1-regions.md under ## Tuning results.
func TestJitterTuning(t *testing.T) {
	const seed int64 = 0x2f6f7a3d
	// Sample a 1024×1024 tile window (16×16 super-chunks): gives a full
	// count of real region areas for all Voronoi cells that have their
	// anchor within roughly the centre 8×8 super-chunks. Cells near the
	// border are artificially small because we stop counting at the window
	// edge; we exclude them from statistics by only counting regions that
	// have their anchor inside the inner 512×512 area.
	const windowSize = 1024
	const innerHalf = windowSize / 4 // inner anchor zone: [256, 768)

	type config struct {
		name string
		min  int
		max  int
	}
	configs := []config{
		{"[8,56] ~75%", 8, SuperChunkSize - 8},
		{"[3,61] ~95%", 3, SuperChunkSize - 3},
	}

	for _, cfg := range configs {
		anchorOf := func(sc SuperChunkCoord) Position {
			h := anchorMix(seed, sc)
			hi := uint32(h >> 32)
			lo := uint32(h)
			span := cfg.max - cfg.min + 1
			dx := int(hi%uint32(span)) + cfg.min
			dy := int(lo%uint32(span)) + cfg.min
			return Position{
				X: sc.X*SuperChunkSize + dx,
				Y: sc.Y*SuperChunkSize + dy,
			}
		}

		anchorAt := func(worldX, worldY int) SuperChunkCoord {
			home := WorldToSuperChunk(worldX, worldY)
			bestSC := home
			bestAnchor := anchorOf(home)
			bestDist := sqDist(bestAnchor.X, bestAnchor.Y, worldX, worldY)
			for dy := -1; dy <= 1; dy++ {
				for dx := -1; dx <= 1; dx++ {
					if dx == 0 && dy == 0 {
						continue
					}
					cand := SuperChunkCoord{X: home.X + dx, Y: home.Y + dy}
					a := anchorOf(cand)
					d := sqDist(a.X, a.Y, worldX, worldY)
					if d < bestDist || (d == bestDist && lessSC(cand, bestSC)) {
						bestDist = d
						bestSC = cand
						bestAnchor = a
					}
				}
			}
			return bestSC
		}

		// Count tiles per region over the full window.
		regionCounts := make(map[SuperChunkCoord]int)
		for y := 0; y < windowSize; y++ {
			for x := 0; x < windowSize; x++ {
				sc := anchorAt(x, y)
				regionCounts[sc]++
			}
		}

		// Only include regions whose anchor falls within the inner zone to
		// avoid edge-clipped partial regions skewing min/max downward.
		var counts []int
		for sc, count := range regionCounts {
			a := anchorOf(sc)
			// Anchor world coord relative to window: we started at (0,0).
			ax, ay := a.X, a.Y
			if ax >= innerHalf && ax < windowSize-innerHalf &&
				ay >= innerHalf && ay < windowSize-innerHalf {
				counts = append(counts, count)
			}
		}

		total := 0
		minArea := math.MaxInt
		maxArea := 0
		for _, c := range counts {
			total += c
			if c < minArea {
				minArea = c
			}
			if c > maxArea {
				maxArea = c
			}
		}
		if len(counts) == 0 {
			t.Logf("=== Jitter %s === (no inner-zone anchors found)", cfg.name)
			continue
		}
		mean := float64(total) / float64(len(counts))

		var sumSq float64
		for _, c := range counts {
			d := float64(c) - mean
			sumSq += d * d
		}
		stddev := math.Sqrt(sumSq / float64(len(counts)))
		variancePct := (stddev / mean) * 100.0

		t.Logf("=== Jitter %s ===", cfg.name)
		t.Logf("  Inner-zone regions:     %d", len(counts))
		t.Logf("  Total regions (window): %d", len(regionCounts))
		t.Logf("  Mean tiles/region:      %.1f", mean)
		t.Logf("  Min tiles/region:       %d", minArea)
		t.Logf("  Max tiles/region:       %d", maxArea)
		t.Logf("  Variance (stddev/mean): %.1f%%", variancePct)
		fmt.Printf("=== Jitter %s ===\n", cfg.name)
		fmt.Printf("  Inner-zone regions:     %d\n", len(counts))
		fmt.Printf("  Total regions (window): %d\n", len(regionCounts))
		fmt.Printf("  Mean tiles/region:      %.1f\n", mean)
		fmt.Printf("  Min tiles/region:       %d\n", minArea)
		fmt.Printf("  Max tiles/region:       %d\n", maxArea)
		fmt.Printf("  Variance (stddev/mean): %.1f%%\n\n", variancePct)
	}
}

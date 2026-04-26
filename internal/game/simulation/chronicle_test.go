//go:build diag
// +build diag

package simulation

import (
	"strings"
	"testing"

	"github.com/Rioverde/gongeons/internal/game/polity"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

// TestE2EChronicle runs a full 200-year simulation on Tiny seed=42
// and prints a complete narrative chronicle:
//   - Starting state (how many camps, what regions, faith mix)
//   - Every event line in order (camp-died, camps-merged, etc.)
//   - Every notable promotion and merge
//   - Final state by tier
//   - Notable settlements (longest-surviving, most populous, most-merged)
//
// Build-tagged with `diag` so it's only run on demand:
//
//	go test -tags=diag -run TestE2EChronicle -v ./internal/game/simulation/
//
// The output is dense — pipe through `less` for readability:
//
//	go test -tags=diag -run TestE2EChronicle -v ./internal/game/simulation/ 2>&1 | less
func TestE2EChronicle(t *testing.T) {
	const seed int64 = 42

	t.Logf("=== SIMULATION CHRONICLE — seed %d, world Tiny, 200 years ===\n", seed)

	// Build world.
	w := worldgen.Generate(seed, worldgen.WorldSizeTiny)
	regions := worldgen.NewRegionSource(w, seed)
	landmarks := worldgen.NewLandmarkSource(w, seed, worldgen.LandmarkSourceConfig{Regions: regions})
	volcanoes := worldgen.NewVolcanoSource(w, seed)
	deposits := worldgen.NewDepositSource(w, seed, worldgen.DepositSourceConfig{Volcanoes: volcanoes})
	src := worldgen.NewCampSource(w, seed, worldgen.CampSourceConfig{
		Regions:   regions,
		Landmarks: landmarks,
		Volcanoes: volcanoes,
		Deposits:  deposits,
	})

	// Starting state.
	startCamps := src.All()
	t.Logf("--- STARTING STATE (year 0) ---")
	t.Logf("Total camps: %d", len(startCamps))

	regionCounts := map[polity.RegionCharacter]int{}
	for _, c := range startCamps {
		regionCounts[c.Region]++
	}
	t.Logf("By region:")
	for region, count := range regionCounts {
		t.Logf("  %-10s  %d camps", region.String(), count)
	}

	// Sample 5 named camps with rulers.
	t.Logf("Sample camps:")
	for i, c := range startCamps {
		if i >= 5 {
			break
		}
		t.Logf("  '%s' (id %d) under elder '%s' — pop %d, region %s",
			c.Name, c.ID, c.Ruler.Name, c.Population, c.Region.String())
	}

	// Run sim with full logger.
	var logBuf strings.Builder
	r := Run(seed, src, WithLogger(&logBuf))

	// Final state.
	finalSrc := r.SettlementSource()
	camps := finalSrc.AllCamps()
	hamlets := finalSrc.AllHamlets()
	villages := finalSrc.AllVillages()
	total := len(camps) + len(hamlets) + len(villages)

	t.Logf("\n--- FINAL STATE (year 199) ---")
	t.Logf("Total settlements: %d (down from %d starting camps)",
		total, len(startCamps))
	t.Logf("By tier:")
	t.Logf("  Camps:    %d", len(camps))
	t.Logf("  Hamlets:  %d", len(hamlets))
	t.Logf("  Villages: %d", len(villages))

	if len(villages) > 0 {
		t.Logf("\n--- VILLAGES (the success stories) ---")
		for _, v := range villages {
			t.Logf("  '%s' (id %d) — pop %d, %d absorbed hamlets, region %s, ruler %s",
				v.Name, v.ID, v.Population, len(v.AbsorbedHamletIDs),
				v.Region.String(), v.Ruler.Name)
			t.Logf("    Faiths: %s", formatFaiths(v.Faiths))
		}
	}

	if len(hamlets) > 0 {
		t.Logf("\n--- HAMLETS (sample 10) ---")
		for i, h := range hamlets {
			if i >= 10 {
				break
			}
			t.Logf("  '%s' (id %d) — pop %d, %d absorbed camps, region %s, ruler %s",
				h.Name, h.ID, h.Population, len(h.AbsorbedCampIDs),
				h.Region.String(), h.Ruler.Name)
		}
	}

	// Event-line breakdown by type.
	t.Logf("\n--- EVENT BREAKDOWN ---")
	lines := strings.Split(logBuf.String(), "\n")
	eventCounts := map[string]int{}
	for _, line := range lines {
		if strings.HasPrefix(line, "[year +") {
			// Extract event type after the year tag.
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				eventCounts[parts[2]]++
			}
		}
	}
	for event, count := range eventCounts {
		t.Logf("  %-18s %d", event, count)
	}

	// Print FULL chronicle (every log line in order).
	t.Logf("\n--- FULL CHRONICLE ---")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		t.Logf("%s", line)
	}
}

// formatFaiths renders a FaithDistribution as a comma-separated
// "Faith N.NN%" list, sorted descending by share, omitting < 1%.
func formatFaiths(fd polity.FaithDistribution) string {
	type entry struct {
		faith polity.Faith
		share float64
	}
	entries := make([]entry, 0, polity.FaithCount)
	for i, v := range fd {
		entries = append(entries, entry{polity.Faith(i), v})
	}
	// Insertion sort descending — small slice, stable.
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].share > entries[j-1].share; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
	var sb strings.Builder
	first := true
	for _, e := range entries {
		if e.share < 0.01 {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(e.faith.String())
		sb.WriteString(" ")
		sb.WriteString(formatPercent(e.share))
	}
	return sb.String()
}

func formatPercent(v float64) string {
	pct := v * 100
	intPart := int(pct)
	frac := int((pct - float64(intPart)) * 100)
	if frac < 0 {
		frac = -frac
	}
	return formatInt(intPart) + "." + padInt(frac, 2) + "%"
}

func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

func padInt(n, width int) string {
	s := formatInt(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

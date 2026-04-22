// Command print_deposits dumps a summary of resource deposits around a
// chosen world-space centre for a given seed. Operator tool — not part
// of the runtime surface.
//
// Usage:
//
//	go run ./cmd/devtools/print_deposits -seed 42 -cx 0 -cy 0 -radius 128
package main

import (
	"flag"
	"fmt"
	"sort"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

func main() {
	seed := flag.Int64("seed", 42, "world seed")
	cx := flag.Int("cx", 0, "centre X")
	cy := flag.Int("cy", 0, "centre Y")
	radius := flag.Int("radius", 128, "chebyshev radius to scan")
	list := flag.Bool("list", false, "list every deposit (not just counts)")
	flag.Parse()

	wg := worldgen.NewChunkedSource(*seed)
	regionSrc := worldgen.NewNoiseRegionSource(*seed)
	lmSrc := worldgen.NewNoiseLandmarkSource(*seed, regionSrc, wg.Generator())
	volSrc := worldgen.NewNoiseVolcanoSource(*seed, wg.Generator(), lmSrc)
	depSrc := worldgen.NewNoiseDepositSource(*seed, wg.Generator(), lmSrc, volSrc)

	centre := game.Position{X: *cx, Y: *cy}
	rect := game.Rect{
		MinX: centre.X - *radius,
		MinY: centre.Y - *radius,
		MaxX: centre.X + *radius + 1,
		MaxY: centre.Y + *radius + 1,
	}
	deposits := depSrc.DepositsIn(rect)

	counts := map[game.DepositKind]int{}
	for _, d := range deposits {
		counts[d.Kind]++
	}

	fmt.Printf("seed=%d centre=(%d,%d) radius=%d rect=%dx%d tiles\n",
		*seed, centre.X, centre.Y, *radius, 2*(*radius)+1, 2*(*radius)+1)
	fmt.Printf("total deposits = %d\n\n", len(deposits))

	kinds := make([]game.DepositKind, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Slice(kinds, func(i, j int) bool { return counts[kinds[i]] > counts[kinds[j]] })
	for _, k := range kinds {
		fmt.Printf("  %-10s %5d\n", k, counts[k])
	}

	if *list {
		fmt.Println("\nper-deposit (sorted by chebyshev distance from centre):")
		sort.Slice(deposits, func(i, j int) bool {
			di := chebyshev(deposits[i].Position, centre)
			dj := chebyshev(deposits[j].Position, centre)
			if di != dj {
				return di < dj
			}
			if deposits[i].Position.X != deposits[j].Position.X {
				return deposits[i].Position.X < deposits[j].Position.X
			}
			return deposits[i].Position.Y < deposits[j].Position.Y
		})
		for _, d := range deposits {
			fmt.Printf("  d=%-4d %+v kind=%-10s amount=%d\n",
				chebyshev(d.Position, centre), d.Position, d.Kind, d.MaxAmount)
		}
	}

	near := depSrc.DepositsNear(centre, 20)
	fmt.Printf("\nDepositsNear(centre, 20) = %d deposit(s) within 20 tiles\n", len(near))
	for _, d := range near {
		fmt.Printf("  %+v kind=%s\n", d.Position, d.Kind)
	}
}

func chebyshev(a, b game.Position) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return max(dx, dy)
}

// Command worldgen-explorer is a developer tool for iterating on the
// Gongeons worldgen pipeline. It presents a bubbletea TUI with three
// phases: a menu for picking world size and seed, a short building
// screen while the demo world generates, and a scrollable viewer that
// lets the dev navigate the baked grids with arrow keys, toggle
// visualisation layers, and compare cases by regenerating with a new
// seed without leaving the tool.
//
// The explorer is intentionally decoupled from the real game server
// and client — it does not dial, does not open a TCP socket, does not
// touch the session layer. It exists so every future pipeline stage
// lands with immediate visual feedback the same day it is written.
//
// Invoke without args for the interactive menu:
//
//	go run ./cmd/worldgen-explorer
//
// Or skip the menu by supplying both flags:
//
//	go run ./cmd/worldgen-explorer --size=standard --seed=42
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/game/worldgen"
)

func main() {
	sizeFlag := flag.String("size", "", "world size (tiny, small, standard, large, huge); empty opens menu")
	seedFlag := flag.Int64("seed", 0, "explicit seed; 0 with --size means random")
	flag.Parse()

	initial := initialModel()
	if *sizeFlag != "" {
		sz, err := worldgen.ParseWorldSize(*sizeFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		seed := *seedFlag
		if seed == 0 {
			seed = int64(rand.Uint64())
		}
		initial = modelStartingBuild(sz, seed)
	}

	p := tea.NewProgram(initial, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "explorer:", err)
		os.Exit(1)
	}
}

// Command sim-explorer is a developer tool for stepping through the
// Gongeons settlement simulation year by year. It presents a bubbletea
// TUI with three phases: a menu for picking world size and seed; a short
// building screen while the world generates and the simulation runs; and
// a scrollable viewer that lets the dev navigate the map, toggle layers,
// and scrub through yearly snapshots with play/pause/step controls.
//
// The explorer is intentionally decoupled from the real game server and
// client — it does not dial, does not open a TCP socket, does not touch
// the session layer. It exists so every future simulation stage lands
// with immediate visual feedback the same day it is written.
//
// Invoke without args for the interactive menu:
//
//	go run ./cmd/devtools/sim-explorer
//
// Or skip the menu by supplying the flags:
//
//	go run ./cmd/devtools/sim-explorer --size=small --seed=42
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

	// No WithMouseCellMotion — mouse motion would fire an Update for every
	// cursor move, triggering a full renderViewport rebuild each time.
	p := tea.NewProgram(initial, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sim-explorer:", err)
		os.Exit(1)
	}
}

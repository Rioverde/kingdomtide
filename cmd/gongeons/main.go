// Command gongeons is the Bubble Tea terminal client. It dials a
// Gongeons gRPC server, joins as a named player, and renders the shared
// world as key presses are translated into movement commands.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Rioverde/gongeons/internal/ui"
)

const defaultServerAddr = "localhost:50051"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run owns the program lifecycle: flag parsing, signal plumbing,
// Bubble Tea program construction and graceful shutdown.
func run() error {
	addr := flag.String("server", defaultServerAddr, "gongeons server address")
	flag.Parse()

	// Root context cancelled by Ctrl-C or SIGTERM. Propagated into the
	// UI so any stream goroutine the Model starts winds down with us.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	model := ui.New(ctx, *addr)
	p := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithContext(ctx),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tea run: %w", err)
	}
	return nil
}

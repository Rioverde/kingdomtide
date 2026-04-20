// Command gongeons boots the game world and serves its rendered map over HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Rioverde/gongeons/internal/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run wires up dependencies and runs the HTTP server until an interrupt is received.
// Keeping the body out of main follows Mat Ryer's "run() returns error" pattern and keeps
// graceful shutdown straightforward.
func run() error {
	var (
		addr     string
		tilesDir string
		seed     int64
	)
	flag.StringVar(&addr, "addr", ":8080", "HTTP listen address")
	flag.StringVar(&tilesDir, "tiles", "assets/tiles", "directory containing terrain tile PNGs")
	flag.Int64Var(&seed, "seed", time.Now().UnixNano(), "initial world generation seed")
	flag.Parse()

	srv, err := web.NewServer(web.Config{
		TilesDir: tilesDir,
		Seed:     seed,
	})
	if err != nil {
		return fmt.Errorf("new server: %w", err)
	}

	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      srv.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Signal-cancelled context so Ctrl-C / SIGTERM triggers graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("gongeons listening on http://localhost%s (seed=%d)", addr, seed)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		log.Print("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

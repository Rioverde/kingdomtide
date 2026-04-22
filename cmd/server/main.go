// Command gongeonsd is the authoritative multiplayer server for gongeons.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/server"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// run wires up the server and blocks until a signal triggers graceful shutdown.
// Keeping the body out of main follows Mat Ryer's "run() returns error" pattern.
func run() error {
	var (
		addr     string
		logLevel string
		seed     int64
	)
	flag.StringVar(&addr, "addr", ":50051", "gRPC listen address")
	flag.StringVar(&logLevel, "log-level", "info", "log level: debug | info | warn | error")
	flag.Int64Var(&seed, "seed", 0, "world seed; 0 = random from wall clock")
	flag.Parse()

	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	logger := newLogger(logLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	grpcSrv := grpc.NewServer()
	svc := server.NewService(buildWorld(seed), logger)
	pb.RegisterGameServiceServer(grpcSrv, svc)

	go svc.Run(ctx)

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("gongeonsd listening", "addr", lis.Addr().String(), "seed", seed)
		if err := grpcSrv.Serve(lis); err != nil {
			serveErr <- err
			return
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("serve: %w", err)
		}
		return nil
	}

	grpcSrv.GracefulStop()
	logger.Info("graceful shutdown complete")
	return nil
}

// buildWorld constructs the production world: a procedural tile source keyed
// on seed, a matching NoiseRegionSource for Voronoi regions, a
// NoiseLandmarkSource for Layer 1.5 landmarks, and the seed itself threaded
// through so AnchorAt stays deterministic. Split out of run for testability
// and so the wiring is visible at a glance.
func buildWorld(seed int64) *game.World {
	wg := worldgen.NewChunkedSource(seed)
	regionSrc := worldgen.NewNoiseRegionSource(seed)
	landmarkSrc := worldgen.NewNoiseLandmarkSource(seed, regionSrc, wg.Generator())
	return game.NewWorld(
		wg,
		game.WithSeed(seed),
		game.WithRegionSource(regionSrc),
		game.WithLandmarkSource(landmarkSrc),
	)
}

// newLogger builds a text-handler slog.Logger at the requested level. Unknown
// levels fall back to info so typos do not silence logging entirely.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

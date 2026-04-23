// Command gongeonsd is the authoritative multiplayer server for gongeons.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	cbssh "github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	bm "github.com/charmbracelet/wish/bubbletea"
	wishlog "github.com/charmbracelet/wish/logging"
	"google.golang.org/grpc"

	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/calendar"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/server"
	"github.com/Rioverde/gongeons/internal/session"
)

// sshShutdownTimeout caps how long the main loop waits for in-flight
// Bubble Tea sessions to drain once a shutdown signal lands. Ten
// seconds matches the gRPC GracefulStop latency so the two transports
// unwind on comparable timescales.
const sshShutdownTimeout = 10 * time.Second

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
		sshAddr  string
		hostKey  string
		logLevel string
		seed     int64
	)
	flag.StringVar(&addr, "addr", ":50051", "gRPC listen address")
	flag.StringVar(&sshAddr, "ssh-addr", ":2222", "SSH listen address")
	flag.StringVar(&hostKey, "ssh-host-key", ".ssh/gongeons_host_ed25519",
		"SSH host key path (auto-generated on first run)")
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

	sshSrv, err := wish.NewServer(
		wish.WithAddress(sshAddr),
		wish.WithHostKeyPath(hostKey),
		wish.WithMiddleware(
			bm.Middleware(session.Handler(svc)),
			activeterm.Middleware(),
			wishlog.Middleware(),
		),
	)
	if err != nil {
		return fmt.Errorf("new ssh server: %w", err)
	}
	sshServeErr := make(chan error, 1)
	go func() {
		logger.Info("ssh listening", "addr", sshAddr)
		if err := sshSrv.ListenAndServe(); err != nil && !errors.Is(err, cbssh.ErrServerClosed) {
			sshServeErr <- err
			return
		}
		close(sshServeErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serveErr:
		if err != nil {
			return fmt.Errorf("grpc serve: %w", err)
		}
		return nil
	case err := <-sshServeErr:
		if err != nil {
			return fmt.Errorf("ssh serve: %w", err)
		}
		return nil
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), sshShutdownTimeout)
	defer shutdownCancel()
	if err := sshSrv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("ssh shutdown", "err", err)
	}
	grpcSrv.GracefulStop()
	logger.Info("graceful shutdown complete")
	return nil
}

// buildWorld constructs the production world: a procedural tile source keyed
// on seed, a matching NoiseRegionSource for Voronoi regions, a
// NoiseLandmarkSource for Layer 1.5 landmarks, a NoiseVolcanoSource for the
// multi-tile volcano layer, a NoiseDepositSource for the resource-deposit
// layer, and the seed itself threaded through so AnchorAt stays
// deterministic. Split out of run for testability and so the wiring is
// visible at a glance. Source order matters: the volcano source
// depends on the landmark source for anchor-collision rejection, and
// the deposit source depends on both volcano (for obsidian / sulfur
// structural placement) and landmark (for point-like collision
// rejection), so deposits are constructed last.
func buildWorld(seed int64) *world.World {
	wg := worldgen.NewChunkedSource(seed)
	regionSrc := worldgen.NewNoiseRegionSource(seed)
	landmarkSrc := worldgen.NewNoiseLandmarkSource(seed, regionSrc, wg.Generator())
	volcanoSrc := worldgen.NewNoiseVolcanoSource(seed, wg.Generator(), landmarkSrc)
	depositSrc := worldgen.NewNoiseDepositSource(seed, wg.Generator(), landmarkSrc, volcanoSrc)
	cal := calendar.NewCalendar(
		calendar.DefaultCalendarConfig.TicksPerDay,
		calendar.DefaultCalendarConfig.DaysPerMonth,
		calendar.DefaultCalendarConfig.MonthsPerYear,
		calendar.DefaultEpochOffset(seed),
	)
	return world.NewWorld(
		wg,
		world.WithSeed(seed),
		world.WithRegionSource(regionSrc),
		world.WithLandmarkSource(landmarkSrc),
		world.WithVolcanoSource(volcanoSrc),
		world.WithDepositSource(depositSrc),
		world.WithCalendar(cal),
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

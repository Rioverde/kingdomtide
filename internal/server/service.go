package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// tickInterval is the cadence at which the world simulation advances. 100 ms
// (10 Hz) balances responsiveness — worst-case action latency is one interval —
// against CPU cost on an otherwise-idle server. Changing this constant is the
// single knob for tuning tick rate; all gameplay speeds (SpeedNormal = 12,
// baseActionCost = 12) stay meaningful regardless of wall-clock cadence.
const tickInterval = 100 * time.Millisecond

// viewportDims is the per-client Snapshot window size. Both dimensions must
// be positive; clampViewport in mapper.go enforces the floor.
type viewportDims struct {
	width, height int
}

// ViewportDims is the exported alias used by session-mode callers (the
// UI package when running under SSH) to request a snapshot size
// without reaching for the unexported internal shape. The field names
// are exported so the caller can construct one by name; conversion
// into the internal form happens at the Service boundary.
type ViewportDims struct {
	Width, Height int
}

// toInternal converts the exported shape into the unexported one used
// across the service's internal paths. Kept tiny so the two stays
// trivially in-sync when new fields arrive.
func (d ViewportDims) toInternal() viewportDims {
	return viewportDims{width: d.Width, height: d.Height}
}

// Service is the authoritative gRPC server for a single shared world. All
// gameplay mutations funnel through ApplyCommand under mu; mu is held for
// the smallest possible window, never across network I/O. The viewports
// map stores each connected player's Snapshot size so dispatch can re-send
// a centred view after self-moves. regions caches region lookups keyed by
// SuperChunkCoord so a tile-crossing snapshot does not re-sample six noise
// fields per hop. landmarks caches landmark slices per SuperChunkCoord so
// repeated snapshots over the same terrain area do not re-run the placement
// algorithm. Localization is entirely client-side; the server is language-agnostic.
type Service struct {
	pb.UnimplementedGameServiceServer

	mu        sync.Mutex
	world     *game.World
	hub       *Hub
	sessions  *sessionHub
	log       *slog.Logger
	viewports map[string]viewportDims
	regions   *regionCache
	landmarks *landmarkCache
	volcanoes *volcanoCache
}

// NewService constructs a Service around the given world. If log is nil,
// slog.Default is used. If the world exposes a non-nil RegionSource
// (configured via game.WithRegionSource), the service wires an LRU-backed
// region cache around it so repeated snapshots on the same super-chunk do
// not re-sample six noise fields per call. Similarly, if the world exposes
// a non-nil LandmarkSource (configured via game.WithLandmarkSource), the
// service wires an LRU-backed landmark cache so the placement algorithm
// runs at most once per super-chunk per session.
func NewService(w *game.World, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	svc := &Service{
		world:     w,
		hub:       NewHub(log),
		sessions:  newSessionHub(log),
		log:       log,
		viewports: make(map[string]viewportDims),
	}
	if src := w.RegionSource(); src != nil {
		svc.regions = newRegionCache(src, DefaultRegionCacheCapacity)
	}
	if src := w.LandmarkSource(); src != nil {
		svc.landmarks = newLandmarkCache(src, DefaultLandmarkCacheCapacity)
	}
	if src := w.VolcanoSource(); src != nil {
		svc.volcanoes = newVolcanoCache(src, DefaultVolcanoCacheCapacity)
	}
	return svc
}

// Play implements pb.GameServiceServer. Each call is one client session:
// Join, stream events until disconnect, LeaveCmd on exit.
func (s *Service) Play(stream pb.GameService_PlayServer) error {
	ctx := stream.Context()

	name, dims, stats, err := readJoinFrame(stream)
	if err != nil {
		return err
	}

	playerID := uuid.NewString()
	spawn, snap, joinEvents, err := s.bootJoin(playerID, name, dims, stats)
	if err != nil {
		return err
	}
	s.log.Info("player joined", "id", playerID, "name", name, "spawn", spawn,
		"viewport", dims)

	outbox, unsub := s.hub.Subscribe(playerID)
	writeCtx, cancelWrite := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.pumpWrites(writeCtx, stream, outbox, playerID, &wg)

	defer s.cleanup(playerID, name, cancelWrite, &wg, unsub)

	s.hub.SendTo(playerID, acceptedResponse(playerID, spawn, s.world.Seed()))
	s.hub.SendTo(playerID, snapshotResponse(snap))
	s.broadcastEvents(joinEvents)

	s.readLoop(ctx, stream, playerID)
	return nil
}

// readJoinFrame reads the first frame, enforces that it is a JoinRequest,
// and returns the player name, requested viewport dims, and validated
// CoreStats. JoinRequest.language is accepted on the wire for future
// uses (e.g. telemetry) but the server does not act on it —
// localization is entirely client-side.
//
// Stats handling follows a two-tier contract: a missing or all-zero
// stats field is treated as "legacy client, no character creation UI"
// and falls back to DefaultCoreStats (all 10s) so the stream still
// succeeds; a present-but-invalid distribution is rejected with a
// localized status whose message_id is KeyErrorInvalidStats. The
// assumption is that conformant clients (CS4) validate Point Buy in
// their UI before sending, so a bad value on the wire signals either a
// stale client or an attempt to tamper — fail closed.
func readJoinFrame(stream pb.GameService_PlayServer) (string, viewportDims, game.CoreStats, error) {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", viewportDims{}, game.CoreStats{}, nil
		}
		return "", viewportDims{}, game.CoreStats{}, err
	}
	join := first.GetJoin()
	if join == nil {
		return "", viewportDims{}, game.CoreStats{},
			status.Error(codes.InvalidArgument, "first message must be Join")
	}
	name := join.GetName()
	if name == "" {
		return "", viewportDims{}, game.CoreStats{},
			status.Error(codes.InvalidArgument, "name required")
	}

	stats, err := validateJoinStats(join.GetStats())
	if err != nil {
		return "", viewportDims{}, game.CoreStats{}, err
	}
	return name, viewportDims{
		width:  int(join.GetViewportWidth()),
		height: int(join.GetViewportHeight()),
	}, stats, nil
}

// validateJoinStats turns the wire CoreStats into the domain type. The
// rules follow the join-handshake contract:
//
//   - nil wire payload (field not set, or whole JoinRequest from a
//     legacy client) ⇒ DefaultCoreStats. No validation; the server
//     silently substitutes a neutral baseline so older clients that
//     predate character creation keep working.
//   - non-nil with every field zero ⇒ same DefaultCoreStats fallback.
//     A genuine all-zeros distribution is unreachable under Point Buy
//     (min is 8) so this is necessarily a default-serialized message.
//   - any other non-nil payload ⇒ run Point Buy validation; on failure
//     return a LocalizedMessage-enriched gRPC status keyed on
//     KeyErrorInvalidStats so the client can render the rejection in
//     its own language.
func validateJoinStats(pbStats *pb.CoreStats) (game.CoreStats, error) {
	if pbStats == nil {
		return game.DefaultCoreStats(), nil
	}
	return validateDomainJoinStats(coreStatsFromPB(pbStats))
}

// validateDomainJoinStats is the transport-agnostic half of
// validateJoinStats: runs Point Buy validation on a domain CoreStats
// value and returns the same DefaultCoreStats fallback / localized
// status shape. Both the gRPC readJoinFrame path (via validateJoinStats)
// and the SSH JoinSession path call this helper so wire-format drift
// (pb vs. direct domain value) never drifts the error shape. A localized
// rejection always carries the KeyErrorInvalidStats message_id so the
// client's catalog lookup resolves identically across transports.
func validateDomainJoinStats(stats game.CoreStats) (game.CoreStats, error) {
	if stats == (game.CoreStats{}) {
		return game.DefaultCoreStats(), nil
	}
	if _, err := game.NewStatsPointBuy(
		stats.Strength,
		stats.Dexterity,
		stats.Constitution,
		stats.Intelligence,
		stats.Wisdom,
		stats.Charisma,
	); err != nil {
		return game.CoreStats{}, localizedStatus(
			codes.InvalidArgument,
			fmt.Sprintf("invalid stats: %v", err),
			locale.KeyErrorInvalidStats,
			nil,
		)
	}
	return stats, nil
}

// bootJoin applies the Join command under the world mutex, records the
// client's viewport preference, and captures the spawn-centred snapshot
// in the same critical section. Stats have already been validated by
// readJoinFrame; by the time we reach bootJoin the distribution is
// guaranteed to satisfy the Point Buy invariant or to be the default
// baseline. Language is a client-side concern and is not stored
// server-side.
func (s *Service) bootJoin(
	playerID, name string,
	dims viewportDims,
	stats game.CoreStats,
) (game.Position, *pb.Snapshot, []game.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.world.ApplyCommand(game.JoinCmd{
		PlayerID: playerID,
		Name:     name,
		Stats:    stats,
	})
	if err != nil {
		s.log.Warn("join failed", "err", err, "name", name)
		return game.Position{}, nil, nil, status.Errorf(codes.ResourceExhausted, "join failed: %v", err)
	}
	s.viewports[playerID] = dims
	spawn := spawnFromEvents(events)
	snap := snapshotOf(s.world, spawn, dims.width, dims.height, s.regionAt(spawn), s.landmarks, s.volcanoes)
	return spawn, snap, events, nil
}

// regionAt resolves the wire Region for a player position via the
// cache, returning nil when no RegionSource is configured (e.g. tests
// that skip region wiring). Callers must hold s.mu — regionAt reads
// the world's seed but does not take the mutex itself. Names are
// language-agnostic Parts so no lang argument is needed.
func (s *Service) regionAt(p game.Position) *pb.Region {
	if s.regions == nil {
		return nil
	}
	_, sc := game.AnchorAt(s.world.Seed(), p.X, p.Y)
	return regionPB(s.regions.At(sc))
}

// spawnFromEvents pulls the PlayerJoined position out of the event slice.
// Falls back to origin if the event is absent, which should never happen
// in the current domain rules.
func spawnFromEvents(events []game.Event) game.Position {
	for _, ev := range events {
		if pj, ok := ev.(game.PlayerJoinedEvent); ok {
			return pj.Position
		}
	}
	return game.Position{}
}

// readLoop drains ClientMessages until the peer disconnects.
func (s *Service) readLoop(ctx context.Context, stream pb.GameService_PlayServer, playerID string) {
	for {
		msg, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				s.log.Warn("recv failed", "err", err, "id", playerID)
			}
			return
		}
		s.dispatch(msg, playerID)
	}
}

// dispatch routes a client message. ViewportCmd updates the stored dims
// and re-sends a fresh snapshot. Lifecycle commands (Join is rejected
// here because the stream has already joined; Leave is not client-sendable
// today) are immediate. Gameplay commands become intents on the world and
// resolve inside Tick — no broadcast happens here for MoveCmd; the tick
// driver fans out the resulting EntityMoved / IntentFailed events.
func (s *Service) dispatch(msg *pb.ClientMessage, playerID string) {
	if vp := msg.GetViewport(); vp != nil {
		s.updateViewport(playerID, int(vp.GetWidth()), int(vp.GetHeight()))
		return
	}

	cmd, cerr := clientMessageToCommand(msg, playerID)
	if cerr != nil {
		s.sendError(playerID, "bad command: "+cerr.Error(), pb.ErrCodeInvalidArgument)
		return
	}
	if _, isJoin := cmd.(game.JoinCmd); isJoin {
		s.sendError(playerID, "already joined", pb.ErrCodeInvalidProtocol)
		return
	}
	if move, ok := cmd.(game.MoveCmd); ok {
		s.enqueueMoveIntent(playerID, move)
		return
	}

	s.mu.Lock()
	events, aerr := s.world.ApplyCommand(cmd)
	s.mu.Unlock()
	if aerr != nil {
		s.sendError(playerID, aerr.Error(), pb.ErrCodeRuleViolation)
		return
	}
	s.broadcastEvents(events)
}

// enqueueMoveIntent translates a MoveCmd into a MoveIntent on the world's
// single pending-intent slot for the player. The intent is resolved at
// the next Tick; this function intentionally does not broadcast anything
// because the tick driver is the one event source for gameplay actions.
// A missing player surfaces as an invalid-protocol error (the stream has
// not joined); any other enqueue error reads as a rule violation so the
// code shape matches the rest of dispatch.
func (s *Service) enqueueMoveIntent(playerID string, c game.MoveCmd) {
	s.mu.Lock()
	err := s.world.EnqueueIntent(playerID, game.MoveIntent{DX: c.DX, DY: c.DY})
	s.mu.Unlock()
	if err == nil {
		return
	}
	code := pb.ErrCodeRuleViolation
	if errors.Is(err, game.ErrPlayerNotFound) {
		code = pb.ErrCodeInvalidProtocol
	}
	s.sendError(playerID, err.Error(), code)
}

// sendError targets a single subscriber with a non-fatal error message.
// Errors are per-player — the stream stays open for the next command.
func (s *Service) sendError(id, msg, code string) {
	s.hub.SendTo(id, errorResponse(msg, code))
}

// snapshotFor builds a viewport Snapshot sized to this player's stored
// dims. Uses the server defaults when the player is unknown or dims
// are unset. The caller must hold s.mu — snapshotFor never takes the
// mutex itself, so callers can compose it into a larger critical
// section. Names travel the wire as structured Parts records; the
// client composes localized display text using its own Model.lang.
func (s *Service) snapshotFor(id string, pos game.Position) *pb.Snapshot {
	dims := s.viewports[id]
	return snapshotOf(s.world, pos, dims.width, dims.height, s.regionAt(pos), s.landmarks, s.volcanoes)
}

// applyCmd is a convenience wrapper used by cleanup: take the mutex,
// apply the command, release. Returning outside the lock lets the caller
// broadcast events without re-entering it.
func (s *Service) applyCmd(cmd game.Command) ([]game.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.world.ApplyCommand(cmd)
}

// updateViewport changes the stored per-player dims and pushes a fresh
// snapshot at the new size. Typically fires when the client's terminal
// resizes.
func (s *Service) updateViewport(playerID string, width, height int) {
	s.mu.Lock()
	s.viewports[playerID] = viewportDims{width: width, height: height}
	pos, ok := s.world.PositionOf(playerID)
	var snap *pb.Snapshot
	if ok {
		snap = s.snapshotFor(playerID, pos)
	}
	s.mu.Unlock()
	if snap != nil {
		s.hub.SendTo(playerID, snapshotResponse(snap))
	}
}

// DoTick advances the world one simulation step and fans out the
// resulting events. Runs the world tick under s.mu (domain mutation
// boundary), then releases the mutex before any broadcast so no
// stream.Send happens while the lock is held. Players whose position
// changed during this tick also receive a fresh viewport snapshot so
// their camera follows across the infinite world. Exposed (exported)
// because M3's ticker goroutine and the service integration tests both
// drive the same path.
//
// Events reach two parallel subscriber groups: the gRPC Hub (wire-
// encoded as pb.ServerMessage) and the in-process session hub (domain
// events delivered directly to SSH Bubble Tea sessions). Both
// broadcasts are non-blocking so one stuck subscriber cannot stall the
// tick loop.
func (s *Service) DoTick() {
	s.mu.Lock()
	events := s.world.Tick()
	snaps := s.followSnapshotsLocked(events)
	s.mu.Unlock()

	s.broadcastEvents(events)
	for id, snap := range snaps {
		s.hub.SendTo(id, snapshotResponse(snap))
	}
	s.publishEventsToSessions(events)
	s.publishSnapshotsToSessions(snaps)
}

// followSnapshotsLocked inspects the events emitted by World.Tick and
// builds a per-player follow-up snapshot for anyone whose position has
// changed. Returns a map keyed by player id so the caller can iterate
// outside the mutex. Caller MUST hold s.mu. Sizing the map to the number
// of EntityMovedEvents avoids an initial grow for typical ticks where
// most events do not move players.
func (s *Service) followSnapshotsLocked(events []game.Event) map[string]*pb.Snapshot {
	moved := movedPlayers(events)
	if len(moved) == 0 {
		return nil
	}
	out := make(map[string]*pb.Snapshot, len(moved))
	for id := range moved {
		pos, ok := s.world.PositionOf(id)
		if !ok {
			continue
		}
		out[id] = s.snapshotFor(id, pos)
	}
	return out
}

// movedPlayers returns the set of entity IDs that appear as the subject
// of an EntityMovedEvent in events. The set form collapses duplicate
// moves in a single tick (the current M2 cap is one action per entity
// per tick, but future intents — knockback, teleport — may emit two).
func movedPlayers(events []game.Event) map[string]struct{} {
	out := make(map[string]struct{})
	for _, ev := range events {
		if em, ok := ev.(game.EntityMovedEvent); ok {
			out[em.EntityID] = struct{}{}
		}
	}
	return out
}

// broadcastEvents converts each domain event to a wire message and fans
// it out to every gRPC subscriber, and publishes the same domain events
// to in-process session subscribers (SSH Bubble Tea sessions). Both
// paths share the same event stream so a gRPC and an SSH client
// connected to the same Service see identical gameplay transitions.
func (s *Service) broadcastEvents(events []game.Event) {
	for _, ev := range events {
		if msg := eventToServerMessage(ev); msg != nil {
			s.hub.Broadcast(msg)
		}
	}
	s.publishEventsToSessions(events)
}

// cleanup runs on Play exit: apply Leave to free the tile, broadcast the
// PlayerLeft event, stop the write pump, join it, and finally unsubscribe.
func (s *Service) cleanup(
	playerID, name string,
	cancelWrite context.CancelFunc,
	wg *sync.WaitGroup,
	unsub func(),
) {
	leaveEvents, _ := s.applyCmd(game.LeaveCmd{PlayerID: playerID})
	s.mu.Lock()
	delete(s.viewports, playerID)
	s.mu.Unlock()
	s.broadcastEvents(leaveEvents)
	cancelWrite()
	wg.Wait()
	unsub()
	s.log.Info("player left", "id", playerID, "name", name)
}

// pumpWrites is the single writer to stream.Send.
func (s *Service) pumpWrites(
	ctx context.Context,
	stream pb.GameService_PlayServer,
	outbox <-chan *pb.ServerMessage,
	id string,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-outbox:
			if !ok {
				return
			}
			if err := stream.Send(msg); err != nil {
				s.log.Warn("send failed, closing writer", "id", id, "err", err)
				return
			}
		}
	}
}

// Run starts the Service's tick loop. It blocks until ctx is cancelled, then
// returns. Intended to be launched as a goroutine from main after the gRPC
// server is ready. The ticker is stopped via defer so no goroutine leak
// occurs: the select on ctx.Done() exits the loop and Stop() reclaims the
// ticker's internal resources before Run returns. Shutdown latency is bounded
// by tickInterval — the loop wakes at most once more after cancellation before
// observing ctx.Done().
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(tickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.DoTick()
		}
	}
}

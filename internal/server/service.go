package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// viewportDims is the per-client Snapshot window size. Both dimensions must
// be positive; clampViewport in mapper.go enforces the floor.
type viewportDims struct {
	width, height int
}

// defaultClientLanguage is the BCP 47 fallback tag used when a client joins
// without a language preference. English keeps every server-generated
// LocalizedMessage catalog lookup resolvable even before Sub-phase 1d
// lands the Russian bundle.
const defaultClientLanguage = "en"

// Service is the authoritative gRPC server for a single shared world. All
// gameplay mutations funnel through ApplyCommand under mu; mu is held for
// the smallest possible window, never across network I/O. The viewports
// map stores each connected player's Snapshot size so dispatch can re-send
// a centred view after self-moves; languages stores each player's BCP 47
// locale tag so future LocalizedMessage emissions reach the correct
// catalog. regions caches region lookups keyed by SuperChunkCoord so a
// tile-crossing snapshot does not re-sample six noise fields per hop.
type Service struct {
	pb.UnimplementedGameServiceServer

	mu        sync.Mutex
	world     *game.World
	hub       *Hub
	log       *slog.Logger
	viewports map[string]viewportDims
	languages map[string]string
	regions   *regionCache
}

// NewService constructs a Service around the given world. If log is nil,
// slog.Default is used. If the world exposes a non-nil RegionSource
// (configured via game.WithRegionSource), the service wires an LRU-backed
// region cache around it so repeated snapshots on the same super-chunk do
// not re-sample six noise fields per call.
func NewService(w *game.World, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	svc := &Service{
		world:     w,
		hub:       NewHub(log),
		log:       log,
		viewports: make(map[string]viewportDims),
		languages: make(map[string]string),
	}
	if src := w.RegionSource(); src != nil {
		svc.regions = newRegionCache(src, DefaultRegionCacheCapacity)
	}
	return svc
}

// Play implements pb.GameServiceServer. Each call is one client session:
// Join, stream events until disconnect, LeaveCmd on exit.
func (s *Service) Play(stream pb.GameService_PlayServer) error {
	ctx := stream.Context()

	name, dims, lang, err := readJoinFrame(stream)
	if err != nil {
		return err
	}

	playerID := uuid.NewString()
	spawn, snap, joinEvents, err := s.bootJoin(playerID, name, dims, lang)
	if err != nil {
		return err
	}
	s.log.Info("player joined", "id", playerID, "name", name, "spawn", spawn,
		"viewport", dims, "lang", lang)

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
// and returns the name, requested viewport dims, and BCP 47 language tag.
// An empty language is normalised to defaultClientLanguage by the caller so
// this helper reflects exactly what the client sent.
func readJoinFrame(stream pb.GameService_PlayServer) (string, viewportDims, string, error) {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", viewportDims{}, "", nil
		}
		return "", viewportDims{}, "", err
	}
	join := first.GetJoin()
	if join == nil {
		return "", viewportDims{}, "", status.Error(codes.InvalidArgument, "first message must be Join")
	}
	name := join.GetName()
	if name == "" {
		return "", viewportDims{}, "", status.Error(codes.InvalidArgument, "name required")
	}
	return name, viewportDims{
		width:  int(join.GetViewportWidth()),
		height: int(join.GetViewportHeight()),
	}, join.GetLanguage(), nil
}

// bootJoin applies the Join command under the world mutex, records the
// client's viewport preference and locale, and captures the spawn-centred
// snapshot in the same critical section.
func (s *Service) bootJoin(
	playerID, name string,
	dims viewportDims,
	lang string,
) (game.Position, *pb.Snapshot, []game.Event, error) {
	if lang == "" {
		lang = defaultClientLanguage
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := s.world.ApplyCommand(game.JoinCmd{PlayerID: playerID, Name: name})
	if err != nil {
		s.log.Warn("join failed", "err", err, "name", name)
		return game.Position{}, nil, nil, status.Errorf(codes.ResourceExhausted, "join failed: %v", err)
	}
	s.viewports[playerID] = dims
	s.languages[playerID] = lang
	spawn := spawnFromEvents(events)
	snap := snapshotOf(s.world, spawn, dims.width, dims.height, s.regionAt(spawn))
	return spawn, snap, events, nil
}

// regionAt resolves the wire Region for a player position via the cache,
// returning nil when no RegionSource is configured (e.g. tests that skip
// region wiring). Callers must hold s.mu — regionAt reads the world's
// seed but does not take the mutex itself.
func (s *Service) regionAt(p game.Position) *pb.Region {
	if s.regions == nil {
		return nil
	}
	_, sc := game.AnchorAt(s.world.Seed(), p.X, p.Y)
	return regionPB(s.regions.At(sc))
}

// LanguageOf returns the stored BCP 47 locale tag for the given player,
// or defaultClientLanguage when the player is unknown. Exported for tests
// that assert the join-side language plumbing; production code reads via
// the languages map directly under s.mu.
func (s *Service) LanguageOf(playerID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lang, ok := s.languages[playerID]; ok {
		return lang
	}
	return defaultClientLanguage
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
// and re-sends a fresh snapshot; commands go through ApplyCommand and, if
// they moved this player, trigger a follow-up snapshot so the camera
// tracks the player across the infinite world.
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

	s.mu.Lock()
	events, aerr := s.world.ApplyCommand(cmd)
	var followSnap *pb.Snapshot
	if aerr == nil && movedSelf(events, playerID) {
		if pos, ok := s.world.PositionOf(playerID); ok {
			followSnap = s.snapshotFor(playerID, pos)
		}
	}
	s.mu.Unlock()
	if aerr != nil {
		s.sendError(playerID, aerr.Error(), pb.ErrCodeRuleViolation)
		return
	}
	s.broadcastEvents(events)
	if followSnap != nil {
		s.hub.SendTo(playerID, snapshotResponse(followSnap))
	}
}

// sendError targets a single subscriber with a non-fatal error message.
// Errors are per-player — the stream stays open for the next command.
func (s *Service) sendError(id, msg, code string) {
	s.hub.SendTo(id, errorResponse(msg, code))
}

// snapshotFor builds a viewport Snapshot sized to this player's stored
// dims. Uses the server defaults when the player is unknown or dims are
// unset. The caller must hold s.mu — snapshotFor never takes the mutex
// itself, so callers can compose it into a larger critical section.
func (s *Service) snapshotFor(id string, pos game.Position) *pb.Snapshot {
	dims := s.viewports[id]
	return snapshotOf(s.world, pos, dims.width, dims.height, s.regionAt(pos))
}

// applyCmd is the short critical section that every world mutation goes
// through: take the mutex, apply, release. Returning outside the lock
// lets callers broadcast events without re-entering it.
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

// movedSelf reports whether events includes an EntityMovedEvent whose
// subject is the given player ID.
func movedSelf(events []game.Event, id string) bool {
	for _, ev := range events {
		if em, ok := ev.(game.EntityMovedEvent); ok && em.EntityID == id {
			return true
		}
	}
	return false
}

// broadcastEvents converts each domain event to a wire message and fans it
// out to every subscriber.
func (s *Service) broadcastEvents(events []game.Event) {
	for _, ev := range events {
		if msg := eventToServerMessage(ev); msg != nil {
			s.hub.Broadcast(msg)
		}
	}
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
	delete(s.languages, playerID)
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

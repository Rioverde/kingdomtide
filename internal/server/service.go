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

// Service is the authoritative gRPC server for a single shared world. All
// gameplay mutations funnel through ApplyCommand under mu; mu is held for the
// smallest possible window, never across network I/O.
type Service struct {
	pb.UnimplementedGameServiceServer

	mu    sync.Mutex
	world *game.World
	hub   *Hub
	log   *slog.Logger
}

// NewService constructs a Service around the given world. The world becomes
// the property of the service — do not mutate it from the outside after this
// call. If log is nil, slog.Default is used.
func NewService(w *game.World, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		world: w,
		hub:   NewHub(log),
		log:   log,
	}
}

// Play implements pb.GameServiceServer. Each call is one client session: Join,
// stream events until disconnect, LeaveCmd on exit. The body is thin by
// design — the heavy lifting is factored into handshake/session helpers so
// the state machine is readable at a glance.
func (s *Service) Play(stream pb.GameService_PlayServer) error {
	ctx := stream.Context()

	name, err := readJoinName(stream)
	if err != nil {
		return err
	}

	playerID := uuid.NewString()
	snap, joinEvents, err := s.bootJoin(playerID, name)
	if err != nil {
		return err
	}
	s.log.Info("player joined", "id", playerID, "name", name)

	outbox, unsub := s.hub.Subscribe(playerID)
	writeCtx, cancelWrite := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go s.pumpWrites(writeCtx, stream, outbox, playerID, &wg)

	defer s.cleanup(playerID, name, cancelWrite, &wg, unsub)

	s.hub.SendTo(playerID, acceptedResponse(playerID))
	s.hub.SendTo(playerID, snapshotResponse(snap))
	s.broadcastEvents(joinEvents)

	s.readLoop(ctx, stream, playerID)
	return nil
}

// readJoinName reads the first frame, enforces that it is a JoinRequest and
// returns the name. Any protocol violation results in a gRPC status error.
func readJoinName(stream pb.GameService_PlayServer) (string, error) {
	first, err := stream.Recv()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", err
	}
	join := first.GetJoin()
	if join == nil {
		return "", status.Error(codes.InvalidArgument, "first message must be Join")
	}
	name := join.GetName()
	if name == "" {
		return "", status.Error(codes.InvalidArgument, "name required")
	}
	return name, nil
}

// bootJoin applies the Join command under the world mutex and captures a
// snapshot in the same critical section so the joining client gets a view
// consistent with the Join event other clients will receive.
func (s *Service) bootJoin(playerID, name string) (*pb.Snapshot, []game.Event, error) {
	s.mu.Lock()
	events, err := s.world.ApplyCommand(game.JoinCmd{PlayerID: playerID, Name: name})
	snap := snapshotOf(s.world)
	s.mu.Unlock()
	if err != nil {
		s.log.Warn("join failed", "err", err, "name", name)
		return nil, nil, status.Errorf(codes.ResourceExhausted, "join failed: %v", err)
	}
	return snap, events, nil
}

// readLoop drains ClientMessages until the peer disconnects. Any terminal
// condition (EOF, ctx cancel, recv error) ends the loop; per-message handling
// lives in dispatch.
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

// dispatch applies a single client message to the world and broadcasts the
// resulting events. Rule violations and protocol errors are reported back to
// the originating client only; they do not terminate the session.
func (s *Service) dispatch(msg *pb.ClientMessage, playerID string) {
	cmd, cerr := clientMessageToCommand(msg, playerID)
	if cerr != nil {
		s.hub.SendTo(playerID, errorResponse("bad command: "+cerr.Error(), "invalid_argument"))
		return
	}
	if _, isJoin := cmd.(game.JoinCmd); isJoin {
		s.hub.SendTo(playerID, errorResponse("already joined", "invalid_protocol"))
		return
	}

	s.mu.Lock()
	events, aerr := s.world.ApplyCommand(cmd)
	s.mu.Unlock()
	if aerr != nil {
		s.hub.SendTo(playerID, errorResponse(aerr.Error(), "rule_violation"))
		return
	}
	s.broadcastEvents(events)
}

// broadcastEvents converts each domain event to a wire message and fans it
// out to every subscriber. Unknown event types (nil wire form) are silently
// skipped.
func (s *Service) broadcastEvents(events []game.Event) {
	for _, ev := range events {
		if msg := eventToServerMessage(ev); msg != nil {
			s.hub.Broadcast(msg)
		}
	}
}

// cleanup runs on Play exit: apply Leave to free the tile, broadcast the
// PlayerLeft event, stop the write pump, join it, and finally unsubscribe.
// Order matters — the Leave broadcast must happen while the subscriber is
// still registered so the disconnecting client also observes its own farewell
// if the stream is still alive.
func (s *Service) cleanup(
	playerID, name string,
	cancelWrite context.CancelFunc,
	wg *sync.WaitGroup,
	unsub func(),
) {
	s.mu.Lock()
	leaveEvents, _ := s.world.ApplyCommand(game.LeaveCmd{PlayerID: playerID})
	s.mu.Unlock()
	s.broadcastEvents(leaveEvents)
	cancelWrite()
	wg.Wait()
	unsub()
	s.log.Info("player left", "id", playerID, "name", name)
}

// pumpWrites is the single writer to stream.Send. Exits on context
// cancellation, on outbox closure, or on the first send error.
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

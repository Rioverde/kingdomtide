package server

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Rioverde/gongeons/internal/game/world"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// recvTimeout bounds any single wait on the test stream. Two seconds is long
// enough for goroutine scheduling hiccups on a loaded CI host, short enough
// that a genuine deadlock surfaces as a failure, not a 30-second timeout.
const recvTimeout = 2 * time.Second

// testWorld returns a deterministic world for integration tests. Uses a
// fixed seed so failure modes are reproducible, not whim-of-the-clock.
func testWorld() *world.World { return worldgen.NewWorld(1) }

// startTestServer brings up a real gRPC server on a random localhost port and
// returns a client plus a cleanup function. Each test gets its own world, so
// they cannot interact. The Service pointer is returned so M2+ tests can
// drive the tick manually via svc.DoTick — no background ticker runs here
// (M3 wires that in production main.go, not in the test fixture).
func startTestServer(t *testing.T) (pb.GameServiceClient, *Service, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	svc := NewService(testWorld(), silentLog())
	pb.RegisterGameServiceServer(grpcSrv, svc)

	serveErr := make(chan error, 1)
	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			serveErr <- err
		}
		close(serveErr)
	}()

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		grpcSrv.Stop()
		_ = lis.Close()
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
		<-serveErr
	}
	return pb.NewGameServiceClient(conn), svc, cleanup
}

// recvUntil reads from the stream until a message matching pred arrives, or
// recvTimeout elapses. Returns the matched message or fails the test.
func recvUntil(
	t *testing.T,
	stream pb.GameService_PlayClient,
	pred func(*pb.ServerMessage) bool,
) *pb.ServerMessage {
	t.Helper()
	deadline := time.Now().Add(recvTimeout)
	for time.Now().Before(deadline) {
		msg, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				t.Fatal("stream closed before match")
			}
			t.Fatalf("recv: %v", err)
		}
		if pred(msg) {
			return msg
		}
	}
	t.Fatal("timed out waiting for matching message")
	return nil
}

func TestIntegrationSingleJoin(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice"},
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	accepted := first.GetAccepted()
	if accepted == nil || accepted.GetPlayerId() == "" {
		t.Fatalf("expected JoinAccepted with id, got %v", first)
	}

	second := recvUntil(t, stream, func(m *pb.ServerMessage) bool {
		return m.GetSnapshot() != nil
	})
	snap := second.GetSnapshot()
	if snap.GetWidth() != int32(DefaultViewportWidth) || snap.GetHeight() != int32(DefaultViewportHeight) {
		t.Fatalf("snapshot size: %dx%d, want %dx%d",
			snap.GetWidth(), snap.GetHeight(), DefaultViewportWidth, DefaultViewportHeight)
	}
	var found bool
	for _, e := range snap.GetEntities() {
		if e.GetName() == "alice" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("alice missing from snapshot entities")
	}
}

func TestIntegrationTwoClientsAndMove(t *testing.T) {
	client, svc, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	alice, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("alice Play: %v", err)
	}
	if err := alice.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice"},
	}}); err != nil {
		t.Fatalf("alice send join: %v", err)
	}
	aliceAccepted, err := alice.Recv()
	if err != nil {
		t.Fatalf("alice recv: %v", err)
	}
	aliceID := aliceAccepted.GetAccepted().GetPlayerId()
	if aliceID == "" {
		t.Fatalf("alice missing id: %v", aliceAccepted)
	}
	recvUntil(t, alice, func(m *pb.ServerMessage) bool { return m.GetSnapshot() != nil })

	bob, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("bob Play: %v", err)
	}
	if err := bob.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "bob"},
	}}); err != nil {
		t.Fatalf("bob send join: %v", err)
	}
	bobAccepted, err := bob.Recv()
	if err != nil {
		t.Fatalf("bob recv: %v", err)
	}
	bobID := bobAccepted.GetAccepted().GetPlayerId()
	if bobID == "" {
		t.Fatalf("bob missing id: %v", bobAccepted)
	}
	bobSnap := recvUntil(t, bob, func(m *pb.ServerMessage) bool {
		return m.GetSnapshot() != nil
	}).GetSnapshot()
	if len(bobSnap.GetEntities()) < 2 {
		t.Fatalf("bob snapshot entities: %d, want 2", len(bobSnap.GetEntities()))
	}

	recvUntil(t, alice, func(m *pb.ServerMessage) bool {
		pj := m.GetEvent().GetPlayerJoined()
		return pj != nil && pj.GetEntity().GetId() == bobID
	})

	// Alice moves south. East would be the next spawn tile picked for bob,
	// which would block. MoveCmd now enqueues an intent — resolution
	// happens inside DoTick. Poll the service until the enqueue is
	// visible so the subsequent DoTick deterministically produces an
	// EntityMoved event for alice; a naked DoTick could race the gRPC
	// in-flight Send and tick against an empty intent slot.
	if err := alice.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{
		Move: &pb.MoveCmd{Dx: 0, Dy: 1},
	}}); err != nil {
		t.Fatalf("alice send move: %v", err)
	}
	waitForIntent(t, svc, aliceID)
	svc.DoTick()

	recvUntil(t, alice, func(m *pb.ServerMessage) bool {
		em := m.GetEvent().GetEntityMoved()
		return em != nil && em.GetEntityId() == aliceID
	})
	recvUntil(t, bob, func(m *pb.ServerMessage) bool {
		em := m.GetEvent().GetEntityMoved()
		return em != nil && em.GetEntityId() == aliceID
	})
}

// waitForIntent spins for up to recvTimeout waiting for the given
// player's Intent slot to be set on the server's world. Bridges the
// unavoidable gap between a gRPC client Send returning and the server
// goroutine that actually runs dispatch — without this the test could
// race DoTick against an empty slot and spuriously fail.
func waitForIntent(t *testing.T, svc *Service, playerID string) {
	t.Helper()
	deadline := time.Now().Add(recvTimeout)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		p, ok := svc.world.PlayerByID(playerID)
		has := ok && p != nil && p.Intent != nil
		svc.mu.Unlock()
		if has {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for intent on %s", playerID)
}

// TestIntegrationMoveQueuesIntentResolvesOnTick asserts M2 semantics: a
// MoveCmd enqueues an intent on the world (visible via the player's
// Intent slot) and the EntityMoved event materialises only when DoTick
// runs. The intent-slot poll doubles as the negative assertion — under
// the pre-M2 immediate-apply path, MoveCmd never set Intent, so
// waitForIntent would time out.
func TestIntegrationMoveQueuesIntentResolvesOnTick(t *testing.T) {
	client, svc, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice"},
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}
	accepted, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv accepted: %v", err)
	}
	aliceID := accepted.GetAccepted().GetPlayerId()
	if aliceID == "" {
		t.Fatalf("missing player id: %v", accepted)
	}
	recvUntil(t, stream, func(m *pb.ServerMessage) bool {
		return m.GetSnapshot() != nil
	})

	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{
		Move: &pb.MoveCmd{Dx: 0, Dy: 1},
	}}); err != nil {
		t.Fatalf("send move: %v", err)
	}
	// Intent slot must be populated — proof that dispatch went through
	// EnqueueIntent, not the legacy ApplyCommand(MoveCmd) path.
	waitForIntent(t, svc, aliceID)

	// Position must still be the spawn tile: the intent has not yet
	// resolved. Tick runs inside the service's own mutex, so this read
	// takes it too. Under the pre-M2 path the player would already be
	// one tile south here.
	svc.mu.Lock()
	spawn, _ := svc.world.PositionOf(aliceID)
	svc.mu.Unlock()

	svc.DoTick()

	recvUntil(t, stream, func(m *pb.ServerMessage) bool {
		em := m.GetEvent().GetEntityMoved()
		return em != nil && em.GetEntityId() == aliceID
	})

	svc.mu.Lock()
	after, _ := svc.world.PositionOf(aliceID)
	svc.mu.Unlock()
	if after.Y-spawn.Y != 1 || after.X != spawn.X {
		t.Fatalf("position after tick = %+v, want one step south of %+v",
			after, spawn)
	}
}

// startTestServerWithTicker is like startTestServer but also launches svc.Run
// in a goroutine using the provided context. Use this for tests that need the
// real ticker to fire rather than manual DoTick calls. The existing
// startTestServer tests remain unaffected — they drive ticks explicitly.
func startTestServerWithTicker(t *testing.T, ctx context.Context) (pb.GameServiceClient, *Service, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	svc := NewService(testWorld(), silentLog())
	pb.RegisterGameServiceServer(grpcSrv, svc)

	serveErr := make(chan error, 1)
	go func() {
		if err := grpcSrv.Serve(lis); err != nil {
			serveErr <- err
		}
		close(serveErr)
	}()

	go svc.Run(ctx)

	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		grpcSrv.Stop()
		_ = lis.Close()
		t.Fatalf("dial: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		grpcSrv.GracefulStop()
		<-serveErr
	}
	return pb.NewGameServiceClient(conn), svc, cleanup
}

// TestIntegrationTickerMovesEntityAfterDelay verifies that the real ticker
// picks up an enqueued MoveIntent and emits an EntityMoved event within a
// bounded time window — no manual DoTick required. The budget is generous
// (500 ms) to avoid flakes on loaded CI hosts while still catching genuine
// hangs (< recvTimeout = 2 s).
func TestIntegrationTickerMovesEntityAfterDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, _, cleanup := startTestServerWithTicker(t, ctx)
	defer cleanup()

	streamCtx, streamCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer streamCancel()

	stream, err := client.Play(streamCtx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice"},
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}

	accepted, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv accepted: %v", err)
	}
	aliceID := accepted.GetAccepted().GetPlayerId()
	if aliceID == "" {
		t.Fatalf("missing player id: %v", accepted)
	}
	recvUntil(t, stream, func(m *pb.ServerMessage) bool { return m.GetSnapshot() != nil })

	// Send a move and wait for EntityMoved. The real ticker fires every
	// tickInterval (100 ms); the player has Energy = baseActionCost from join
	// so the first tick after the intent arrives resolves it. Budget: 500 ms.
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{
		Move: &pb.MoveCmd{Dx: 0, Dy: 1},
	}}); err != nil {
		t.Fatalf("send move: %v", err)
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		msg, err := stream.Recv()
		if err != nil {
			t.Fatalf("recv: %v", err)
		}
		if em := msg.GetEvent().GetEntityMoved(); em != nil && em.GetEntityId() == aliceID {
			return // success — ticker fired and resolved the intent
		}
	}
	t.Fatal("EntityMoved not received within 500 ms of MoveCmd (ticker did not fire or resolve intent)")
}

// TestServiceRunStopsOnContextCancel verifies that Run exits within 500 ms
// of ctx cancellation. The done channel is closed by the goroutine that runs
// Run so the select below races the timer — a timeout means a goroutine leak.
func TestServiceRunStopsOnContextCancel(t *testing.T) {
	svc := NewService(testWorld(), silentLog())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() { svc.Run(ctx); close(done) }()

	// Let the ticker fire at least once to confirm it actually started.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Run exited cleanly after ctx cancel.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Run did not exit within 500 ms of ctx cancel")
	}
}

func TestIntegrationMissingJoinRejected(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{
		Move: &pb.MoveCmd{Dx: 1, Dy: 0},
	}}); err != nil {
		t.Fatalf("send move: %v", err)
	}
	if _, err := stream.Recv(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

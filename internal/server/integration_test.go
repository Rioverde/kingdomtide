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

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// recvTimeout bounds any single wait on the test stream. Two seconds is long
// enough for goroutine scheduling hiccups on a loaded CI host, short enough
// that a genuine deadlock surfaces as a failure, not a 30-second timeout.
const recvTimeout = 2 * time.Second

// testWorld returns a deterministic world for integration tests. Uses a
// fixed seed so failure modes are reproducible, not whim-of-the-clock.
func testWorld() *game.World { return worldgen.NewWorld(1) }

// startTestServer brings up a real gRPC server on a random localhost port and
// returns a client plus a cleanup function. Each test gets its own world, so
// they cannot interact.
func startTestServer(t *testing.T) (pb.GameServiceClient, func()) {
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
	return pb.NewGameServiceClient(conn), cleanup
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
	client, cleanup := startTestServer(t)
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
	client, cleanup := startTestServer(t)
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
	// which would block.
	if err := alice.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Move{
		Move: &pb.MoveCmd{Dx: 0, Dy: 1},
	}}); err != nil {
		t.Fatalf("alice send move: %v", err)
	}

	recvUntil(t, alice, func(m *pb.ServerMessage) bool {
		em := m.GetEvent().GetEntityMoved()
		return em != nil && em.GetEntityId() == aliceID
	})
	recvUntil(t, bob, func(m *pb.ServerMessage) bool {
		em := m.GetEvent().GetEntityMoved()
		return em != nil && em.GetEntityId() == aliceID
	})
}

func TestIntegrationMissingJoinRejected(t *testing.T) {
	client, cleanup := startTestServer(t)
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

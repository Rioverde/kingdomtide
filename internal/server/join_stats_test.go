package server

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Rioverde/gongeons/internal/game"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// validPointBuy returns the standard-array distribution
// (15, 14, 13, 12, 10, 8) — canonical 27-point build — as a wire
// CoreStats. Shared by every stats-on-join test so the success path is
// exercised with a single well-known payload.
func validPointBuy() *pb.CoreStats {
	return &pb.CoreStats{
		Strength:     15,
		Dexterity:    14,
		Constitution: 13,
		Intelligence: 12,
		Wisdom:       10,
		Charisma:     8,
	}
}

// TestJoinAcceptsValidStats verifies that a JoinRequest carrying a Point
// Buy-compliant distribution is accepted: the server emits
// JoinAccepted, the subsequent snapshot lists the player, and the
// in-world Player reflects the derived stats (MaxHP, Speed) computed
// from the payload.
func TestJoinAcceptsValidStats(t *testing.T) {
	client, svc, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice", Stats: validPointBuy()},
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}

	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	accepted := first.GetAccepted()
	if accepted == nil || accepted.GetPlayerId() == "" {
		t.Fatalf("first message: want JoinAccepted with id, got %v", first)
	}

	recvUntil(t, stream, func(m *pb.ServerMessage) bool {
		return m.GetSnapshot() != nil
	})

	// Cross-check the in-world Player carries the derived fields from
	// the validated distribution so the domain → server → wire chain is
	// proven end-to-end.
	expected, err := game.NewStatsPointBuy(15, 14, 13, 12, 10, 8)
	if err != nil {
		t.Fatalf("NewStatsPointBuy: %v", err)
	}
	svc.mu.Lock()
	p, ok := svc.world.PlayerByID(accepted.GetPlayerId())
	svc.mu.Unlock()
	if !ok {
		t.Fatalf("player missing from world after join")
	}
	if p.Stats != *expected {
		t.Errorf("Stats = %+v, want %+v", p.Stats, *expected)
	}
	if p.MaxHP != expected.MaxHP() {
		t.Errorf("MaxHP = %d, want %d", p.MaxHP, expected.MaxHP())
	}
	if p.Speed != expected.DerivedSpeed() {
		t.Errorf("Speed = %d, want %d", p.Speed, expected.DerivedSpeed())
	}
}

// TestJoinRejectsInvalidStats covers the failure path. A distribution
// that sums to 28 instead of 27 must be rejected with an
// InvalidArgument gRPC status whose LocalizedMessage carries the
// KeyErrorInvalidStats key.
func TestJoinRejectsInvalidStats(t *testing.T) {
	client, _, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	// 15,15,13,12,10,8 -> 9+9+5+4+2+0 = 29. Over budget, invalid.
	bad := &pb.CoreStats{
		Strength: 15, Dexterity: 15, Constitution: 13,
		Intelligence: 12, Wisdom: 10, Charisma: 8,
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice", Stats: bad},
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("want rejection error, got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("error is not a gRPC status: %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("status code = %s, want InvalidArgument", st.Code())
	}
	// The LocalizedMessage detail must carry the invalid-stats key so
	// clients can render the error in their own locale.
	var foundKey string
	for _, d := range st.Details() {
		if lm, ok := d.(*pb.LocalizedMessage); ok {
			foundKey = lm.GetMessageId()
			break
		}
	}
	if foundKey != "error.invalid_stats" {
		t.Errorf("LocalizedMessage.message_id = %q, want error.invalid_stats", foundKey)
	}
}

// TestJoinAcceptsMissingStats checks the graceful fallback: a legacy
// JoinRequest with no stats field still succeeds; the server
// substitutes the neutral baseline so DefaultCoreStats(10s) hydrates
// every derived field.
func TestJoinAcceptsMissingStats(t *testing.T) {
	client, svc, cleanup := startTestServer(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.ClientMessage{Payload: &pb.ClientMessage_Join{
		Join: &pb.JoinRequest{Name: "alice"}, // no stats
	}}); err != nil {
		t.Fatalf("send join: %v", err)
	}
	first, err := stream.Recv()
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	accepted := first.GetAccepted()
	if accepted == nil {
		t.Fatalf("first message: want JoinAccepted, got %v", first)
	}

	svc.mu.Lock()
	p, ok := svc.world.PlayerByID(accepted.GetPlayerId())
	svc.mu.Unlock()
	if !ok {
		t.Fatalf("player missing from world after join")
	}
	if p.Stats != game.DefaultCoreStats() {
		t.Errorf("Stats = %+v, want DefaultCoreStats", p.Stats)
	}
}

// TestCoreStatsRoundTrip verifies the mapper helpers are inverse
// operations: every domain CoreStats survives a PB round trip
// unchanged. A fast regression signal if the proto field numbering or
// type widths drift.
func TestCoreStatsRoundTrip(t *testing.T) {
	src := game.CoreStats{
		Strength: 15, Dexterity: 14, Constitution: 13,
		Intelligence: 12, Wisdom: 10, Charisma: 8,
	}
	got := coreStatsFromPB(coreStatsToPB(src))
	if got != src {
		t.Errorf("round trip = %+v, want %+v", got, src)
	}
}

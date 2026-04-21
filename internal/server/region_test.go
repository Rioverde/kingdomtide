package server

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/Rioverde/gongeons/internal/game"
	"github.com/Rioverde/gongeons/internal/game/worldgen"
	pb "github.com/Rioverde/gongeons/internal/proto"
)

// testRegionSeed is a fixed seed used by every region integration test so
// failures are reproducible. Decoupled from the tile-source seed in
// testWorld to prove the wiring honours the RegionSource option rather
// than leaking its seed from the tile source.
const testRegionSeed int64 = 0x2f6f7a3d

// testRegionWorld builds a procedural world seeded at testRegionSeed and
// wired with a NoiseRegionSource, matching the production buildWorld path
// in cmd/server/main.go. Kept local to this file because the existing
// integration harness (integration_test.go testWorld) deliberately avoids
// region wiring to keep its assertions terse.
func testRegionWorld() *game.World {
	return game.NewWorld(
		worldgen.NewChunkedSource(testRegionSeed),
		game.WithSeed(testRegionSeed),
		game.WithRegionSource(worldgen.NewNoiseRegionSource(testRegionSeed)),
	)
}

// startRegionTestServer mirrors startTestServer but swaps in testRegionWorld
// so region-aware assertions see a fully-wired Service. The Service pointer
// is returned for completeness; callers that do not need it use _.
func startRegionTestServer(t *testing.T) (pb.GameServiceClient, *Service, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	grpcSrv := grpc.NewServer()
	svc := NewService(testRegionWorld(), silentLog())
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

func TestSnapshotRegionPopulated(t *testing.T) {
	client, _, cleanup := startRegionTestServer(t)
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

	snap := recvUntil(t, stream, func(m *pb.ServerMessage) bool {
		return m.GetSnapshot() != nil
	}).GetSnapshot()

	region := snap.GetRegion()
	if region == nil {
		t.Fatal("Snapshot.Region: want non-nil, got nil")
	}
	name := region.GetName()
	if name == nil {
		t.Fatalf("Region.Name: want non-nil NameParts, got %+v", region)
	}
	if name.GetCharacter() == "" {
		t.Fatalf("Region.Name.Character: want non-empty, got %+v", name)
	}
	if region.GetCharacter() < pb.RegionCharacter_REGION_CHARACTER_NORMAL ||
		region.GetCharacter() > pb.RegionCharacter_REGION_CHARACTER_WILD {
		t.Fatalf("Region.Character: out of enum range, got %v", region.GetCharacter())
	}
	if region.GetInfluence() == nil {
		t.Fatal("Region.Influence: want non-nil, got nil")
	}
}

func TestJoinAcceptedCarriesWorldSeed(t *testing.T) {
	client, _, cleanup := startRegionTestServer(t)
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
	if accepted == nil {
		t.Fatalf("first message: want JoinAccepted, got %v", first)
	}
	if got := accepted.GetWorldSeed(); got != testRegionSeed {
		t.Fatalf("JoinAccepted.WorldSeed: want %d, got %d", testRegionSeed, got)
	}
}

// countingRegionSource wraps an inner source with an atomic hit counter.
// It is intentionally NOT safe for mutation after construction — tests
// exercise read-only behaviour only.
type countingRegionSource struct {
	inner game.RegionSource
	calls atomic.Int64
}

func (c *countingRegionSource) RegionAt(sc game.SuperChunkCoord) game.Region {
	c.calls.Add(1)
	return c.inner.RegionAt(sc)
}

func TestRegionCacheHitRate(t *testing.T) {
	counter := &countingRegionSource{
		inner: worldgen.NewNoiseRegionSource(testRegionSeed),
	}
	cache := newRegionCache(counter, DefaultRegionCacheCapacity)

	sc := game.SuperChunkCoord{X: 3, Y: -4}
	const repeats = 10
	for range repeats {
		_ = cache.At(sc)
	}

	if got := counter.calls.Load(); got != 1 {
		t.Fatalf("source call count after %d lookups on one coord: want 1, got %d",
			repeats, got)
	}
	if got := cache.Len(); got != 1 {
		t.Fatalf("cache.Len: want 1, got %d", got)
	}
}

func TestRegionCacheDistinctCoords(t *testing.T) {
	counter := &countingRegionSource{
		inner: worldgen.NewNoiseRegionSource(testRegionSeed),
	}
	cache := newRegionCache(counter, DefaultRegionCacheCapacity)

	coords := []game.SuperChunkCoord{
		{X: 0, Y: 0},
		{X: 1, Y: 0},
		{X: 0, Y: 1},
		{X: -2, Y: 3},
	}
	for _, sc := range coords {
		_ = cache.At(sc)
		// A second call must hit the cache, not the source.
		_ = cache.At(sc)
	}

	if got := counter.calls.Load(); got != int64(len(coords)) {
		t.Fatalf("source calls: want %d (one per unique coord), got %d",
			len(coords), got)
	}
}

// TestRegionCacheRace smokes concurrent reads of a shared cache so the
// -race detector flags any accidental shared-state mutation introduced
// by future refactors. The assertion is "no race" — the hit-count sanity
// is covered by the single-thread tests above.
func TestRegionCacheRace(t *testing.T) {
	counter := &countingRegionSource{
		inner: worldgen.NewNoiseRegionSource(testRegionSeed),
	}
	cache := newRegionCache(counter, DefaultRegionCacheCapacity)

	const readers = 8
	const iter = 200
	coords := []game.SuperChunkCoord{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1},
	}

	var wg sync.WaitGroup
	wg.Add(readers)
	for r := range readers {
		go func(r int) {
			defer wg.Done()
			for i := range iter {
				_ = cache.At(coords[(r+i)%len(coords)])
			}
		}(r)
	}
	wg.Wait()
}

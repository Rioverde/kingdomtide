package server

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// silentLog is a noop slog.Logger for tests so log output does not pollute `go test`.
func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeMsg(tag string) *pb.ServerMessage {
	return &pb.ServerMessage{Payload: &pb.ServerMessage_Error{Error: &pb.ErrorResponse{Message: tag}}}
}

func TestHubSubscribeBroadcast(t *testing.T) {
	h := NewHub(silentLog())
	ch, unsub := h.Subscribe("alice")
	defer unsub()

	h.Broadcast(makeMsg("hello"))

	select {
	case got := <-ch:
		if got.GetError().GetMessage() != "hello" {
			t.Fatalf("unexpected payload: %v", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("broadcast did not reach subscriber in 100ms")
	}
}

func TestHubUnsubscribeRemoves(t *testing.T) {
	h := NewHub(silentLog())
	_, unsub := h.Subscribe("alice")
	if got := h.Count(); got != 1 {
		t.Fatalf("count after subscribe: want 1, got %d", got)
	}
	unsub()
	if got := h.Count(); got != 0 {
		t.Fatalf("count after unsubscribe: want 0, got %d", got)
	}
	// Broadcast after the only subscriber left must not panic.
	h.Broadcast(makeMsg("echo"))
}

func TestHubDoubleUnsubscribe(t *testing.T) {
	h := NewHub(silentLog())
	_, unsub := h.Subscribe("alice")
	unsub()
	unsub() // second call must be a no-op, not a panic.
	if got := h.Count(); got != 0 {
		t.Fatalf("count: want 0, got %d", got)
	}
}

func TestHubSlowSubscriberDoesNotBlock(t *testing.T) {
	h := NewHub(silentLog())
	// Subscribe but never read from the channel.
	_, unsub := h.Subscribe("slow")
	defer unsub()

	// Fill the buffer + overflow.
	done := make(chan struct{})
	go func() {
		for range subscriberChanCap + 10 {
			h.Broadcast(makeMsg("m"))
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Broadcast blocked on slow subscriber")
	}
}

func TestHubSendToTargetedAndUnknown(t *testing.T) {
	h := NewHub(silentLog())
	ch, unsub := h.Subscribe("alice")
	defer unsub()

	if ok := h.SendTo("alice", makeMsg("hi-alice")); !ok {
		t.Fatal("SendTo alice should succeed")
	}
	if ok := h.SendTo("bob", makeMsg("hi-bob")); ok {
		t.Fatal("SendTo bob should fail — bob not subscribed")
	}

	select {
	case got := <-ch:
		if got.GetError().GetMessage() != "hi-alice" {
			t.Fatalf("wrong message: %v", got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("alice did not receive targeted message")
	}
}

func TestHubConcurrentRace(t *testing.T) {
	h := NewHub(silentLog())

	const writers = 4
	const messages = 50
	const subscribers = 8

	var wg sync.WaitGroup
	// Subscribers: subscribe, drain for a bit, unsubscribe.
	for i := range subscribers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ch, unsub := h.Subscribe(idFor(id))
			defer unsub()
			timeout := time.After(50 * time.Millisecond)
			for {
				select {
				case <-ch:
				case <-timeout:
					return
				}
			}
		}(i)
	}

	// Broadcasters: fire messages concurrently.
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range messages {
				h.Broadcast(makeMsg("m"))
			}
		}()
	}

	wg.Wait()
	// Race detector in `go test -race` is what matters here; if we got this far
	// without panic and the count is sane, we're good.
	if got := h.Count(); got != 0 {
		t.Fatalf("expected all subscribers to have unsubscribed, got %d", got)
	}
}

func idFor(i int) string {
	return "sub-" + string(rune('a'+i))
}

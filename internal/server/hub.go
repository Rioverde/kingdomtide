package server

import (
	"log/slog"
	"sync"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// subscriberChanCap is the per-subscriber outbox buffer. Small enough that a
// slow reader is detected quickly, large enough that a short burst of events
// during a single turn is not dropped.
const subscriberChanCap = 16

// Hub fans out ServerMessage values to connected subscribers. Sends are
// non-blocking: a subscriber whose outbox is full has the offending message
// dropped and logged. The hub never stalls the writer.
//
// Hub is safe for concurrent use from multiple goroutines.
type Hub struct {
	mu   sync.Mutex
	subs map[string]chan *pb.ServerMessage
	log  *slog.Logger
}

// NewHub constructs an empty Hub. If log is nil, slog.Default is used.
func NewHub(log *slog.Logger) *Hub {
	if log == nil {
		log = slog.Default()
	}
	return &Hub{
		subs: make(map[string]chan *pb.ServerMessage),
		log:  log,
	}
}

// Subscribe registers a subscriber by id and returns its read-only outbox plus
// an unsubscribe function. Calling unsubscribe is idempotent; the channel is
// closed at most once, inside the hub's lock.
//
// If id is already subscribed, the previous subscription wins — the new call
// returns a closed channel and a no-op unsubscribe. Callers should not rely on
// that path; UUID ids make it effectively unreachable in practice.
func (h *Hub) Subscribe(id string) (<-chan *pb.ServerMessage, func()) {
	ch := make(chan *pb.ServerMessage, subscriberChanCap)

	h.mu.Lock()
	if _, exists := h.subs[id]; exists {
		h.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	h.subs[id] = ch
	h.mu.Unlock()

	var once sync.Once
	unsub := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			existing, ok := h.subs[id]
			if !ok || existing != ch {
				return
			}
			delete(h.subs, id)
			close(ch)
		})
	}
	return ch, unsub
}

// Broadcast sends msg to every current subscriber without blocking. Each send
// is tried under a short critical section using a non-blocking select; if a
// subscriber's outbox is full, the message is dropped for that subscriber only.
//
// Holding the mutex across sends is safe because sends are non-blocking. It
// serialises Broadcast against Subscribe/Unsubscribe so a channel can never be
// closed mid-send.
func (h *Hub) Broadcast(msg *pb.ServerMessage) {
	if msg == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ch := range h.subs {
		select {
		case ch <- msg:
		default:
			h.log.Warn("hub: dropped broadcast for slow subscriber", "id", id)
		}
	}
}

// SendTo delivers msg only to the subscriber with the given id. Returns true
// iff the subscriber exists and the outbox accepted the message.
func (h *Hub) SendTo(id string, msg *pb.ServerMessage) bool {
	if msg == nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.subs[id]
	if !ok {
		return false
	}
	select {
	case ch <- msg:
		return true
	default:
		h.log.Warn("hub: dropped targeted message for slow subscriber", "id", id)
		return false
	}
}

// Count returns the number of active subscribers. Test-only.
func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs)
}

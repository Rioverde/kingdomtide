package server

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

func TestLocalizedStatusRoundTrip(t *testing.T) {
	err := localizedStatus(
		codes.FailedPrecondition,
		"move blocked",
		"error.move_blocked",
		map[string]string{"Reason": "destination_occupied", "Tile": "5,7"},
	)
	if err == nil {
		t.Fatal("localizedStatus returned nil error")
	}

	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("status.FromError: not a Status error, got %T", err)
	}
	if st.Code() != codes.FailedPrecondition {
		t.Fatalf("Code: want %v, got %v", codes.FailedPrecondition, st.Code())
	}
	if st.Message() != "move blocked" {
		t.Fatalf("Message: want %q, got %q", "move blocked", st.Message())
	}

	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("Details: want 1, got %d (%v)", len(details), details)
	}
	detail, ok := details[0].(*pb.LocalizedMessage)
	if !ok {
		t.Fatalf("detail kind: want *pb.LocalizedMessage, got %T", details[0])
	}
	if detail.GetMessageId() != "error.move_blocked" {
		t.Fatalf("MessageId: want %q, got %q", "error.move_blocked", detail.GetMessageId())
	}
	args := detail.GetArgs()
	if got := args["Reason"]; got != "destination_occupied" {
		t.Fatalf("args[Reason]: want %q, got %q", "destination_occupied", got)
	}
	if got := args["Tile"]; got != "5,7" {
		t.Fatalf("args[Tile]: want %q, got %q", "5,7", got)
	}
}

func TestLocalizedStatusNilArgsDoesNotPanic(t *testing.T) {
	err := localizedStatus(codes.Internal, "boom", "error.unknown", nil)
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("status.FromError: not a Status error, got %T", err)
	}
	details := st.Details()
	if len(details) != 1 {
		t.Fatalf("Details: want 1, got %d", len(details))
	}
	detail, ok := details[0].(*pb.LocalizedMessage)
	if !ok {
		t.Fatalf("detail kind: want *pb.LocalizedMessage, got %T", details[0])
	}
	if detail.GetMessageId() != "error.unknown" {
		t.Fatalf("MessageId: want %q, got %q", "error.unknown", detail.GetMessageId())
	}
	// Nil args collapse to an empty map on the wire; proto3 round-trips it
	// back as nil but iteration over a nil map is a no-op so clients can
	// range freely without a guard. Asserting len covers both encodings.
	if got := len(detail.GetArgs()); got != 0 {
		t.Fatalf("args: want empty (len 0), got %d entries: %v", got, detail.GetArgs())
	}
}

// TestServerErrorCodesHaveClientKeys asserts that every pb.ErrCode* wire
// constant has a matching "error.<code>" entry in the client locale catalog.
// This prevents future drift where server adds a code without a catalog entry,
// which would cause the client to silently fall back to "unknown error".
func TestServerErrorCodesHaveClientKeys(t *testing.T) {
	// Hard-code the list of wire codes so the test remains runnable without
	// reflection. When a new pb.ErrCode* constant is added, this list and
	// locale.AllKeys() must both be updated — the compile-time constant
	// definitions and the runtime membership check form a two-sided guard.
	wireCodes := []string{
		pb.ErrCodeInvalidArgument,
		pb.ErrCodeInvalidProtocol,
		pb.ErrCodeRuleViolation,
	}

	allKeys := make(map[string]struct{}, len(locale.AllKeys()))
	for _, k := range locale.AllKeys() {
		allKeys[k] = struct{}{}
	}

	for _, code := range wireCodes {
		key := "error." + code
		if _, ok := allKeys[key]; !ok {
			t.Errorf("wire code %q has no client catalog key %q — add it to locale/keys.go and active.*.toml", code, key)
		}
	}
}

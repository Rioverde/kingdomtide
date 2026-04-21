package ui

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// localizedErr builds a gRPC Status with a LocalizedMessage detail attached,
// mirroring the server's localizedStatus helper. Kept local to the test file
// because internal/server pulls in a larger dependency graph than we want to
// drag into a client-side unit test.
func localizedErr(t *testing.T, code codes.Code, message, id string, args map[string]string) error {
	t.Helper()
	st := status.New(code, message)
	enriched, err := st.WithDetails(&pb.LocalizedMessage{
		MessageId: id,
		Args:      args,
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	return enriched.Err()
}

func TestRenderServerErrorNilReturnsEmpty(t *testing.T) {
	t.Parallel()
	if got := renderServerError(nil, "en"); got != "" {
		t.Fatalf("nil error: got %q, want empty", got)
	}
}

func TestRenderServerErrorWithDetailEnglish(t *testing.T) {
	t.Parallel()
	// Use a known-good key: error.connection_lost has no placeholders in the
	// catalog, so matching on the English text is robust.
	err := localizedErr(t, codes.FailedPrecondition,
		"move blocked", locale.KeyErrorConnectionLost, nil)
	got := renderServerError(err, "en")
	want := locale.Tr("en", locale.KeyErrorConnectionLost)
	if got != want {
		t.Fatalf("en: got %q, want %q", got, want)
	}
	// Guard against an accidental regression where the function returns
	// the raw key unchanged. Tr only returns the key when lookup fails; if
	// English catalog loading broke, this assertion catches it.
	if got == locale.KeyErrorConnectionLost {
		t.Fatalf("render returned bare catalog key %q", got)
	}
}

func TestRenderServerErrorWithDetailRussian(t *testing.T) {
	t.Parallel()
	err := localizedErr(t, codes.FailedPrecondition,
		"move blocked", locale.KeyErrorConnectionLost, nil)
	got := renderServerError(err, "ru")
	want := locale.Tr("ru", locale.KeyErrorConnectionLost)
	if got != want {
		t.Fatalf("ru: got %q, want %q", got, want)
	}
	// English fallback would match the en catalog; ensure the Russian
	// rendering is distinct so we know the lang argument actually routes.
	if got == locale.Tr("en", locale.KeyErrorConnectionLost) {
		t.Fatalf("ru render fell back to english: %q", got)
	}
}

func TestRenderServerErrorWithoutDetailFallsBack(t *testing.T) {
	t.Parallel()
	// Legacy Status with no LocalizedMessage detail at all.
	err := status.Error(codes.FailedPrecondition, "dev-only english text")
	got := renderServerError(err, "en")
	want := locale.Tr("en", locale.KeyErrorUnknown)
	if got != want {
		t.Fatalf("legacy: got %q, want %q", got, want)
	}
	// The developer-facing Status.Message must not leak to the player.
	if strings.Contains(got, "dev-only english text") {
		t.Fatalf("player-facing render contains developer string: %q", got)
	}
}

func TestRenderServerErrorNonStatusError(t *testing.T) {
	t.Parallel()
	got := renderServerError(errors.New("boom"), "en")
	want := locale.Tr("en", locale.KeyErrorUnknown)
	if got != want {
		t.Fatalf("non-status: got %q, want %q", got, want)
	}
}

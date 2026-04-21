package ui

import (
	"log"

	"google.golang.org/grpc/status"

	pb "github.com/Rioverde/gongeons/internal/proto"
	"github.com/Rioverde/gongeons/internal/ui/locale"
)

// renderServerError converts an error from the Play stream into a localized,
// player-facing string. The canonical shape is a gRPC Status carrying a
// LocalizedMessage detail (see internal/server/errors.go); that detail is
// looked up against the locale bundle and its template args are
// substituted. A nil error yields the empty string so callers can use this
// directly as the body of a serverErrorMsg log line.
//
// Legacy or malformed errors that do not carry a LocalizedMessage fall back
// to the generic error.unknown catalog entry. The underlying Status.Message
// is logged to stderr via the standard logger so the developer still sees
// the original text in the client's console while the player sees a clean
// localized line.
func renderServerError(err error, lang string) string {
	if err == nil {
		return ""
	}
	st, ok := status.FromError(err)
	if !ok {
		log.Printf("ui: non-Status server error: %v", err)
		return locale.Tr(lang, locale.KeyErrorUnknown)
	}
	for _, d := range st.Details() {
		msg, ok := d.(*pb.LocalizedMessage)
		if !ok {
			continue
		}
		id := msg.GetMessageId()
		if id == "" {
			continue
		}
		return locale.Tr(lang, id, localizedArgsToKV(msg.GetArgs())...)
	}
	log.Printf("ui: server error without LocalizedMessage detail: %s", st.Message())
	return locale.Tr(lang, locale.KeyErrorUnknown)
}

// localizedArgsToKV flattens a LocalizedMessage.Args map into the
// alternating key/value list locale.Tr consumes. Keys are copied as-is so
// the i18n template data matches the server-chosen placeholder names.
func localizedArgsToKV(args map[string]string) []any {
	if len(args) == 0 {
		return nil
	}
	kv := make([]any, 0, 2*len(args))
	for k, v := range args {
		kv = append(kv, k, v)
	}
	return kv
}

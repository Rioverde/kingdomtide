package server

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/Rioverde/gongeons/internal/proto"
)

// localizedStatus builds a gRPC Status with a LocalizedMessage detail
// attached, producing the canonical error shape for any user-facing server
// error. The English message argument is kept for developer tooling (grpcurl
// transcripts, server logs) and never shown to players; clients look up
// messageID in their i18n catalog and substitute args to render the player-
// facing string.
//
// args may be nil — the helper always stores a non-nil map on the wire so
// clients can use a single code path for detail extraction.
//
// status.WithDetails only fails if the supplied proto messages cannot be
// marshalled via anypb.New. Our detail here is a plain generated message
// with no oneofs or required-field tricks, so a failure would indicate a
// runtime/ABI break in the protobuf stack. We surface that case by falling
// back to a detail-less Status so the caller still returns the gRPC code
// it asked for rather than losing the error entirely.
func localizedStatus(code codes.Code, message, messageID string, args map[string]string) error {
	if args == nil {
		args = map[string]string{}
	}
	detail := &pb.LocalizedMessage{
		MessageId: messageID,
		Args:      args,
	}
	st := status.New(code, message)
	enriched, err := st.WithDetails(detail)
	if err != nil {
		return st.Err()
	}
	return enriched.Err()
}

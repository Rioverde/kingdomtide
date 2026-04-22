// Package gongeonspb is the generated protobuf package for the Gongeons
// wire protocol, plus hand-maintained error code constants shared between
// server and client (see errorcodes.go).
package gongeonspb

// Error code constants for ErrorResponse.Code. These are part of the
// wire protocol — shared between server (which populates the field)
// and client (which derives a locale messageID as "error.<code>").
//
// Every value here MUST have matching catalog entries in
// internal/ui/locale/active.*.toml under the key "error.<value>",
// and a corresponding KeyError* constant in internal/ui/locale/keys.go.
const (
	ErrCodeInvalidArgument = "invalid_argument"
	ErrCodeInvalidProtocol = "invalid_protocol"
	ErrCodeRuleViolation   = "rule_violation"
	// ErrCodeInvalidStats is emitted when the server rejects a
	// JoinRequest whose CoreStats fail the Point Buy validator.
	// Clients surface it via the locale key "error.invalid_stats".
	ErrCodeInvalidStats = "invalid_stats"
)

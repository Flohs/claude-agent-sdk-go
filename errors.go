package claude

import (
	"fmt"
	"strings"
)

// SDKError is the base error type for all Claude SDK errors.
type SDKError struct {
	Message string
}

func (e *SDKError) Error() string {
	return e.Message
}

// ConnectionError is returned when unable to connect to Claude Code.
type ConnectionError struct {
	SDKError
}

// NotFoundError is returned when Claude Code is not found or not installed.
type NotFoundError struct {
	ConnectionError
	CLIPath string
}

func (e *NotFoundError) Error() string {
	if e.CLIPath != "" {
		return fmt.Sprintf("%s: %s", e.Message, e.CLIPath)
	}
	return e.Message
}

// ProcessError is returned when the CLI process fails.
type ProcessError struct {
	SDKError
	ExitCode *int
	Stderr   string
}

func (e *ProcessError) Error() string {
	msg := e.Message
	if e.ExitCode != nil {
		msg = fmt.Sprintf("%s (exit code: %d)", msg, *e.ExitCode)
	}
	if e.Stderr != "" {
		msg = fmt.Sprintf("%s\nError output: %s", msg, e.Stderr)
	}
	return msg
}

// ResultError is returned when the CLI exits after reporting a terminal
// error result. The CLI ends a failed run by emitting a "result" message
// with IsError true (delivered to callers as a *ResultMessage) and then
// exiting non-zero. ResultError replaces the bare "exit code 1" ProcessError
// for that case and carries the result's payload, so callers can branch on
// why the run failed instead of string-matching the error text. It embeds
// ProcessError, so existing `errors.As(err, &processErr)` / type-switch
// handling for *ProcessError keeps working via errors.As.
// Port of Python SDK commit 90ab957 (anthropics/claude-agent-sdk-python#1205).
type ResultError struct {
	ProcessError
	// Subtype is the result subtype ("error_max_turns",
	// "error_during_execution", ... or "success" when the agent loop itself
	// completed but the last turn was an API error). Empty when not
	// provided.
	Subtype string
	// Errors are the error strings reported by the CLI. May be empty.
	Errors []string
	// Result is the result text, if any. For API failures this holds the
	// "API Error: ..." prose.
	Result string
	// APIErrorStatus is the HTTP status of the failing API call, if any.
	APIErrorStatus *int
	// TerminalReason is why the run ended (e.g. "api_error", "max_turns"),
	// if reported by the CLI.
	TerminalReason string
	// SessionID is the session the result belongs to, if reported.
	SessionID string
	// Data is the raw "result" message payload as emitted by the CLI.
	Data map[string]any
}

// Unwrap exposes the embedded ProcessError to errors.Is/errors.As, so
// `errors.As(err, &processErr)` finds it even though ResultError embeds
// ProcessError by value rather than by pointer (Go's automatic method
// promotion makes *ResultError satisfy the error interface via the embedded
// type's Error() method, but does not make *ResultError itself assignable
// to *ProcessError — Unwrap is what lets errors.As bridge the two types).
func (e *ResultError) Unwrap() error {
	return &e.ProcessError
}

// newResultError builds a *ResultError from an is_error "result" message
// payload, extracting the same fields ResultMessage itself parses (see
// parseResultMessage in message_parser.go). message is the already-composed
// error text (see resultErrorText); data is the raw result message map;
// exitCode is the subprocess's exit code, if known.
func newResultError(message string, data map[string]any, exitCode *int) *ResultError {
	e := &ResultError{
		ProcessError: ProcessError{
			SDKError: SDKError{Message: message},
			ExitCode: exitCode,
			// Stderr deliberately not carried over: the transport's stderr
			// tail is a generic placeholder, and the result text is the real
			// cause. Mirrors the Python SDK's ResultError construction.
		},
		Subtype:        stringField(data, "subtype"),
		Result:         stringField(data, "result"),
		TerminalReason: stringField(data, "terminal_reason"),
		SessionID:      stringField(data, "session_id"),
		Data:           data,
	}
	e.Errors = normalizeResultErrors(data["errors"])
	if v, ok := data["api_error_status"]; ok {
		if status := intFromAny(v); status != 0 {
			e.APIErrorStatus = &status
		}
	}
	return e
}

// normalizeResultErrors coerces a result message's "errors" field into a
// []string: the CLI emits a list of strings, but this tolerates a bare
// string (older/buggy emitters) and drops non-string or blank (after
// trimming) entries, so the structured ResultError.Errors and the derived
// error text always agree. Mirrors the Python SDK's
// _normalize_result_errors.
func normalizeResultErrors(v any) []string {
	var raw []any
	switch t := v.(type) {
	case string:
		raw = []any{t}
	case []any:
		raw = t
	default:
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			if trimmed := strings.TrimSpace(s); trimmed != "" {
				out = append(out, trimmed)
			}
		}
	}
	return out
}

// JSONDecodeError is returned when unable to decode JSON from CLI output.
type JSONDecodeError struct {
	SDKError
	Line          string
	OriginalError error
}

func (e *JSONDecodeError) Error() string {
	line := e.Line
	if len(line) > 100 {
		line = line[:100] + "..."
	}
	return fmt.Sprintf("Failed to decode JSON: %s", line)
}

func (e *JSONDecodeError) Unwrap() error {
	return e.OriginalError
}

// MessageParseError is returned when unable to parse a message from CLI output.
type MessageParseError struct {
	SDKError
	Data map[string]any
}

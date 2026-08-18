package claude

import (
	"errors"
	"fmt"
	"testing"
)

// ---------------------------------------------------------------------------
// resultErrorText precedence (#602, mirrors Python SDK's _error_result_text,
// commit 90ab957): errors[] -> result -> non-"success" subtype ->
// "API error (HTTP <status>)" -> "unknown error".
// ---------------------------------------------------------------------------

func TestResultErrorText(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]any
		want string
	}{
		{
			name: "prefers errors list, joined",
			msg: map[string]any{
				"subtype": "error_during_execution",
				"errors":  []any{"first problem", "second problem"},
				"result":  "ignored because errors is non-empty",
			},
			want: "first problem; second problem",
		},
		{
			name: "falls back to result text when errors is empty",
			msg: map[string]any{
				"subtype": "error_during_execution",
				"errors":  []any{},
				"result":  "  API Error: 529 Overloaded  ",
			},
			want: "API Error: 529 Overloaded",
		},
		{
			// The critical case the Python SDK fix targeted: an API failure
			// arrives as subtype "success" with an empty errors[] and the
			// real prose in "result" — falling back to bare subtype would
			// produce the self-contradictory "... returned an error result:
			// success".
			name: "success subtype with API error result text uses result, not subtype",
			msg: map[string]any{
				"subtype":          "success",
				"is_error":         true,
				"result":           "API Error: 500 Internal Server Error",
				"api_error_status": 500,
			},
			want: "API Error: 500 Internal Server Error",
		},
		{
			name: "falls back to non-success subtype when no errors or result",
			msg: map[string]any{
				"subtype": "error_max_turns",
			},
			want: "error_max_turns",
		},
		{
			name: "success subtype with no result text falls back past subtype to HTTP status",
			msg: map[string]any{
				"subtype":          "success",
				"result":           "",
				"api_error_status": 529,
			},
			want: "API error (HTTP 529)",
		},
		{
			name: "unknown error when nothing usable is present",
			msg:  map[string]any{"subtype": "success"},
			want: "unknown error",
		},
		{
			name: "blank errors entries are dropped",
			msg: map[string]any{
				"errors": []any{"", "  ", "real problem"},
			},
			want: "real problem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resultErrorText(tt.msg)
			if got != tt.want {
				t.Errorf("resultErrorText(%v) = %q, want %q", tt.msg, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// newResultError field extraction.
// ---------------------------------------------------------------------------

func TestNewResultError_ExtractsFields(t *testing.T) {
	exitCode := 1
	data := map[string]any{
		"subtype":          "error_during_execution",
		"is_error":         true,
		"errors":           []any{"tool execution failed"},
		"result":           "some result text",
		"api_error_status": 529,
		"terminal_reason":  "api_error",
		"session_id":       "sess-abc",
	}

	err := newResultError("boom", data, &exitCode)

	if err.Subtype != "error_during_execution" {
		t.Errorf("Subtype = %q, want %q", err.Subtype, "error_during_execution")
	}
	if len(err.Errors) != 1 || err.Errors[0] != "tool execution failed" {
		t.Errorf("Errors = %v, want [%q]", err.Errors, "tool execution failed")
	}
	if err.Result != "some result text" {
		t.Errorf("Result = %q, want %q", err.Result, "some result text")
	}
	if err.APIErrorStatus == nil || *err.APIErrorStatus != 529 {
		t.Errorf("APIErrorStatus = %v, want 529", err.APIErrorStatus)
	}
	if err.TerminalReason != "api_error" {
		t.Errorf("TerminalReason = %q, want %q", err.TerminalReason, "api_error")
	}
	if err.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", err.SessionID, "sess-abc")
	}
	if err.ExitCode == nil || *err.ExitCode != 1 {
		t.Errorf("ExitCode = %v, want 1", err.ExitCode)
	}
	if err.Message != "boom" {
		t.Errorf("Message = %q, want %q", err.Message, "boom")
	}
	if len(err.Data) != len(data) {
		t.Errorf("Data = %v, want a copy of %v", err.Data, data)
	}
	// Stderr is deliberately never populated by newResultError: the
	// transport's stderr tail is a generic placeholder, and the result text
	// (already folded into Message by the caller) is the real cause.
	if err.Stderr != "" {
		t.Errorf("Stderr = %q, want empty", err.Stderr)
	}
}

func TestNewResultError_WithoutAPIErrorStatus(t *testing.T) {
	data := map[string]any{
		"subtype": "error_max_turns",
		"errors":  []any{"Max turns (5) reached"},
	}

	err := newResultError("boom", data, nil)

	if err.APIErrorStatus != nil {
		t.Errorf("APIErrorStatus = %v, want nil", err.APIErrorStatus)
	}
	if err.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil", err.ExitCode)
	}
	if err.SessionID != "" {
		t.Errorf("SessionID = %q, want empty", err.SessionID)
	}
}

func TestNewResultError_MalformedAPIErrorStatusIgnored(t *testing.T) {
	data := map[string]any{
		"subtype":          "success",
		"api_error_status": "not-a-number",
	}

	err := newResultError("boom", data, nil)

	if err.APIErrorStatus != nil {
		t.Errorf("APIErrorStatus = %v, want nil for a non-numeric value", err.APIErrorStatus)
	}
}

// ---------------------------------------------------------------------------
// ResultError satisfies error, formats like ProcessError, and unwraps via
// errors.As to *ProcessError (it embeds ProcessError).
// ---------------------------------------------------------------------------

func TestResultError_ErrorString(t *testing.T) {
	exitCode := 1
	err := newResultError("Claude Code returned an error result: boom", map[string]any{}, &exitCode)

	got := err.Error()
	want := "Claude Code returned an error result: boom (exit code: 1)"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestResultError_ErrorsAsUnwrapsToProcessError(t *testing.T) {
	exitCode := 1
	var err error = newResultError("boom", map[string]any{"subtype": "error_max_turns"}, &exitCode)

	var procErr *ProcessError
	if !errors.As(err, &procErr) {
		t.Fatal("errors.As(err, &procErr) = false, want true (ResultError embeds ProcessError)")
	}
	if procErr.ExitCode == nil || *procErr.ExitCode != 1 {
		t.Errorf("unwrapped ProcessError.ExitCode = %v, want 1", procErr.ExitCode)
	}

	var resErr *ResultError
	if !errors.As(err, &resErr) {
		t.Fatal("errors.As(err, &resErr) = false, want true")
	}
	if resErr.Subtype != "error_max_turns" {
		t.Errorf("unwrapped ResultError.Subtype = %q, want %q", resErr.Subtype, "error_max_turns")
	}
}

func TestResultError_ErrorsAsUnwrapsWhenWrapped(t *testing.T) {
	exitCode := 1
	inner := newResultError("boom", map[string]any{"subtype": "error_max_turns"}, &exitCode)
	wrapped := fmt.Errorf("query failed: %w", inner)

	var resErr *ResultError
	if !errors.As(wrapped, &resErr) {
		t.Fatal("errors.As(wrapped, &resErr) = false, want true through fmt.Errorf %w wrapping")
	}
	if resErr != inner {
		t.Error("unwrapped *ResultError should be the same value that was wrapped")
	}

	var procErr *ProcessError
	if !errors.As(wrapped, &procErr) {
		t.Fatal("errors.As(wrapped, &procErr) = false, want true")
	}
}

// ---------------------------------------------------------------------------
// normalizeResultErrors
// ---------------------------------------------------------------------------

func TestNormalizeResultErrors(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{"nil", nil, nil},
		{"other non-list, non-string types return nil", 42, nil},
		{"tolerates a bare string as a single-element list", "solo error", []string{"solo error"}},
		{"list of strings", []any{"a", "b"}, []string{"a", "b"}},
		{"drops blanks and non-strings, trims whitespace", []any{"", "  ", " ok ", 42, nil}, []string{"ok"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeResultErrors(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("normalizeResultErrors(%v) = %v, want %v", tt.in, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("normalizeResultErrors(%v)[%d] = %q, want %q", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

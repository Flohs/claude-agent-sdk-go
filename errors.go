package claude

import "fmt"

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

package claude

import "context"

// HookEvent represents the type of hook event.
type HookEvent string

const (
	HookEventPreToolUse          HookEvent = "PreToolUse"
	HookEventPostToolUse         HookEvent = "PostToolUse"
	HookEventPostToolUseFailure  HookEvent = "PostToolUseFailure"
	HookEventUserPromptSubmit    HookEvent = "UserPromptSubmit"
	HookEventStop                HookEvent = "Stop"
	HookEventSubagentStop        HookEvent = "SubagentStop"
	HookEventPreCompact          HookEvent = "PreCompact"
	HookEventNotification        HookEvent = "Notification"
	HookEventSubagentStart       HookEvent = "SubagentStart"
	HookEventPermissionRequest   HookEvent = "PermissionRequest"
)

// HookInput represents the input data for a hook callback.
// The map contains fields specific to each hook event type.
// Common fields: session_id, transcript_path, cwd, permission_mode, hook_event_name.
//
// Use [ParseHookInput] to convert a HookInput into a typed struct.
type HookInput map[string]any

// TypedHookInput is a marker interface implemented by all typed hook input structs.
// Use [ParseHookInput] to obtain a TypedHookInput from a raw [HookInput] map.
type TypedHookInput interface {
	hookInputMarker()
}

// BaseHookInput contains fields common to all hook events.
type BaseHookInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path"`
	Cwd            string `json:"cwd"`
	PermissionMode string `json:"permission_mode"`
	HookEventName  string `json:"hook_event_name"`
}

// SubagentContext carries optional sub-agent attribution fields.
// Present only when a hook fires from inside a Task-spawned sub-agent.
// The AgentID matches the value emitted by that sub-agent's SubagentStart/SubagentStop hooks.
type SubagentContext struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentType string `json:"agent_type,omitempty"`
}

// PreToolUseHookInput is the typed input for PreToolUse hook events.
type PreToolUseHookInput struct {
	BaseHookInput
	SubagentContext
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
	ToolUseID string         `json:"tool_use_id"`
}

func (*PreToolUseHookInput) hookInputMarker() {}

// PostToolUseHookInput is the typed input for PostToolUse hook events.
type PostToolUseHookInput struct {
	BaseHookInput
	SubagentContext
	ToolName     string         `json:"tool_name"`
	ToolInput    map[string]any `json:"tool_input"`
	ToolResponse any            `json:"tool_response"`
	ToolUseID    string         `json:"tool_use_id"`
}

func (*PostToolUseHookInput) hookInputMarker() {}

// PostToolUseFailureHookInput is the typed input for PostToolUseFailure hook events.
type PostToolUseFailureHookInput struct {
	BaseHookInput
	SubagentContext
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	ToolUseID   string         `json:"tool_use_id"`
	Error       string         `json:"error"`
	IsInterrupt bool           `json:"is_interrupt,omitempty"`
}

func (*PostToolUseFailureHookInput) hookInputMarker() {}

// PermissionRequestHookInput is the typed input for PermissionRequest hook events.
type PermissionRequestHookInput struct {
	BaseHookInput
	SubagentContext
	ToolName              string         `json:"tool_name"`
	ToolInput             map[string]any `json:"tool_input"`
	PermissionSuggestions []any          `json:"permission_suggestions,omitempty"`
}

func (*PermissionRequestHookInput) hookInputMarker() {}

// UserPromptSubmitHookInput is the typed input for UserPromptSubmit hook events.
type UserPromptSubmitHookInput struct {
	BaseHookInput
	Prompt string `json:"prompt"`
}

func (*UserPromptSubmitHookInput) hookInputMarker() {}

// StopHookInput is the typed input for Stop hook events.
type StopHookInput struct {
	BaseHookInput
	StopHookActive bool `json:"stop_hook_active"`
}

func (*StopHookInput) hookInputMarker() {}

// SubagentStopHookInput is the typed input for SubagentStop hook events.
type SubagentStopHookInput struct {
	BaseHookInput
	StopHookActive       bool   `json:"stop_hook_active"`
	AgentID              string `json:"agent_id"`
	AgentTranscriptPath  string `json:"agent_transcript_path"`
	AgentType            string `json:"agent_type"`
}

func (*SubagentStopHookInput) hookInputMarker() {}

// SubagentStartHookInput is the typed input for SubagentStart hook events.
type SubagentStartHookInput struct {
	BaseHookInput
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type"`
}

func (*SubagentStartHookInput) hookInputMarker() {}

// PreCompactHookInput is the typed input for PreCompact hook events.
type PreCompactHookInput struct {
	BaseHookInput
	Trigger            string `json:"trigger"`
	CustomInstructions string `json:"custom_instructions,omitempty"`
}

func (*PreCompactHookInput) hookInputMarker() {}

// NotificationHookInput is the typed input for Notification hook events.
type NotificationHookInput struct {
	BaseHookInput
	Message          string `json:"message"`
	Title            string `json:"title,omitempty"`
	NotificationType string `json:"notification_type"`
}

func (*NotificationHookInput) hookInputMarker() {}

// HookContext provides context for hook callbacks.
type HookContext struct {
	Signal any // Reserved for future abort signal support
}

// HookJSONOutput represents the output of a hook callback.
// See https://code.claude.com/docs/en/hooks#advanced%3A-json-output
type HookJSONOutput map[string]any

// HookCallback is the function type for hook callbacks.
type HookCallback func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error)

// HookMatcher configures which hooks run for which events.
type HookMatcher struct {
	// Matcher is a tool name pattern (e.g., "Bash", "Write|MultiEdit|Edit").
	Matcher string
	// Hooks is the list of hook callbacks to run.
	Hooks []HookCallback
	// Timeout in seconds for all hooks in this matcher (default: 60).
	Timeout *float64
}

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
	// HookEventTeammateIdle fires when a sub-agent idles waiting for input.
	// Port of TypeScript SDK v0.2.33.
	HookEventTeammateIdle HookEvent = "TeammateIdle"
	// HookEventTaskCompleted fires when a Task-spawned sub-agent completes.
	// Port of TypeScript SDK v0.2.33.
	HookEventTaskCompleted HookEvent = "TaskCompleted"
	// HookEventConfigChange fires when session configuration changes (e.g.
	// permission mode switch, model change). Port of TypeScript SDK v0.2.49.
	HookEventConfigChange HookEvent = "ConfigChange"
	// HookEventElicitation fires when an MCP server requests user input via
	// the MCP elicitation protocol (MCP 2025-11-05). The hook callback can
	// return a response map to provide the input programmatically, skipping
	// any interactive prompt. Port of TypeScript SDK v0.2.76.
	HookEventElicitation HookEvent = "Elicitation"
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
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool call arguments. For well-known tools, dedicated
	// typed structs are available (e.g. [ExitPlanModeToolInput] for ExitPlanMode).
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
	PermissionSuggestions []map[string]any `json:"permission_suggestions,omitempty"`
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

// TeammateIdleHookInput is the typed input for TeammateIdle hook events.
type TeammateIdleHookInput struct {
	BaseHookInput
	SubagentContext
}

func (*TeammateIdleHookInput) hookInputMarker() {}

// TaskCompletedHookInput is the typed input for TaskCompleted hook events.
type TaskCompletedHookInput struct {
	BaseHookInput
	SubagentContext
	TaskID    string `json:"task_id,omitempty"`
	ToolUseID string `json:"tool_use_id,omitempty"`
}

func (*TaskCompletedHookInput) hookInputMarker() {}

// ConfigChangeHookInput is the typed input for ConfigChange hook events.
type ConfigChangeHookInput struct {
	BaseHookInput
	// Changes carries the set of configuration keys that changed with their
	// new values (e.g. {"permission_mode": "acceptEdits"}). The shape is
	// preserved as a raw map to allow forward compatibility with new fields.
	Changes map[string]any `json:"changes,omitempty"`
}

func (*ConfigChangeHookInput) hookInputMarker() {}

// ElicitationHookInput is the typed input for Elicitation hook events.
// Fired when an MCP server requests user input via the MCP elicitation
// protocol (MCP 2025-11-05). Port of TypeScript SDK v0.2.76.
type ElicitationHookInput struct {
	BaseHookInput
	// RequestID is the identifier of the elicitation request.
	RequestID string `json:"request_id,omitempty"`
	// ServerName is the name of the MCP server requesting input.
	ServerName string `json:"server_name,omitempty"`
	// Message is the human-readable prompt from the MCP server.
	Message string `json:"message,omitempty"`
	// RequestedSchema is the JSON schema describing the form fields the server
	// expects the user to fill in. Nil when no schema was provided.
	RequestedSchema map[string]any `json:"requestedSchema,omitempty"`
}

func (*ElicitationHookInput) hookInputMarker() {}

// HookContext provides context for hook callbacks.
type HookContext struct {
	Signal any // Reserved for future abort signal support
}

// Well-known keys for [HookJSONOutput].
//
// PreToolUse hooks may return:
//   - [HookOutputKeyDecision] — "approve", "block", or "defer".
//   - [HookOutputKeyReason] — human-readable reason shown to the user on block.
//
// PostToolUse hooks may return:
//   - [HookOutputKeyUpdatedToolOutput] — replacement value for the tool's
//     output before it reaches the model. Works for any tool type (Bash,
//     Write, MCP tools, …). Supersedes [HookOutputKeyUpdatedMCPToolOutput].
//   - [HookOutputKeyUpdatedMCPToolOutput] — deprecated; use
//     [HookOutputKeyUpdatedToolOutput] instead.
const (
	// HookOutputKeyDecision is the output key for PreToolUse hook permission
	// decisions ("approve", "block", "defer").
	HookOutputKeyDecision = "decision"
	// HookOutputKeyReason is the optional human-readable message shown when a
	// PreToolUse hook blocks a tool call.
	HookOutputKeyReason = "reason"
	// HookOutputKeyUpdatedToolOutput is the PostToolUse output key for
	// replacing a tool's output before the model sees it. Works for any tool
	// type (Bash, Write, MCP servers, …).
	// Port of TypeScript SDK v0.2.121 / Python SDK v0.1.74 PR #911.
	HookOutputKeyUpdatedToolOutput = "updatedToolOutput"
	// HookOutputKeyUpdatedMCPToolOutput is the legacy PostToolUse key for
	// replacing MCP tool output. Deprecated — use
	// [HookOutputKeyUpdatedToolOutput] instead, which works for all tools.
	//
	// Deprecated: use [HookOutputKeyUpdatedToolOutput].
	HookOutputKeyUpdatedMCPToolOutput = "updatedMCPToolOutput"
)

// HookJSONOutput represents the output of a hook callback.
//
// Return nil (or an empty map) to take no action. Use the [HookOutputKey…]
// constants for the well-known keys so callers avoid hard-coding strings:
//
//	// Block a tool call from a PreToolUse hook:
//	return claude.HookJSONOutput{
//	    claude.HookOutputKeyDecision: "block",
//	    claude.HookOutputKeyReason:   "not allowed in this context",
//	}, nil
//
//	// Replace tool output from a PostToolUse hook:
//	return claude.HookJSONOutput{
//	    claude.HookOutputKeyUpdatedToolOutput: sanitized,
//	}, nil
//
// See https://code.claude.com/docs/en/hooks#advanced%3A-json-output
type HookJSONOutput map[string]any

// PostToolUseHookOutput is the typed output for [HookEventPostToolUse] callbacks.
// Use [PostToolUseHookOutput.ToHookJSONOutput] to convert to the [HookJSONOutput] map
// that [HookCallback] must return.
//
// Port of TypeScript SDK v0.2.121.
type PostToolUseHookOutput struct {
	// UpdatedToolOutput, when non-nil, replaces the tool's output that the model sees.
	// Applies to all tool types (Bash, Write, MCP tools, etc.). Port of TypeScript SDK v0.2.121.
	UpdatedToolOutput any
	// UpdatedMCPToolOutput is deprecated: use UpdatedToolOutput instead.
	// When non-nil and UpdatedToolOutput is nil, replaces the output for MCP tools only.
	//
	// Deprecated: Use UpdatedToolOutput.
	UpdatedMCPToolOutput any
}

// ToHookJSONOutput converts the typed struct to a [HookJSONOutput] map suitable for
// returning from a [HookCallback]. Fields with a nil value are omitted.
func (o PostToolUseHookOutput) ToHookJSONOutput() HookJSONOutput {
	out := HookJSONOutput{}
	if o.UpdatedToolOutput != nil {
		out["updatedToolOutput"] = o.UpdatedToolOutput
	}
	if o.UpdatedMCPToolOutput != nil {
		out["updatedMCPToolOutput"] = o.UpdatedMCPToolOutput
	}
	return out
}

// HookCallback is the function type for hook callbacks.
//
// When multiple callbacks match the same event — either via multiple [HookMatcher]
// entries for the same [HookEvent] or multiple callbacks in a single matcher's
// Hooks slice — the CLI dispatches them concurrently. Each callback is invoked in
// its own goroutine from the SDK's perspective (the SDK handles each
// hook_callback control request in a separate goroutine). Callbacks that share
// mutable state must use appropriate synchronisation (e.g. sync.Mutex).
//
// Port of Python SDK v0.2.82 PR #956.
type HookCallback func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error)

// HookMatcher configures which hooks run for which events.
//
// # Dispatch order
//
// All matchers registered for a given event are dispatched **concurrently**
// (in parallel). Total execution time is approximately the latency of the
// slowest single hook, not the sum. Registration order does not determine
// execution order, and hooks do not "gate" each other.
//
// This means designs that depend on sequential ordering are not supported.
// For example, a rate-limiter placed first in the list cannot block
// subsequent hooks from starting, because all hooks for the event begin at
// the same time.
type HookMatcher struct {
	// Matcher is a tool name pattern (e.g., "Bash", "Write|MultiEdit|Edit").
	Matcher string
	// Hooks is the list of hook callbacks to run.
	Hooks []HookCallback
	// Timeout in seconds for all hooks in this matcher (default: 60).
	Timeout *float64
}

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
	// HookEventMessageDisplay fires during assistant message streaming, allowing
	// hooks to transform or hide the displayed text. The Delta field contains
	// newly completed lines since the prior flush. Display-only: the stored
	// message and what the model sees are untouched.
	// Port of TypeScript SDK v0.3.152.
	HookEventMessageDisplay HookEvent = "MessageDisplay"
	// HookEventSessionStart fires at the beginning of a session, before the first
	// user turn. Port of TypeScript SDK v0.3.152.
	HookEventSessionStart HookEvent = "SessionStart"
	// HookEventSessionEnd fires when a session ends.
	HookEventSessionEnd HookEvent = "SessionEnd"
	// HookEventStopFailure fires when the Stop hook itself encounters an error.
	HookEventStopFailure HookEvent = "StopFailure"
	// HookEventPostCompact fires after a context compaction completes.
	HookEventPostCompact HookEvent = "PostCompact"
	// HookEventPostToolBatch fires after a batch of tool calls completes.
	// Unlike PostToolUse (which fires per tool), this fires once for the whole batch.
	HookEventPostToolBatch HookEvent = "PostToolBatch"
	// HookEventPermissionDenied fires when a tool call is blocked by a permission check.
	HookEventPermissionDenied HookEvent = "PermissionDenied"
	// HookEventElicitationResult fires when an MCP server elicitation request
	// completes (complements [HookEventElicitation] which fires when received).
	HookEventElicitationResult HookEvent = "ElicitationResult"
	// HookEventInstructionsLoaded fires when Claude loads a CLAUDE.md or
	// equivalent instructions file during session initialization or directory
	// traversal. Observability only — output is not acted on.
	HookEventInstructionsLoaded HookEvent = "InstructionsLoaded"
	// HookEventCwdChanged fires when the working directory changes during a
	// session. Observability only — output is not acted on.
	HookEventCwdChanged HookEvent = "CwdChanged"
	// HookEventFileChanged fires when a watched file is modified, created, or
	// deleted. Watch paths are registered via the SessionStart hook output.
	// Observability only — output is not acted on.
	HookEventFileChanged HookEvent = "FileChanged"
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

// MessageDisplayHookInput is the typed input for MessageDisplay hook events.
// Fires during assistant message streaming. Delta contains newly completed
// lines since the prior flush. Display-only: the stored message and what the
// model sees are untouched. Port of TypeScript SDK v0.3.152.
type MessageDisplayHookInput struct {
	BaseHookInput
	// TurnID identifies the current conversation turn.
	TurnID string `json:"turn_id"`
	// MessageID identifies the specific message being displayed.
	MessageID string `json:"message_id"`
	// Index is the position of this delta within the message stream.
	Index int `json:"index"`
	// Final indicates this is the last delta for the message.
	Final bool `json:"final"`
	// Delta contains the newly completed lines since the prior flush.
	Delta string `json:"delta"`
}

func (*MessageDisplayHookInput) hookInputMarker() {}

// MessageDisplayHookOutput is the typed output for [HookEventMessageDisplay]
// callbacks. Port of TypeScript SDK v0.3.152.
type MessageDisplayHookOutput struct {
	// DisplayContent, when non-nil, replaces the text shown to the user.
	// Display-only: does not affect the stored message or what the model sees.
	DisplayContent *string
}

// ToHookJSONOutput converts the typed struct to a [HookJSONOutput] map suitable
// for returning from a [HookCallback]. Fields with a nil value are omitted.
func (o MessageDisplayHookOutput) ToHookJSONOutput() HookJSONOutput {
	out := HookJSONOutput{}
	if o.DisplayContent != nil {
		out["displayContent"] = *o.DisplayContent
	}
	return out
}

// SessionStartHookInput is the typed input for SessionStart hook events.
// Fires at session initialization before the first user turn.
// Port of TypeScript SDK v0.3.152.
type SessionStartHookInput struct {
	BaseHookInput
}

func (*SessionStartHookInput) hookInputMarker() {}

// SessionStartHookOutput is the typed output for [HookEventSessionStart]
// callbacks. Port of TypeScript SDK v0.3.152.
type SessionStartHookOutput struct {
	// ReloadSkills, when true, triggers a skill re-scan during session start.
	ReloadSkills bool
	// SessionTitle, when non-empty, sets the session title during initialization.
	SessionTitle string
}

// ToHookJSONOutput converts the typed struct to a [HookJSONOutput] map.
func (o SessionStartHookOutput) ToHookJSONOutput() HookJSONOutput {
	out := HookJSONOutput{}
	if o.ReloadSkills {
		out["reloadSkills"] = true
	}
	if o.SessionTitle != "" {
		out["hookSpecificOutput"] = map[string]any{"sessionTitle": o.SessionTitle}
	}
	return out
}

// PermissionDeniedHookInput is the typed input for PermissionDenied hook events.
// Fires when a tool call is blocked by a permission check.
type PermissionDeniedHookInput struct {
	BaseHookInput
	// ToolName is the name of the tool that was denied.
	ToolName string `json:"tool_name"`
	// ToolInput contains the tool call arguments.
	ToolInput map[string]any `json:"tool_input"`
	// ToolUseID is the ID of the tool use that was denied.
	ToolUseID string `json:"tool_use_id"`
	// Reason is the human-readable reason for the denial.
	Reason string `json:"reason"`
}

func (*PermissionDeniedHookInput) hookInputMarker() {}

// PermissionDeniedHookOutput is the typed output for [HookEventPermissionDenied] callbacks.
type PermissionDeniedHookOutput struct {
	// Retry, when true, asks the CLI to retry the permission check.
	Retry bool
}

// ToHookJSONOutput converts the typed struct to a [HookJSONOutput] map.
func (o PermissionDeniedHookOutput) ToHookJSONOutput() HookJSONOutput {
	out := HookJSONOutput{}
	if o.Retry {
		out["retry"] = true
	}
	return out
}

// SessionEndHookInput is the typed input for SessionEnd hook events.
// Fires when a session ends.
type SessionEndHookInput struct {
	BaseHookInput
}

func (*SessionEndHookInput) hookInputMarker() {}

// StopFailureHookInput is the typed input for StopFailure hook events.
// Fires when the Stop hook itself encounters an error.
type StopFailureHookInput struct {
	BaseHookInput
}

func (*StopFailureHookInput) hookInputMarker() {}

// PostCompactHookInput is the typed input for PostCompact hook events.
// Fires after a context compaction completes.
type PostCompactHookInput struct {
	BaseHookInput
	// Trigger indicates how the compaction was triggered: "manual" or "auto".
	Trigger string `json:"trigger"`
	// CompactSummary is the summary generated by the compaction.
	CompactSummary string `json:"compact_summary"`
}

func (*PostCompactHookInput) hookInputMarker() {}

// PostToolBatchToolCall describes a single tool call in a [PostToolBatchHookInput].
type PostToolBatchToolCall struct {
	ToolName     string         `json:"tool_name"`
	ToolInput    map[string]any `json:"tool_input"`
	ToolUseID    string         `json:"tool_use_id"`
	ToolResponse any            `json:"tool_response,omitempty"`
}

// PostToolBatchHookInput is the typed input for PostToolBatch hook events.
// Fires once after a batch of tool calls completes.
type PostToolBatchHookInput struct {
	BaseHookInput
	// ToolCalls is the list of tool calls that completed in this batch.
	ToolCalls []PostToolBatchToolCall `json:"tool_calls"`
}

func (*PostToolBatchHookInput) hookInputMarker() {}

// ElicitationResultHookInput is the typed input for ElicitationResult hook events.
// Fires when an MCP server elicitation request completes.
type ElicitationResultHookInput struct {
	BaseHookInput
	// McpServerName is the name of the MCP server that initiated elicitation.
	McpServerName string `json:"mcp_server_name"`
	// ElicitationID is the identifier of the elicitation request (optional).
	ElicitationID string `json:"elicitation_id,omitempty"`
	// Mode is the elicitation mode: "form" or "url" (optional).
	Mode string `json:"mode,omitempty"`
	// Action is the user's response: "accept", "decline", or "cancel".
	Action string `json:"action"`
	// Content contains the user-provided form content when Action is "accept" (optional).
	Content map[string]any `json:"content,omitempty"`
}

func (*ElicitationResultHookInput) hookInputMarker() {}

// InstructionsLoadedHookInput is the typed input for InstructionsLoaded hook
// events. Fires when Claude loads a CLAUDE.md or rules file during session
// initialization or directory traversal.
type InstructionsLoadedHookInput struct {
	BaseHookInput
	// FilePath is the absolute path to the loaded instructions file.
	FilePath string `json:"file_path"`
	// MemoryType describes the configuration tier: "User", "Project", "Local",
	// or "Managed".
	MemoryType string `json:"memory_type"`
	// LoadReason explains why the file was loaded: "session_start",
	// "nested_traversal", "path_glob_match", "include", or "compact".
	LoadReason string `json:"load_reason"`
	// Globs lists glob patterns that triggered the load (optional).
	Globs []string `json:"globs,omitempty"`
	// TriggerFilePath is the file whose presence triggered loading (optional).
	TriggerFilePath string `json:"trigger_file_path,omitempty"`
	// ParentFilePath is the file that included this one via an include directive
	// (optional).
	ParentFilePath string `json:"parent_file_path,omitempty"`
}

func (*InstructionsLoadedHookInput) hookInputMarker() {}

// CwdChangedHookInput is the typed input for CwdChanged hook events.
// Fires when the working directory changes during a session.
type CwdChangedHookInput struct {
	BaseHookInput
	// OldCwd is the previous working directory.
	OldCwd string `json:"old_cwd"`
	// NewCwd is the new working directory.
	NewCwd string `json:"new_cwd"`
}

func (*CwdChangedHookInput) hookInputMarker() {}

// FileChangedHookInput is the typed input for FileChanged hook events.
// Fires when a watched file is modified, created, or deleted. Watch paths are
// registered via the SessionStart hook output's WatchPaths field.
type FileChangedHookInput struct {
	BaseHookInput
	// FilePath is the absolute path of the file that changed.
	FilePath string `json:"file_path"`
	// ChangeType describes the change: "modified", "created", or "deleted".
	ChangeType string `json:"change_type"`
}

func (*FileChangedHookInput) hookInputMarker() {}

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

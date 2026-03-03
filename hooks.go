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
type HookInput map[string]any

// HookContext provides context for hook callbacks.
type HookContext struct {
	Signal any // Reserved for future abort signal support
}

// HookJSONOutput represents the output of a hook callback.
// See https://docs.anthropic.com/en/docs/claude-code/hooks#advanced%3A-json-output
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

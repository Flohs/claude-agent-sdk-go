package claude

import "strings"

// Message is the interface implemented by all message types returned from the SDK.
type Message interface {
	messageMarker()
}

// ContentBlock is the interface implemented by all content block types.
type ContentBlock interface {
	contentBlockMarker()
}

// TextBlock represents a text content block.
type TextBlock struct {
	Text string `json:"text"`
}

func (TextBlock) contentBlockMarker() {}

// Base64Source describes a base64-encoded source for image and document content blocks.
type Base64Source struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png", "application/pdf", "text/plain"
	Data      string `json:"data"`       // base64-encoded data
}

// Base64Block represents an image or document content block for multimodal messages.
// The Type field distinguishes block kinds: "image" for images, "document" for PDFs and text files.
type Base64Block struct {
	Type   string       `json:"type"` // "image" or "document"
	Source Base64Source `json:"source"`
}

func (Base64Block) contentBlockMarker() {}

// NewTextContent creates a text content block for use with [Client.SendQueryWithContent].
func NewTextContent(text string) map[string]any {
	return map[string]any{"type": "text", "text": text}
}

// NewBase64Content creates a base64-encoded content block for use with [Client.SendQueryWithContent].
// The block type is inferred from the media type: image/* media types produce an "image" block,
// all others (application/pdf, text/plain, text/html, text/csv, etc.) produce a "document" block.
// Both mediaType and base64Data must be non-empty.
func NewBase64Content(mediaType, base64Data string) map[string]any {
	if mediaType == "" {
		panic("claude: NewBase64Content called with empty mediaType")
	}
	if base64Data == "" {
		panic("claude: NewBase64Content called with empty base64Data")
	}
	blockType := "document"
	if strings.HasPrefix(mediaType, "image/") {
		blockType = "image"
	}
	return map[string]any{
		"type": blockType,
		"source": map[string]any{
			"type":       "base64",
			"media_type": mediaType,
			"data":       base64Data,
		},
	}
}

// ThinkingBlock represents a thinking content block.
type ThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

func (ThinkingBlock) contentBlockMarker() {}

// ToolUseBlock represents a tool use content block.
type ToolUseBlock struct {
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func (ToolUseBlock) contentBlockMarker() {}

// ToolResultBlock represents a tool result content block.
type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content,omitempty"` // string | []map[string]any | nil
	IsError   *bool  `json:"is_error,omitempty"`
}

func (ToolResultBlock) contentBlockMarker() {}

// ServerToolName names one of the server-side tools the API can invoke on
// the model's behalf. Callers branch on Name to know which tool was used.
type ServerToolName string

const (
	ServerToolAdvisor                 ServerToolName = "advisor"
	ServerToolWebSearch               ServerToolName = "web_search"
	ServerToolWebFetch                ServerToolName = "web_fetch"
	ServerToolCodeExecution           ServerToolName = "code_execution"
	ServerToolBashCodeExecution       ServerToolName = "bash_code_execution"
	ServerToolTextEditorCodeExecution ServerToolName = "text_editor_code_execution"
	ServerToolSearchRegex             ServerToolName = "tool_search_tool_regex"
	ServerToolSearchBM25              ServerToolName = "tool_search_tool_bm25"
)

// ServerToolUseBlock represents a server-side tool use (e.g. advisor,
// web_search, web_fetch). These tools execute server-side on the model's
// behalf; the caller never needs to return a result.
type ServerToolUseBlock struct {
	ID    string         `json:"id"`
	Name  ServerToolName `json:"name"`
	Input map[string]any `json:"input"`
}

func (ServerToolUseBlock) contentBlockMarker() {}

// ServerToolResultBlock represents the result of a server-side tool call
// (e.g. advisor_tool_result). Content is the raw payload from the API;
// callers that care about a specific tool's result schema can inspect
// Content["type"].
type ServerToolResultBlock struct {
	ToolUseID string         `json:"tool_use_id"`
	Content   map[string]any `json:"content,omitempty"`
}

func (ServerToolResultBlock) contentBlockMarker() {}

// UserMessage represents a user message.
type UserMessage struct {
	Content         any            `json:"content"` // string | []ContentBlock
	UUID            string         `json:"uuid,omitempty"`
	ParentToolUseID string         `json:"parent_tool_use_id,omitempty"`
	ToolUseResult   map[string]any `json:"tool_use_result,omitempty"`
}

func (UserMessage) messageMarker() {}

// AssistantMessageError represents possible error types on assistant messages.
type AssistantMessageError string

const (
	AssistantMessageErrorAuthFailed     AssistantMessageError = "authentication_failed"
	AssistantMessageErrorBilling        AssistantMessageError = "billing_error"
	AssistantMessageErrorRateLimit      AssistantMessageError = "rate_limit"
	AssistantMessageErrorInvalidRequest AssistantMessageError = "invalid_request"
	AssistantMessageErrorServer         AssistantMessageError = "server_error"
	AssistantMessageErrorUnknown        AssistantMessageError = "unknown"
	AssistantMessageErrorModelNotFound  AssistantMessageError = "model_not_found"
)

// AssistantMessage represents an assistant message with content blocks.
type AssistantMessage struct {
	Content         []ContentBlock        `json:"content"`
	Model           string                `json:"model"`
	ParentToolUseID string                `json:"parent_tool_use_id,omitempty"`
	Error           AssistantMessageError `json:"error,omitempty"`
	Usage           map[string]any        `json:"usage,omitempty"`
	// MessageID is the API-side message identifier (from the nested message
	// object). Empty when not provided by the CLI.
	MessageID string `json:"message_id,omitempty"`
	// SessionID is the session this message belongs to.
	SessionID string `json:"session_id,omitempty"`
	// UUID uniquely identifies this message in the session transcript.
	UUID string `json:"uuid,omitempty"`
	// StopReason is why the model stopped generating (e.g. "end_turn",
	// "tool_use", "max_tokens"). Empty when not provided.
	StopReason string `json:"stop_reason,omitempty"`
	// RequestID is the API request identifier for this message.
	RequestID string `json:"request_id,omitempty"`
	// RawData contains the full raw message data for forward compatibility
	// with fields not yet modeled by the SDK.
	RawData map[string]any `json:"-"`
}

func (AssistantMessage) messageMarker() {}

// SystemMessage represents a system message with metadata.
type SystemMessage struct {
	Subtype string         `json:"subtype"`
	Data    map[string]any `json:"data"`
}

func (SystemMessage) messageMarker() {}

// TaskUsage contains usage statistics for task messages.
type TaskUsage struct {
	TotalTokens int `json:"total_tokens"`
	ToolUses    int `json:"tool_uses"`
	DurationMs  int `json:"duration_ms"`
}

// TaskNotificationStatus represents the status of a task notification.
type TaskNotificationStatus string

const (
	TaskNotificationStatusCompleted TaskNotificationStatus = "completed"
	TaskNotificationStatusFailed    TaskNotificationStatus = "failed"
	TaskNotificationStatusStopped   TaskNotificationStatus = "stopped"
)

// TaskStartedMessage is emitted when a task starts.
type TaskStartedMessage struct {
	SystemMessage
	TaskID          string `json:"task_id"`
	Description     string `json:"description"`
	UUID            string `json:"uuid"`
	SessionID       string `json:"session_id"`
	ToolUseID       string `json:"tool_use_id,omitempty"`
	TaskType        string `json:"task_type,omitempty"`
	// SubagentType identifies the type of subagent that started this task.
	SubagentType    string `json:"subagent_type,omitempty"`
	// TaskDescription is a human-readable description of the task.
	TaskDescription string `json:"task_description,omitempty"`
}

// TaskProgressMessage is emitted while a task is in progress.
type TaskProgressMessage struct {
	SystemMessage
	TaskID       string    `json:"task_id"`
	Description  string    `json:"description"`
	Usage        TaskUsage `json:"usage"`
	UUID         string    `json:"uuid"`
	SessionID    string    `json:"session_id"`
	ToolUseID    string    `json:"tool_use_id,omitempty"`
	LastToolName string    `json:"last_tool_name,omitempty"`
	// Summary is an AI-generated progress summary when AgentProgressSummaries
	// is enabled in Options.
	Summary string `json:"summary,omitempty"`
	// SubagentType identifies the type of subagent that owns this task.
	SubagentType string `json:"subagent_type,omitempty"`
	// TaskDescription is a human-readable description of the task.
	TaskDescription string `json:"task_description,omitempty"`
}

// TaskNotificationMessage is emitted when a task completes, fails, or is stopped.
type TaskNotificationMessage struct {
	SystemMessage
	TaskID     string                 `json:"task_id"`
	Status     TaskNotificationStatus `json:"status"`
	OutputFile string                 `json:"output_file"`
	Summary    string                 `json:"summary"`
	UUID       string                 `json:"uuid"`
	SessionID  string                 `json:"session_id"`
	ToolUseID  string                 `json:"tool_use_id,omitempty"`
	Usage      *TaskUsage             `json:"usage,omitempty"`
	// SubagentType identifies the type of subagent that completed this task.
	SubagentType string `json:"subagent_type,omitempty"`
	// TaskDescription is a human-readable description of the completed task.
	TaskDescription string `json:"task_description,omitempty"`
}

// TaskStatus represents the lifecycle state of a task managed by the Task tools.
type TaskStatus string

const (
	TaskStatusPending    TaskStatus = "pending"
	TaskStatusInProgress TaskStatus = "in_progress"
	TaskStatusCompleted  TaskStatus = "completed"
	TaskStatusDeleted    TaskStatus = "deleted"
)

// ExitPlanModeToolInput is the typed input for the ExitPlanMode tool.
// Accessible from [PreToolUseHookInput].ToolInput when ToolName is "ExitPlanMode".
// Port of TypeScript SDK v0.2.76.
type ExitPlanModeToolInput struct {
	// PlanFilePath is the filesystem path of the plan file written during plan
	// mode, if the user requested saving the plan. Empty string when no file
	// was written.
	PlanFilePath string `json:"planFilePath,omitempty"`
}

// TaskCreateInput is the input schema for the TaskCreate tool.
type TaskCreateInput struct {
	Subject     string         `json:"subject"`
	Description string         `json:"description"`
	ActiveForm  string         `json:"activeForm,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// TaskCreateOutput is the output schema for the TaskCreate tool.
type TaskCreateOutput struct {
	Task struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
	} `json:"task"`
}

// TaskGetInput is the input schema for the TaskGet tool.
type TaskGetInput struct {
	TaskID string `json:"taskId"`
}

// TaskGetOutput is the output schema for the TaskGet tool. Task is nil when
// no task matches the requested ID.
type TaskGetOutput struct {
	Task *struct {
		ID          string     `json:"id"`
		Subject     string     `json:"subject"`
		Description string     `json:"description"`
		Status      TaskStatus `json:"status"`
		Blocks      []string   `json:"blocks"`
		BlockedBy   []string   `json:"blockedBy"`
	} `json:"task"`
}

// TaskUpdateInput is the input schema for the TaskUpdate tool.
type TaskUpdateInput struct {
	TaskID       string     `json:"taskId"`
	Subject      string     `json:"subject,omitempty"`
	Description  string     `json:"description,omitempty"`
	ActiveForm   string     `json:"activeForm,omitempty"`
	Status       TaskStatus `json:"status,omitempty"`
	AddBlocks    []string   `json:"addBlocks,omitempty"`
	AddBlockedBy []string   `json:"addBlockedBy,omitempty"`
	Owner        string     `json:"owner,omitempty"`
}

// TaskUpdateOutput is the output schema for the TaskUpdate tool.
type TaskUpdateOutput struct {
	Success       bool     `json:"success"`
	TaskID        string   `json:"taskId"`
	UpdatedFields []string `json:"updatedFields"`
	Error         string   `json:"error,omitempty"`
	StatusChange  *struct {
		From string `json:"from"`
		To   string `json:"to"`
	} `json:"statusChange,omitempty"`
}

// TaskListInput is the input schema for the TaskList tool. The CLI accepts
// no parameters; the struct exists for symmetry with the other Task tool
// schemas.
type TaskListInput struct{}

// TaskListOutput is the output schema for the TaskList tool.
type TaskListOutput struct {
	Tasks []struct {
		ID        string     `json:"id"`
		Subject   string     `json:"subject"`
		Status    TaskStatus `json:"status"`
		Owner     string     `json:"owner,omitempty"`
		BlockedBy []string   `json:"blockedBy"`
	} `json:"tasks"`
}

// MirrorErrorMessage is an SDK-synthesized system message emitted when the
// transcript mirror batcher exhausts its retry budget for a pending
// [SessionStore.Append]. It never originates from the CLI — the SDK injects
// it into the inbound message stream so callers can react to mirror
// failures through the normal [Client.ReceiveMessages] channel.
//
// Key identifies the transcript stream that failed; Error is the
// underlying adapter error (or a timeout sentinel); UUID is a fresh
// identifier the caller can use for correlation; SessionID duplicates
// Key.SessionID for parity with other typed system messages.
type MirrorErrorMessage struct {
	SystemMessage
	Key       *SessionKey `json:"key,omitempty"`
	Error     string      `json:"error"`
	UUID      string      `json:"uuid,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
}

// ApiRetryMessage is emitted before each API retry attempt when the CLI
// encounters a transient API error. Port of TypeScript SDK v0.2.77.
type ApiRetryMessage struct {
	SystemMessage
	// AttemptNumber is the current attempt (1-based; 1 = first retry after the initial failure).
	AttemptNumber int `json:"attempt_number"`
	// MaxAttempts is the maximum number of attempts, including the initial one.
	MaxAttempts int `json:"max_attempts"`
	// DelayMs is the delay in milliseconds before this attempt.
	DelayMs int `json:"delay_ms"`
	// ErrorStatus is the HTTP status code that triggered the retry (e.g. 429, 529).
	// Nil when the error was not an HTTP error.
	ErrorStatus *int `json:"error_status,omitempty"`
	// ErrorMessage is a human-readable description of the error that triggered the retry.
	ErrorMessage string `json:"error_message,omitempty"`
}

// MemoryRecallMessage is emitted when Claude loads memory files during a session.
// Port of TypeScript SDK v0.2.105.
type MemoryRecallMessage struct {
	SystemMessage
	// Paths is the list of memory file paths that were loaded.
	Paths []string `json:"paths,omitempty"`
}

// ElicitationCompleteMessage is emitted when an MCP server's elicitation
// request completes. MCP elicitation allows MCP servers to request user input
// programmatically (MCP protocol 2025-11-05). Port of TypeScript SDK v0.2.76.
type ElicitationCompleteMessage struct {
	SystemMessage
	// RequestID is the identifier of the elicitation request.
	RequestID string `json:"request_id,omitempty"`
	// ServerName is the name of the MCP server that initiated elicitation.
	ServerName string `json:"server_name,omitempty"`
	// Result contains the user's response to the elicitation form. The shape
	// matches the server-provided JSON schema from the original elicit request.
	Result map[string]any `json:"result,omitempty"`
}

// HookDecision represents a hook's permission decision value.
type HookDecision string

const (
	// HookDecisionApprove allows the tool to proceed.
	HookDecisionApprove HookDecision = "approve"
	// HookDecisionBlock denies the tool.
	HookDecisionBlock HookDecision = "block"
	// HookDecisionDefer pauses tool execution and surfaces the pending tool use
	// on ResultMessage.DeferredToolUse so the SDK caller can prompt the user
	// for a decision and then continue via a follow-up query.
	HookDecisionDefer HookDecision = "defer"
)

// DeferredToolUse holds the pending tool call when a PreToolUse hook returns
// {"decision": "defer"}. The CLI pauses execution and echoes the pending call
// on ResultMessage so the SDK caller can inspect it, prompt the user, and
// continue the session.
type DeferredToolUse struct {
	ToolUseID string         `json:"tool_use_id"`
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

// ResultMessage contains cost and usage information for a completed query.
type ResultMessage struct {
	Subtype       string `json:"subtype"`
	DurationMs    int    `json:"duration_ms"`
	DurationAPIMs int    `json:"duration_api_ms"`
	IsError       bool   `json:"is_error"`
	Errors        []any  `json:"errors,omitempty"`
	NumTurns      int    `json:"num_turns"`
	SessionID     string `json:"session_id"`
	StopReason    string `json:"stop_reason,omitempty"`
	// TerminalReason describes why the session terminated (e.g. "completed",
	// "aborted_tools", "max_turns", "blocking_limit"). Empty when not
	// provided by the CLI.
	TerminalReason string `json:"terminal_reason,omitempty"`
	// APIErrorStatus is the HTTP status code (e.g. 429, 500, 529) from a
	// failing API call when IsError is true. Zero when not provided by the
	// CLI (requires CLI >= v2.1.110).
	APIErrorStatus *int `json:"api_error_status,omitempty"`
	// Origin forwards the triggering message's origin so consumers can
	// distinguish user-prompted results from task-notification followups.
	Origin string `json:"origin,omitempty"`
	// RequestID is the API request identifier for the final API call.
	RequestID        string         `json:"request_id,omitempty"`
	TotalCostUSD     *float64       `json:"total_cost_usd,omitempty"`
	Usage            map[string]any `json:"usage,omitempty"`
	Result           string         `json:"result,omitempty"`
	StructuredOutput any            `json:"structured_output,omitempty"`
	// DeferredToolUse is populated when a PreToolUse hook returned
	// {"decision": "defer"}, surfacing the pending tool call so the caller
	// can prompt the user and resume. Nil when no deferral occurred.
	DeferredToolUse *DeferredToolUse `json:"deferred_tool_use,omitempty"`
	// RawData contains the full raw message data for forward compatibility
	// with fields not yet modeled by the SDK.
	RawData map[string]any `json:"-"`
}

func (ResultMessage) messageMarker() {}

// StreamEvent represents a partial message update during streaming.
type StreamEvent struct {
	UUID            string         `json:"uuid"`
	SessionID       string         `json:"session_id"`
	Event           map[string]any `json:"event"`
	ParentToolUseID string         `json:"parent_tool_use_id,omitempty"`
}

func (StreamEvent) messageMarker() {}

// RateLimitStatus represents the status of a rate limit check.
type RateLimitStatus string

const (
	RateLimitStatusAllowed        RateLimitStatus = "allowed"
	RateLimitStatusAllowedWarning RateLimitStatus = "allowed_warning"
	RateLimitStatusRejected       RateLimitStatus = "rejected"
)

// RateLimitInfo contains detailed rate limit information.
type RateLimitInfo struct {
	Status                RateLimitStatus `json:"status"`
	ResetsAt              *string         `json:"resets_at,omitempty"`
	RateLimitType         *string         `json:"rate_limit_type,omitempty"`
	Utilization           *float64        `json:"utilization,omitempty"`
	OverageStatus         *string         `json:"overage_status,omitempty"`
	OverageResetsAt       *string         `json:"overage_resets_at,omitempty"`
	OverageDisabledReason *string         `json:"overage_disabled_reason,omitempty"`
}

// RateLimitEvent represents a rate limit status change from the CLI.
type RateLimitEvent struct {
	Type          string        `json:"type"`
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
	UUID          string        `json:"uuid,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
}

func (RateLimitEvent) messageMarker() {}

// ContextUsage contains context window utilization broken down by category.
type ContextUsage struct {
	TotalTokens     int            `json:"total_tokens"`
	UsedTokens      int            `json:"used_tokens"`
	UsageByCategory map[string]int `json:"usage_by_category,omitempty"`
}

// SDKSessionInfo contains session metadata returned by ListSessions and GetSessionInfo.
type SDKSessionInfo struct {
	SessionID    string  `json:"session_id"`
	Summary      string  `json:"summary"`
	LastModified int64   `json:"last_modified"`
	FileSize     *int64  `json:"file_size,omitempty"`
	CustomTitle  string  `json:"custom_title,omitempty"`
	FirstPrompt  string  `json:"first_prompt,omitempty"`
	GitBranch    string  `json:"git_branch,omitempty"`
	Cwd          string  `json:"cwd,omitempty"`
	Tag          *string `json:"tag,omitempty"`
	CreatedAt    *int64  `json:"created_at,omitempty"`
}

// ReadStateEntry is a single file-state record used by
// [Client.SeedReadState]. It tells the CLI which files the caller has read
// out-of-band so that Edit-style tools can operate across context
// compactions without a fresh Read.
type ReadStateEntry struct {
	Path  string `json:"path"`
	Mtime int64  `json:"mtime"`
}

// ServerCapabilities describes model-level capabilities reported by the CLI
// during session initialization. These fields allow callers to dynamically
// check which effort levels and thinking modes the currently active model
// supports, rather than hard-coding assumptions.
type ServerCapabilities struct {
	// SupportsEffort is true when the current model accepts the --effort flag.
	SupportsEffort bool `json:"supportsEffort"`
	// SupportedEffortLevels lists the effort values the model accepts.
	// Empty when SupportsEffort is false.
	SupportedEffortLevels []Effort `json:"supportedEffortLevels,omitempty"`
	// SupportsAdaptiveThinking is true when the model supports adaptive
	// thinking mode (ThinkingConfigAdaptive).
	SupportsAdaptiveThinking bool `json:"supportsAdaptiveThinking"`
	// MemoryPaths is the list of memory file paths loaded at session initialization.
	// Empty when no memory files are configured.
	MemoryPaths []string `json:"memoryPaths,omitempty"`
}

// SessionMessage represents a user or assistant message from a session transcript.
type SessionMessage struct {
	Type            string `json:"type"` // "user" or "assistant"
	UUID            string `json:"uuid"`
	SessionID       string `json:"session_id"`
	Message         any    `json:"message"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
}

// SessionKey identifies a single transcript stream in a [SessionStore].
//
// ProjectKey is the per-project namespace (typically derived via
// [ProjectKeyForDirectory] from an absolute project directory). SessionID is
// the session's UUID. Subpath is empty for the main session transcript and
// non-empty for sibling streams such as subagent transcripts (e.g.
// "subagents/agent-xyz").
type SessionKey struct {
	ProjectKey string `json:"project_key"`
	SessionID  string `json:"session_id"`
	Subpath    string `json:"subpath,omitempty"`
}

// SessionStoreEntry is one JSONL line from a transcript, represented as a
// parsed JSON object. [SessionStore] adapters persist these verbatim —
// they must round-trip through [SessionStore.Append] and [SessionStore.Load]
// without the adapter interpreting individual fields.
type SessionStoreEntry = map[string]any

// SessionStoreListEntry is one row returned by
// [SessionStoreLister.ListSessions]. Mtime is the adapter's storage write
// time in Unix epoch milliseconds, and must share a clock with the mtime
// embedded in [SessionSummaryEntry] for the same session so the fast-path
// staleness check (summary.Mtime < list mtime) is meaningful.
type SessionStoreListEntry struct {
	SessionID string `json:"session_id"`
	Mtime     int64  `json:"mtime"`
}

// SessionSummaryEntry is a per-session summary sidecar maintained by
// adapters that implement [SessionStoreSummarizer]. Mtime is the storage
// write time in Unix epoch milliseconds. Data is opaque state produced by
// [FoldSessionSummary] — adapters persist it verbatim and do not interpret
// individual keys.
type SessionSummaryEntry struct {
	SessionID string         `json:"session_id"`
	Mtime     int64          `json:"mtime"`
	Data      map[string]any `json:"data"`
}

// SessionListSubkeysKey identifies the main transcript whose sibling
// subkeys (subagent transcripts and other sub-streams) should be listed
// via [SessionStoreSubkeys.ListSubkeys].
type SessionListSubkeysKey struct {
	ProjectKey string `json:"project_key"`
	SessionID  string `json:"session_id"`
}

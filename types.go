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
	// ServerToolReadMcpResource is the server tool name for reading a single MCP
	// resource by URI. Port of TypeScript SDK v0.3.186.
	ServerToolReadMcpResource ServerToolName = "read_mcp_resource"
	// ServerToolReadMcpResourceDir is the server tool name for listing MCP
	// resource directory contents by URI. A dedicated tool as of TypeScript SDK
	// v0.3.186; previously a fallback inside ReadMcpResourceTool.
	ServerToolReadMcpResourceDir ServerToolName = "read_mcp_resource_dir"
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
	Timestamp       string         `json:"timestamp,omitempty"`
	// IsMeta indicates this message is metadata rather than visible
	// conversation content (e.g. a synthetic message generated internally by
	// the SDK or CLI). Port of TypeScript SDK v0.3.198.
	IsMeta bool `json:"isMeta,omitempty"`
	// Origin identifies what triggered this message (e.g. a direct human
	// prompt vs. a peer relay or task-notification replay). Nil when the CLI
	// omits the field. Port of TypeScript SDK bundled sdk.d.ts
	// (SDKMessageOrigin).
	Origin *MessageOrigin `json:"origin,omitempty"`
	// ToolResultMeta carries classification metadata alongside a
	// tool_use_result (e.g. distinguishing a denied, interrupted, or
	// cancelled tool call). Nil when the CLI omits the field. Port of
	// TypeScript SDK v0.3.216.
	ToolResultMeta *ToolResultMeta `json:"tool_result_meta,omitempty"`
}

func (UserMessage) messageMarker() {}

// ToolResultMeta carries classification metadata alongside a
// tool_use_result, letting consumers distinguish denied, interrupted, or
// cancelled tool calls without string-matching the result text.
//
// Field shape is not present in the TypeScript SDK's public sdk.d.ts as of
// v0.3.216 (also checked sdk-tools.d.ts and bridge.d.ts, and the
// claude-agent-sdk-python source tree — not found in either). It is modeled
// directly from the upstream changelog's prose description only; field
// types are our best-effort interpretation and may need adjustment once a
// typed source becomes available.
type ToolResultMeta struct {
	NonExecutionKind string `json:"non_execution_kind,omitempty"`
	UserFeedback     string `json:"user_feedback,omitempty"`
}

// AssistantMessageError represents possible error types on assistant messages.
type AssistantMessageError string

const (
	AssistantMessageErrorAuthFailed         AssistantMessageError = "authentication_failed"
	AssistantMessageErrorOAuthOrgNotAllowed AssistantMessageError = "oauth_org_not_allowed"
	AssistantMessageErrorBilling            AssistantMessageError = "billing_error"
	AssistantMessageErrorRateLimit          AssistantMessageError = "rate_limit"
	AssistantMessageErrorOverloaded         AssistantMessageError = "overloaded"
	AssistantMessageErrorInvalidRequest     AssistantMessageError = "invalid_request"
	AssistantMessageErrorModelNotFound      AssistantMessageError = "model_not_found"
	AssistantMessageErrorServer             AssistantMessageError = "server_error"
	AssistantMessageErrorUnknown            AssistantMessageError = "unknown"
	AssistantMessageErrorMaxOutputTokens    AssistantMessageError = "max_output_tokens"
)

// ToolUseMetaEntry holds display-friendly metadata for a single tool call.
// Port of TypeScript SDK v0.3.179.
type ToolUseMetaEntry struct {
	// Name is the human-readable display name for the tool call (e.g. the MCP
	// tool's user-facing label instead of its wire name).
	Name string `json:"name,omitempty"`
	// IconURL is an optional icon URL sourced from the MCP server's directory
	// metadata. Empty when not provided.
	IconURL string `json:"icon_url,omitempty"`
}

// ToolUseMeta maps tool-use IDs to their display-friendly metadata.
// Port of TypeScript SDK v0.3.179.
type ToolUseMeta map[string]ToolUseMetaEntry

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
	// Aborted is true when this message was truncated by an interrupt/abort
	// before the stream completed: StopReason was never received and Content
	// may end mid-word. False on normally completed messages. Port of
	// TypeScript SDK v0.3.214.
	Aborted bool `json:"aborted,omitempty"`
	// StopDetails contains structured metadata when StopReason is "refusal" or
	// another stop condition that carries additional context. Nil when not
	// provided by the CLI.
	StopDetails map[string]any `json:"stop_details,omitempty"`
	// RequestID is the API request identifier for this message.
	RequestID string `json:"request_id,omitempty"`
	// Timestamp is the ISO-8601 datetime when this message was recorded in the
	// CLI transcript. Empty when not provided by the CLI.
	Timestamp string `json:"timestamp,omitempty"`
	// ToolUseMeta is an optional sidecar carrying display-friendly names and
	// icon URLs for each tool call in Content, keyed by tool-use ID. Nil when
	// not provided by the CLI. Port of TypeScript SDK v0.3.179.
	ToolUseMeta ToolUseMeta `json:"tool_use_meta,omitempty"`
	// ContextUsage is a structured twin of the /context report, carried on
	// the synthetic assistant message that delivers the markdown table. Nil
	// except on /context results from CLIs new enough to attach it; the
	// markdown in Content remains the canonical fallback. Distinct from
	// [Client.GetContextUsage]'s [ContextUsage] return type, which is a
	// different, on-demand control-request/response shape. Port of
	// TypeScript SDK v0.3.232.
	ContextUsage *AssistantContextUsage `json:"context_usage,omitempty"`
	// RawData contains the full raw message data for forward compatibility
	// with fields not yet modeled by the SDK.
	RawData map[string]any `json:"-"`
}

// AssistantContextUsageOverLimit describes how far current usage exceeds the
// resolved context window, present only when over it.
type AssistantContextUsageOverLimit struct {
	TokensOver int `json:"tokens_over"`
	// Kind is "hard_limit" when the window is the model's believed limit
	// (the API will refuse past it), or "compaction_window" when it's a
	// compaction-policy window that may or may not coincide with the
	// model's hard limit.
	Kind string `json:"kind"`
}

// AssistantContextUsageCategory is one row of the /context usage-by-category
// breakdown.
type AssistantContextUsageCategory struct {
	// Name is the display name of the row as the CLI renders it (e.g.
	// "Messages" or "MCP tools (deferred)"); use Kind, not Name, to classify
	// the row.
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
	// Kind is "used" (occupies the window), "free" (remaining window),
	// "buffer" (compaction reserve), or "deferred" (out-of-window tool
	// schemas, excluded from usage math).
	Kind string `json:"kind"`
}

// AssistantContextUsageMCPTool is one MCP tool's contribution to context
// usage.
type AssistantContextUsageMCPTool struct {
	// Name is the wire tool name, e.g. "mcp__linear__create_issue".
	Name       string `json:"name"`
	ServerName string `json:"server_name"`
	Tokens     int    `json:"tokens"`
}

// AssistantContextUsageMemoryFile is one memory file's contribution to
// context usage.
type AssistantContextUsageMemoryFile struct {
	Path string `json:"path"`
	// Type is the display label of the memory-file source, e.g. "Project" or
	// "User".
	Type   string `json:"type"`
	Tokens int    `json:"tokens"`
}

// AssistantContextUsageAgent is one subagent definition's contribution to
// context usage.
type AssistantContextUsageAgent struct {
	AgentType string `json:"agent_type"`
	// Source is the raw source identifier, e.g. "projectSettings",
	// "userSettings", or "plugin". Built-in agents are excluded.
	Source string `json:"source"`
	Tokens int    `json:"tokens"`
}

// AssistantContextUsageSkill is one skill's contribution to context usage.
type AssistantContextUsageSkill struct {
	Name string `json:"name"`
	// Source is the raw source identifier, e.g. "userSettings", "plugin", or
	// "syncedSkills".
	Source     string `json:"source"`
	PluginName string `json:"plugin_name,omitempty"`
	Tokens     int    `json:"tokens"`
}

// AssistantContextUsage is a structured twin of the /context report,
// carried on [AssistantMessage.ContextUsage]. Evolves additively (new
// optional fields). Port of TypeScript SDK v0.3.232 (SDKContextUsage).
type AssistantContextUsage struct {
	// Model is the main-loop model the usage was computed for.
	Model string `json:"model"`
	// TotalTokens is the estimated tokens in use, unclamped — may exceed
	// RawMaxTokens when over limit.
	TotalTokens int `json:"total_tokens"`
	// RawMaxTokens is the window usage is measured against: the resolved
	// autocompact window (the model's believed limit, or a smaller
	// compaction-policy window).
	RawMaxTokens int `json:"raw_max_tokens"`
	// Percentage is the rounded TotalTokens / RawMaxTokens, 0-100+.
	Percentage int `json:"percentage"`
	// OverLimit is set when TotalTokens exceeds RawMaxTokens.
	OverLimit   *AssistantContextUsageOverLimit   `json:"over_limit,omitempty"`
	Categories  []AssistantContextUsageCategory   `json:"categories"`
	MCPTools    []AssistantContextUsageMCPTool    `json:"mcp_tools"`
	MemoryFiles []AssistantContextUsageMemoryFile `json:"memory_files"`
	Agents      []AssistantContextUsageAgent      `json:"agents"`
	// Skills is nil when no skills contribute tokens.
	Skills []AssistantContextUsageSkill `json:"skills,omitempty"`
}

func (AssistantMessage) messageMarker() {}

// SystemMessage represents a system message with metadata.
type SystemMessage struct {
	Subtype   string         `json:"subtype"`
	Data      map[string]any `json:"data"`
	Timestamp string         `json:"timestamp,omitempty"`
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
	TaskID      string `json:"task_id"`
	Description string `json:"description"`
	UUID        string `json:"uuid"`
	SessionID   string `json:"session_id"`
	ToolUseID   string `json:"tool_use_id,omitempty"`
	TaskType    string `json:"task_type,omitempty"`
	// SubagentType identifies the type of subagent that started this task.
	SubagentType string `json:"subagent_type,omitempty"`
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
	// Blocked indicates the agent was blocked by the auto-mode safety
	// classifier. Port of TypeScript SDK v0.3.199.
	Blocked bool `json:"blocked,omitempty"`
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

// TaskUpdatedStatus represents the lifecycle status reported in a task_updated patch.
// Port of Python SDK v0.2.101.
type TaskUpdatedStatus string

const (
	TaskUpdatedStatusPending   TaskUpdatedStatus = "pending"
	TaskUpdatedStatusRunning   TaskUpdatedStatus = "running"
	TaskUpdatedStatusPaused    TaskUpdatedStatus = "paused"
	TaskUpdatedStatusCompleted TaskUpdatedStatus = "completed"
	TaskUpdatedStatusFailed    TaskUpdatedStatus = "failed"
	TaskUpdatedStatusKilled    TaskUpdatedStatus = "killed"
)

// TerminalTaskStatuses is the set of [TaskUpdatedStatus] values that indicate
// a task has finished. A background task may reach a terminal state only via a
// task_updated message (with no accompanying TaskNotificationMessage); callers
// tracking active tasks should check this set to avoid hanging.
// Port of Python SDK v0.2.101.
var TerminalTaskStatuses = map[TaskUpdatedStatus]bool{
	TaskUpdatedStatusCompleted: true,
	TaskUpdatedStatusFailed:    true,
	TaskUpdatedStatusKilled:    true,
	// "stopped" appears in TaskNotificationMessage.Status but may also surface
	// in task_updated patches; include it for cross-vocabulary completeness.
	"stopped": true,
}

// TaskUpdatedMessage is emitted as a system/task_updated event when a
// background task's state changes. Background tasks may report a terminal
// state only via this message — with no accompanying
// [TaskNotificationMessage] — so consumers tracking active tasks must check
// [TerminalTaskStatuses] on this message to avoid hanging.
// Port of Python SDK v0.2.101.
type TaskUpdatedMessage struct {
	SystemMessage
	// TaskID is the identifier of the task that changed.
	TaskID string `json:"task_id"`
	// Patch contains the changed fields (e.g. {"status": "completed"}).
	Patch map[string]any `json:"patch"`
	// Status is extracted from Patch["status"] for convenience. Empty when
	// the patch does not contain a status change.
	Status TaskUpdatedStatus `json:"status,omitempty"`
	// SessionID is the session this event belongs to.
	SessionID string `json:"session_id,omitempty"`
	// UUID uniquely identifies this event.
	UUID string `json:"uuid,omitempty"`
}

// CommandLifecycleState is the lifecycle state of a queued command reported
// by a [CommandLifecycleMessage].
// Port of TypeScript SDK v0.3.206.
type CommandLifecycleState string

const (
	CommandLifecycleStateQueued    CommandLifecycleState = "queued"
	CommandLifecycleStateStarted   CommandLifecycleState = "started"
	CommandLifecycleStateCompleted CommandLifecycleState = "completed"
	CommandLifecycleStateCancelled CommandLifecycleState = "cancelled"
	CommandLifecycleStateDiscarded CommandLifecycleState = "discarded"
)

// CommandLifecycleMessage is emitted as a system/command_lifecycle event
// reporting the lifecycle state of a previously-queued command. CommandUUID
// identifies the queued message this event pertains to. A Cancelled state
// may result from an explicit cancel or from the turn that would have
// consumed the command dying (e.g. a ResultMessage with TerminalReason
// "tool_deferred_unavailable", "turn_setup_failed", "api_error",
// "malformed_tool_use_exhausted", "budget_exhausted", or
// "structured_output_retry_exhausted") — such turns previously reported
// "completed" for commands they consumed.
// Port of TypeScript SDK v0.3.204-v0.3.206.
type CommandLifecycleMessage struct {
	SystemMessage
	// CommandUUID is the uuid of the queued command this event reports on.
	CommandUUID string `json:"uuid"`
	// State is the lifecycle state of the command.
	State CommandLifecycleState `json:"state"`
	// SessionID is the session this event belongs to.
	SessionID string `json:"session_id,omitempty"`
}

// BackgroundTaskInfo describes one live background task in a
// [BackgroundTasksChangedMessage] payload.
// Port of TypeScript SDK v0.3.203.
type BackgroundTaskInfo struct {
	TaskID      string `json:"task_id"`
	TaskType    string `json:"task_type"`
	Description string `json:"description"`
}

// BackgroundTasksChangedMessage is emitted as a system/background_tasks_changed
// event whenever background-task membership changes: a start, a completion, a
// kill, or a foreground agent being backgrounded. Unlike the
// [TaskStartedMessage]/[TaskNotificationMessage] edge bookends, this is a level
// signal — Tasks is the full set of live background tasks after the change, so
// consumers tracking "is background work running" should replace their tracked
// set with Tasks on every message rather than pairing edges, ensuring a missed
// bookend cannot wedge a stale indicator. The level is per-process: nothing is
// emitted at startup, so consumers must reset to the empty set whenever the
// session's CLI process (re)starts and let the next message repopulate it.
// Port of TypeScript SDK v0.3.203.
type BackgroundTasksChangedMessage struct {
	SystemMessage
	// Tasks is every live background task after the change. REPLACE
	// semantics: swap your tracked set for this payload.
	Tasks []BackgroundTaskInfo `json:"tasks"`
	// UUID uniquely identifies this event.
	UUID string `json:"uuid,omitempty"`
	// SessionID is the session this event belongs to.
	SessionID string `json:"session_id,omitempty"`
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

// NotebookEditOperation specifies which edit operation to perform on a notebook cell.
// Port of TypeScript SDK v0.3.191.
type NotebookEditOperation string

const (
	// NotebookEditOperationReplace replaces the content of an existing cell.
	NotebookEditOperationReplace NotebookEditOperation = "replace"
	// NotebookEditOperationInsert inserts a new cell before the target cell.
	NotebookEditOperationInsert NotebookEditOperation = "insert"
	// NotebookEditOperationInsertAfter inserts a new cell after the target cell.
	NotebookEditOperationInsertAfter NotebookEditOperation = "insert_after"
	// NotebookEditOperationDelete deletes the target cell.
	NotebookEditOperationDelete NotebookEditOperation = "delete"
)

// NotebookEditToolInput is the typed input for the NotebookEdit tool.
// Accessible from [PreToolUseHookInput].ToolInput when ToolName is "NotebookEdit".
// Port of TypeScript SDK v0.3.191.
type NotebookEditToolInput struct {
	// NotebookPath is the filesystem path to the Jupyter notebook file.
	NotebookPath string `json:"notebook_path"`
	// CellID identifies the target cell by its notebook-assigned identifier.
	CellID string `json:"cell_id"`
	// EditMode specifies the operation to perform on the cell.
	EditMode NotebookEditOperation `json:"edit_mode"`
	// NewSource is the replacement source text. Required for replace, insert, and insert_after.
	NewSource string `json:"new_source,omitempty"`
	// CellType is the type of cell to create ("code" or "markdown"). Used for insert/insert_after.
	CellType string `json:"cell_type,omitempty"`
}

// NotebookEditResult is the result returned by the NotebookEdit tool.
// Port of TypeScript SDK v0.3.191.
type NotebookEditResult struct {
	// OldSource contains the prior cell content before the edit. Present only for
	// replace and delete operations; empty for insert and insert_after.
	OldSource string `json:"old_source,omitempty"`
}

// SendFeedbackInput is the typed input for the SendFeedback tool.
// Accessible from [PreToolUseHookInput].ToolInput when ToolName is "SendFeedback".
// Port of TypeScript SDK v0.3.214.
type SendFeedbackInput struct {
	// Type classifies the feedback: "bug", "idea", or "missing_capability".
	Type string `json:"type"`
	// Title is a short, specific one-line summary of the issue.
	Title string `json:"title"`
	// Details is a factual, reproducible report: what was attempted, what
	// happened, exact error text if short, repro steps.
	Details string `json:"details"`
	// Area is an optional short tag naming the part of Claude Code this is
	// about (e.g. "hooks config", "/help", "file editing").
	Area string `json:"area,omitempty"`
}

// SendFeedbackOutput is the result returned by the SendFeedback tool.
// Accessible from [PostToolUseHookInput].ToolResponse when ToolName is
// "SendFeedback" (decode the map[string]any with json.Marshal/Unmarshal, as
// with [NotebookEditResult]). Port of TypeScript SDK v0.3.214.
type SendFeedbackOutput struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// SkillProposal describes a single proposed new or improved skill, part of
// [ProposeSkillsInput.Proposals]. Port of TypeScript SDK v0.3.214.
type SkillProposal struct {
	// Name is the kebab-case skill slug.
	Name string `json:"name"`
	// Kind is "new" or "improvement".
	Kind string `json:"kind"`
	// Target is the existing skill name to amend. Required when Kind is
	// "improvement"; empty for "new".
	Target string `json:"target,omitempty"`
	// Description is the one-line summary shown on the review card.
	Description string `json:"description"`
	// Evidence lists memory file paths where this procedure was observed.
	Evidence []string `json:"evidence,omitempty"`
	// SkillMd is the complete SKILL.md draft (frontmatter + body).
	SkillMd string `json:"skillMd"`
}

// ProposeSkillsInput is the typed input for the ProposeSkills tool.
// Accessible from [PreToolUseHookInput].ToolInput when ToolName is
// "ProposeSkills". Upstream models Proposals as a 1-3 item tuple union; a Go
// slice is sufficient. Port of TypeScript SDK v0.3.214.
type ProposeSkillsInput struct {
	Proposals []SkillProposal `json:"proposals"`
}

// ProposeSkillsOutput is the result returned by the ProposeSkills tool.
// Accessible from [PostToolUseHookInput].ToolResponse when ToolName is
// "ProposeSkills" (decode the map[string]any with json.Marshal/Unmarshal, as
// with [NotebookEditResult]). Port of TypeScript SDK v0.3.214.
type ProposeSkillsOutput struct {
	// ProposalCount is the number of proposals shown on the review card.
	ProposalCount int `json:"proposalCount"`
}

// ProposeGoalInput is the typed input for the ProposeGoal tool. Accessible
// from [PreToolUseHookInput].ToolInput when ToolName is "ProposeGoal". Port of
// TypeScript SDK v0.3.227.
type ProposeGoalInput struct {
	// Condition is the completion condition to propose, written so a
	// separate evaluator can verify it from the conversation (e.g. "all
	// tests in test/auth pass (bun test exits 0)"). At most 500 characters.
	Condition string `json:"condition"`
	// AskUser controls whether the user is asked for approval before the
	// goal is set. Upstream defaults this to true (an approval dialog is
	// shown) when omitted; false sets the goal directly, with a visible
	// notice in the transcript, and is only used when the user's own words
	// in the conversation stated the outcome directly.
	AskUser bool `json:"ask_user,omitempty"`
}

// ProposeGoalOutput is the result returned by the ProposeGoal tool.
// Accessible from [PostToolUseHookInput].ToolResponse when ToolName is
// "ProposeGoal" (decode the map[string]any with json.Marshal/Unmarshal, as
// with [NotebookEditResult]). Port of TypeScript SDK v0.3.227.
type ProposeGoalOutput struct {
	// Condition is the condition shown to the user for approval, or set
	// directly when AskUser was false.
	Condition string `json:"condition"`
	// AskUser is true when the user was asked for approval, false when the
	// goal was set directly.
	AskUser bool `json:"askUser"`
}

// SkillToolOutput is the result returned by the Skill tool.
// Accessible from [PostToolUseHookInput].ToolResponse when ToolName is "Skill"
// (decode the map[string]any with json.Marshal/Unmarshal, as with [NotebookEditResult]).
// Port of TypeScript SDK v0.3.218 — unlike most other TS-only wire types in
// this file, this struct does not appear in the published
// @anthropic-ai/claude-agent-sdk@0.3.220 npm package's bundled sdk-tools.d.ts
// (checked, no "Skill" occurrence at all); modeled directly from the upstream
// changelog's prose description only, so the field is a best-effort
// interpretation and may need revision once a typed source is available,
// consistent with how issue #530 (ToolResultMeta) handled the same situation.
type SkillToolOutput struct {
	// Background is true when the skill was dispatched as a detached
	// background agent (a "forked" skill) rather than run inline.
	Background bool `json:"background,omitempty"`
}

// BashToolOutput is the result returned by the Bash tool.
// Accessible from [PostToolUseHookInput].ToolResponse when ToolName is "Bash"
// (decode the map[string]any with json.Marshal/Unmarshal, as with [NotebookEditResult]).
// Port of TypeScript SDK v0.3.210 and v0.3.227.
type BashToolOutput struct {
	// TimedOutAfterMs is set when the command was auto-backgrounded after
	// exceeding its timeout, giving the elapsed time in milliseconds at the
	// point of backgrounding. Nil when the command completed normally.
	TimedOutAfterMs *int `json:"timedOutAfterMs,omitempty"`
	// BackgroundEndsWithFinalResponse is true when this backgrounded command
	// is owned by a synchronous subagent and is therefore terminated when
	// that agent gives its final response. Nil when the command survives the
	// call that started it (main loop, async subagents).
	BackgroundEndsWithFinalResponse *bool `json:"backgroundEndsWithFinalResponse,omitempty"`
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

// AgentOutputStatus discriminates the shape of the structured result payload
// for the subagent-spawning "Agent" tool, which appears on the wire as
// "Task" (distinct from the task-management TaskCreate/TaskGet/TaskUpdate/
// TaskList family above). Port of TypeScript SDK v0.2.75/v0.3.207.
type AgentOutputStatus string

const (
	// AgentOutputStatusCompleted indicates the subagent finished and
	// [UserMessage.ToolUseResult] decodes as [AgentToolCompletedOutput].
	AgentOutputStatusCompleted AgentOutputStatus = "completed"
	// AgentOutputStatusAsyncLaunched indicates the subagent was launched in
	// the background and [UserMessage.ToolUseResult] decodes as
	// [AgentToolAsyncLaunchedOutput].
	AgentOutputStatusAsyncLaunched AgentOutputStatus = "async_launched"
	// AgentOutputStatusRemoteLaunched indicates the subagent was launched as
	// a remote cloud session and [UserMessage.ToolUseResult] decodes as
	// [AgentToolRemoteLaunchedOutput].
	AgentOutputStatusRemoteLaunched AgentOutputStatus = "remote_launched"
)

// AgentContentBlock is a text block within
// [AgentToolCompletedOutput].Content.
type AgentContentBlock struct {
	Type      string `json:"type"` // "text"
	Text      string `json:"text"`
	Citations []any  `json:"citations,omitempty"`
}

// AgentToolUsage mirrors the token-usage object embedded in
// [AgentToolCompletedOutput].
type AgentToolUsage struct {
	InputTokens              int  `json:"input_tokens"`
	OutputTokens             int  `json:"output_tokens"`
	CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
	ServerToolUse            *struct {
		WebSearchRequests int `json:"web_search_requests"`
		WebFetchRequests  int `json:"web_fetch_requests"`
	} `json:"server_tool_use"`
	ServiceTier   *string `json:"service_tier"`
	CacheCreation *struct {
		Ephemeral1hInputTokens int `json:"ephemeral_1h_input_tokens"`
		Ephemeral5mInputTokens int `json:"ephemeral_5m_input_tokens"`
	} `json:"cache_creation"`
	InferenceGeo *string `json:"inference_geo,omitempty"`
	Speed        *string `json:"speed,omitempty"`
	// OutputTokensDetails breaks down OutputTokens by category (currently
	// just thinking tokens). Port of TypeScript SDK v0.3.228.
	OutputTokensDetails *struct {
		ThinkingTokens *int `json:"thinking_tokens,omitempty"`
	} `json:"output_tokens_details,omitempty"`
}

// AgentToolStats mirrors the optional per-run tool-usage counters embedded
// in [AgentToolCompletedOutput].
type AgentToolStats struct {
	ReadCount      int  `json:"readCount"`
	SearchCount    int  `json:"searchCount"`
	BashCount      int  `json:"bashCount"`
	EditFileCount  int  `json:"editFileCount"`
	LinesAdded     int  `json:"linesAdded"`
	LinesRemoved   int  `json:"linesRemoved"`
	OtherToolCount int  `json:"otherToolCount"`
	FrameCount     *int `json:"frameCount,omitempty"`
}

// AgentToolCompletedOutput is the structured result payload for a completed
// Agent/Task tool call: the subagent's final report, without the
// model-directed agentId/usage trailer the tool_result text carries, plus
// run totals. Accessible from [UserMessage.ToolUseResult] when the preceding
// [ToolUseBlock].Name is "Task" and Status is [AgentOutputStatusCompleted]
// (decode the map[string]any with json.Marshal/Unmarshal, as with
// [NotebookEditResult]). Render from this instead of parsing the tool_result
// text. Port of TypeScript SDK v0.3.207.
type AgentToolCompletedOutput struct {
	Status        AgentOutputStatus   `json:"status"` // always "completed"
	AgentID       string              `json:"agentId"`
	AgentType     string              `json:"agentType,omitempty"`
	Content       []AgentContentBlock `json:"content"`
	ResolvedModel string              `json:"resolvedModel,omitempty"`
	// ModelsUsed lists, in order, the distinct models used by this subagent
	// run. A length greater than 1 indicates a mid-run model swap. Port of
	// TypeScript SDK v0.3.212.
	ModelsUsed        []string        `json:"modelsUsed,omitempty"`
	TotalToolUseCount int             `json:"totalToolUseCount"`
	TotalDurationMs   int             `json:"totalDurationMs"`
	TotalTokens       int             `json:"totalTokens"`
	Usage             AgentToolUsage  `json:"usage"`
	ToolStats         *AgentToolStats `json:"toolStats,omitempty"`
	Prompt            string          `json:"prompt"`
	WorktreePath      string          `json:"worktreePath,omitempty"`
	WorktreeBranch    string          `json:"worktreeBranch,omitempty"`
}

// AgentToolAsyncLaunchedOutput is the structured result payload for an
// Agent/Task tool call that launched a background subagent. Accessible from
// [UserMessage.ToolUseResult] when the preceding [ToolUseBlock].Name is
// "Task" and Status is [AgentOutputStatusAsyncLaunched]. Port of TypeScript
// SDK v0.3.207.
type AgentToolAsyncLaunchedOutput struct {
	Status        AgentOutputStatus `json:"status"` // always "async_launched"
	AgentID       string            `json:"agentId"`
	Description   string            `json:"description"`
	ResolvedModel string            `json:"resolvedModel,omitempty"`
	// ModelsUsed lists, in order, the distinct models used before this
	// subagent was backgrounded. A length greater than 1 indicates a mid-run
	// model swap. Port of TypeScript SDK v0.3.212.
	ModelsUsed        []string `json:"modelsUsed,omitempty"`
	Prompt            string   `json:"prompt"`
	OutputFile        string   `json:"outputFile"`
	CanReadOutputFile bool     `json:"canReadOutputFile,omitempty"`
}

// AgentToolRemoteLaunchedOutput is the structured result payload for an
// Agent/Task tool call that launched a remote cloud session. Accessible from
// [UserMessage.ToolUseResult] when the preceding [ToolUseBlock].Name is
// "Task" and Status is [AgentOutputStatusRemoteLaunched]. Port of
// TypeScript SDK v0.3.207.
type AgentToolRemoteLaunchedOutput struct {
	Status      AgentOutputStatus `json:"status"` // always "remote_launched"
	TaskID      string            `json:"taskId"`
	SessionURL  string            `json:"sessionUrl"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	OutputFile  string            `json:"outputFile"`
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

// ModelFallbackTrigger is the reason why the CLI fell back to a different model.
// Port of TypeScript SDK v0.3.174.
type ModelFallbackTrigger string

const (
	// ModelFallbackTriggerModelNotFound is set when the requested model was not found.
	ModelFallbackTriggerModelNotFound ModelFallbackTrigger = "model_not_found"
	// ModelFallbackTriggerPermissionDenied is set when the model was denied by policy.
	ModelFallbackTriggerPermissionDenied ModelFallbackTrigger = "permission_denied"
	// ModelFallbackTriggerOverloaded is set when the model was overloaded.
	ModelFallbackTriggerOverloaded ModelFallbackTrigger = "overloaded"
	// ModelFallbackTriggerServerError is set when the model returned a server error.
	ModelFallbackTriggerServerError ModelFallbackTrigger = "server_error"
	// ModelFallbackTriggerLastResort is set when all preferred models failed and the CLI chose a last-resort fallback.
	ModelFallbackTriggerLastResort ModelFallbackTrigger = "last_resort"
)

// FastModeState describes whether fast inference mode is currently active for
// the session. Port of TypeScript SDK v0.3.219.
type FastModeState string

const (
	// FastModeStateOff means fast mode is not active.
	FastModeStateOff FastModeState = "off"
	// FastModeStateCooldown means fast mode is temporarily unavailable after
	// recent use and will become available again after a cooldown period.
	FastModeStateCooldown FastModeState = "cooldown"
	// FastModeStateOn means fast mode is active.
	FastModeStateOn FastModeState = "on"
)

// FastModeDisabledReason explains why fast mode is unavailable when
// FastModeState is not FastModeStateOn. Port of TypeScript SDK v0.3.219.
type FastModeDisabledReason string

const (
	// FastModeDisabledReasonFree is set when the account's plan doesn't include fast mode.
	FastModeDisabledReasonFree FastModeDisabledReason = "free"
	// FastModeDisabledReasonPreference is set when the user has turned fast mode off.
	FastModeDisabledReasonPreference FastModeDisabledReason = "preference"
	// FastModeDisabledReasonExtraUsageDisabled is set when fast mode would incur extra usage that is disabled.
	FastModeDisabledReasonExtraUsageDisabled FastModeDisabledReason = "extra_usage_disabled"
	// FastModeDisabledReasonNetworkError is set when a network error prevented enabling fast mode.
	FastModeDisabledReasonNetworkError FastModeDisabledReason = "network_error"
	// FastModeDisabledReasonUnknown is set when the CLI could not determine a specific reason.
	FastModeDisabledReasonUnknown FastModeDisabledReason = "unknown"
	// FastModeDisabledReasonNotFirstParty is set when the current provider isn't Anthropic's first-party API.
	FastModeDisabledReasonNotFirstParty FastModeDisabledReason = "not_first_party"
	// FastModeDisabledReasonDisabledByEnv is set when an environment variable disables fast mode.
	FastModeDisabledReasonDisabledByEnv FastModeDisabledReason = "disabled_by_env"
	// FastModeDisabledReasonModelNotAllowed is set when the current model doesn't support fast mode.
	FastModeDisabledReasonModelNotAllowed FastModeDisabledReason = "model_not_allowed"
	// FastModeDisabledReasonSDKOptInRequired is set when the SDK host must opt in via Options.FastMode.
	FastModeDisabledReasonSDKOptInRequired FastModeDisabledReason = "sdk_opt_in_required"
	// FastModeDisabledReasonPending is set while the CLI is still determining fast mode eligibility.
	FastModeDisabledReasonPending FastModeDisabledReason = "pending"
)

// ModelFallbackMessage is emitted when the CLI falls back to a different model.
// Received for all fallback triggers: model_not_found, permission_denied,
// overloaded, server_error, and last_resort. Port of TypeScript SDK v0.3.174.
type ModelFallbackMessage struct {
	SystemMessage
	// Trigger is the reason the model fallback occurred.
	Trigger ModelFallbackTrigger `json:"trigger,omitempty"`
	// Model is the fallback model that was selected.
	Model string `json:"model,omitempty"`
	// OriginalModel is the model that was originally requested.
	OriginalModel string `json:"original_model,omitempty"`
}

// HookEventMessage is a system message emitted for hook lifecycle events when
// Options.IncludeHookEvents is true. Subtypes: "hook_started", "hook_response".
// Port of Python SDK PR anthropics/claude-agent-sdk-python#917.
type HookEventMessage struct {
	SystemMessage
	// HookEvent is the hook event type (e.g. "PreToolUse", "PostToolUse", "Stop").
	HookEvent string `json:"hook_event"`
	// HookID is the unique identifier for this hook invocation.
	HookID string `json:"hook_id"`
	// HookName is the name of the hook callback.
	HookName string `json:"hook_name"`
	// Output is the hook callback's return value (hook_response subtype only).
	Output map[string]any `json:"output,omitempty"`
	// ExitCode is the hook process exit code (hook_response subtype only).
	ExitCode *int `json:"exit_code,omitempty"`
	// Outcome is the result outcome string (hook_response subtype only).
	Outcome string `json:"outcome,omitempty"`
}

// ApiRetryError represents the machine-readable error category on [ApiRetryMessage].
type ApiRetryError string

const (
	// ApiRetryErrorRateLimit is set when the API returned a 429 rate-limit response.
	ApiRetryErrorRateLimit ApiRetryError = "rate_limit"
	// ApiRetryErrorOverloaded is set when the API returned a 529 server-overloaded response.
	ApiRetryErrorOverloaded ApiRetryError = "overloaded"
)

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
	// Error is the machine-readable error category: "rate_limit" for 429
	// responses and "overloaded" for 529 responses. Empty when not provided by
	// the CLI. Port of TypeScript SDK v0.3.150.
	Error ApiRetryError `json:"error,omitempty"`
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

// WorkerShuttingDownMessage is emitted when a Remote Control worker exits
// gracefully. Consumers can use this to distinguish a clean worker shutdown
// from an unexpected disconnect or connection loss.
// Port of TypeScript SDK v0.3.178.
type WorkerShuttingDownMessage struct {
	SystemMessage
	// Reason is an optional human-readable description of why the worker
	// is shutting down. Empty when not provided by the CLI.
	Reason string `json:"reason,omitempty"`
}

// PermissionDeniedAdvisoryReason is the machine-readable reason a tool call
// was denied in a permission_denied_advisory system message.
// Port of TypeScript SDK v0.3.178.
type PermissionDeniedAdvisoryReason string

const (
	// PermissionDeniedAdvisoryReasonSafetyCheck is set when the denial was
	// triggered by a safety-check policy.
	PermissionDeniedAdvisoryReasonSafetyCheck PermissionDeniedAdvisoryReason = "safetyCheck"
	// PermissionDeniedAdvisoryReasonAsyncAgent is set when the denial was
	// triggered because the tool was called from an async agent context that
	// does not allow the operation.
	PermissionDeniedAdvisoryReasonAsyncAgent PermissionDeniedAdvisoryReason = "asyncAgent"
)

// PermissionDeniedAdvisoryMessage is emitted when a tool call is denied and
// the CLI sends an advisory notification. The DenialReason field lets
// consumers programmatically distinguish safety-policy denials from
// async-agent context denials without inspecting raw JSON.
// Port of TypeScript SDK v0.3.178.
type PermissionDeniedAdvisoryMessage struct {
	SystemMessage
	// ToolName is the name of the tool that was denied.
	ToolName string `json:"tool_name,omitempty"`
	// DenialReason is the machine-readable reason for the denial.
	// One of PermissionDeniedAdvisoryReasonSafetyCheck or
	// PermissionDeniedAdvisoryReasonAsyncAgent when provided by the CLI.
	DenialReason PermissionDeniedAdvisoryReason `json:"denial_reason,omitempty"`
}

// PermissionDeniedMessage is emitted for the system/permission_denied subtype:
// bare headless (`-p` / SDK query() without CanUseTool) auto-denying a tool
// call. Distinct from PermissionDeniedAdvisoryMessage (permission_denied_advisory),
// which only carries ToolName/DenialReason; this subtype carries the full
// denied-call details, matching PermissionDeniedHookInput's shape. Port of
// TypeScript SDK v0.3.223. ([#562])
//
// [#562]: https://github.com/Flohs/claude-agent-sdk-go/issues/562
type PermissionDeniedMessage struct {
	SystemMessage
	// ToolName is the name of the tool that was denied.
	ToolName string `json:"tool_name,omitempty"`
	// ToolInput is the input the tool call was made with.
	ToolInput any `json:"tool_input,omitempty"`
	// ToolUseID is the tool-use ID of the denied call.
	ToolUseID string `json:"tool_use_id,omitempty"`
	// Reason is a human-readable explanation for the denial.
	Reason string `json:"reason,omitempty"`
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

// MessageOriginKind identifies what triggered a message, distinguishing
// direct human input from background agents, scheduled tasks, and other
// non-interactive senders.
// Port of TypeScript SDK bundled sdk.d.ts (SDKMessageOrigin).
type MessageOriginKind string

const (
	// MessageOriginKindHuman is a message sent directly by the human user.
	MessageOriginKindHuman MessageOriginKind = "human"
	// MessageOriginKindChannel is a message delivered via a named channel
	// (e.g. Slack). Server carries the channel server name.
	MessageOriginKindChannel MessageOriginKind = "channel"
	// MessageOriginKindPeer is a message relayed from another agent peer.
	// From, Name, SenderTaskID, and Body may be populated.
	MessageOriginKindPeer MessageOriginKind = "peer"
	// MessageOriginKindTaskNotification is a message delivered as the result
	// of a completed task. Subkind is MessageOriginSubkindScheduledTrigger
	// when the delivery is the fired prompt of a scheduled task/routine, or
	// MessageOriginSubkindPeerSendMessage when it's a cross-session
	// SendMessage delivery from another of the same user's sessions.
	MessageOriginKindTaskNotification MessageOriginKind = "task-notification"
	// MessageOriginKindCoordinator is a message sent by the coordinator.
	MessageOriginKindCoordinator MessageOriginKind = "coordinator"
	// MessageOriginKindUnclassified is a message whose origin the CLI could
	// not classify into one of the other kinds.
	MessageOriginKindUnclassified MessageOriginKind = "unclassified"
	// MessageOriginKindObserver is a message sent by an observer agent.
	// From and SenderTaskID are populated.
	MessageOriginKindObserver MessageOriginKind = "observer"
	// MessageOriginKindAutoContinuation is a message the CLI generated to
	// automatically continue a turn.
	MessageOriginKindAutoContinuation MessageOriginKind = "auto-continuation"
	// MessageOriginKindObserverActivity is a message reporting observer
	// activity.
	MessageOriginKindObserverActivity MessageOriginKind = "observer-activity"
)

// MessageOriginSubkind values further classify
// MessageOriginKindTaskNotification, populated in MessageOrigin.Subkind.
// Port of TypeScript SDK v0.3.224.
const (
	// MessageOriginSubkindScheduledTrigger marks a delivery that is the
	// fired stored prompt of a scheduled task/routine (server-asserted
	// provenance; the schedule attests storage, not authorship). The harness
	// frames this delivery as the session's assigned task instead of the
	// generic background-notification frame.
	MessageOriginSubkindScheduledTrigger = "scheduled-trigger"
	// MessageOriginSubkindPeerSendMessage marks a coordinator co-member
	// SendMessage delivery: model-authored text from another of the same
	// user's sessions, verified by server-stamped receiver co-membership.
	// Distinguishable from a plain task-notification so the receive-side
	// crossSessionInbound setting can apply to it.
	MessageOriginSubkindPeerSendMessage = "peer-send-message"
)

// MessageOrigin identifies what triggered a message (e.g. distinguishing a
// direct human prompt from a background task notification or a peer relay).
// Which fields are populated depends on Kind; see each field's doc comment.
// Port of TypeScript SDK bundled sdk.d.ts (SDKMessageOrigin).
type MessageOrigin struct {
	Kind MessageOriginKind `json:"kind"`
	// Server is the channel server name. Populated for MessageOriginKindChannel.
	Server string `json:"server,omitempty"`
	// From identifies the sending peer or observer. Populated for
	// MessageOriginKindPeer and MessageOriginKindObserver.
	From string `json:"from,omitempty"`
	// Name is the sender's display name. Populated for MessageOriginKindPeer.
	Name string `json:"name,omitempty"`
	// SenderTaskID is the sending peer's or observer's task ID. Populated for
	// MessageOriginKindPeer (optional) and MessageOriginKindObserver
	// (required).
	SenderTaskID string `json:"senderTaskId,omitempty"`
	// Body is the raw relayed message body. Populated for
	// MessageOriginKindPeer.
	Body string `json:"body,omitempty"`
	// FromMode is the sending session's permission class as declared by the
	// host that injects this message on local stdin: "bypass" for sessions
	// that run tools without asking, "prompting" otherwise. Lets the
	// recipient deliver a same-class message immediately while a
	// cross-class or undeclared sender is still held at a recipient that
	// runs without asking. Populated for MessageOriginKindPeer. Only
	// honored from the injecting host on local stdin; empty when the host
	// doesn't declare it.
	FromMode string `json:"fromMode,omitempty"`
	// Subkind further classifies MessageOriginKindTaskNotification: either
	// MessageOriginSubkindScheduledTrigger or
	// MessageOriginSubkindPeerSendMessage. Absent on webhook, PR-steward,
	// plugin, and background-event deliveries.
	Subkind string `json:"subkind,omitempty"`
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
	// StopDetails contains structured metadata accompanying the stop reason,
	// e.g. when StopReason is "refusal". Nil when not provided by the CLI.
	StopDetails map[string]any `json:"stop_details,omitempty"`
	// TerminalReason describes why the session terminated (e.g. "completed",
	// "aborted_tools", "max_turns", "blocking_limit"). Empty when not
	// provided by the CLI.
	TerminalReason string `json:"terminal_reason,omitempty"`
	// FastModeState reports whether fast inference mode was active for this
	// turn. Empty when not provided by the CLI. Port of TypeScript SDK
	// v0.3.219.
	FastModeState FastModeState `json:"fast_mode_state,omitempty"`
	// FastModeDisabledReason explains why FastModeState isn't
	// FastModeStateOn. Empty when fast mode is on or the CLI didn't report a
	// reason. Port of TypeScript SDK v0.3.219.
	FastModeDisabledReason FastModeDisabledReason `json:"fast_mode_disabled_reason,omitempty"`
	// APIErrorStatus is the HTTP status code (e.g. 429, 500, 529) from a
	// failing API call when IsError is true. Zero when not provided by the
	// CLI (requires CLI >= v2.1.110).
	APIErrorStatus *int `json:"api_error_status,omitempty"`
	// Origin forwards the triggering message's origin so consumers can
	// distinguish user-prompted results from task-notification followups.
	// Nil when the CLI omits the field (the common case for main-session
	// results).
	Origin *MessageOrigin `json:"origin,omitempty"`
	// RequestID is the API request identifier for the final API call.
	RequestID    string   `json:"request_id,omitempty"`
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
	// Usage is the raw main-loop usage blob for this turn only — it does not
	// include tokens consumed by subagents, background tasks, or other
	// query-pipeline calls outside the main loop. For cumulative,
	// cost-accounting-authoritative usage across the whole query pipeline,
	// use ModelUsage instead. Documented distinction per TypeScript SDK
	// v0.3.223. ([#563])
	//
	// [#563]: https://github.com/Flohs/claude-agent-sdk-go/issues/563
	Usage map[string]any `json:"usage,omitempty"`
	// ModelUsage contains a per-model token usage and cost breakdown, keyed
	// by the raw model string reported by the CLI. Nil when not provided.
	// Unlike Usage, this is cumulative across every call in the query
	// pipeline (main loop, subagents, background tasks), not just the main
	// loop, and is the field to use for cost accounting. Port of TypeScript
	// SDK v0.3.218 / Python SDK v0.2.126; cumulative-vs-per-turn distinction
	// documented in TypeScript SDK v0.3.223. ([#563])
	ModelUsage       map[string]ModelUsage `json:"model_usage,omitempty"`
	Result           string                `json:"result,omitempty"`
	StructuredOutput any                   `json:"structured_output,omitempty"`
	// DeferredToolUse is populated when a PreToolUse hook returned
	// {"decision": "defer"}, surfacing the pending tool call so the caller
	// can prompt the user and resume. Nil when no deferral occurred.
	DeferredToolUse *DeferredToolUse `json:"deferred_tool_use,omitempty"`
	// Timestamp is the ISO-8601 datetime when this message was recorded in the
	// CLI transcript. Empty when not provided by the CLI.
	Timestamp string `json:"timestamp,omitempty"`
	// UserMessageUUID is the uuid of the user message that triggered this
	// result, for cross-host request-latency correlation. Empty when not
	// provided by the CLI (e.g. non-success subtypes).
	UserMessageUUID string `json:"user_message_uuid,omitempty"`
	// RequestSentWallMs is the wall-clock timestamp (ms) when the request was
	// sent, for cross-host request-latency correlation. Nil when not
	// provided by the CLI.
	RequestSentWallMs *int64 `json:"request_sent_wall_ms,omitempty"`
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

// RateLimitType is a machine-readable category for rate-limit events.
// Port of TypeScript SDK v0.3.191.
type RateLimitType = string

const (
	// RateLimitTypeSevenDayOverageIncluded identifies a seven-day rolling
	// overage-included rate limit window.
	RateLimitTypeSevenDayOverageIncluded RateLimitType = "seven_day_overage_included"
)

// RateLimitInfo contains detailed rate limit information.
type RateLimitInfo struct {
	Status                          RateLimitStatus `json:"status"`
	ResetsAt                        *string         `json:"resets_at,omitempty"`
	RateLimitType                   *string         `json:"rate_limit_type,omitempty"`
	Utilization                     *float64        `json:"utilization,omitempty"`
	OverageStatus                   *string         `json:"overage_status,omitempty"`
	OverageResetsAt                 *string         `json:"overage_resets_at,omitempty"`
	OverageDisabledReason           *string         `json:"overage_disabled_reason,omitempty"`
	ErrorCode                       *string         `json:"error_code,omitempty"`
	CanUserPurchaseCredits          *bool           `json:"can_user_purchase_credits,omitempty"`
	HasChargeableSavedPaymentMethod *bool           `json:"has_chargeable_saved_payment_method,omitempty"`
}

// RateLimitEvent represents a rate limit status change from the CLI.
type RateLimitEvent struct {
	Type          string        `json:"type"`
	RateLimitInfo RateLimitInfo `json:"rate_limit_info"`
	UUID          string        `json:"uuid,omitempty"`
	SessionID     string        `json:"session_id,omitempty"`
}

func (RateLimitEvent) messageMarker() {}

// ConversationResetMessage is emitted when the session's conversation is
// replaced without ending the connection — e.g. after "/clear", or any other
// flow that discards the transcript mid-session.
//
// In streaming input mode a single connection can carry many user turns, and
// a reset clears the conversation history *and* zeroes the running totals
// reported on subsequent [ResultMessage] values (e.g. TotalCostUSD). If a
// caller accumulates those totals across a long-lived session, it should
// snapshot them when this message arrives.
//
// Port of Python SDK commit 54dd3b4 (anthropics/claude-agent-sdk-python#1196).
type ConversationResetMessage struct {
	// NewConversationID is an opaque identifier for the fresh conversation,
	// for UIs to key an empty transcript on (and discard any cached session
	// title). This is NOT the SessionID of subsequent messages — read that
	// from the next message.
	NewConversationID string `json:"new_conversation_id"`
	// UUID is the unique ID of this message.
	UUID string `json:"uuid"`
	// SessionID is the ID of the session that was reset (the outgoing
	// session; messages after the reset carry a new session ID).
	SessionID string `json:"session_id"`
}

func (ConversationResetMessage) messageMarker() {}

// SubagentRetryInfo carries retry-attempt bookkeeping for a subagent that is
// being retried after a failure. Populated on ToolProgressMessage.SubagentRetry
// when the CLI is currently retrying a subagent launch.
type SubagentRetryInfo struct {
	AgentID      string `json:"agent_id"`
	Attempt      int    `json:"attempt"`
	MaxRetries   int    `json:"max_retries"`
	RetryDelayMs int    `json:"retry_delay_ms"`
	// ErrorStatus is the HTTP status code from the failing attempt, when
	// available. Nil when not provided by the CLI.
	ErrorStatus   *int   `json:"error_status"`
	ErrorCategory string `json:"error_category"`
}

// ToolProgressMessage is a top-level "tool_progress" message emitted while a
// tool call (including subagent-spawning Task/Agent calls) is in progress.
// Unlike TaskProgressMessage (a "system"/"task_progress" subtype), this is
// its own top-level message type and does not embed SystemMessage.
// Port of TypeScript SDK bundled sdk-tools.d.ts (SDKToolProgressMessage).
type ToolProgressMessage struct {
	ToolUseID string `json:"tool_use_id"`
	ToolName  string `json:"tool_name"`
	// ParentToolUseID is the tool-use ID of the enclosing tool call, or nil
	// when this tool call is not nested inside another.
	ParentToolUseID    *string `json:"parent_tool_use_id"`
	ElapsedTimeSeconds float64 `json:"elapsed_time_seconds"`
	// TaskID identifies the task this progress report belongs to, when the
	// tool call is running as part of a task. Empty when not provided.
	TaskID    string `json:"task_id,omitempty"`
	UUID      string `json:"uuid"`
	SessionID string `json:"session_id"`
	// Heartbeat indicates this progress report is a periodic keep-alive
	// rather than a report tied to actual tool progress.
	Heartbeat bool `json:"heartbeat,omitempty"`
	// SubagentType identifies the type of subagent this progress report
	// belongs to, when the tool call is a subagent-spawning call. Empty when
	// not applicable.
	SubagentType string `json:"subagent_type,omitempty"`
	// SubagentRetry is populated when the CLI is currently retrying a
	// subagent launch after a failure. Nil when no retry is in progress.
	SubagentRetry *SubagentRetryInfo `json:"subagent_retry,omitempty"`
}

func (ToolProgressMessage) messageMarker() {}

// The following prefix buckets classify a rate-limit-related message string
// (e.g. surfaced in a ResultMessage or system message) by matching its
// leading text with strings.HasPrefix, without hand-mirroring the literal
// strings at each call site. They are plain []string, not arrays, so callers
// must treat them as read-only: Go has no way to enforce immutability on a
// slice. Port of TypeScript SDK v0.3.211 (published as @alpha exports;
// values confirmed from the npm package's bundled sdk.d.ts, since the
// TypeScript SDK's GitHub repository does not expose its .ts source).
var (
	// OrgPolicyLimitPrefixes matches messages indicating the org has
	// disabled this service via policy.
	OrgPolicyLimitPrefixes = []string{
		"This service is disabled for your org",
	}

	// UsageLimitErrorPrefixes matches messages indicating usage credits or
	// allocation have been exhausted or disabled, blocking further use.
	UsageLimitErrorPrefixes = []string{
		"You've hit your",
		"You've reached your",
		"You're out of usage credits",
		"Your org is out of usage · add funds to continue",
		"Your org is out of usage · contact your admin",
		"Your seat type doesn't include usage credits",
		"Your seat type doesn't include usage",
		"Your usage allocation has been disabled by your admin",
		"Your group's usage limit is set to $0",
		"Fable 5 requires usage credits",
		"You're out of extra usage",
		"Your seat type doesn't include extra usage",
	}

	// UsageTransitionPrefixes matches messages announcing a switch onto
	// usage credits or a usage allocation after a prior limit condition.
	UsageTransitionPrefixes = []string{
		"You're now using usage credits",
		"You're now using your usage allocation",
		"Now using your usage allocation",
		"Now using usage credits",
		"You're now using extra usage",
		"Now using extra usage",
	}

	// UsageWarningPrefixes matches messages warning that usage is
	// approaching a limit, short of an outright rejection.
	UsageWarningPrefixes = []string{
		"You've used",
		"You're close to",
	}
)

// ContextUsage contains context window utilization broken down by category.
type ContextUsage struct {
	TotalTokens     int            `json:"total_tokens"`
	UsedTokens      int            `json:"used_tokens"`
	UsageByCategory map[string]int `json:"usage_by_category,omitempty"`
}

// ModelScopedUsage holds token usage counts for a single model within a session.
// Port of TypeScript SDK v0.3.191.
type ModelScopedUsage struct {
	// Model is the model identifier (e.g. "claude-opus-4-8").
	Model string `json:"model"`
	// InputTokens is the total number of input tokens consumed by this model.
	InputTokens int `json:"input_tokens"`
	// OutputTokens is the total number of output tokens generated by this model.
	OutputTokens int `json:"output_tokens"`
	// CacheCreationInputTokens is the number of tokens written to the prompt cache.
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	// CacheReadInputTokens is the number of tokens read from the prompt cache.
	CacheReadInputTokens int `json:"cache_read_input_tokens,omitempty"`
}

// ModelUsage holds the per-model token usage and cost breakdown reported on
// ResultMessage.ModelUsage. Field names mirror the TypeScript SDK's
// ModelUsage shape verbatim, since the CLI's raw modelUsage entries are
// passed through as-is. Port of TypeScript SDK v0.3.218 / Python SDK
// v0.2.126.
type ModelUsage struct {
	InputTokens              int     `json:"inputTokens"`
	OutputTokens             int     `json:"outputTokens"`
	CacheReadInputTokens     int     `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int     `json:"cacheCreationInputTokens"`
	WebSearchRequests        int     `json:"webSearchRequests"`
	CostUSD                  float64 `json:"costUSD"`
	ContextWindow            int     `json:"contextWindow"`
	MaxOutputTokens          int     `json:"maxOutputTokens"`
	// CanonicalModel is the canonical model id used for the pricing lookup
	// (e.g. "claude-opus-4-8"), which may differ from the raw model string
	// this entry is keyed by (provider-specific ids, aliases). Empty when
	// not provided by the CLI.
	CanonicalModel string `json:"canonicalModel,omitempty"`
	// Provider is the API provider that served this model ("firstParty",
	// "bedrock", "vertex", "foundry", "anthropicAws", "anthropicGoogleCloud",
	// "mantle", "gateway"). Empty when not provided by the CLI.
	Provider string `json:"provider,omitempty"`
}

// UsageDataExperimental contains session cost, plan rate-limit, and local
// usage data returned by Client.GetUsageExperimental.
//
// WARNING: This API shape is experimental and may change or be removed
// without notice. Port of TypeScript SDK v0.3.169.
type UsageDataExperimental struct {
	// TotalCostUSD is the cumulative cost for this session in US dollars.
	TotalCostUSD *float64 `json:"total_cost_usd,omitempty"`
	// PlanRateLimit contains plan-tier rate-limit status.
	PlanRateLimit map[string]any `json:"plan_rate_limit,omitempty"`
	// LocalUsage contains local usage-behavior data.
	LocalUsage map[string]any `json:"local_usage,omitempty"`
	// ModelScoped contains per-model token usage breakdowns for the session.
	// Port of TypeScript SDK v0.3.191.
	ModelScoped []ModelScopedUsage `json:"model_scoped,omitempty"`
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

// InterruptReceipt is the result of [Client.Interrupt] on CLIs advertising
// the "interrupt_receipt_v1" protocol capability (see
// [ServerCapabilities.Capabilities]); older CLIs return a zero-value receipt
// with a nil StillQueued. Port of TypeScript SDK v0.3.205.
type InterruptReceipt struct {
	// StillQueued lists uuids of async user messages that survive this
	// interrupt: commands still in the queue, plus any batch already
	// dequeued for the imminent turn but not yet reachable by the abort.
	// These WILL run unless cancelled first via a mechanism outside this
	// SDK's current surface. Coverage caveats: only uuid-stamped messages
	// appear (a message enqueued without a uuid still runs but is never
	// listed, so an empty slice does not mean "nothing will run"); only
	// main-thread messages are listed; the list may include
	// internally-enqueued uuids the caller never sent (cron triggers,
	// auto-resume continuations) — treat unknown uuids as informational
	// rather than an error.
	StillQueued []string
	// Cancelled lists uuids of main-thread commands cancelled by this
	// interrupt. Populated only by [Client.InterruptCancelQueued] on CLIs
	// advertising the "interrupt_cancel_queued_v1" protocol capability (see
	// [ServerCapabilities.Capabilities]); nil for [Client.Interrupt] and on
	// older CLIs, in which case StillQueued reports the same commands
	// instead. Port of TypeScript SDK v0.3.219.
	Cancelled []string
}

// RewindFilesResult is the result of a [Client.RewindFiles] operation.
// Port of TypeScript SDK v0.3.216.
type RewindFilesResult struct {
	// CanRewind reports whether the rewind was (or, for a dry run, would be)
	// possible.
	CanRewind bool `json:"canRewind"`
	// Error contains a human-readable explanation when CanRewind is false.
	Error string `json:"error,omitempty"`
	// FilesChanged lists the tracked file paths that were restored or deleted.
	FilesChanged []string `json:"filesChanged,omitempty"`
	// Insertions is the number of lines inserted across all changed files.
	Insertions int `json:"insertions,omitempty"`
	// Deletions is the number of lines deleted across all changed files.
	Deletions int `json:"deletions,omitempty"`
	// SkippedLinks counts tracked files that were NOT restored or deleted
	// because a symlink, hard link, or other non-regular file was detected at
	// the tracked path, its parent directory no longer resolves to where it
	// pointed when the checkpoint was taken, or its backup could not be
	// safely read. Only populated by a real (non-dry-run) rewind — on a dry
	// run response the field is never set and the preview counts do not
	// reflect link-safety refusals. Absent or 0 on a real rewind means no
	// link-safety refusals occurred; other per-file failures (for example a
	// missing backup file) are logged and reported in telemetry but are not
	// counted here.
	SkippedLinks int `json:"skippedLinks,omitempty"`
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
	// SupportsFastMode is true when the current model supports fast mode
	// (e.g. Opus fast mode). Port of TypeScript SDK v0.2.69.
	SupportsFastMode bool `json:"supportsFastMode"`
	// FastModeState reports whether fast inference mode is currently active
	// for the session, distinct from SupportsFastMode's static per-model
	// capability bit. Empty on CLIs that predate this field. Port of
	// TypeScript SDK v0.3.219.
	FastModeState FastModeState `json:"fast_mode_state,omitempty"`
	// FastModeDisabledReason explains why FastModeState isn't
	// FastModeStateOn. Empty when fast mode is on or the CLI didn't report a
	// reason. Port of TypeScript SDK v0.3.219.
	FastModeDisabledReason FastModeDisabledReason `json:"fast_mode_disabled_reason,omitempty"`
	// MemoryPaths is the list of memory file paths loaded at session initialization.
	// Empty when no memory files are configured.
	MemoryPaths []string `json:"memoryPaths,omitempty"`
	// Capabilities lists protocol capabilities the CLI supports, letting
	// callers feature-detect instead of version-sniffing. This is an open
	// set: ignore unknown values and check only for the specific values you
	// use. Known values include "interrupt_receipt_v1" (the interrupt
	// control response's success payload carries a still_queued list of
	// uuids) and "interrupt_cancel_queued_v1" ([Client.InterruptCancelQueued]
	// is honored; cancelled commands are listed under the response's
	// cancelled field). Empty on CLIs that predate this field. Port of
	// TypeScript SDK v0.3.205 / v0.3.219.
	Capabilities []string `json:"capabilities,omitempty"`
}

// SessionMessage represents a user or assistant message from a session transcript.
//
// For subagent transcripts (returned by [GetSubagentMessages] and
// [GetSubagentMessagesFromStore]), ParentToolUseID and ParentAgentID are
// populated from the subagent's metadata sidecar (the on-disk ".meta.json"
// file, or the synthetic "agent_metadata" entry in a [SessionStore]) and
// applied uniformly to every message in that subagent's transcript:
// ParentToolUseID identifies the tool call that spawned the subagent, and
// ParentAgentID identifies its immediate parent agent, which lets callers
// reconstruct depth-2+ agent trees when a subagent itself spawned further
// nested subagents. Both are empty for main-session messages.
type SessionMessage struct {
	Type            string `json:"type"` // "user" or "assistant"
	UUID            string `json:"uuid"`
	SessionID       string `json:"session_id"`
	Message         any    `json:"message"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
	ParentAgentID   string `json:"parent_agent_id,omitempty"`
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

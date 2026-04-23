package claude

// PermissionMode controls tool execution permissions.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
)

// SdkBeta represents beta feature flags.
type SdkBeta string

const (
	SdkBetaContext1M SdkBeta = "context-1m-2025-08-07"
)

// SettingSource indicates where a setting comes from.
type SettingSource string

const (
	SettingSourceUser    SettingSource = "user"
	SettingSourceProject SettingSource = "project"
	SettingSourceLocal   SettingSource = "local"
)

// Effort controls thinking depth.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
	EffortMax    Effort = "max"
)

// SystemPrompt is the interface for system prompt configuration.
type SystemPrompt interface {
	systemPromptMarker()
}

// StringPrompt is a plain string system prompt.
type StringPrompt string

func (StringPrompt) systemPromptMarker() {}

// PresetPrompt is a preset system prompt (e.g. "claude_code") with optional appended text.
type PresetPrompt struct {
	Preset                 string `json:"preset"` // e.g. "claude_code"
	Append                 string `json:"append,omitempty"`
	ExcludeDynamicSections bool   `json:"excludeDynamicSections,omitempty"`
}

func (PresetPrompt) systemPromptMarker() {}

// ToolsPreset represents a tools preset configuration.
type ToolsPreset struct {
	Preset string `json:"preset"` // e.g. "claude_code"
}

// AgentDefinition is an agent definition configuration.
type AgentDefinition struct {
	Description     string   `json:"description"`
	Prompt          string   `json:"prompt"`
	Tools           []string `json:"tools,omitempty"`
	Model           string   `json:"model,omitempty"` // "sonnet", "opus", "haiku", "inherit"
	Skills          []string `json:"skills,omitempty"`
	Memory          string   `json:"memory,omitempty"` // "user" | "project" | "local"
	MCPServers      []any    `json:"mcpServers,omitempty"`
	Background      bool     `json:"background,omitempty"`
	Effort          string   `json:"effort,omitempty"`
	PermissionMode  string   `json:"permissionMode,omitempty"`
	DisallowedTools []string `json:"disallowedTools,omitempty"`
	MaxTurns        *int     `json:"maxTurns,omitempty"`
	InitialPrompt   string   `json:"initialPrompt,omitempty"`
}

// ThinkingConfig is the interface for thinking configuration.
type ThinkingConfig interface {
	thinkingConfigMarker()
}

// ThinkingConfigAdaptive enables adaptive thinking.
type ThinkingConfigAdaptive struct{}

func (ThinkingConfigAdaptive) thinkingConfigMarker() {}

// ThinkingConfigEnabled enables thinking with a specific budget.
type ThinkingConfigEnabled struct {
	BudgetTokens int `json:"budget_tokens"`
}

func (ThinkingConfigEnabled) thinkingConfigMarker() {}

// ThinkingConfigDisabled disables thinking.
type ThinkingConfigDisabled struct{}

func (ThinkingConfigDisabled) thinkingConfigMarker() {}

// SdkPluginConfig is an SDK plugin configuration.
type SdkPluginConfig struct {
	Type string `json:"type"` // "local"
	Path string `json:"path"`
}

// SandboxNetworkConfig contains network configuration for sandbox.
type SandboxNetworkConfig struct {
	AllowUnixSockets    []string `json:"allowUnixSockets,omitempty"`
	AllowAllUnixSockets *bool    `json:"allowAllUnixSockets,omitempty"`
	AllowLocalBinding   *bool    `json:"allowLocalBinding,omitempty"`
	HTTPProxyPort       *int     `json:"httpProxyPort,omitempty"`
	SOCKSProxyPort      *int     `json:"socksProxyPort,omitempty"`
}

// SandboxIgnoreViolations specifies violations to ignore in sandbox.
type SandboxIgnoreViolations struct {
	File    []string `json:"file,omitempty"`
	Network []string `json:"network,omitempty"`
}

// SandboxSettings controls bash command sandboxing.
type SandboxSettings struct {
	Enabled                    *bool                    `json:"enabled,omitempty"`
	AutoAllowBashIfSandboxed   *bool                    `json:"autoAllowBashIfSandboxed,omitempty"`
	ExcludedCommands           []string                 `json:"excludedCommands,omitempty"`
	AllowUnsandboxedCommands   *bool                    `json:"allowUnsandboxedCommands,omitempty"`
	Network                    *SandboxNetworkConfig    `json:"network,omitempty"`
	IgnoreViolations           *SandboxIgnoreViolations `json:"ignoreViolations,omitempty"`
	EnableWeakerNestedSandbox  *bool                    `json:"enableWeakerNestedSandbox,omitempty"`
}

// Options configures a Claude SDK query or client.
type Options struct {
	// Tools is the base set of tools. Use []string for explicit list or *ToolsPreset for preset.
	Tools any // []string | *ToolsPreset | nil
	// AllowedTools is a permission allowlist that auto-approves the listed tools
	// without invoking the CanUseTool callback. Tools not in this list fall through
	// to PermissionMode + CanUseTool evaluation. This is NOT an availability filter —
	// it does not restrict which tools are available, only which are pre-approved.
	AllowedTools []string
	// SystemPrompt configures the system prompt. Use StringPrompt or PresetPrompt.
	SystemPrompt SystemPrompt
	// SystemPromptFile is a path to a file containing the system prompt.
	// Mutually exclusive with SystemPrompt.
	SystemPromptFile string
	// McpServers maps server names to their config. Use map[string]McpServerConfig or a string/path.
	McpServers any // map[string]McpServerConfig | string | nil
	// PermissionMode controls tool execution permissions. Used as the fallback
	// for tools not matched by AllowedTools or DisallowedTools.
	PermissionMode PermissionMode
	// ContinueConversation continues the most recent conversation.
	ContinueConversation bool
	// Resume resumes a specific session by ID.
	Resume string
	// SessionID specifies a custom session ID for the conversation.
	SessionID string
	// Title sets the session title and skips auto-generation.
	Title string
	// MaxTurns limits the number of conversation turns.
	MaxTurns *int
	// MaxBudgetUSD limits the total cost.
	MaxBudgetUSD *float64
	// TaskBudget sets a token budget per task.
	TaskBudget *int
	// DisallowedTools lists tools to explicitly deny. Takes precedence over
	// AllowedTools — a tool in both lists will be denied.
	DisallowedTools []string
	// Model specifies the AI model to use.
	Model string
	// FallbackModel specifies a fallback model.
	FallbackModel string
	// Betas enables beta features.
	Betas []SdkBeta
	// PermissionPromptToolName sets the permission prompt tool name.
	PermissionPromptToolName string
	// Cwd sets the working directory for the CLI process.
	Cwd string
	// CLIPath overrides the path to the Claude CLI binary.
	CLIPath string
	// Settings is a JSON string or file path for settings.
	Settings string
	// ManagedSettings is a JSON string of policy-tier settings forwarded to the
	// spawned CLI in-memory. Honored below IT-controlled managed sources.
	ManagedSettings string
	// AddDirs adds additional directories.
	AddDirs []string
	// Env sets additional environment variables for the CLI process.
	Env map[string]string
	// ExtraArgs passes arbitrary CLI flags. Keys are flag names, values are flag values (empty string for boolean flags).
	ExtraArgs map[string]string
	// MaxBufferSize sets the maximum bytes when buffering CLI stdout.
	MaxBufferSize *int
	// Stderr is a callback for stderr output from the CLI.
	Stderr func(string)
	// CanUseTool is a callback invoked for tool permission decisions when a tool
	// is not matched by AllowedTools or DisallowedTools.
	CanUseTool CanUseToolFunc
	// Hooks configures hook callbacks.
	Hooks map[HookEvent][]HookMatcher
	// User sets the user for the CLI process.
	User string
	// IncludePartialMessages enables partial message streaming.
	IncludePartialMessages bool
	// IncludeHookEvents enables hook lifecycle system messages
	// (hook_started, hook_progress, hook_response) for all hook event types.
	IncludeHookEvents bool
	// AgentProgressSummaries enables periodic AI-generated progress summaries
	// on task_progress messages. When set, task progress messages carry a
	// Summary field describing the subagent's current activity.
	AgentProgressSummaries bool
	// ForkSession forks resumed sessions to a new session ID.
	ForkSession bool
	// Agents defines custom agent configurations.
	Agents map[string]AgentDefinition
	// Skills enables skills on the main session without manually configuring
	// AllowedTools and SettingSources. Use []string for named skills,
	// the string "all" for every discovered skill, or leave nil to disable.
	// When set, the SDK automatically injects Skill tool entries into
	// AllowedTools and defaults SettingSources to [user, project] if unset.
	Skills any // []string | "all" | nil
	// SettingSources specifies which setting sources to load.
	SettingSources []SettingSource
	// Sandbox configures bash command isolation.
	Sandbox *SandboxSettings
	// Plugins configures custom plugins.
	Plugins []SdkPluginConfig
	// MaxThinkingTokens limits thinking block tokens. Deprecated: use Thinking instead.
	MaxThinkingTokens *int
	// Thinking controls extended thinking behavior. Takes precedence over MaxThinkingTokens.
	Thinking ThinkingConfig
	// Effort controls thinking depth.
	Effort Effort
	// OutputFormat configures structured output format.
	OutputFormat map[string]any
	// EnableFileCheckpointing enables file change tracking for rewind support.
	EnableFileCheckpointing bool
}

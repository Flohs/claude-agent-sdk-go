package claude

import (
	"fmt"
	"os"
	"strings"
)

// Model string constants for use with Options.Model.
// Callers may also pass any valid model identifier string directly.
const (
	// ModelFable5 is the full identifier for Claude Fable 5.
	ModelFable5 = "claude-fable-5"
	// ModelFable is the short alias for claude-fable-5.
	ModelFable = "fable"
	// ModelOpus48 is the full identifier for Claude Opus 4.8.
	ModelOpus48 = "claude-opus-4-8"
	// ModelOpus is the short alias for Opus.
	ModelOpus = "opus"
	// ModelSonnet46 is the full identifier for Claude Sonnet 4.6.
	ModelSonnet46 = "claude-sonnet-4-6"
	// ModelSonnet is the short alias for Sonnet.
	ModelSonnet = "sonnet"
	// ModelHaiku45 is the full identifier for Claude Haiku 4.5.
	ModelHaiku45 = "claude-haiku-4-5-20251001"
	// ModelHaiku is the short alias for Haiku.
	ModelHaiku = "haiku"
)

// PermissionMode controls tool execution permissions.
type PermissionMode string

const (
	PermissionModeDefault           PermissionMode = "default"
	PermissionModeAcceptEdits       PermissionMode = "acceptEdits"
	PermissionModePlan              PermissionMode = "plan"
	PermissionModeBypassPermissions PermissionMode = "bypassPermissions"
	PermissionModeDontAsk           PermissionMode = "dontAsk"
	PermissionModeAuto              PermissionMode = "auto"
	// PermissionModeManual is an accepted alias for PermissionModeDefault.
	PermissionModeManual PermissionMode = "manual"
)

// validPermissionModes enumerates every value validatePermissionMode accepts,
// used both for the membership check and to render the error message's list
// of valid values.
var validPermissionModes = []PermissionMode{
	PermissionModeDefault,
	PermissionModeAcceptEdits,
	PermissionModePlan,
	PermissionModeBypassPermissions,
	PermissionModeDontAsk,
	PermissionModeAuto,
	PermissionModeManual,
}

// validatePermissionMode rejects a PermissionMode value that is neither the
// empty string (unset — the CLI's own default applies) nor one of the known
// PermissionMode* constants. Callers should validate before forwarding the
// mode to the CLI subprocess (as a --permission-mode flag) or a live
// set_permission_mode control request, so a typo surfaces as an immediate,
// actionable Go error instead of an opaque CLI failure.
func validatePermissionMode(mode PermissionMode) error {
	if mode == "" {
		return nil
	}
	for _, valid := range validPermissionModes {
		if mode == valid {
			return nil
		}
	}
	valid := make([]string, len(validPermissionModes))
	for i, v := range validPermissionModes {
		valid[i] = string(v)
	}
	return &SDKError{Message: fmt.Sprintf(
		"invalid PermissionMode %q: must be one of %s",
		string(mode), strings.Join(valid, ", "),
	)}
}

// SdkBeta represents beta feature flags.
type SdkBeta string

const (
	SdkBetaContext1M SdkBeta = "context-1m-2025-08-07"
)

// SessionStoreFlushMode controls how the transcript mirror batcher delivers
// frames to a [SessionStore].
type SessionStoreFlushMode string

const (
	// SessionStoreFlushModeBatched is the default: frames are flushed at
	// turn boundaries (before each result message) and when internal thresholds
	// are reached ([MirrorMaxPendingEntries] / [MirrorMaxPendingBytes]).
	SessionStoreFlushModeBatched SessionStoreFlushMode = "batched"
	// SessionStoreFlushModeEager delivers every frame to the store in
	// near-real-time: each [transcriptMirrorBatcher.Enqueue] call triggers an
	// immediate background flush. Suitable for live-tail UIs, cross-process
	// resume, and crash-durability use cases.
	SessionStoreFlushModeEager SessionStoreFlushMode = "eager"
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
	EffortXHigh  Effort = "xhigh"
)

// EffortLevel is an alias for [Effort] for compatibility with the Python SDK
// naming convention. Both names resolve to the same underlying type and are
// interchangeable in all contexts.
type EffortLevel = Effort

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

// ContentBlocksPrompt is a system prompt expressed as an array of content blocks,
// matching the Anthropic API's list-form system parameter.
// Each block is a map with at minimum {"type": "text", "text": "..."}.
type ContentBlocksPrompt []map[string]any

func (ContentBlocksPrompt) systemPromptMarker() {}

// ToolsPreset represents a tools preset configuration.
type ToolsPreset struct {
	Preset string `json:"preset"` // e.g. "claude_code"
}

// AdvisorToolConfig configures the advisor tool for an AgentDefinition.
// It mirrors the API's BetaAdvisorTool20260301Param shape.
type AdvisorToolConfig struct {
	// Model is the advisor model to consult (e.g. "claude-opus-4-5").
	Model string `json:"model,omitempty"`
	// MaxUses limits how many times the advisor may be consulted per turn.
	MaxUses *int `json:"max_uses,omitempty"`
	// Caching, when true, enables prompt caching for advisor calls.
	Caching *bool `json:"caching,omitempty"`
	// AllowedCallers restricts which agent IDs may invoke this advisor.
	AllowedCallers []string `json:"allowed_callers,omitempty"`
}

// AskUserQuestionToolConfig configures the AskUserQuestion built-in tool.
// Port of TypeScript SDK v0.2.69.
type AskUserQuestionToolConfig struct {
	// PreviewFormat sets the content format used for the preview field.
	// Typical values: "markdown", "text".
	PreviewFormat string `json:"previewFormat,omitempty"`
}

// BuiltinToolConfig configures individual built-in tools. Pass a pointer to
// Options.ToolConfig to enable per-tool configuration.
// Port of TypeScript SDK v0.2.69.
type BuiltinToolConfig struct {
	// AskUserQuestion configures the AskUserQuestion tool.
	AskUserQuestion *AskUserQuestionToolConfig `json:"askUserQuestion,omitempty"`
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
	// Advisor enables the advisor tool for this agent definition.
	// Set to true to enable with default config, or provide an *AdvisorToolConfig
	// for fine-grained control. When nil (default), the field is omitted.
	Advisor any `json:"advisor,omitempty"`
}

// ThinkingConfig is the interface for thinking configuration.
type ThinkingConfig interface {
	thinkingConfigMarker()
}

// ThinkingDisplay controls whether thinking text is shown. Opus 4.7 defaults
// to "omitted"; callers can opt in to "summarized" to receive summarized
// thinking text.
type ThinkingDisplay string

const (
	ThinkingDisplaySummarized ThinkingDisplay = "summarized"
	ThinkingDisplayOmitted    ThinkingDisplay = "omitted"
)

// ThinkingConfigAdaptive enables adaptive thinking.
type ThinkingConfigAdaptive struct {
	// Display overrides the default thinking display behavior.
	Display ThinkingDisplay `json:"display,omitempty"`
}

func (ThinkingConfigAdaptive) thinkingConfigMarker() {}

// ThinkingConfigEnabled enables thinking with a specific budget.
type ThinkingConfigEnabled struct {
	BudgetTokens int `json:"budget_tokens"`
	// Display overrides the default thinking display behavior.
	Display ThinkingDisplay `json:"display,omitempty"`
}

func (ThinkingConfigEnabled) thinkingConfigMarker() {}

// ThinkingConfigDisabled disables thinking.
type ThinkingConfigDisabled struct{}

func (ThinkingConfigDisabled) thinkingConfigMarker() {}

// SdkPluginConfig is an SDK plugin configuration.
type SdkPluginConfig struct {
	Type string `json:"type"` // "local"
	Path string `json:"path"`
	// SkipMcpDiscovery, when true, prevents the CLI from re-reading the
	// plugin's .mcp.json during plugin load. Use this when the SDK host
	// manages the plugin's MCP server connections independently. Port of
	// TypeScript SDK v0.3.172.
	SkipMcpDiscovery bool `json:"skipMcpDiscovery,omitempty"`
}

// SandboxNetworkConfig contains network configuration for sandbox.
type SandboxNetworkConfig struct {
	AllowUnixSockets    []string `json:"allowUnixSockets,omitempty"`
	AllowAllUnixSockets *bool    `json:"allowAllUnixSockets,omitempty"`
	AllowLocalBinding   *bool    `json:"allowLocalBinding,omitempty"`
	HTTPProxyPort       *int     `json:"httpProxyPort,omitempty"`
	SOCKSProxyPort      *int     `json:"socksProxyPort,omitempty"`
	// AllowedDomains restricts outbound connections to the listed hostnames.
	AllowedDomains []string `json:"allowedDomains,omitempty"`
	// DeniedDomains blocks outbound connections to the listed hostnames.
	DeniedDomains []string `json:"deniedDomains,omitempty"`
	// AllowManagedDomainsOnly restricts traffic to organization-managed domains.
	AllowManagedDomainsOnly *bool `json:"allowManagedDomainsOnly,omitempty"`
	// AllowMachLookup permits mach port lookup (macOS only).
	AllowMachLookup *bool `json:"allowMachLookup,omitempty"`
}

// SandboxIgnoreViolations specifies violations to ignore in sandbox.
type SandboxIgnoreViolations struct {
	File    []string `json:"file,omitempty"`
	Network []string `json:"network,omitempty"`
}

// SandboxFilesystemConfig configures filesystem access restrictions for sandboxed commands.
type SandboxFilesystemConfig struct {
	// AllowWrite lists additional paths that sandboxed commands may write to.
	AllowWrite []string `json:"allowWrite,omitempty"`
	// DenyWrite lists paths that sandboxed commands may not write to.
	DenyWrite []string `json:"denyWrite,omitempty"`
	// DenyRead lists paths that sandboxed commands may not read from.
	DenyRead []string `json:"denyRead,omitempty"`
	// AllowRead re-allows reading specific paths within DenyRead regions.
	AllowRead []string `json:"allowRead,omitempty"`
	// AllowManagedReadPathsOnly, when true, restricts reads to AllowRead entries only.
	AllowManagedReadPathsOnly *bool `json:"allowManagedReadPathsOnly,omitempty"`
}

// SandboxCredentialFileEntry declares a single file or directory path whose reads
// are denied inside sandboxed commands. Mode is always "deny".
type SandboxCredentialFileEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
}

// SandboxCredentialEnvVarEntry declares a single environment variable to protect
// inside sandboxed commands. Mode "deny" unsets the variable for sandboxed
// commands. Mode "mask" shows sandboxed commands a sentinel value instead,
// and the host proxy swaps sentinel for the real value on egress to
// InjectHosts. InjectHosts is only meaningful when Mode is "mask"; it is
// accepted but ignored for "deny". If InjectHosts is unset, the credential
// is injected at every host reachable via the sandbox's allowed network
// domains. Port of TypeScript SDK v0.3.199.
type SandboxCredentialEnvVarEntry struct {
	Name        string   `json:"name"`
	Mode        string   `json:"mode"`
	InjectHosts []string `json:"injectHosts,omitempty"`
}

// SandboxCredentialsConfig declares credential sources to protect in sandboxed commands.
// Files listed in Files are denied for reads (Mode is always "deny" for file
// entries). Variables in EnvVars are unset ("deny") or masked with a
// sentinel value ("mask"); see SandboxCredentialEnvVarEntry. Only explicitly
// listed entries are restricted — there is no built-in deny list.
type SandboxCredentialsConfig struct {
	Files   []SandboxCredentialFileEntry   `json:"files,omitempty"`
	EnvVars []SandboxCredentialEnvVarEntry `json:"envVars,omitempty"`
	// AllowPlaintextInject allows sentinel-to-real substitution on the
	// plain-HTTP proxy path for "mask" mode entries. Defaults to false:
	// without TLS termination the upstream identity is unverified and the
	// credential travels in cleartext. Set only for trusted-network test
	// fixtures. Only honored from user, managed/policy, or CLI settings —
	// project settings are ignored. Port of TypeScript SDK v0.3.199.
	AllowPlaintextInject *bool `json:"allowPlaintextInject,omitempty"`
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
	// FailIfUnavailable controls behavior when sandboxing is requested but the
	// platform's sandbox mechanism is unavailable (no bwrap on Linux, no
	// Seatbelt on macOS, etc). When true (the CLI default when Enabled is
	// true), the CLI emits an error result message instead of silently running
	// commands unsandboxed.
	FailIfUnavailable *bool `json:"failIfUnavailable,omitempty"`
	// Filesystem configures filesystem access restrictions for sandboxed commands.
	Filesystem *SandboxFilesystemConfig `json:"filesystem,omitempty"`
	// EnableWeakerNetworkIsolation allows system TLS access on macOS when
	// network isolation is enabled. Has no effect on Linux.
	EnableWeakerNetworkIsolation *bool `json:"enableWeakerNetworkIsolation,omitempty"`
	// Credentials configures credential sources to protect in sandboxed commands.
	// File paths are denied for reads; environment variables are unset.
	// Port of TypeScript SDK v0.3.187.
	Credentials *SandboxCredentialsConfig `json:"credentials,omitempty"`
}

// Options configures a Claude SDK query or client.
type Options struct {
	// Tools is the base set of tools. Use []string for explicit list or *ToolsPreset for preset.
	Tools any // []string | *ToolsPreset | nil
	// AllowedTools is a permission allowlist that auto-approves the listed tools
	// without invoking the CanUseTool callback. Tools not in this list fall through
	// to PermissionMode + CanUseTool evaluation. This is NOT an availability filter —
	// it does not restrict which tools are available, only which are pre-approved.
	//
	// Deprecated: adding "Skill" to AllowedTools directly is deprecated. Use
	// Options.Skills instead, which automatically injects the appropriate
	// AllowedTools entries and defaults SettingSources.
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
	// Env sets additional environment variables for the CLI subprocess. Entries
	// are merged with the inherited process environment using a four-layer order:
	//   1. SDK defaults (e.g. CLAUDE_CODE_ENTRYPOINT=sdk-go)
	//   2. Inherited os.Environ() (CLAUDECODE is stripped to prevent nested-SDK interference)
	//   3. Entries from Env (these override step 2)
	//   4. SDK-controlled vars appended last (CLAUDE_AGENT_SDK_VERSION is always set
	//      by the SDK and cannot be overridden via Env)
	//
	// Note: unlike the TypeScript SDK (where options.env replaces the subprocess
	// environment), the Go SDK merges Env on top of the inherited environment.
	Env map[string]string
	// InheritEnv controls whether the CLI subprocess inherits the parent process
	// environment. Defaults to true when nil. When set to false, the subprocess
	// starts with a clean environment containing only SDK-required variables
	// (ANTHROPIC_API_KEY, CLAUDE_CODE_ENTRYPOINT, CLAUDE_AGENT_SDK_VERSION)
	// plus any entries from Options.Env.
	InheritEnv *bool
	// Debug enables verbose debug logging from the CLI subprocess.
	Debug bool
	// DebugFile writes debug output to the specified file path instead of stderr.
	// Has no effect when Debug is false.
	DebugFile string
	// ExtraArgs passes arbitrary CLI flags. Keys are flag names, values are flag values (empty string for boolean flags).
	ExtraArgs map[string]string
	// MaxBufferSize sets the maximum bytes when buffering CLI stdout.
	MaxBufferSize *int
	// Stderr is a callback for stderr output from the CLI.
	Stderr func(string)
	// CanUseTool is invoked when the CLI's permission decision is "ask" for a
	// tool call, replacing the interactive permission prompt. It is NOT called for
	// tools already permitted by AllowedTools, PermissionMode, or
	// permissions.allow rules in settings — use a PreToolUse hook for universal
	// interception regardless of permission state. Port of Python SDK PR
	// anthropics/claude-agent-sdk-python#912.
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
	// StrictMcpConfig, when true, tells the CLI to only use MCP servers passed
	// via McpServers, ignoring project, user, and global MCP configurations.
	// Enables fully deterministic server sets for reproducible deployments.
	StrictMcpConfig bool
	// ForwardSubagentText, when true, streams subagent text deltas to SDK consumers.
	ForwardSubagentText bool
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
	// ToolConfig configures individual built-in tools. Currently supports
	// AskUserQuestion.PreviewFormat to control the content format of the
	// preview field. Port of TypeScript SDK v0.2.69.
	ToolConfig *BuiltinToolConfig
	// EnableFileCheckpointing enables file change tracking for rewind support.
	EnableFileCheckpointing bool
	// TraceParent is the W3C `traceparent` header value to propagate to the
	// CLI subprocess (forwarded as the `TRACEPARENT` env var). Callers using
	// OpenTelemetry can obtain it via `propagation.TraceContext{}` or format
	// the span context manually. When empty, the parent process environment
	// is inherited unchanged — so an externally-set `TRACEPARENT` env var
	// still reaches the subprocess.
	TraceParent string
	// TraceState is the W3C `tracestate` header value paired with TraceParent.
	// Set together with TraceParent when forwarding a specific span.
	TraceState string
	// SessionStore, when non-nil, enables active mirroring: every transcript
	// line the CLI writes to disk is parallel-copied to the store. Terminal
	// mirror failures surface on the normal message channel as
	// *MirrorErrorMessage. Mutually exclusive with EnableFileCheckpointing —
	// see [validateSessionStoreOptions]. Forwarded to the CLI as
	// `--session-mirror`.
	SessionStore SessionStore
	// LoadTimeoutMs bounds [SessionStore.Load] during resume (Sub-C) and the
	// flush-before-result wait used by the transcript mirror batcher. Zero
	// means the internal default (10s) is used.
	LoadTimeoutMs int
	// SessionStoreFlush controls how the transcript mirror batcher delivers
	// frames to the [SessionStore]. Defaults to [SessionStoreFlushModeBatched]
	// when empty. Set to [SessionStoreFlushModeEager] for near-real-time
	// delivery. Has no effect when SessionStore is nil.
	SessionStoreFlush SessionStoreFlushMode
	// FastMode enables fast inference mode for supported models (e.g. Opus).
	// When true, the SDK merges {"isFast": true} into the managed-settings
	// layer so that settingSources cannot reset the flag between turns.
	// Port of TypeScript SDK v0.3.191.
	FastMode bool
}

// warnCanUseToolPermissionConflicts emits a non-fatal stderr warning when
// CanUseTool is configured alongside AllowedTools or
// PermissionModeBypassPermissions, since either can silently short-circuit
// the CanUseTool callback for matching tools. Port of TypeScript SDK v0.3.198.
func warnCanUseToolPermissionConflicts(opts *Options) {
	if opts.CanUseTool == nil {
		return
	}
	if len(opts.AllowedTools) > 0 {
		fmt.Fprintln(os.Stderr,
			"Warning: CanUseTool is configured alongside AllowedTools. "+
				"Tools matched by AllowedTools are auto-approved without invoking CanUseTool.")
	}
	if opts.PermissionMode == PermissionModeBypassPermissions {
		fmt.Fprintln(os.Stderr,
			"Warning: CanUseTool is configured alongside PermissionMode "+
				"PermissionModeBypassPermissions. Tool calls will bypass CanUseTool entirely.")
	}
}

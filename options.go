package claude

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
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

// validateSkills rejects an Options.Skills value that isn't nil, "all", or
// []string, and validates every name in a []string. Names are formatted by
// applySkillsDefaults into "Skill(name)" rules joined into the CLI's
// --allowedTools argument, which is tokenized on commas and spaces outside
// parentheses with no escape sequences — a name carrying one of those
// delimiters cannot be passed through reliably. Names that tokenize cleanly
// but can never match the listed skill (surrounding whitespace, a leading
// "/", a wildcard suffix) are rejected too, so a dead rule fails loudly here
// instead of silently granting nothing. Port of Python SDK PR
// anthropics/claude-agent-sdk-python#1145, mirrored in TypeScript SDK
// v0.3.221. ([#557](https://github.com/Flohs/claude-agent-sdk-go/issues/557))
func validateSkills(skills any) error {
	if skills == nil {
		return nil
	}
	switch s := skills.(type) {
	case string:
		if s == "all" {
			return nil
		}
		return &SDKError{Message: fmt.Sprintf(
			`Options.Skills must be []string or "all", got %q. Did you mean []string{%q}?`,
			s, s,
		)}
	case []string:
		for _, name := range s {
			if err := validateSkillName(name); err != nil {
				return err
			}
		}
		return nil
	default:
		return &SDKError{Message: fmt.Sprintf(
			`Options.Skills must be []string or "all", got %T`, skills,
		)}
	}
}

// validateSkillName rejects a single skill name unsafe for or unable to
// match through a "Skill(name)" --allowedTools rule. See validateSkills.
func validateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return &SDKError{Message: "skill names must be non-empty strings"}
	}
	if !utf8.ValidString(name) {
		return &SDKError{Message: fmt.Sprintf(
			"invalid skill name %q: not valid UTF-8, which can never match a skill the CLI discovered",
			name,
		)}
	}
	if strings.TrimSpace(name) != name {
		return &SDKError{Message: fmt.Sprintf(
			"invalid skill name %q: leading or trailing whitespace can never match — the Skill tool trims the invoked name",
			name,
		)}
	}
	for _, r := range name {
		if r == '(' || r == ')' || r == ',' || r == '\ufeff' || unicode.IsControl(r) {
			return &SDKError{Message: fmt.Sprintf(
				"invalid skill name %q: parentheses, commas, control characters, and byte-order marks are not allowed. Names match the skill's directory name, or \"plugin:skill\" for plugin-qualified skills",
				name,
			)}
		}
	}
	if name == "*" {
		return &SDKError{Message: `invalid skill name "*": use Skills: "all" to enable every skill`}
	}
	if strings.HasSuffix(name, ":*") || strings.HasSuffix(name, " *") {
		return &SDKError{Message: fmt.Sprintf(
			"invalid skill name %q: wildcard-suffix names are not allowed; list each skill by its exact name",
			name,
		)}
	}
	if strings.HasPrefix(name, "/") {
		return &SDKError{Message: fmt.Sprintf(
			`invalid skill name %q: skill names may not start with "/". The Skills option takes the canonical name, not the slash-command form`,
			name,
		)}
	}
	if strings.Contains(name, `\\`) {
		return &SDKError{Message: fmt.Sprintf(
			"invalid skill name %q: consecutive backslashes are not allowed — the per-rule parser collapses them, so the rule would name a different skill",
			name,
		)}
	}
	if strings.HasSuffix(name, `\`) {
		return &SDKError{Message: fmt.Sprintf(
			"invalid skill name %q: names may not end with an unpaired backslash",
			name,
		)}
	}
	return nil
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
	// Snapshot, when true, records the conversation's system prompt once (in
	// the session transcript) and reuses it verbatim on every later request
	// and resume/continue, instead of rendering it fresh each time.
	// Recommended for stability with extended thinking and the API's prompt
	// cache, since a re-rendered prompt invalidates both. A mid-session
	// change to the model or the system prompt (e.g. a different Append)
	// then has no effect until the next compaction or a new session.
	// Omitted/default only records the bare "claude_code" preset with no
	// Append; passing Append turns recording off unless Snapshot is set.
	Snapshot bool `json:"snapshot,omitempty"`
}

func (PresetPrompt) systemPromptMarker() {}

// CustomPrompt is a custom system prompt with the option to record it in
// the session transcript (Snapshot) for reuse across requests and resume,
// instead of re-rendering it fresh every time — see PresetPrompt.Snapshot.
// A bare StringPrompt is always snapshot:false; use CustomPrompt to opt in.
type CustomPrompt struct {
	Prompt   string
	Snapshot bool
}

func (CustomPrompt) systemPromptMarker() {}

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

// WorkflowSizeGuideline is an advisory size guideline for the dynamic
// workflows Claude writes, set via Options.WorkflowSizeGuideline. Port of
// TypeScript SDK v0.3.219.
type WorkflowSizeGuideline string

const (
	// WorkflowSizeGuidelineUnrestricted sends no size guideline.
	WorkflowSizeGuidelineUnrestricted WorkflowSizeGuideline = "unrestricted"
	// WorkflowSizeGuidelineSmall aims for fewer than 5 agents.
	WorkflowSizeGuidelineSmall WorkflowSizeGuideline = "small"
	// WorkflowSizeGuidelineMedium aims for fewer than 15 agents. This is the
	// CLI's default when no guideline is set.
	WorkflowSizeGuidelineMedium WorkflowSizeGuideline = "medium"
	// WorkflowSizeGuidelineLarge aims for fewer than 50 agents.
	WorkflowSizeGuidelineLarge WorkflowSizeGuideline = "large"
)

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
	// StrictAllowlist, when true, makes the sandbox runtime deterministically
	// deny hosts not in AllowedDomains instead of prompting. Enforced for
	// sandboxed commands only — in-process tools such as WebFetch are not
	// gated by this setting. Only honored from user, managed/policy, or CLI
	// (--settings) settings; project settings (.claude/settings.json and
	// .claude/settings.local.json) are ignored. Port of TypeScript SDK
	// v0.3.219.
	StrictAllowlist *bool `json:"strictAllowlist,omitempty"`
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

// SandboxCredentialFileEntry declares a single file or directory path to
// protect inside sandboxed commands. Mode "deny" blocks reads inside the
// sandbox. Mode "mask" substitutes a sentinel (whole-file, or only the spans
// captured by Extract) and the host proxy swaps sentinel for the real value
// on egress to InjectHosts; on macOS and Windows, "mask" currently degrades
// to "deny".
type SandboxCredentialFileEntry struct {
	Path string `json:"path"`
	Mode string `json:"mode"`
	// Extract is an optional regex for structured masking when Mode is
	// "mask". Applied globally to the file; capture group 1 of each match is
	// a credential value, and only those captured spans are replaced with
	// sentinels — the rest of the file is preserved so a tool that parses it
	// (.netrc, JSON, YAML) still succeeds. Without Extract, the entire file
	// content is replaced with one sentinel (whole-file masking, suited to
	// single-secret files). Accepted but ignored for "deny". Port of
	// TypeScript SDK v0.3.224.
	Extract string `json:"extract,omitempty"`
	// OnExtractNoMatch controls behavior when Extract matches nothing in the
	// file — or, with Decode, when no candidate survives verification.
	// "warn" (default) emits a stderr warning and leaves the file readable
	// as-is (fail-open); "deny" degrades the entry to Mode "deny"
	// (fail-closed); "error" aborts at sandbox setup. Only meaningful when
	// Mode is "mask" and Extract or Decode is set; accepted but ignored
	// otherwise. Port of TypeScript SDK v0.3.224.
	OnExtractNoMatch string `json:"onExtractNoMatch,omitempty"`
	// Decode is an optional encoded-credential format for Mode "mask". "jwt":
	// candidates are located with a built-in JWT regex (or Extract, if set),
	// verified to actually be JWTs, and replaced with a structurally valid
	// fake JWT so client-side token parsing inside the sandbox keeps
	// working. Accepted but ignored for "deny". Port of TypeScript SDK
	// v0.3.224.
	Decode string `json:"decode,omitempty"`
	// MaskClaims names top-level JWT payload claims to mask inside each
	// decoded value, instead of replacing the whole token; all other claims
	// are preserved. Requires Decode. Only meaningful when Mode is "mask";
	// accepted but ignored for "deny". Port of TypeScript SDK v0.3.224.
	MaskClaims []string `json:"maskClaims,omitempty"`
	// MaskDuplicates, when true, also replaces verbatim occurrences of each
	// captured credential value outside the regex-matched spans (e.g. a
	// secret repeated where the regex does not reach). Matches raw
	// substrings, so short or common values may corrupt unrelated content;
	// intended for long, high-entropy secrets. Only meaningful when Mode is
	// "mask" and Extract or Decode is set; accepted but ignored otherwise.
	// Port of TypeScript SDK v0.3.224.
	MaskDuplicates *bool `json:"maskDuplicates,omitempty"`
	// InjectHosts optionally narrows where the proxy substitutes this
	// credential. Only meaningful when Mode is "mask"; accepted but ignored
	// for "deny". If unset, defaults to the sandbox's allowed network
	// domains. Port of TypeScript SDK v0.3.224.
	InjectHosts []string `json:"injectHosts,omitempty"`
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
	// Extract is an optional regex for structured masking when Mode is
	// "mask". Applied globally to the value; capture group 1 of each match
	// is a credential value, and only those captured spans are replaced with
	// sentinels — the rest of the value is preserved so a tool that parses
	// it (e.g. a DATABASE_URL connection string) still succeeds. Without
	// Extract, the entire value is replaced with one sentinel. Cannot be
	// combined with Decode. Accepted but ignored for "deny". Port of
	// TypeScript SDK v0.3.224.
	Extract string `json:"extract,omitempty"`
	// OnExtractNoMatch controls behavior when Extract matches nothing in the
	// value. "warn" (default) lets the variable pass through unmasked
	// (fail-open); "deny" unsets the variable (fail-closed); "error" aborts
	// at sandbox setup. Only meaningful when Mode is "mask" and Extract is
	// set without Decode. Port of TypeScript SDK v0.3.224.
	OnExtractNoMatch string `json:"onExtractNoMatch,omitempty"`
	// Decode is an optional encoded-credential format for Mode "mask". "jwt":
	// the variable's whole value is verified to actually be a JWT and
	// replaced with a structurally valid fake JWT. Cannot be combined with
	// Extract. Accepted but ignored for "deny". Port of TypeScript SDK
	// v0.3.224.
	Decode string `json:"decode,omitempty"`
	// MaskClaims names top-level JWT payload claims to mask inside the
	// decoded value, instead of replacing the whole token. Requires Decode.
	// Only meaningful when Mode is "mask"; accepted but ignored for "deny".
	// Port of TypeScript SDK v0.3.224.
	MaskClaims []string `json:"maskClaims,omitempty"`
}

// SandboxAwsCredentialPair explicitly groups masked env vars into an AWS
// credential pair for SigV4 re-signing, for non-standard variable names. The
// conventional AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN
// trio is paired automatically when masked. Only honored from user,
// managed/policy, or CLI (--settings) settings — project settings are
// ignored. Port of TypeScript SDK v0.3.224.
type SandboxAwsCredentialPair struct {
	AccessKeyIDVar     string `json:"accessKeyIdVar"`
	SecretAccessKeyVar string `json:"secretAccessKeyVar"`
	// SessionTokenVar optionally names the masked env var holding the AWS
	// session token (temporary credentials).
	SessionTokenVar string `json:"sessionTokenVar,omitempty"`
}

// SandboxSigv4Policy sets policies for AWS SigV4 request shapes the sandbox
// proxy cannot re-sign (streaming, presigned, sigv4a) when they reference a
// masked credential pair. Each field is "deny" (default) or "passthrough".
// Only honored from user, managed/policy, or CLI (--settings) settings —
// project settings are ignored. Port of TypeScript SDK v0.3.224.
type SandboxSigv4Policy struct {
	// Streaming policies aws-chunked streaming uploads, whose per-chunk
	// signatures chain off a seed signature that can't be recomputed without
	// rewriting the body.
	Streaming string `json:"streaming,omitempty"`
	// Presigned policies presigned URLs, where the signature lives in the
	// query string itself.
	Presigned string `json:"presigned,omitempty"`
	// Sigv4a policies SigV4A (AWS4-ECDSA-P256-SHA256) asymmetric signatures,
	// which have no shared-key HMAC to recompute.
	Sigv4a string `json:"sigv4a,omitempty"`
}

// SandboxCredentialsConfig declares credential sources to protect in sandboxed commands.
// Files listed in Files and variables listed in EnvVars are unset ("deny")
// or masked with a sentinel value ("mask"); see SandboxCredentialFileEntry
// and SandboxCredentialEnvVarEntry. Only explicitly listed entries are
// restricted — there is no built-in deny list.
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
	// AwsPairs explicitly groups masked env vars into AWS credential pairs
	// for SigV4 re-signing, for non-standard variable names. Port of
	// TypeScript SDK v0.3.224.
	AwsPairs []SandboxAwsCredentialPair `json:"awsPairs,omitempty"`
	// Sigv4 sets policies for AWS SigV4 request shapes the proxy cannot
	// re-sign when they reference a masked credential pair. Port of
	// TypeScript SDK v0.3.224.
	Sigv4 *SandboxSigv4Policy `json:"sigv4,omitempty"`
}

// SandboxSettings controls bash command sandboxing.
type SandboxSettings struct {
	Enabled                   *bool                    `json:"enabled,omitempty"`
	AutoAllowBashIfSandboxed  *bool                    `json:"autoAllowBashIfSandboxed,omitempty"`
	ExcludedCommands          []string                 `json:"excludedCommands,omitempty"`
	AllowUnsandboxedCommands  *bool                    `json:"allowUnsandboxedCommands,omitempty"`
	Network                   *SandboxNetworkConfig    `json:"network,omitempty"`
	IgnoreViolations          *SandboxIgnoreViolations `json:"ignoreViolations,omitempty"`
	EnableWeakerNestedSandbox *bool                    `json:"enableWeakerNestedSandbox,omitempty"`
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
	// ResumeSessionAt truncates a Resume'd session to a specific chain-entry
	// UUID instead of loading the full transcript — only messages up to and
	// including this UUID are resumed. Use together with Resume. Accepts any
	// chain-entry UUID, typically an AssistantMessage's UUID. Print/headless
	// lane only: interactive `--resume` and background-job worker boots
	// ignore this option and load the full chain unmodified. Port of
	// TypeScript SDK v0.3.212. ([#561])
	//
	// [#561]: https://github.com/Flohs/claude-agent-sdk-go/issues/561
	ResumeSessionAt string
	// ResumeDropsTurn declares, together with ResumeSessionAt, the prompt
	// UUID of the turn a truncating resume intends to discard. The CLI
	// validates at fork time that every entry past ResumeSessionAt is
	// attributable to that turn and refuses the resume — an
	// error_during_execution result whose message starts with "Resume
	// rejected by --resume-drops-turn:" — if the discarded range contains
	// anything else (e.g. a queued user message the caller hadn't yet
	// observed). The refusal is deterministic: map it to a rewind-recovery
	// path rather than retrying the same fork request. Print/headless lane
	// only, same as ResumeSessionAt. Port of TypeScript SDK v0.3.223.
	// ([#561])
	ResumeDropsTurn string
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
	// PermissionPrompts controls who answers permission prompts: "host"
	// (default) routes them to this process via CanUseTool or
	// PermissionPromptToolName; "none" means nobody — the permission mode
	// (including auto mode's classifier), rules and hooks still decide, and
	// anything that would otherwise prompt is denied immediately with a
	// message telling Claude the session has no approval surface, without
	// ever invoking CanUseTool. Empty leaves the CLI's default in effect.
	// Port of TypeScript SDK v0.3.259.
	PermissionPrompts string
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
	//
	// The CLI also honors two subagent-limiting env vars (set here, not via a
	// typed Options field, since the TS SDK exposes no corresponding schema
	// change either): CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH (default 1) caps
	// subagent nesting depth, and CLAUDE_CODE_MAX_CONCURRENT_SUBAGENTS (default
	// 20) caps concurrently-running subagents.
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
	// ForwardSubagentText, when true, asks the CLI to forward subagent text and
	// thinking blocks as messages in the stream, not just tool_use/tool_result
	// blocks. By default only tool_use/tool_result blocks from subagents
	// (spawned via the Agent tool) are emitted as AssistantMessage/UserMessage
	// values whose ParentToolUseID is the spawning Agent tool_use id — enough
	// for a progress heartbeat; when true, the subagent's text and thinking
	// blocks are forwarded the same way, so consumers can render the full
	// nested transcript. Matches the TypeScript SDK's forwardSubagentText.
	// Port of Python SDK commit c97420c (anthropics/claude-agent-sdk-python#1206).
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
	// Any other value, and any []string name containing delimiters
	// (parentheses, commas, control characters), a leading "/", a wildcard
	// suffix, surrounding whitespace, or invalid UTF-8, is rejected with an
	// error at connect time.
	Skills any // []string | "all" | nil
	// SettingSources specifies which setting sources to load.
	SettingSources []SettingSource
	// Sandbox configures bash command isolation.
	Sandbox *SandboxSettings
	// WorkflowSizeGuideline sets an advisory size guideline for the dynamic
	// workflows Claude writes: WorkflowSizeGuidelineSmall aims for fewer than
	// 5 agents, WorkflowSizeGuidelineMedium (the CLI default) fewer than 15,
	// WorkflowSizeGuidelineLarge fewer than 50, and WorkflowSizeGuidelineUnrestricted
	// sends no guideline. This is a guideline, not an enforced limit. Empty
	// leaves the CLI/managed-settings default in place. Port of TypeScript
	// SDK v0.3.219.
	WorkflowSizeGuideline WorkflowSizeGuideline
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

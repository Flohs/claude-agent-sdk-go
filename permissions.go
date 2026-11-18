package claude

import (
	"context"
	"fmt"
)

// DenialReason is the machine-readable reason why a tool call was denied.
type DenialReason string

const (
	// DenialReasonSafetyCheck means the tool call was blocked by a safety check.
	DenialReasonSafetyCheck DenialReason = "safetyCheck"
	// DenialReasonAsyncAgent means the tool call was blocked because it was
	// made by an async agent that lacks permission to use the tool.
	DenialReasonAsyncAgent DenialReason = "asyncAgent"
)

// PermissionBehavior represents the behavior of a permission rule.
type PermissionBehavior string

const (
	PermissionBehaviorAllow PermissionBehavior = "allow"
	PermissionBehaviorDeny  PermissionBehavior = "deny"
	PermissionBehaviorAsk   PermissionBehavior = "ask"
)

// PermissionUpdateDestination indicates where to store permission updates.
type PermissionUpdateDestination string

const (
	PermissionUpdateDestUserSettings    PermissionUpdateDestination = "userSettings"
	PermissionUpdateDestProjectSettings PermissionUpdateDestination = "projectSettings"
	PermissionUpdateDestLocalSettings   PermissionUpdateDestination = "localSettings"
	PermissionUpdateDestSession         PermissionUpdateDestination = "session"
)

// PermissionRuleValue represents a permission rule.
type PermissionRuleValue struct {
	ToolName string `json:"toolName"`
	// RuleContent is what the rule matches within the tool, e.g. "npm test"
	// for Bash. Empty makes a rule that covers every use of the tool.
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionUpdateType represents the type of permission update.
type PermissionUpdateType string

const (
	PermissionUpdateAddRules          PermissionUpdateType = "addRules"
	PermissionUpdateReplaceRules      PermissionUpdateType = "replaceRules"
	PermissionUpdateRemoveRules       PermissionUpdateType = "removeRules"
	PermissionUpdateSetMode           PermissionUpdateType = "setMode"
	PermissionUpdateAddDirectories    PermissionUpdateType = "addDirectories"
	PermissionUpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdate represents a permission update configuration.
type PermissionUpdate struct {
	Type        PermissionUpdateType  `json:"type"`
	Rules       []PermissionRuleValue `json:"rules,omitempty"`
	Behavior    PermissionBehavior    `json:"behavior,omitempty"`
	Mode        PermissionMode        `json:"mode,omitempty"`
	Directories []string              `json:"directories,omitempty"`
	// Destination is where the update applies and is kept:
	// PermissionUpdateDestSession (in memory until the session ends),
	// PermissionUpdateDestLocalSettings, PermissionUpdateDestProjectSettings
	// or PermissionUpdateDestUserSettings. Empty is sent as "session".
	Destination PermissionUpdateDestination `json:"destination,omitempty"`
}

// Validate reports whether the update can be sent to Claude Code. Claude
// Code rejects an update that lacks a field its type needs, and then ignores
// every update in the same response, so Validate returns an error when:
//
//   - Rules is nil or Behavior is empty for PermissionUpdateAddRules,
//     PermissionUpdateReplaceRules and PermissionUpdateRemoveRules (an empty,
//     non-nil Rules is allowed);
//   - Mode is empty for PermissionUpdateSetMode;
//   - Directories is nil for PermissionUpdateAddDirectories and
//     PermissionUpdateRemoveDirectories;
//   - Type is none of those six.
//
// Port of Python SDK v0.2.184.
func (p *PermissionUpdate) Validate() error {
	switch p.Type {
	case PermissionUpdateAddRules, PermissionUpdateReplaceRules, PermissionUpdateRemoveRules:
		if p.Rules == nil {
			return fmt.Errorf("PermissionUpdate(Type=%q) has no Rules; set Rules to a slice of PermissionRuleValue", p.Type)
		}
		if p.Behavior == "" {
			return fmt.Errorf("PermissionUpdate(Type=%q) has no Behavior; set Behavior to allow, deny or ask", p.Type)
		}
	case PermissionUpdateSetMode:
		if p.Mode == "" {
			return fmt.Errorf("PermissionUpdate(Type=%q) has no Mode; set Mode to a permission mode such as acceptEdits", p.Type)
		}
	case PermissionUpdateAddDirectories, PermissionUpdateRemoveDirectories:
		if p.Directories == nil {
			return fmt.Errorf("PermissionUpdate(Type=%q) has no Directories; set Directories to a slice of paths", p.Type)
		}
	default:
		return fmt.Errorf("PermissionUpdate has Type %q, which this version of the SDK cannot send; use one of "+
			"addRules, replaceRules, removeRules, setMode, addDirectories or removeDirectories, "+
			"or upgrade the SDK if this update came from Claude Code", p.Type)
	}
	return nil
}

// ToDict converts a PermissionUpdate to a map matching the control protocol.
// Destination is sent as "session" when empty, and a rule's ruleContent is
// left out when RuleContent is empty, which is how Claude Code expects a rule
// for the whole tool (Python SDK v0.2.184). ToDict does not check the update;
// call [PermissionUpdate.Validate] for that. Fields the type does not use are
// left out.
func (p *PermissionUpdate) ToDict() map[string]any {
	dest := p.Destination
	if dest == "" {
		dest = PermissionUpdateDestSession
	}
	result := map[string]any{
		"type":        string(p.Type),
		"destination": string(dest),
	}

	switch p.Type {
	case PermissionUpdateAddRules, PermissionUpdateReplaceRules, PermissionUpdateRemoveRules:
		if p.Rules != nil {
			rules := make([]map[string]any, len(p.Rules))
			for i, rule := range p.Rules {
				r := map[string]any{"toolName": rule.ToolName}
				if rule.RuleContent != "" {
					r["ruleContent"] = rule.RuleContent
				}
				rules[i] = r
			}
			result["rules"] = rules
		}
		if p.Behavior != "" {
			result["behavior"] = string(p.Behavior)
		}
	case PermissionUpdateSetMode:
		if p.Mode != "" {
			result["mode"] = string(p.Mode)
		}
	case PermissionUpdateAddDirectories, PermissionUpdateRemoveDirectories:
		if p.Directories != nil {
			result["directories"] = p.Directories
		}
	}

	return result
}

// permissionUpdatesToWire converts the UpdatedPermissions of a
// PermissionResultAllow. It returns nil, and the reason as an error, if any
// update fails [PermissionUpdate.Validate]: the updates are sent all
// together or not at all, because the one that failed may be a restriction
// the others were meant to come with.
func permissionUpdatesToWire(updates []PermissionUpdate) ([]map[string]any, error) {
	wire := make([]map[string]any, len(updates))
	for i := range updates {
		if err := updates[i].Validate(); err != nil {
			return nil, fmt.Errorf("UpdatedPermissions[%d] cannot be sent: %w", i, err)
		}
		wire[i] = updates[i].ToDict()
	}
	return wire, nil
}

// PermissionResult is the interface for tool permission callback results.
type PermissionResult interface {
	permissionResultMarker()
}

// PermissionResultAllow allows tool execution.
type PermissionResultAllow struct {
	UpdatedInput map[string]any `json:"updatedInput,omitempty"`
	// UpdatedPermissions are permission updates to apply along with this
	// allow. If any of them fails [PermissionUpdate.Validate], none are sent
	// and a warning is written to stderr; the tool call is still allowed
	// (Python SDK v0.2.184).
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`
}

func (PermissionResultAllow) permissionResultMarker() {}

// PermissionResultDeny denies tool execution.
type PermissionResultDeny struct {
	Message   string `json:"message,omitempty"`
	Interrupt bool   `json:"interrupt,omitempty"`
}

func (PermissionResultDeny) permissionResultMarker() {}

// ToolPermissionContext provides context for tool permission callbacks.
type ToolPermissionContext struct {
	Suggestions []PermissionUpdate
	// ToolUseID is the ID of the tool use that triggered this permission request.
	ToolUseID string
	// RequestID is the control protocol request ID for this permission
	// request. A consumer answering out-of-band (e.g. a signed HTTP POST
	// instead of a CanUseToolFunc return value) must echo this value in its
	// own control_response for the CLI to match it. See CanUseToolFunc.
	RequestID string
	// AgentID is the ID of the sub-agent requesting permission, if applicable.
	AgentID string
	// DecisionReason is the CLI's suggested reason for this permission decision,
	// if provided (e.g. "allow_once", "deny", "allow_rule").
	DecisionReason string
	// BlockedPath is the filesystem path that triggered a deny decision, if any.
	BlockedPath string
	// Title is the human-readable display title for this permission request.
	Title string
	// DisplayName is the tool's display name as shown in the permission UI.
	DisplayName string
	// Description is additional descriptive context for this permission request.
	Description string
	// SuppressAlwaysAllowRule is true when the dialog must not offer a
	// persistent "don't ask again" affordance for this ask: accepting it
	// would write a whole-tool allow rule broader than the ask's own verb,
	// or the ask comes from a run a plugin started, where Claude Code saves
	// no rule from the answer, not even for the session (TypeScript SDK
	// v0.3.317). Port of TypeScript SDK v0.3.214.
	SuppressAlwaysAllowRule bool
	// MatchedAskRule is set when a user-configured ask rule (permissions.ask)
	// forced this prompt while the ask still carries the tool's own
	// DecisionReason. It may also be set, whatever the DecisionReason, when
	// an ask rule reaches an MCP tool only under another name of its server
	// (such as a connector's id or its claude.ai name), not under the name
	// in ToolName, so a host that looks the rule up by ToolName does not find
	// it (TypeScript SDK v0.3.319). Treat an ask carrying it as rule-forced.
	// Nil when no ask rule matched. Port of TypeScript SDK v0.3.214.
	MatchedAskRule *MatchedAskRule
	// DefaultToNo is true when the terminal prompt for this ask must default
	// to declining (no digit-shortcut auto-accept). Port of TypeScript SDK
	// v0.3.242.
	DefaultToNo bool
	// McpServer identifies the MCP server the requested tool belongs to. Nil
	// for built-in tools. Callers can key trust on McpServer.Source == "sdk"
	// to distinguish an in-process server the host itself defined via
	// [NewSdkMcpServer] from an externally configured one. Port of
	// TypeScript SDK v0.3.274.
	McpServer *ToolMcpServerRef
}

// MatchedAskRule describes the user-configured permission ask rule that
// forced a permission prompt. See [ToolPermissionContext.MatchedAskRule].
// Port of TypeScript SDK v0.3.214.
type MatchedAskRule struct {
	// Source identifies where the ask rule was configured (e.g. a settings
	// file path).
	Source string
	// ToolName is the tool name the matched rule applies to.
	ToolName string
	// RuleContent is the matched rule's pattern, if any.
	RuleContent string
}

// parsePermissionUpdate converts a raw map from the CLI protocol into a
// PermissionUpdate value.
func parsePermissionUpdate(m map[string]any) PermissionUpdate {
	p := PermissionUpdate{
		Type:        PermissionUpdateType(stringField(m, "type")),
		Behavior:    PermissionBehavior(stringField(m, "behavior")),
		Mode:        PermissionMode(stringField(m, "mode")),
		Destination: PermissionUpdateDestination(stringField(m, "destination")),
	}
	if rawRules, ok := m["rules"].([]any); ok {
		p.Rules = make([]PermissionRuleValue, 0, len(rawRules))
		for _, r := range rawRules {
			if rm, ok := r.(map[string]any); ok {
				p.Rules = append(p.Rules, PermissionRuleValue{
					ToolName:    stringField(rm, "toolName"),
					RuleContent: stringField(rm, "ruleContent"),
				})
			}
		}
	}
	if rawDirs, ok := m["directories"].([]any); ok {
		p.Directories = make([]string, 0, len(rawDirs))
		for _, d := range rawDirs {
			if s, ok := d.(string); ok {
				p.Directories = append(p.Directories, s)
			}
		}
	}
	return p
}

// CanUseToolFunc is called when the CLI's permission evaluation resolves to
// "ask" for a tool call — that is, when the tool is not already permitted by
// AllowedTools, DisallowedTools, PermissionMode, or the allow/deny rules in
// settings. It replaces the interactive permission prompt.
//
// CanUseTool is NOT a universal tool interceptor: it does not fire for tools
// that are pre-approved. Use a PreToolUse hook to observe or gate every tool
// call regardless of permission settings.
//
// ToolPermissionContext.ToolUseID is always non-empty when delivered via
// CanUseTool; the omitempty tag exists for JSON marshaling compatibility only.
//
// Returning a nil PermissionResult with a nil error suppresses the SDK's
// automatic control_response for this request. Use this ONLY after already
// sending a control_response out-of-band (e.g. a signed HTTP POST echoing
// ToolPermissionContext.RequestID) — permission prompts have no park
// deadline, so an accidental nil/nil return leaves the tool call blocked
// indefinitely with no response ever sent. Port of TypeScript SDK v0.3.199.
type CanUseToolFunc func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)

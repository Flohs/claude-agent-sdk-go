package claude

import "context"

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
	ToolName    string `json:"toolName"`
	RuleContent string `json:"ruleContent,omitempty"`
}

// PermissionUpdateType represents the type of permission update.
type PermissionUpdateType string

const (
	PermissionUpdateAddRules         PermissionUpdateType = "addRules"
	PermissionUpdateReplaceRules     PermissionUpdateType = "replaceRules"
	PermissionUpdateRemoveRules      PermissionUpdateType = "removeRules"
	PermissionUpdateSetMode          PermissionUpdateType = "setMode"
	PermissionUpdateAddDirectories   PermissionUpdateType = "addDirectories"
	PermissionUpdateRemoveDirectories PermissionUpdateType = "removeDirectories"
)

// PermissionUpdate represents a permission update configuration.
type PermissionUpdate struct {
	Type        PermissionUpdateType        `json:"type"`
	Rules       []PermissionRuleValue       `json:"rules,omitempty"`
	Behavior    PermissionBehavior          `json:"behavior,omitempty"`
	Mode        PermissionMode              `json:"mode,omitempty"`
	Directories []string                    `json:"directories,omitempty"`
	Destination PermissionUpdateDestination `json:"destination,omitempty"`
}

// ToDict converts a PermissionUpdate to a map matching the TypeScript control protocol.
func (p *PermissionUpdate) ToDict() map[string]any {
	result := map[string]any{
		"type": string(p.Type),
	}

	if p.Destination != "" {
		result["destination"] = string(p.Destination)
	}

	switch p.Type {
	case PermissionUpdateAddRules, PermissionUpdateReplaceRules, PermissionUpdateRemoveRules:
		if len(p.Rules) > 0 {
			rules := make([]map[string]any, len(p.Rules))
			for i, rule := range p.Rules {
				rules[i] = map[string]any{
					"toolName":    rule.ToolName,
					"ruleContent": rule.RuleContent,
				}
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
		if len(p.Directories) > 0 {
			result["directories"] = p.Directories
		}
	}

	return result
}

// PermissionResult is the interface for tool permission callback results.
type PermissionResult interface {
	permissionResultMarker()
}

// PermissionResultAllow allows tool execution.
type PermissionResultAllow struct {
	UpdatedInput       map[string]any     `json:"updatedInput,omitempty"`
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
type CanUseToolFunc func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)

package claude

import "context"

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
}

// CanUseToolFunc is the callback type for tool permission decisions.
// It is invoked for tools not matched by AllowedTools or DisallowedTools.
type CanUseToolFunc func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error)

package claude

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestPermissionUpdate_ToDict_DefaultsAndOmissions(t *testing.T) {
	p := PermissionUpdate{
		Type:     PermissionUpdateAddRules,
		Behavior: PermissionBehaviorDeny,
		Rules: []PermissionRuleValue{
			{ToolName: "Bash", RuleContent: "rm -rf *"},
			{ToolName: "WebFetch"},
		},
	}
	got := p.ToDict()
	want := map[string]any{
		"type":        "addRules",
		"destination": "session",
		"behavior":    "deny",
		"rules": []map[string]any{
			{"toolName": "Bash", "ruleContent": "rm -rf *"},
			{"toolName": "WebFetch"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToDict() = %#v, want %#v", got, want)
	}

	p.Destination = PermissionUpdateDestProjectSettings
	if d := p.ToDict()["destination"]; d != "projectSettings" {
		t.Errorf("destination = %v, want projectSettings", d)
	}

	empty := PermissionUpdate{Type: PermissionUpdateReplaceRules, Behavior: PermissionBehaviorAllow, Rules: []PermissionRuleValue{}}
	rules, ok := empty.ToDict()["rules"].([]map[string]any)
	if !ok || len(rules) != 0 {
		t.Errorf("empty non-nil Rules should be sent as an empty list, got %#v", empty.ToDict()["rules"])
	}
	if err := empty.Validate(); err != nil {
		t.Errorf("empty non-nil Rules should be valid: %v", err)
	}
}

func TestPermissionUpdate_Validate(t *testing.T) {
	tests := []struct {
		name    string
		update  PermissionUpdate
		wantErr string
	}{
		{"addRules ok", PermissionUpdate{Type: PermissionUpdateAddRules, Behavior: PermissionBehaviorAllow, Rules: []PermissionRuleValue{{ToolName: "Read"}}}, ""},
		{"addRules no rules", PermissionUpdate{Type: PermissionUpdateAddRules, Behavior: PermissionBehaviorAllow}, "has no Rules"},
		{"removeRules no behavior", PermissionUpdate{Type: PermissionUpdateRemoveRules, Rules: []PermissionRuleValue{{ToolName: "Read"}}}, "has no Behavior"},
		{"setMode ok", PermissionUpdate{Type: PermissionUpdateSetMode, Mode: PermissionModeAcceptEdits}, ""},
		{"setMode no mode", PermissionUpdate{Type: PermissionUpdateSetMode}, "has no Mode"},
		{"addDirectories ok", PermissionUpdate{Type: PermissionUpdateAddDirectories, Directories: []string{"/tmp"}}, ""},
		{"removeDirectories none", PermissionUpdate{Type: PermissionUpdateRemoveDirectories}, "has no Directories"},
		{"unknown type", PermissionUpdate{Type: "grantEverything"}, "cannot send"},
		{"empty type", PermissionUpdate{}, "cannot send"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.update.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func canUseToolAllowResponse(t *testing.T, result PermissionResultAllow) map[string]any {
	t.Helper()
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			return result, nil
		},
	})
	q.handleControlRequest(map[string]any{
		"request_id": "req-perm",
		"request": map[string]any{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"tool_use_id": "tu-perm",
			"input":       map[string]any{"command": "ls"},
		},
	})
	if len(mt.written) != 1 {
		t.Fatalf("expected one control_response write, got %d", len(mt.written))
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(mt.written[0]), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	inner, _ := resp["response"].(map[string]any)
	if inner["subtype"] != "success" {
		t.Fatalf("expected success response, got %v", resp)
	}
	body, _ := inner["response"].(map[string]any)
	if body["behavior"] != "allow" {
		t.Fatalf("expected allow, got %v", body)
	}
	return body
}

func TestHandleCanUseTool_SendsValidPermissionUpdates(t *testing.T) {
	body := canUseToolAllowResponse(t, PermissionResultAllow{
		UpdatedPermissions: []PermissionUpdate{
			{Type: PermissionUpdateAddRules, Behavior: PermissionBehaviorAllow, Rules: []PermissionRuleValue{{ToolName: "Bash"}}},
			{Type: PermissionUpdateSetMode, Mode: PermissionModeAcceptEdits, Destination: PermissionUpdateDestLocalSettings},
		},
	})
	perms, ok := body["updatedPermissions"].([]any)
	if !ok || len(perms) != 2 {
		t.Fatalf("expected 2 updatedPermissions, got %#v", body["updatedPermissions"])
	}
	first := perms[0].(map[string]any)
	if first["destination"] != "session" {
		t.Errorf("first destination = %v, want session", first["destination"])
	}
	rule := first["rules"].([]any)[0].(map[string]any)
	if _, has := rule["ruleContent"]; has {
		t.Errorf("ruleContent should be omitted for a whole-tool rule, got %#v", rule)
	}
	if second := perms[1].(map[string]any); second["destination"] != "localSettings" || second["mode"] != "acceptEdits" {
		t.Errorf("second = %#v", second)
	}
}

func TestHandleCanUseTool_DropsAllPermissionUpdatesWhenOneIsInvalid(t *testing.T) {
	body := canUseToolAllowResponse(t, PermissionResultAllow{
		UpdatedPermissions: []PermissionUpdate{
			{Type: PermissionUpdateAddRules, Behavior: PermissionBehaviorAllow, Rules: []PermissionRuleValue{{ToolName: "Bash"}}},
			{Type: PermissionUpdateAddRules, Rules: []PermissionRuleValue{{ToolName: "Write"}}}, // no Behavior
		},
	})
	if _, has := body["updatedPermissions"]; has {
		t.Fatalf("updatedPermissions should be omitted when one update is invalid, got %#v", body["updatedPermissions"])
	}
	if _, has := body["updatedInput"]; !has {
		t.Error("the tool call should still be allowed with its input")
	}
}

func TestPermissionUpdatesToWire_ErrorNamesIndex(t *testing.T) {
	_, err := permissionUpdatesToWire([]PermissionUpdate{
		{Type: PermissionUpdateSetMode, Mode: PermissionModePlan},
		{Type: PermissionUpdateSetMode},
	})
	if err == nil || !strings.Contains(err.Error(), "UpdatedPermissions[1]") {
		t.Fatalf("err = %v, want it to name UpdatedPermissions[1]", err)
	}
}

package claude

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStderr redirects os.Stderr for the duration of fn and returns
// whatever was written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	_ = w.Close()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read captured stderr: %v", err)
	}
	return string(out)
}

func TestWarnCanUseToolPermissionConflicts_NoCanUseTool(t *testing.T) {
	out := captureStderr(t, func() {
		warnCanUseToolPermissionConflicts(&Options{
			AllowedTools:   []string{"Bash"},
			PermissionMode: PermissionModeBypassPermissions,
		})
	})
	if out != "" {
		t.Errorf("expected no warning without CanUseTool, got %q", out)
	}
}

func TestWarnCanUseToolPermissionConflicts_NoConflict(t *testing.T) {
	out := captureStderr(t, func() {
		warnCanUseToolPermissionConflicts(&Options{
			CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
				return nil, nil
			},
		})
	})
	if out != "" {
		t.Errorf("expected no warning for CanUseTool alone, got %q", out)
	}
}

func TestWarnCanUseToolPermissionConflicts_AllowedTools(t *testing.T) {
	out := captureStderr(t, func() {
		warnCanUseToolPermissionConflicts(&Options{
			CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
				return nil, nil
			},
			AllowedTools: []string{"Bash"},
		})
	})
	if !strings.Contains(out, "AllowedTools") {
		t.Errorf("expected warning mentioning AllowedTools, got %q", out)
	}
}

func TestWarnCanUseToolPermissionConflicts_BypassPermissions(t *testing.T) {
	out := captureStderr(t, func() {
		warnCanUseToolPermissionConflicts(&Options{
			CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
				return nil, nil
			},
			PermissionMode: PermissionModeBypassPermissions,
		})
	})
	if !strings.Contains(out, "PermissionModeBypassPermissions") {
		t.Errorf("expected warning mentioning PermissionModeBypassPermissions, got %q", out)
	}
}

func TestWarnCanUseToolPermissionConflicts_Both(t *testing.T) {
	out := captureStderr(t, func() {
		warnCanUseToolPermissionConflicts(&Options{
			CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
				return nil, nil
			},
			AllowedTools:   []string{"Bash"},
			PermissionMode: PermissionModeBypassPermissions,
		})
	})
	if !strings.Contains(out, "AllowedTools") || !strings.Contains(out, "PermissionModeBypassPermissions") {
		t.Errorf("expected warning mentioning both AllowedTools and PermissionModeBypassPermissions, got %q", out)
	}
}

func TestAgentDefinition_JSONMarshal(t *testing.T) {
	def := AgentDefinition{
		Description: "test agent",
		Prompt:      "You are a test agent",
		Tools:       []string{"Bash", "Read"},
		Model:       "sonnet",
		Skills:      []string{"commit", "review-pr"},
		Memory:      "project",
		MCPServers:  []any{map[string]any{"name": "test-server"}},
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["description"] != "test agent" {
		t.Errorf("expected description 'test agent', got %v", result["description"])
	}
	if result["memory"] != "project" {
		t.Errorf("expected memory 'project', got %v", result["memory"])
	}

	skills, ok := result["skills"].([]any)
	if !ok || len(skills) != 2 {
		t.Errorf("expected 2 skills, got %v", result["skills"])
	}

	mcpServers, ok := result["mcpServers"].([]any)
	if !ok || len(mcpServers) != 1 {
		t.Errorf("expected 1 mcpServer, got %v", result["mcpServers"])
	}
}

func TestAgentDefinition_JSONMarshal_OmitEmpty(t *testing.T) {
	def := AgentDefinition{
		Description: "minimal agent",
		Prompt:      "You are minimal",
	}

	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	for _, key := range []string{"tools", "model", "skills", "memory", "mcpServers"} {
		if _, ok := result[key]; ok {
			t.Errorf("expected %q to be omitted when empty, but it was present", key)
		}
	}
}

func TestSandboxCredentialsConfig_JSONMarshal_MaskMode(t *testing.T) {
	allowPlaintext := true
	cfg := SandboxCredentialsConfig{
		Files: []SandboxCredentialFileEntry{
			{Path: "~/.aws/credentials", Mode: "deny"},
		},
		EnvVars: []SandboxCredentialEnvVarEntry{
			{Name: "GITHUB_TOKEN", Mode: "mask", InjectHosts: []string{"api.github.com"}},
			{Name: "SECRET_KEY", Mode: "deny"},
		},
		AllowPlaintextInject: &allowPlaintext,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if result["allowPlaintextInject"] != true {
		t.Errorf("expected allowPlaintextInject true, got %v", result["allowPlaintextInject"])
	}

	envVars, ok := result["envVars"].([]any)
	if !ok || len(envVars) != 2 {
		t.Fatalf("expected 2 envVars, got %v", result["envVars"])
	}

	masked, ok := envVars[0].(map[string]any)
	if !ok || masked["mode"] != "mask" {
		t.Fatalf("expected first envVar mode 'mask', got %v", envVars[0])
	}
	hosts, ok := masked["injectHosts"].([]any)
	if !ok || len(hosts) != 1 || hosts[0] != "api.github.com" {
		t.Errorf("expected injectHosts [api.github.com], got %v", masked["injectHosts"])
	}

	denied, ok := envVars[1].(map[string]any)
	if !ok || denied["mode"] != "deny" {
		t.Fatalf("expected second envVar mode 'deny', got %v", envVars[1])
	}
	if _, ok := denied["injectHosts"]; ok {
		t.Errorf("expected injectHosts to be omitted for deny mode entry, got %v", denied["injectHosts"])
	}
}

func TestSandboxCredentialsConfig_JSONMarshal_OmitEmpty(t *testing.T) {
	cfg := SandboxCredentialsConfig{}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	for _, key := range []string{"files", "envVars", "allowPlaintextInject"} {
		if _, ok := result[key]; ok {
			t.Errorf("expected %q to be omitted when empty, but it was present", key)
		}
	}
}

// TestValidatePermissionMode_ValidValues verifies that every known
// PermissionMode constant (including the "manual" alias) and the empty
// string (unset — CLI default applies) are accepted.
func TestValidatePermissionMode_ValidValues(t *testing.T) {
	valid := []PermissionMode{
		"",
		PermissionModeDefault,
		PermissionModeAcceptEdits,
		PermissionModePlan,
		PermissionModeBypassPermissions,
		PermissionModeDontAsk,
		PermissionModeAuto,
		PermissionModeManual,
	}
	for _, mode := range valid {
		if err := validatePermissionMode(mode); err != nil {
			t.Errorf("validatePermissionMode(%q) = %v, want nil", mode, err)
		}
	}
}

// TestValidatePermissionMode_InvalidValue verifies that a typo'd
// PermissionMode is rejected with an error naming the bad value.
func TestValidatePermissionMode_InvalidValue(t *testing.T) {
	err := validatePermissionMode(PermissionMode("acceptEdit"))
	if err == nil {
		t.Fatal("expected error for invalid PermissionMode, got nil")
	}
	if !strings.Contains(err.Error(), "acceptEdit") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "acceptEdit")
	}
	if _, ok := err.(*SDKError); !ok {
		t.Errorf("expected error to be a *SDKError, got %T", err)
	}
}

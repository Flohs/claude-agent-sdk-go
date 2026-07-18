package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

// Compile-time interface satisfaction checks.
var (
	_ TypedHookInput = (*PreToolUseHookInput)(nil)
	_ TypedHookInput = (*PostToolUseHookInput)(nil)
	_ TypedHookInput = (*PostToolUseFailureHookInput)(nil)
	_ TypedHookInput = (*PermissionRequestHookInput)(nil)
	_ TypedHookInput = (*UserPromptSubmitHookInput)(nil)
	_ TypedHookInput = (*StopHookInput)(nil)
	_ TypedHookInput = (*SubagentStopHookInput)(nil)
	_ TypedHookInput = (*SubagentStartHookInput)(nil)
	_ TypedHookInput = (*PreCompactHookInput)(nil)
	_ TypedHookInput = (*NotificationHookInput)(nil)
	_ TypedHookInput = (*TeammateIdleHookInput)(nil)
	_ TypedHookInput = (*TaskCompletedHookInput)(nil)
	_ TypedHookInput = (*ConfigChangeHookInput)(nil)
	_ TypedHookInput = (*ElicitationHookInput)(nil)
	_ TypedHookInput = (*MessageDisplayHookInput)(nil)
	_ TypedHookInput = (*SessionStartHookInput)(nil)
	_ TypedHookInput = (*SessionEndHookInput)(nil)
	_ TypedHookInput = (*StopFailureHookInput)(nil)
	_ TypedHookInput = (*PostCompactHookInput)(nil)
	_ TypedHookInput = (*PostToolBatchHookInput)(nil)
	_ TypedHookInput = (*PermissionDeniedHookInput)(nil)
	_ TypedHookInput = (*ElicitationResultHookInput)(nil)
	_ TypedHookInput = (*InstructionsLoadedHookInput)(nil)
	_ TypedHookInput = (*CwdChangedHookInput)(nil)
	_ TypedHookInput = (*FileChangedHookInput)(nil)
	_ TypedHookInput = (*WorktreeCreateHookInput)(nil)
	_ TypedHookInput = (*WorktreeRemoveHookInput)(nil)
	_ TypedHookInput = (*UserPromptExpansionHookInput)(nil)
	_ TypedHookInput = (*SetupHookInput)(nil)
	_ TypedHookInput = (*TaskCreatedHookInput)(nil)
)

// base returns a HookInput with common fields pre-filled.
func base(event string) HookInput {
	return HookInput{
		"session_id":      "sess-1",
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             "/home/user",
		"permission_mode": "default",
		"hook_event_name": event,
	}
}

func merge(a, b HookInput) HookInput {
	out := make(HookInput, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func assertBase(t *testing.T, b BaseHookInput, event string) {
	t.Helper()
	if b.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", b.SessionID, "sess-1")
	}
	if b.TranscriptPath != "/tmp/transcript.jsonl" {
		t.Errorf("TranscriptPath = %q, want %q", b.TranscriptPath, "/tmp/transcript.jsonl")
	}
	if b.Cwd != "/home/user" {
		t.Errorf("Cwd = %q, want %q", b.Cwd, "/home/user")
	}
	if b.HookEventName != event {
		t.Errorf("HookEventName = %q, want %q", b.HookEventName, event)
	}
}

func TestParseHookInput_Nil(t *testing.T) {
	result, err := ParseHookInput(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil, got %T", result)
	}
}

func TestParseHookInput_UnknownEvent(t *testing.T) {
	result, err := ParseHookInput(base("FutureEvent"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for unknown event, got %T", result)
	}
}

func TestParseHookInput_PreToolUse(t *testing.T) {
	input := merge(base("PreToolUse"), HookInput{
		"tool_name":   "Bash",
		"tool_input":  map[string]any{"command": "echo hello"},
		"tool_use_id": "toolu_abc123",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed, ok := result.(*PreToolUseHookInput)
	if !ok {
		t.Fatalf("expected *PreToolUseHookInput, got %T", result)
	}
	assertBase(t, typed.BaseHookInput, "PreToolUse")
	if typed.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", typed.ToolName, "Bash")
	}
	if typed.ToolUseID != "toolu_abc123" {
		t.Errorf("ToolUseID = %q, want %q", typed.ToolUseID, "toolu_abc123")
	}
	if typed.AgentID != "" {
		t.Errorf("AgentID should be empty on main thread, got %q", typed.AgentID)
	}
}

func TestParseHookInput_PreToolUse_WithAgentID(t *testing.T) {
	input := merge(base("PreToolUse"), HookInput{
		"tool_name":   "Bash",
		"tool_input":  map[string]any{"command": "echo hello"},
		"tool_use_id": "toolu_abc123",
		"agent_id":    "agent-42",
		"agent_type":  "researcher",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PreToolUseHookInput)
	if typed.AgentID != "agent-42" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-42")
	}
	if typed.AgentType != "researcher" {
		t.Errorf("AgentType = %q, want %q", typed.AgentType, "researcher")
	}
}

func TestParseHookInput_PostToolUse(t *testing.T) {
	input := merge(base("PostToolUse"), HookInput{
		"tool_name":     "Bash",
		"tool_input":    map[string]any{"command": "ls"},
		"tool_response": map[string]any{"content": []any{map[string]any{"type": "text", "text": "file.txt"}}},
		"tool_use_id":   "toolu_def456",
		"agent_id":      "agent-7",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PostToolUseHookInput)
	assertBase(t, typed.BaseHookInput, "PostToolUse")
	if typed.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", typed.ToolName, "Bash")
	}
	if typed.ToolResponse == nil {
		t.Error("ToolResponse should not be nil")
	}
	if typed.AgentID != "agent-7" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-7")
	}
}

func TestParseHookInput_PostToolUseFailure(t *testing.T) {
	input := merge(base("PostToolUseFailure"), HookInput{
		"tool_name":    "Write",
		"tool_input":   map[string]any{"path": "/etc/passwd"},
		"tool_use_id":  "toolu_fail1",
		"error":        "permission denied",
		"is_interrupt":  true,
		"agent_id":     "agent-99",
		"agent_type":   "code-reviewer",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PostToolUseFailureHookInput)
	assertBase(t, typed.BaseHookInput, "PostToolUseFailure")
	if typed.Error != "permission denied" {
		t.Errorf("Error = %q, want %q", typed.Error, "permission denied")
	}
	if !typed.IsInterrupt {
		t.Error("IsInterrupt should be true")
	}
	if typed.AgentID != "agent-99" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-99")
	}
}

func TestParseHookInput_PermissionRequest(t *testing.T) {
	input := merge(base("PermissionRequest"), HookInput{
		"tool_name":  "Bash",
		"tool_input": map[string]any{"command": "rm -rf /"},
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PermissionRequestHookInput)
	assertBase(t, typed.BaseHookInput, "PermissionRequest")
	if typed.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", typed.ToolName, "Bash")
	}
}

func TestParseHookInput_UserPromptSubmit(t *testing.T) {
	input := merge(base("UserPromptSubmit"), HookInput{
		"prompt": "explain this code",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*UserPromptSubmitHookInput)
	assertBase(t, typed.BaseHookInput, "UserPromptSubmit")
	if typed.Prompt != "explain this code" {
		t.Errorf("Prompt = %q, want %q", typed.Prompt, "explain this code")
	}
}

func TestParseHookInput_Stop(t *testing.T) {
	input := merge(base("Stop"), HookInput{
		"stop_hook_active": true,
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*StopHookInput)
	assertBase(t, typed.BaseHookInput, "Stop")
	if !typed.StopHookActive {
		t.Error("StopHookActive should be true")
	}
}

func TestParseHookInput_SubagentStop(t *testing.T) {
	input := merge(base("SubagentStop"), HookInput{
		"stop_hook_active":       false,
		"agent_id":               "agent-42",
		"agent_transcript_path":  "/tmp/agent-42.jsonl",
		"agent_type":             "general-purpose",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*SubagentStopHookInput)
	assertBase(t, typed.BaseHookInput, "SubagentStop")
	if typed.AgentID != "agent-42" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-42")
	}
	if typed.AgentTranscriptPath != "/tmp/agent-42.jsonl" {
		t.Errorf("AgentTranscriptPath = %q, want %q", typed.AgentTranscriptPath, "/tmp/agent-42.jsonl")
	}
	if typed.AgentType != "general-purpose" {
		t.Errorf("AgentType = %q, want %q", typed.AgentType, "general-purpose")
	}
}

func TestParseHookInput_SubagentStart(t *testing.T) {
	input := merge(base("SubagentStart"), HookInput{
		"agent_id":   "agent-42",
		"agent_type": "researcher",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*SubagentStartHookInput)
	assertBase(t, typed.BaseHookInput, "SubagentStart")
	if typed.AgentID != "agent-42" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-42")
	}
	if typed.AgentType != "researcher" {
		t.Errorf("AgentType = %q, want %q", typed.AgentType, "researcher")
	}
}

func TestParseHookInput_PreCompact(t *testing.T) {
	input := merge(base("PreCompact"), HookInput{
		"trigger":              "auto",
		"custom_instructions":  "keep it short",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PreCompactHookInput)
	assertBase(t, typed.BaseHookInput, "PreCompact")
	if typed.Trigger != "auto" {
		t.Errorf("Trigger = %q, want %q", typed.Trigger, "auto")
	}
	if typed.CustomInstructions != "keep it short" {
		t.Errorf("CustomInstructions = %q, want %q", typed.CustomInstructions, "keep it short")
	}
}

func TestParseHookInput_Notification(t *testing.T) {
	input := merge(base("Notification"), HookInput{
		"message":           "task complete",
		"title":             "Done",
		"notification_type": "info",
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*NotificationHookInput)
	assertBase(t, typed.BaseHookInput, "Notification")
	if typed.Message != "task complete" {
		t.Errorf("Message = %q, want %q", typed.Message, "task complete")
	}
	if typed.Title != "Done" {
		t.Errorf("Title = %q, want %q", typed.Title, "Done")
	}
	if typed.NotificationType != "info" {
		t.Errorf("NotificationType = %q, want %q", typed.NotificationType, "info")
	}
}

// --- Backward compatibility tests ---
// These tests replicate the exact map[string]any access patterns that existing
// consumers use (as seen in examples/hooks/main.go before our changes).
// They verify that HookInput remains a plain map[string]any and that the
// original untyped access pattern still works identically.

func TestHookInput_BackwardCompat_PreToolUseMapAccess(t *testing.T) {
	// This is the exact pattern from the original checkBashCommand example.
	input := HookInput{
		"session_id":      "sess-1",
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "echo hello"},
		"tool_use_id":     "toolu_abc123",
	}

	// Original access pattern: direct type assertions on the map.
	toolName, _ := input["tool_name"].(string)
	if toolName != "Bash" {
		t.Errorf("toolName = %q, want %q", toolName, "Bash")
	}

	toolInput, _ := input["tool_input"].(map[string]any)
	command, _ := toolInput["command"].(string)
	if command != "echo hello" {
		t.Errorf("command = %q, want %q", command, "echo hello")
	}
}

func TestHookInput_BackwardCompat_PostToolUseMapAccess(t *testing.T) {
	// This is the exact pattern from the original reviewToolOutput example.
	input := HookInput{
		"session_id":      "sess-1",
		"transcript_path": "/tmp/transcript.jsonl",
		"cwd":             "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PostToolUse",
		"tool_name":       "Bash",
		"tool_response":   "error: file not found",
	}

	// Original access pattern: fmt.Sprintf on tool_response.
	toolResponse := fmt.Sprintf("%v", input["tool_response"])
	if toolResponse != "error: file not found" {
		t.Errorf("toolResponse = %q, want %q", toolResponse, "error: file not found")
	}
}

func TestHookInput_BackwardCompat_CallbackSignature(t *testing.T) {
	// Verify that a HookCallback written with the old map-based style
	// still compiles and works correctly.
	var callback HookCallback = func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
		// Old-style: direct map access, no ParseHookInput.
		name, _ := input["tool_name"].(string)
		return HookJSONOutput{"saw_tool": name}, nil
	}

	input := HookInput{
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
	}
	output, err := callback(context.Background(), input, "toolu_1", HookContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output["saw_tool"] != "Bash" {
		t.Errorf("output[saw_tool] = %v, want %q", output["saw_tool"], "Bash")
	}
}

func TestHookInput_BackwardCompat_HookMatcherConfig(t *testing.T) {
	// Verify the hooks configuration pattern still works.
	hooks := map[HookEvent][]HookMatcher{
		HookEventPreToolUse: {
			{
				Matcher: "Bash",
				Hooks: []HookCallback{
					func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
						return HookJSONOutput{}, nil
					},
				},
			},
		},
		HookEventPostToolUse: {
			{
				Matcher: "Bash",
				Hooks: []HookCallback{
					func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
						return HookJSONOutput{
							"systemMessage": "done",
						}, nil
					},
				},
			},
		},
	}

	if len(hooks) != 2 {
		t.Errorf("expected 2 hook events, got %d", len(hooks))
	}
	if len(hooks[HookEventPreToolUse]) != 1 {
		t.Errorf("expected 1 PreToolUse matcher, got %d", len(hooks[HookEventPreToolUse]))
	}
	if hooks[HookEventPreToolUse][0].Matcher != "Bash" {
		t.Errorf("matcher = %q, want %q", hooks[HookEventPreToolUse][0].Matcher, "Bash")
	}
}

// --- Edge case tests ---

func TestParseHookInput_EmptyMap(t *testing.T) {
	// Empty map has no hook_event_name, should return nil (unknown event).
	result, err := ParseHookInput(HookInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for empty map, got %T", result)
	}
}

func TestParseHookInput_MissingOptionalFields(t *testing.T) {
	// Minimal PreToolUse — only hook_event_name, no tool_name etc.
	// Should parse without error; missing fields get zero values.
	input := HookInput{"hook_event_name": "PreToolUse"}

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PreToolUseHookInput)
	if typed.ToolName != "" {
		t.Errorf("ToolName should be zero value, got %q", typed.ToolName)
	}
	if typed.ToolInput != nil {
		t.Errorf("ToolInput should be nil, got %v", typed.ToolInput)
	}
	if typed.AgentID != "" {
		t.Errorf("AgentID should be zero value, got %q", typed.AgentID)
	}
}

func TestParseHookInput_ExtraUnknownFields(t *testing.T) {
	// CLI sends a field we don't know about — ParseHookInput should not fail.
	input := merge(base("PreToolUse"), HookInput{
		"tool_name":      "Bash",
		"tool_input":     map[string]any{"command": "echo hi"},
		"tool_use_id":    "toolu_xyz",
		"future_field":   "some_value",
		"another_field":  42,
	})

	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PreToolUseHookInput)
	if typed.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", typed.ToolName, "Bash")
	}
}

// --- JSON round-trip tests ---
// Verify that the json struct tags produce correct JSON field names
// and that encoding/json can unmarshal real CLI-shaped JSON into our types.

func TestPreToolUseHookInput_JSONRoundTrip(t *testing.T) {
	// Simulate raw JSON from the CLI.
	raw := `{
		"session_id": "sess-abc",
		"transcript_path": "/tmp/t.jsonl",
		"cwd": "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {"command": "echo hi"},
		"tool_use_id": "toolu_123",
		"agent_id": "agent-5",
		"agent_type": "researcher"
	}`

	var typed PreToolUseHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if typed.SessionID != "sess-abc" {
		t.Errorf("SessionID = %q, want %q", typed.SessionID, "sess-abc")
	}
	if typed.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", typed.ToolName, "Bash")
	}
	if typed.AgentID != "agent-5" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-5")
	}
	if typed.AgentType != "researcher" {
		t.Errorf("AgentType = %q, want %q", typed.AgentType, "researcher")
	}
	cmd, _ := typed.ToolInput["command"].(string)
	if cmd != "echo hi" {
		t.Errorf("ToolInput[command] = %q, want %q", cmd, "echo hi")
	}

	// Marshal back and verify field names are snake_case.
	out, err := json.Marshal(&typed)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	for _, key := range []string{"session_id", "tool_name", "tool_use_id", "agent_id", "agent_type", "hook_event_name"} {
		if _, ok := m[key]; !ok {
			t.Errorf("marshaled JSON missing expected key %q", key)
		}
	}
}

func TestPreToolUseHookInput_JSONRoundTrip_NoAgent(t *testing.T) {
	// Main-thread tool call: no agent_id/agent_type in JSON.
	raw := `{
		"session_id": "sess-abc",
		"transcript_path": "/tmp/t.jsonl",
		"cwd": "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {"command": "ls"},
		"tool_use_id": "toolu_456"
	}`

	var typed PreToolUseHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if typed.AgentID != "" {
		t.Errorf("AgentID should be empty, got %q", typed.AgentID)
	}
	if typed.AgentType != "" {
		t.Errorf("AgentType should be empty, got %q", typed.AgentType)
	}

	// Marshal back: omitempty should omit agent_id and agent_type.
	out, err := json.Marshal(&typed)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if _, ok := m["agent_id"]; ok {
		t.Error("agent_id should be omitted when empty")
	}
	if _, ok := m["agent_type"]; ok {
		t.Error("agent_type should be omitted when empty")
	}
}

func TestSubagentStopHookInput_JSONRoundTrip(t *testing.T) {
	raw := `{
		"session_id": "sess-1",
		"transcript_path": "/tmp/t.jsonl",
		"cwd": "/home/user",
		"permission_mode": "default",
		"hook_event_name": "SubagentStop",
		"stop_hook_active": true,
		"agent_id": "agent-42",
		"agent_transcript_path": "/tmp/agent-42.jsonl",
		"agent_type": "general-purpose"
	}`

	var typed SubagentStopHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if typed.AgentID != "agent-42" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-42")
	}
	if !typed.StopHookActive {
		t.Error("StopHookActive should be true")
	}
	if typed.AgentTranscriptPath != "/tmp/agent-42.jsonl" {
		t.Errorf("AgentTranscriptPath = %q, want %q", typed.AgentTranscriptPath, "/tmp/agent-42.jsonl")
	}
}

func TestPostToolUseFailureHookInput_JSONRoundTrip(t *testing.T) {
	raw := `{
		"session_id": "sess-1",
		"transcript_path": "/tmp/t.jsonl",
		"cwd": "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PostToolUseFailure",
		"tool_name": "Write",
		"tool_input": {"path": "/etc/passwd"},
		"tool_use_id": "toolu_fail",
		"error": "permission denied",
		"is_interrupt": true,
		"agent_id": "agent-99",
		"agent_type": "code-reviewer"
	}`

	var typed PostToolUseFailureHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if typed.Error != "permission denied" {
		t.Errorf("Error = %q, want %q", typed.Error, "permission denied")
	}
	if !typed.IsInterrupt {
		t.Error("IsInterrupt should be true")
	}
	if typed.AgentID != "agent-99" {
		t.Errorf("AgentID = %q, want %q", typed.AgentID, "agent-99")
	}
}

func TestParseHookInput_TeammateIdle(t *testing.T) {
	input := merge(base("TeammateIdle"), HookInput{
		"agent_id":   "agent-7",
		"agent_type": "researcher",
	})
	got, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed, ok := got.(*TeammateIdleHookInput)
	if !ok {
		t.Fatalf("expected *TeammateIdleHookInput, got %T", got)
	}
	assertBase(t, typed.BaseHookInput, "TeammateIdle")
	if typed.AgentID != "agent-7" {
		t.Errorf("AgentID = %q, want agent-7", typed.AgentID)
	}
}

func TestParseHookInput_TaskCompleted(t *testing.T) {
	input := merge(base("TaskCompleted"), HookInput{
		"task_id":     "task-1",
		"tool_use_id": "toolu_x",
		"agent_id":    "agent-7",
	})
	got, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed, ok := got.(*TaskCompletedHookInput)
	if !ok {
		t.Fatalf("expected *TaskCompletedHookInput, got %T", got)
	}
	if typed.TaskID != "task-1" {
		t.Errorf("TaskID = %q, want task-1", typed.TaskID)
	}
	if typed.ToolUseID != "toolu_x" {
		t.Errorf("ToolUseID = %q, want toolu_x", typed.ToolUseID)
	}
}

func TestParseHookInput_ConfigChange(t *testing.T) {
	input := merge(base("ConfigChange"), HookInput{
		"changes": map[string]any{
			"permission_mode": "acceptEdits",
			"model":           "claude-opus-4-7",
		},
	})
	got, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed, ok := got.(*ConfigChangeHookInput)
	if !ok {
		t.Fatalf("expected *ConfigChangeHookInput, got %T", got)
	}
	if typed.Changes["permission_mode"] != "acceptEdits" {
		t.Errorf("Changes[permission_mode] = %v, want acceptEdits", typed.Changes["permission_mode"])
	}
}

// --- ParseHookInput consistency with JSON unmarshal ---
// Verify that ParseHookInput (from map) and json.Unmarshal (from bytes)
// produce equivalent results for the same data.

func TestParseHookInput_ConsistentWithJSON(t *testing.T) {
	// Start from JSON bytes (what the CLI actually sends over the wire).
	raw := `{
		"session_id": "sess-1",
		"transcript_path": "/tmp/t.jsonl",
		"cwd": "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name": "Bash",
		"tool_input": {"command": "echo hello"},
		"tool_use_id": "toolu_abc",
		"agent_id": "agent-42",
		"agent_type": "researcher"
	}`

	// Path 1: json.Unmarshal into map, then ParseHookInput (this is what the SDK does).
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal to map error: %v", err)
	}
	fromMap, err := ParseHookInput(HookInput(m))
	if err != nil {
		t.Fatalf("ParseHookInput error: %v", err)
	}
	parsed := fromMap.(*PreToolUseHookInput)

	// Path 2: json.Unmarshal directly into typed struct.
	var direct PreToolUseHookInput
	if err := json.Unmarshal([]byte(raw), &direct); err != nil {
		t.Fatalf("unmarshal to struct error: %v", err)
	}

	// Both paths should produce the same result.
	if parsed.SessionID != direct.SessionID {
		t.Errorf("SessionID mismatch: %q vs %q", parsed.SessionID, direct.SessionID)
	}
	if parsed.ToolName != direct.ToolName {
		t.Errorf("ToolName mismatch: %q vs %q", parsed.ToolName, direct.ToolName)
	}
	if parsed.ToolUseID != direct.ToolUseID {
		t.Errorf("ToolUseID mismatch: %q vs %q", parsed.ToolUseID, direct.ToolUseID)
	}
	if parsed.AgentID != direct.AgentID {
		t.Errorf("AgentID mismatch: %q vs %q", parsed.AgentID, direct.AgentID)
	}
	if parsed.AgentType != direct.AgentType {
		t.Errorf("AgentType mismatch: %q vs %q", parsed.AgentType, direct.AgentType)
	}
}

func TestExitPlanModeToolInput_Roundtrip(t *testing.T) {
	raw := map[string]any{
		"planFilePath": "/tmp/plan.md",
	}
	// Simulate how a hook callback would decode the ToolInput map.
	// The struct should marshal/unmarshal correctly.
	b, _ := json.Marshal(raw)
	var input ExitPlanModeToolInput
	if err := json.Unmarshal(b, &input); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if input.PlanFilePath != "/tmp/plan.md" {
		t.Errorf("PlanFilePath: got %q, want '/tmp/plan.md'", input.PlanFilePath)
	}
	// Empty case
	var empty ExitPlanModeToolInput
	b2, _ := json.Marshal(empty)
	if string(b2) != "{}" {
		t.Errorf("empty struct should marshal to {}, got %s", b2)
	}
}

func TestParseHookInput_Elicitation(t *testing.T) {
	input := HookInput{
		"session_id":        "sess-elicit",
		"transcript_path":   "/tmp/sess-elicit.jsonl",
		"cwd":               "/project",
		"permission_mode":   "default",
		"hook_event_name":   "Elicitation",
		"request_id":        "req-abc-123",
		"server_name":       "my-mcp-server",
		"message":           "Please provide your API key",
		"requestedSchema":   map[string]any{"type": "object", "properties": map[string]any{"api_key": map[string]any{"type": "string"}}},
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*ElicitationHookInput)
	if !ok {
		t.Fatalf("expected *ElicitationHookInput, got %T", result)
	}
	if m.SessionID != "sess-elicit" {
		t.Errorf("SessionID: got %q, want 'sess-elicit'", m.SessionID)
	}
	if m.RequestID != "req-abc-123" {
		t.Errorf("RequestID: got %q, want 'req-abc-123'", m.RequestID)
	}
	if m.ServerName != "my-mcp-server" {
		t.Errorf("ServerName: got %q, want 'my-mcp-server'", m.ServerName)
	}
	if m.Message != "Please provide your API key" {
		t.Errorf("Message: got %q", m.Message)
	}
	if m.RequestedSchema == nil {
		t.Error("RequestedSchema should not be nil")
	}
}

func TestParseHookInput_SessionStart(t *testing.T) {
	input := base("SessionStart")
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*SessionStartHookInput)
	if !ok {
		t.Fatalf("expected *SessionStartHookInput, got %T", result)
	}
	if m.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q, want 'sess-1'", m.SessionID)
	}
	if m.Source != "" {
		t.Errorf("Source: got %q, want empty when absent", m.Source)
	}
}

func TestParseHookInput_SessionStart_Fork(t *testing.T) {
	input := merge(base("SessionStart"), HookInput{"source": "fork"})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*SessionStartHookInput)
	if !ok {
		t.Fatalf("expected *SessionStartHookInput, got %T", result)
	}
	if m.Source != SessionStartSourceFork {
		t.Errorf("Source: got %q, want %q", m.Source, SessionStartSourceFork)
	}
}

func TestParseHookInput_SessionEnd(t *testing.T) {
	result, err := ParseHookInput(base("SessionEnd"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*SessionEndHookInput)
	if !ok {
		t.Fatalf("expected *SessionEndHookInput, got %T", result)
	}
	if m.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q, want 'sess-1'", m.SessionID)
	}
}

func TestSessionStartHookOutput_ToHookJSONOutput(t *testing.T) {
	out := SessionStartHookOutput{ReloadSkills: true, SessionTitle: "My Session"}
	j := out.ToHookJSONOutput()
	if j["reloadSkills"] != true {
		t.Errorf("reloadSkills: got %v, want true", j["reloadSkills"])
	}
	nested, ok := j["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("hookSpecificOutput should be map, got %T", j["hookSpecificOutput"])
	}
	if nested["sessionTitle"] != "My Session" {
		t.Errorf("sessionTitle: got %v, want 'My Session'", nested["sessionTitle"])
	}
}

func TestSessionStartHookOutput_Empty_ToHookJSONOutput(t *testing.T) {
	out := SessionStartHookOutput{}
	j := out.ToHookJSONOutput()
	if _, ok := j["reloadSkills"]; ok {
		t.Error("reloadSkills should be absent when false")
	}
	if _, ok := j["hookSpecificOutput"]; ok {
		t.Error("hookSpecificOutput should be absent when SessionTitle is empty")
	}
}

func TestParseHookInput_StopFailure(t *testing.T) {
	result, err := ParseHookInput(base("StopFailure"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*StopFailureHookInput)
	if !ok {
		t.Fatalf("expected *StopFailureHookInput, got %T", result)
	}
	if m.SessionID != "sess-1" {
		t.Errorf("SessionID: got %q, want 'sess-1'", m.SessionID)
	}
}

func TestParseHookInput_PostCompact(t *testing.T) {
	input := merge(base("PostCompact"), HookInput{
		"trigger":         "auto",
		"compact_summary": "Session compacted after 50k tokens.",
	})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*PostCompactHookInput)
	if !ok {
		t.Fatalf("expected *PostCompactHookInput, got %T", result)
	}
	if m.Trigger != "auto" {
		t.Errorf("Trigger: got %q, want 'auto'", m.Trigger)
	}
	if m.CompactSummary != "Session compacted after 50k tokens." {
		t.Errorf("CompactSummary: got %q", m.CompactSummary)
	}
}

func TestParseHookInput_PostToolBatch(t *testing.T) {
	input := merge(base("PostToolBatch"), HookInput{
		"tool_calls": []any{
			map[string]any{
				"tool_name":     "Bash",
				"tool_input":    map[string]any{"command": "ls"},
				"tool_use_id":   "toolu_1",
				"tool_response": map[string]any{"content": "file.txt"},
			},
			map[string]any{
				"tool_name":   "Read",
				"tool_input":  map[string]any{"path": "/README.md"},
				"tool_use_id": "toolu_2",
			},
		},
	})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*PostToolBatchHookInput)
	if !ok {
		t.Fatalf("expected *PostToolBatchHookInput, got %T", result)
	}
	if len(m.ToolCalls) != 2 {
		t.Fatalf("ToolCalls: got %d, want 2", len(m.ToolCalls))
	}
	if m.ToolCalls[0].ToolName != "Bash" {
		t.Errorf("ToolCalls[0].ToolName: got %q, want 'Bash'", m.ToolCalls[0].ToolName)
	}
	if m.ToolCalls[0].ToolUseID != "toolu_1" {
		t.Errorf("ToolCalls[0].ToolUseID: got %q, want 'toolu_1'", m.ToolCalls[0].ToolUseID)
	}
	if m.ToolCalls[1].ToolName != "Read" {
		t.Errorf("ToolCalls[1].ToolName: got %q, want 'Read'", m.ToolCalls[1].ToolName)
	}
}

func TestParseHookInput_PermissionDenied(t *testing.T) {
	input := merge(base("PermissionDenied"), HookInput{
		"tool_name":   "Bash",
		"tool_input":  map[string]any{"command": "rm -rf /"},
		"tool_use_id": "toolu_denied1",
		"reason":      "command modifies system files",
	})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*PermissionDeniedHookInput)
	if !ok {
		t.Fatalf("expected *PermissionDeniedHookInput, got %T", result)
	}
	if m.ToolName != "Bash" {
		t.Errorf("ToolName: got %q, want 'Bash'", m.ToolName)
	}
	if m.ToolUseID != "toolu_denied1" {
		t.Errorf("ToolUseID: got %q, want 'toolu_denied1'", m.ToolUseID)
	}
	if m.Reason != "command modifies system files" {
		t.Errorf("Reason: got %q", m.Reason)
	}
}

func TestPermissionDeniedHookOutput_ToHookJSONOutput(t *testing.T) {
	out := PermissionDeniedHookOutput{Retry: true}
	j := out.ToHookJSONOutput()
	if j["retry"] != true {
		t.Errorf("retry: got %v, want true", j["retry"])
	}
	empty := PermissionDeniedHookOutput{}
	j2 := empty.ToHookJSONOutput()
	if _, ok := j2["retry"]; ok {
		t.Error("retry should be absent when false")
	}
}

func TestParseHookInput_ElicitationResult(t *testing.T) {
	input := merge(base("ElicitationResult"), HookInput{
		"mcp_server_name": "my-mcp",
		"elicitation_id":  "elicit-42",
		"mode":            "form",
		"action":          "accept",
		"content":         map[string]any{"api_key": "sk-test"},
	})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*ElicitationResultHookInput)
	if !ok {
		t.Fatalf("expected *ElicitationResultHookInput, got %T", result)
	}
	if m.McpServerName != "my-mcp" {
		t.Errorf("McpServerName: got %q, want 'my-mcp'", m.McpServerName)
	}
	if m.Action != "accept" {
		t.Errorf("Action: got %q, want 'accept'", m.Action)
	}
	if m.Content["api_key"] != "sk-test" {
		t.Errorf("Content: got %v", m.Content)
	}
}

func TestParseHookInput_InstructionsLoaded(t *testing.T) {
	input := HookInput{
		"session_id":        "sess-il",
		"transcript_path":   "/tmp/sess-il.jsonl",
		"cwd":               "/project",
		"permission_mode":   "default",
		"hook_event_name":   "InstructionsLoaded",
		"file_path":         "/project/CLAUDE.md",
		"memory_type":       "Project",
		"load_reason":       "session_start",
		"globs":             []any{"*.go", "*.ts"},
		"trigger_file_path": "/project/CLAUDE.md",
		"parent_file_path":  "",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*InstructionsLoadedHookInput)
	if !ok {
		t.Fatalf("expected *InstructionsLoadedHookInput, got %T", result)
	}
	if m.FilePath != "/project/CLAUDE.md" {
		t.Errorf("FilePath: got %q", m.FilePath)
	}
	if m.MemoryType != "Project" {
		t.Errorf("MemoryType: got %q", m.MemoryType)
	}
	if m.LoadReason != "session_start" {
		t.Errorf("LoadReason: got %q", m.LoadReason)
	}
	if len(m.Globs) != 2 || m.Globs[0] != "*.go" || m.Globs[1] != "*.ts" {
		t.Errorf("Globs: got %v", m.Globs)
	}
	if m.TriggerFilePath != "/project/CLAUDE.md" {
		t.Errorf("TriggerFilePath: got %q", m.TriggerFilePath)
	}
}

func TestParseHookInput_CwdChanged(t *testing.T) {
	input := HookInput{
		"session_id":      "sess-cwd",
		"transcript_path": "/tmp/sess-cwd.jsonl",
		"cwd":             "/new/dir",
		"permission_mode": "default",
		"hook_event_name": "CwdChanged",
		"old_cwd":         "/old/dir",
		"new_cwd":         "/new/dir",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*CwdChangedHookInput)
	if !ok {
		t.Fatalf("expected *CwdChangedHookInput, got %T", result)
	}
	if m.OldCwd != "/old/dir" {
		t.Errorf("OldCwd: got %q", m.OldCwd)
	}
	if m.NewCwd != "/new/dir" {
		t.Errorf("NewCwd: got %q", m.NewCwd)
	}
}

func TestParseHookInput_FileChanged(t *testing.T) {
	input := HookInput{
		"session_id":      "sess-fc",
		"transcript_path": "/tmp/sess-fc.jsonl",
		"cwd":             "/project",
		"permission_mode": "default",
		"hook_event_name": "FileChanged",
		"file_path":       "/project/.env",
		"change_type":     "modified",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*FileChangedHookInput)
	if !ok {
		t.Fatalf("expected *FileChangedHookInput, got %T", result)
	}
	if m.FilePath != "/project/.env" {
		t.Errorf("FilePath: got %q", m.FilePath)
	}
	if m.ChangeType != "modified" {
		t.Errorf("ChangeType: got %q", m.ChangeType)
	}
}

func TestParseMessage_ElicitationComplete(t *testing.T) {
	data := map[string]any{
		"type":        "system",
		"subtype":     "elicitation_complete",
		"request_id":  "req-abc-123",
		"server_name": "my-mcp-server",
		"result":      map[string]any{"api_key": "sk-test-12345"},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*ElicitationCompleteMessage)
	if !ok {
		t.Fatalf("expected *ElicitationCompleteMessage, got %T", msg)
	}
	if m.RequestID != "req-abc-123" {
		t.Errorf("RequestID: got %q, want 'req-abc-123'", m.RequestID)
	}
	if m.ServerName != "my-mcp-server" {
		t.Errorf("ServerName: got %q, want 'my-mcp-server'", m.ServerName)
	}
	if m.Result == nil || m.Result["api_key"] != "sk-test-12345" {
		t.Errorf("Result: got %v", m.Result)
	}
	if m.Subtype != "elicitation_complete" {
		t.Errorf("Subtype: got %q, want 'elicitation_complete'", m.Subtype)
	}
}

func TestParseHookInput_MessageDisplay(t *testing.T) {
	input := HookInput{
		"session_id":       "sess-md",
		"transcript_path":  "/path/session.jsonl",
		"cwd":              "/home/user",
		"permission_mode":  "default",
		"hook_event_name":  "MessageDisplay",
		"turn_id":          "turn-001",
		"message_id":       "msg-xyz",
		"index":            float64(3),
		"final":            true,
		"delta":            "Hello, world!\n",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*MessageDisplayHookInput)
	if !ok {
		t.Fatalf("expected *MessageDisplayHookInput, got %T", result)
	}
	if m.SessionID != "sess-md" {
		t.Errorf("SessionID: got %q, want 'sess-md'", m.SessionID)
	}
	if m.TurnID != "turn-001" {
		t.Errorf("TurnID: got %q, want 'turn-001'", m.TurnID)
	}
	if m.MessageID != "msg-xyz" {
		t.Errorf("MessageID: got %q, want 'msg-xyz'", m.MessageID)
	}
	if m.Index != 3 {
		t.Errorf("Index: got %d, want 3", m.Index)
	}
	if !m.Final {
		t.Error("Final: expected true")
	}
	if m.Delta != "Hello, world!\n" {
		t.Errorf("Delta: got %q, want 'Hello, world!\\n'", m.Delta)
	}
}

func TestMessageDisplayHookOutput_ToHookJSONOutput(t *testing.T) {
	content := "transformed text"
	out := MessageDisplayHookOutput{DisplayContent: &content}
	j := out.ToHookJSONOutput()
	if j["displayContent"] != content {
		t.Errorf("displayContent: got %v, want %q", j["displayContent"], content)
	}

	empty := MessageDisplayHookOutput{}
	j2 := empty.ToHookJSONOutput()
	if _, ok := j2["displayContent"]; ok {
		t.Error("displayContent should be absent when nil")
	}
}

func TestParseHookInput_WorktreeCreate(t *testing.T) {
	input := HookInput{
		"session_id":       "sess-wt",
		"transcript_path":  "/tmp/sess-wt.jsonl",
		"cwd":              "/project",
		"permission_mode":  "default",
		"hook_event_name":  "WorktreeCreate",
		"worktree_name":    "feature-branch",
		"isolation_level":  "worktree",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*WorktreeCreateHookInput)
	if !ok {
		t.Fatalf("expected *WorktreeCreateHookInput, got %T", result)
	}
	if m.WorktreeName != "feature-branch" {
		t.Errorf("WorktreeName: got %q", m.WorktreeName)
	}
	if m.IsolationLevel != "worktree" {
		t.Errorf("IsolationLevel: got %q", m.IsolationLevel)
	}
}

func TestWorktreeCreateHookOutput_ToHookJSONOutput(t *testing.T) {
	o := WorktreeCreateHookOutput{WorktreePath: "/tmp/worktrees/feature-branch"}
	out := o.ToHookJSONOutput()
	specific, ok := out["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("hookSpecificOutput missing or wrong type: %v", out)
	}
	if specific["worktreePath"] != "/tmp/worktrees/feature-branch" {
		t.Errorf("worktreePath: got %v", specific["worktreePath"])
	}

	empty := WorktreeCreateHookOutput{}
	emptyOut := empty.ToHookJSONOutput()
	if len(emptyOut) != 0 {
		t.Errorf("empty output should produce empty map, got %v", emptyOut)
	}
}

func TestParseHookInput_WorktreeRemove(t *testing.T) {
	input := HookInput{
		"session_id":      "sess-wr",
		"transcript_path": "/tmp/sess-wr.jsonl",
		"cwd":             "/project",
		"permission_mode": "default",
		"hook_event_name": "WorktreeRemove",
		"worktree_path":   "/tmp/worktrees/feature-branch",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*WorktreeRemoveHookInput)
	if !ok {
		t.Fatalf("expected *WorktreeRemoveHookInput, got %T", result)
	}
	if m.WorktreePath != "/tmp/worktrees/feature-branch" {
		t.Errorf("WorktreePath: got %q", m.WorktreePath)
	}
}

func TestParseHookInput_UserPromptExpansion(t *testing.T) {
	input := HookInput{
		"session_id":      "sess-upe",
		"transcript_path": "/tmp/sess-upe.jsonl",
		"cwd":             "/project",
		"permission_mode": "default",
		"hook_event_name": "UserPromptExpansion",
		"expansion_type":  "slash_command",
		"command_name":    "build",
		"command_args":    "--release",
		"command_source":  "project",
		"prompt":          "/build --release",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*UserPromptExpansionHookInput)
	if !ok {
		t.Fatalf("expected *UserPromptExpansionHookInput, got %T", result)
	}
	if m.ExpansionType != "slash_command" {
		t.Errorf("ExpansionType: got %q", m.ExpansionType)
	}
	if m.CommandName != "build" {
		t.Errorf("CommandName: got %q", m.CommandName)
	}
	if m.CommandArgs != "--release" {
		t.Errorf("CommandArgs: got %q", m.CommandArgs)
	}
	if m.CommandSource != "project" {
		t.Errorf("CommandSource: got %q", m.CommandSource)
	}
	if m.Prompt != "/build --release" {
		t.Errorf("Prompt: got %q", m.Prompt)
	}
}

func TestParseHookInput_Setup(t *testing.T) {
	input := HookInput{
		"session_id":      "sess-setup",
		"transcript_path": "/tmp/sess-setup.jsonl",
		"cwd":             "/project",
		"permission_mode": "default",
		"hook_event_name": "Setup",
		"trigger":         "init",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*SetupHookInput)
	if !ok {
		t.Fatalf("expected *SetupHookInput, got %T", result)
	}
	if m.Trigger != "init" {
		t.Errorf("Trigger: got %q", m.Trigger)
	}
}

func TestParseHookInput_TaskCreated(t *testing.T) {
	input := HookInput{
		"session_id":       "sess-tc",
		"transcript_path":  "/tmp/sess-tc.jsonl",
		"cwd":              "/project",
		"permission_mode":  "default",
		"hook_event_name":  "TaskCreated",
		"task_name":        "Refactor auth module",
		"task_description": "Extract auth logic into separate package",
		"agent_id":         "agent-77",
		"agent_type":       "planner",
	}
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := result.(*TaskCreatedHookInput)
	if !ok {
		t.Fatalf("expected *TaskCreatedHookInput, got %T", result)
	}
	if m.TaskName != "Refactor auth module" {
		t.Errorf("TaskName: got %q", m.TaskName)
	}
	if m.TaskDescription != "Extract auth logic into separate package" {
		t.Errorf("TaskDescription: got %q", m.TaskDescription)
	}
	if m.AgentID != "agent-77" {
		t.Errorf("AgentID: got %q", m.AgentID)
	}
}

func TestBaseHookInput_PromptID_Present(t *testing.T) {
	// prompt_id is populated when the CLI includes it (after the first user turn).
	input := merge(base("PreToolUse"), HookInput{
		"tool_name":   "Bash",
		"tool_input":  map[string]any{"command": "echo hello"},
		"tool_use_id": "toolu_abc",
		"prompt_id":   "prompt-uuid-1234",
	})
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*PreToolUseHookInput)
	if typed.PromptID != "prompt-uuid-1234" {
		t.Errorf("PromptID: got %q, want %q", typed.PromptID, "prompt-uuid-1234")
	}
}

func TestBaseHookInput_PromptID_Absent(t *testing.T) {
	// prompt_id is absent before the first user turn (e.g. on SessionStart).
	input := base("SessionStart")
	result, err := ParseHookInput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	typed := result.(*SessionStartHookInput)
	if typed.PromptID != "" {
		t.Errorf("PromptID: got %q, want empty", typed.PromptID)
	}
}

func TestBaseHookInput_PromptID_JSONRoundTrip(t *testing.T) {
	// When prompt_id is present it must survive JSON marshal/unmarshal.
	raw := `{
		"session_id":      "sess-1",
		"transcript_path": "/tmp/t.jsonl",
		"cwd":             "/home/user",
		"permission_mode": "default",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      {"command": "ls"},
		"tool_use_id":     "toolu_xyz",
		"prompt_id":       "prompt-uuid-5678"
	}`

	var typed PreToolUseHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if typed.PromptID != "prompt-uuid-5678" {
		t.Errorf("PromptID: got %q, want 'prompt-uuid-5678'", typed.PromptID)
	}

	// Marshal back: prompt_id must appear in output when non-empty.
	out, err := json.Marshal(&typed)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("re-unmarshal error: %v", err)
	}
	if m["prompt_id"] != "prompt-uuid-5678" {
		t.Errorf("marshaled prompt_id: got %v, want 'prompt-uuid-5678'", m["prompt_id"])
	}
}

func TestBaseHookInput_PromptID_OmittedWhenEmpty(t *testing.T) {
	// When prompt_id is absent it must not appear in marshaled JSON (omitempty).
	raw := `{
		"session_id":      "sess-1",
		"transcript_path": "/tmp/t.jsonl",
		"cwd":             "/home/user",
		"permission_mode": "default",
		"hook_event_name": "Stop",
		"stop_hook_active": false
	}`
	var typed StopHookInput
	if err := json.Unmarshal([]byte(raw), &typed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	out, err := json.Marshal(&typed)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("re-unmarshal error: %v", err)
	}
	if _, ok := m["prompt_id"]; ok {
		t.Error("prompt_id should be omitted from JSON when empty")
	}
}

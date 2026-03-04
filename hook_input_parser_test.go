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
	json.Unmarshal(out, &m)
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
	json.Unmarshal(out, &m)
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

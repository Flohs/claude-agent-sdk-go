package claude

import (
	"testing"
)

func TestParseMessage_NilData(t *testing.T) {
	_, err := ParseMessage(nil)
	if err == nil {
		t.Fatal("expected error for nil data")
	}
	if _, ok := err.(*MessageParseError); !ok {
		t.Fatalf("expected MessageParseError, got %T", err)
	}
}

func TestParseMessage_MissingType(t *testing.T) {
	_, err := ParseMessage(map[string]any{"foo": "bar"})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
}

func TestParseMessage_UnknownType(t *testing.T) {
	msg, err := ParseMessage(map[string]any{"type": "future_type"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg != nil {
		t.Fatal("expected nil for unknown type")
	}
}

func TestParseMessage_UserMessage_StringContent(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hello world",
		},
		"uuid": "test-uuid",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if user.Content != "hello world" {
		t.Fatalf("expected content 'hello world', got %v", user.Content)
	}
	if user.UUID != "test-uuid" {
		t.Fatalf("expected uuid 'test-uuid', got %s", user.UUID)
	}
}

func TestParseMessage_AssistantMessage(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "claude-sonnet-4-5-20250514",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello!"},
				map[string]any{"type": "thinking", "thinking": "Let me think...", "signature": "sig123"},
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", msg)
	}
	if asst.Model != "claude-sonnet-4-5-20250514" {
		t.Fatalf("expected model 'claude-sonnet-4-5-20250514', got %s", asst.Model)
	}
	if len(asst.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(asst.Content))
	}

	textBlock, ok := asst.Content[0].(TextBlock)
	if !ok {
		t.Fatalf("expected TextBlock, got %T", asst.Content[0])
	}
	if textBlock.Text != "Hello!" {
		t.Fatalf("expected text 'Hello!', got %s", textBlock.Text)
	}

	thinkingBlock, ok := asst.Content[1].(ThinkingBlock)
	if !ok {
		t.Fatalf("expected ThinkingBlock, got %T", asst.Content[1])
	}
	if thinkingBlock.Thinking != "Let me think..." {
		t.Fatalf("expected thinking text, got %s", thinkingBlock.Thinking)
	}
}

func TestParseMessage_ToolUseBlock(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "test",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"id":    "tool-123",
					"name":  "Bash",
					"input": map[string]any{"command": "ls"},
				},
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst := msg.(*AssistantMessage)
	toolUse, ok := asst.Content[0].(ToolUseBlock)
	if !ok {
		t.Fatalf("expected ToolUseBlock, got %T", asst.Content[0])
	}
	if toolUse.Name != "Bash" {
		t.Fatalf("expected tool name 'Bash', got %s", toolUse.Name)
	}
	if toolUse.Input["command"] != "ls" {
		t.Fatalf("expected command 'ls', got %v", toolUse.Input["command"])
	}
}

func TestParseMessage_SystemMessage(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "init",
		"foo":     "bar",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sys, ok := msg.(*SystemMessage)
	if !ok {
		t.Fatalf("expected *SystemMessage, got %T", msg)
	}
	if sys.Subtype != "init" {
		t.Fatalf("expected subtype 'init', got %s", sys.Subtype)
	}
}

func TestParseMessage_TaskStarted(t *testing.T) {
	data := map[string]any{
		"type":        "system",
		"subtype":     "task_started",
		"task_id":     "t1",
		"description": "Running task",
		"uuid":        "u1",
		"session_id":  "s1",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	task, ok := msg.(*TaskStartedMessage)
	if !ok {
		t.Fatalf("expected *TaskStartedMessage, got %T", msg)
	}
	if task.TaskID != "t1" {
		t.Fatalf("expected task_id 't1', got %s", task.TaskID)
	}
	// TaskStartedMessage embeds SystemMessage
	if task.Subtype != "task_started" {
		t.Fatalf("expected subtype 'task_started', got %s", task.Subtype)
	}
}

func TestParseMessage_ResultMessage(t *testing.T) {
	cost := 0.05
	data := map[string]any{
		"type":           "result",
		"subtype":        "success",
		"duration_ms":    float64(1000),
		"duration_api_ms": float64(800),
		"is_error":       false,
		"num_turns":      float64(3),
		"session_id":     "sess-1",
		"total_cost_usd": cost,
		"result":         "done",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", msg)
	}
	if result.DurationMs != 1000 {
		t.Fatalf("expected duration_ms 1000, got %d", result.DurationMs)
	}
	if result.NumTurns != 3 {
		t.Fatalf("expected num_turns 3, got %d", result.NumTurns)
	}
	if result.TotalCostUSD == nil || *result.TotalCostUSD != cost {
		t.Fatalf("expected cost %f, got %v", cost, result.TotalCostUSD)
	}
}

func TestParseMessage_ResultMessage_StopReasonPresent(t *testing.T) {
	data := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"duration_ms": float64(500),
		"is_error":    false,
		"num_turns":   float64(1),
		"session_id":  "sess-1",
		"stop_reason": "end_turn",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := msg.(*ResultMessage)
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
}

func TestParseMessage_ResultMessage_StopReasonNull(t *testing.T) {
	data := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"duration_ms": float64(500),
		"is_error":    false,
		"num_turns":   float64(1),
		"session_id":  "sess-1",
		"stop_reason": nil,
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := msg.(*ResultMessage)
	if result.StopReason != "" {
		t.Errorf("StopReason = %q, want empty string for nil", result.StopReason)
	}
}

func TestParseMessage_ResultMessage_StopReasonAbsent(t *testing.T) {
	data := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"duration_ms": float64(500),
		"is_error":    false,
		"num_turns":   float64(1),
		"session_id":  "sess-1",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result := msg.(*ResultMessage)
	if result.StopReason != "" {
		t.Errorf("StopReason = %q, want empty string for absent field", result.StopReason)
	}
}

func TestParseMessage_StreamEvent(t *testing.T) {
	data := map[string]any{
		"type":       "stream_event",
		"uuid":       "u1",
		"session_id": "s1",
		"event":      map[string]any{"type": "content_block_delta"},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	event, ok := msg.(*StreamEvent)
	if !ok {
		t.Fatalf("expected *StreamEvent, got %T", msg)
	}
	if event.UUID != "u1" {
		t.Fatalf("expected uuid 'u1', got %s", event.UUID)
	}
}

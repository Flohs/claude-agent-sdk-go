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

func TestParseMessage_AssistantMessage_ServerToolUseAndResult(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "claude-opus-4-7",
			"content": []any{
				map[string]any{
					"type":  "server_tool_use",
					"id":    "srvtooluse_1",
					"name":  "web_search",
					"input": map[string]any{"query": "golang generics"},
				},
				map[string]any{
					"type":        "advisor_tool_result",
					"tool_use_id": "srvtooluse_1",
					"content":     map[string]any{"type": "web_search_result", "results": []any{}},
				},
			},
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	asst := msg.(*AssistantMessage)
	if len(asst.Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(asst.Content))
	}
	server, ok := asst.Content[0].(ServerToolUseBlock)
	if !ok {
		t.Fatalf("expected ServerToolUseBlock, got %T", asst.Content[0])
	}
	if server.Name != ServerToolWebSearch {
		t.Errorf("Name = %q, want %q", server.Name, ServerToolWebSearch)
	}
	result, ok := asst.Content[1].(ServerToolResultBlock)
	if !ok {
		t.Fatalf("expected ServerToolResultBlock, got %T", asst.Content[1])
	}
	if result.ToolUseID != "srvtooluse_1" {
		t.Errorf("ToolUseID = %q, want %q", result.ToolUseID, "srvtooluse_1")
	}
	if result.Content["type"] != "web_search_result" {
		t.Errorf("Content[type] = %v, want web_search_result", result.Content["type"])
	}
}

func TestParseMessage_AssistantMessage_TypedFields(t *testing.T) {
	data := map[string]any{
		"type":       "assistant",
		"session_id": "sess-123",
		"uuid":       "msg-uuid-abc",
		"message": map[string]any{
			"id":          "msg_01",
			"model":       "claude-opus-4-7",
			"stop_reason": "end_turn",
			"content": []any{
				map[string]any{"type": "text", "text": "done"},
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	asst := msg.(*AssistantMessage)

	if asst.MessageID != "msg_01" {
		t.Errorf("MessageID = %q, want msg_01", asst.MessageID)
	}
	if asst.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want sess-123", asst.SessionID)
	}
	if asst.UUID != "msg-uuid-abc" {
		t.Errorf("UUID = %q, want msg-uuid-abc", asst.UUID)
	}
	if asst.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want end_turn", asst.StopReason)
	}
}

func TestParseMessage_AssistantMessage_WithUsage(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "claude-sonnet-4-5-20250514",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello!"},
			},
			"usage": map[string]any{
				"input_tokens":  float64(100),
				"output_tokens": float64(50),
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
	if asst.Usage == nil {
		t.Fatal("expected usage to be set")
	}
	if asst.Usage["input_tokens"] != float64(100) {
		t.Errorf("expected input_tokens=100, got %v", asst.Usage["input_tokens"])
	}
	if asst.Usage["output_tokens"] != float64(50) {
		t.Errorf("expected output_tokens=50, got %v", asst.Usage["output_tokens"])
	}
}

func TestParseMessage_AssistantMessage_UsageAbsent(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "claude-sonnet-4-5-20250514",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello!"},
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst := msg.(*AssistantMessage)
	if asst.Usage != nil {
		t.Errorf("expected Usage to be nil when absent, got %v", asst.Usage)
	}
}

func TestParseMessage_AssistantMessage_UsageNull(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model":   "claude-sonnet-4-5-20250514",
			"content": []any{map[string]any{"type": "text", "text": "Hi"}},
			"usage":   nil,
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst := msg.(*AssistantMessage)
	if asst.Usage != nil {
		t.Errorf("expected Usage to be nil for null value, got %v", asst.Usage)
	}
}

func TestParseMessage_AssistantMessage_UsageWrongType(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model":   "claude-sonnet-4-5-20250514",
			"content": []any{map[string]any{"type": "text", "text": "Hi"}},
			"usage":   "not-a-map",
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asst := msg.(*AssistantMessage)
	if asst.Usage != nil {
		t.Errorf("expected Usage to be nil for wrong type, got %v", asst.Usage)
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

func TestParseMessage_TaskProgress_Summary(t *testing.T) {
	data := map[string]any{
		"type":        "system",
		"subtype":     "task_progress",
		"task_id":     "t1",
		"description": "Reading files",
		"usage": map[string]any{
			"total_tokens": float64(1234),
			"tool_uses":    float64(5),
			"duration_ms":  float64(4200),
		},
		"uuid":           "u1",
		"session_id":     "s1",
		"last_tool_name": "Read",
		"summary":        "Inspecting the transport layer to diagnose the stdin close race.",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	progress, ok := msg.(*TaskProgressMessage)
	if !ok {
		t.Fatalf("expected *TaskProgressMessage, got %T", msg)
	}
	if progress.Summary != "Inspecting the transport layer to diagnose the stdin close race." {
		t.Errorf("Summary = %q, want the full summary", progress.Summary)
	}
	if progress.LastToolName != "Read" {
		t.Errorf("LastToolName = %q, want Read", progress.LastToolName)
	}
	if progress.Usage.TotalTokens != 1234 {
		t.Errorf("Usage.TotalTokens = %d, want 1234", progress.Usage.TotalTokens)
	}
}

func TestParseMessage_TaskProgress_SummaryAbsent(t *testing.T) {
	data := map[string]any{
		"type":       "system",
		"subtype":    "task_progress",
		"task_id":    "t1",
		"uuid":       "u1",
		"session_id": "s1",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	progress := msg.(*TaskProgressMessage)
	if progress.Summary != "" {
		t.Errorf("Summary = %q, want empty when absent", progress.Summary)
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

func TestParseMessage_ResultMessage_TerminalReason(t *testing.T) {
	data := map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     1000,
		"duration_api_ms": 900,
		"is_error":        false,
		"num_turns":       1,
		"session_id":      "s",
		"terminal_reason": "max_turns",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := msg.(*ResultMessage)
	if result.TerminalReason != "max_turns" {
		t.Errorf("TerminalReason = %q, want max_turns", result.TerminalReason)
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

func TestParseMessage_RateLimitEvent(t *testing.T) {
	utilization := float64(0.85)
	data := map[string]any{
		"type":       "rate_limit_event",
		"uuid":       "rl-uuid-1",
		"session_id": "sess-rl-1",
		"rate_limit_info": map[string]any{
			"status":          "allowed_warning",
			"resets_at":       "2026-03-20T12:00:00Z",
			"rate_limit_type": "token",
			"utilization":     utilization,
			"overage_status":  "active",
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	event, ok := msg.(*RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", msg)
	}
	if event.Type != "rate_limit_event" {
		t.Errorf("expected type 'rate_limit_event', got %s", event.Type)
	}
	if event.UUID != "rl-uuid-1" {
		t.Errorf("expected UUID 'rl-uuid-1', got %s", event.UUID)
	}
	if event.SessionID != "sess-rl-1" {
		t.Errorf("expected SessionID 'sess-rl-1', got %s", event.SessionID)
	}
	if event.RateLimitInfo.Status != RateLimitStatusAllowedWarning {
		t.Errorf("expected status 'allowed_warning', got %s", event.RateLimitInfo.Status)
	}
	if event.RateLimitInfo.ResetsAt == nil || *event.RateLimitInfo.ResetsAt != "2026-03-20T12:00:00Z" {
		t.Errorf("expected resets_at '2026-03-20T12:00:00Z', got %v", event.RateLimitInfo.ResetsAt)
	}
	if event.RateLimitInfo.RateLimitType == nil || *event.RateLimitInfo.RateLimitType != "token" {
		t.Errorf("expected rate_limit_type 'token', got %v", event.RateLimitInfo.RateLimitType)
	}
	if event.RateLimitInfo.Utilization == nil || *event.RateLimitInfo.Utilization != 0.85 {
		t.Errorf("expected utilization 0.85, got %v", event.RateLimitInfo.Utilization)
	}
	if event.RateLimitInfo.OverageStatus == nil || *event.RateLimitInfo.OverageStatus != "active" {
		t.Errorf("expected overage_status 'active', got %v", event.RateLimitInfo.OverageStatus)
	}
	if event.RateLimitInfo.OverageResetsAt != nil {
		t.Errorf("expected overage_resets_at nil, got %v", event.RateLimitInfo.OverageResetsAt)
	}
}

func TestParseMessage_RateLimitEvent_Minimal(t *testing.T) {
	data := map[string]any{
		"type": "rate_limit_event",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	event, ok := msg.(*RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", msg)
	}
	if event.Type != "rate_limit_event" {
		t.Errorf("expected type 'rate_limit_event', got %s", event.Type)
	}
	if event.RateLimitInfo.Status != "" {
		t.Errorf("expected empty status for minimal event, got %s", event.RateLimitInfo.Status)
	}
}

func TestParseMessage_RateLimitEvent_ImplementsMessage(t *testing.T) {
	data := map[string]any{
		"type": "rate_limit_event",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify it can be used as a Message interface
	m := Message(msg)
	if m == nil {
		t.Fatal("expected non-nil Message")
	}

	// Verify type switch works
	switch m.(type) {
	case *RateLimitEvent:
		// expected
	default:
		t.Errorf("expected *RateLimitEvent in type switch, got %T", m)
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

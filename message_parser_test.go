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
	if user.IsMeta {
		t.Fatalf("expected IsMeta false when absent, got true")
	}
}

func TestParseMessage_UserMessage_IsMeta(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "synthetic",
		},
		"isMeta": true,
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if !user.IsMeta {
		t.Fatalf("expected IsMeta true, got false")
	}
}

func TestParseMessage_UserMessage_ToolResultMeta(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "denied",
		},
		"tool_result_meta": map[string]any{
			"non_execution_kind": "denied",
			"user_feedback":      "not now, please ask again later",
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if user.ToolResultMeta == nil {
		t.Fatal("ToolResultMeta = nil, want non-nil")
	}
	if user.ToolResultMeta.NonExecutionKind != "denied" {
		t.Errorf("ToolResultMeta.NonExecutionKind = %q, want %q", user.ToolResultMeta.NonExecutionKind, "denied")
	}
	if user.ToolResultMeta.UserFeedback != "not now, please ask again later" {
		t.Errorf("ToolResultMeta.UserFeedback = %q, want %q", user.ToolResultMeta.UserFeedback, "not now, please ask again later")
	}
}

func TestParseMessage_UserMessage_ToolResultMeta_Absent(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hello world",
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if user.ToolResultMeta != nil {
		t.Errorf("ToolResultMeta = %+v, want nil", user.ToolResultMeta)
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
	if progress.Blocked {
		t.Errorf("Blocked = true, want false when absent")
	}
}

func TestParseMessage_TaskProgress_Blocked(t *testing.T) {
	data := map[string]any{
		"type":       "system",
		"subtype":    "task_progress",
		"task_id":    "t1",
		"uuid":       "u1",
		"session_id": "s1",
		"blocked":    true,
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	progress := msg.(*TaskProgressMessage)
	if !progress.Blocked {
		t.Errorf("Blocked = false, want true")
	}
}

func TestParseMessage_ResultMessage(t *testing.T) {
	cost := 0.05
	data := map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     float64(1000),
		"duration_api_ms": float64(800),
		"is_error":        false,
		"num_turns":       float64(3),
		"session_id":      "sess-1",
		"total_cost_usd":  cost,
		"result":          "done",
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

func TestParseMessage_ResultMessage_ModelUsage(t *testing.T) {
	data := map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     1000,
		"duration_api_ms": 900,
		"is_error":        false,
		"num_turns":       1,
		"session_id":      "s",
		"modelUsage": map[string]any{
			"claude-opus-4-8": map[string]any{
				"inputTokens":              float64(100),
				"outputTokens":             float64(50),
				"cacheReadInputTokens":     float64(10),
				"cacheCreationInputTokens": float64(5),
				"webSearchRequests":        float64(2),
				"costUSD":                  0.25,
				"contextWindow":            float64(200000),
				"maxOutputTokens":          float64(8192),
				"canonicalModel":           "claude-opus-4-7",
				"provider":                 "firstParty",
			},
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	result := msg.(*ResultMessage)
	u, ok := result.ModelUsage["claude-opus-4-8"]
	if !ok {
		t.Fatalf("expected model usage entry for claude-opus-4-8, got %v", result.ModelUsage)
	}
	if u.InputTokens != 100 || u.OutputTokens != 50 || u.CacheReadInputTokens != 10 ||
		u.CacheCreationInputTokens != 5 || u.WebSearchRequests != 2 || u.CostUSD != 0.25 ||
		u.ContextWindow != 200000 || u.MaxOutputTokens != 8192 ||
		u.CanonicalModel != "claude-opus-4-7" || u.Provider != "firstParty" {
		t.Errorf("unexpected ModelUsage entry: %+v", u)
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

func TestParseMessage_ToolProgress(t *testing.T) {
	errorStatus := 503
	data := map[string]any{
		"type":                 "tool_progress",
		"tool_use_id":          "toolu_1",
		"tool_name":            "Task",
		"parent_tool_use_id":   "toolu_parent",
		"elapsed_time_seconds": 12.5,
		"task_id":              "task-1",
		"uuid":                 "tp-uuid-1",
		"session_id":           "sess-tp-1",
		"heartbeat":            true,
		"subagent_type":        "general-purpose",
		"subagent_retry": map[string]any{
			"agent_id":       "agent-1",
			"attempt":        2,
			"max_retries":    5,
			"retry_delay_ms": 1000,
			"error_status":   float64(errorStatus),
			"error_category": "overloaded",
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tp, ok := msg.(*ToolProgressMessage)
	if !ok {
		t.Fatalf("expected *ToolProgressMessage, got %T", msg)
	}
	if tp.ToolUseID != "toolu_1" {
		t.Errorf("expected ToolUseID 'toolu_1', got %s", tp.ToolUseID)
	}
	if tp.ToolName != "Task" {
		t.Errorf("expected ToolName 'Task', got %s", tp.ToolName)
	}
	if tp.ParentToolUseID == nil || *tp.ParentToolUseID != "toolu_parent" {
		t.Errorf("expected ParentToolUseID 'toolu_parent', got %v", tp.ParentToolUseID)
	}
	if tp.ElapsedTimeSeconds != 12.5 {
		t.Errorf("expected ElapsedTimeSeconds 12.5, got %v", tp.ElapsedTimeSeconds)
	}
	if tp.TaskID != "task-1" {
		t.Errorf("expected TaskID 'task-1', got %s", tp.TaskID)
	}
	if tp.UUID != "tp-uuid-1" {
		t.Errorf("expected UUID 'tp-uuid-1', got %s", tp.UUID)
	}
	if tp.SessionID != "sess-tp-1" {
		t.Errorf("expected SessionID 'sess-tp-1', got %s", tp.SessionID)
	}
	if !tp.Heartbeat {
		t.Error("expected Heartbeat true")
	}
	if tp.SubagentType != "general-purpose" {
		t.Errorf("expected SubagentType 'general-purpose', got %s", tp.SubagentType)
	}
	if tp.SubagentRetry == nil {
		t.Fatal("expected non-nil SubagentRetry")
	}
	if tp.SubagentRetry.AgentID != "agent-1" {
		t.Errorf("expected AgentID 'agent-1', got %s", tp.SubagentRetry.AgentID)
	}
	if tp.SubagentRetry.Attempt != 2 {
		t.Errorf("expected Attempt 2, got %d", tp.SubagentRetry.Attempt)
	}
	if tp.SubagentRetry.MaxRetries != 5 {
		t.Errorf("expected MaxRetries 5, got %d", tp.SubagentRetry.MaxRetries)
	}
	if tp.SubagentRetry.RetryDelayMs != 1000 {
		t.Errorf("expected RetryDelayMs 1000, got %d", tp.SubagentRetry.RetryDelayMs)
	}
	if tp.SubagentRetry.ErrorStatus == nil || *tp.SubagentRetry.ErrorStatus != errorStatus {
		t.Errorf("expected ErrorStatus %d, got %v", errorStatus, tp.SubagentRetry.ErrorStatus)
	}
	if tp.SubagentRetry.ErrorCategory != "overloaded" {
		t.Errorf("expected ErrorCategory 'overloaded', got %s", tp.SubagentRetry.ErrorCategory)
	}
}

func TestParseMessage_ToolProgress_Minimal(t *testing.T) {
	data := map[string]any{
		"type":                 "tool_progress",
		"tool_use_id":          "toolu_2",
		"tool_name":            "Bash",
		"parent_tool_use_id":   nil,
		"elapsed_time_seconds": 0.5,
		"uuid":                 "tp-uuid-2",
		"session_id":           "sess-tp-2",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tp, ok := msg.(*ToolProgressMessage)
	if !ok {
		t.Fatalf("expected *ToolProgressMessage, got %T", msg)
	}
	if tp.ToolUseID != "toolu_2" {
		t.Errorf("expected ToolUseID 'toolu_2', got %s", tp.ToolUseID)
	}
	if tp.ToolName != "Bash" {
		t.Errorf("expected ToolName 'Bash', got %s", tp.ToolName)
	}
	if tp.ParentToolUseID != nil {
		t.Errorf("expected nil ParentToolUseID, got %v", *tp.ParentToolUseID)
	}
	if tp.ElapsedTimeSeconds != 0.5 {
		t.Errorf("expected ElapsedTimeSeconds 0.5, got %v", tp.ElapsedTimeSeconds)
	}
	if tp.TaskID != "" {
		t.Errorf("expected empty TaskID, got %s", tp.TaskID)
	}
	if tp.Heartbeat {
		t.Error("expected Heartbeat false")
	}
	if tp.SubagentType != "" {
		t.Errorf("expected empty SubagentType, got %s", tp.SubagentType)
	}
	if tp.SubagentRetry != nil {
		t.Errorf("expected nil SubagentRetry, got %v", tp.SubagentRetry)
	}
}

func TestParseMessage_ToolProgress_ImplementsMessage(t *testing.T) {
	data := map[string]any{
		"type":                 "tool_progress",
		"tool_use_id":          "toolu_3",
		"tool_name":            "Read",
		"elapsed_time_seconds": 1.0,
		"uuid":                 "tp-uuid-3",
		"session_id":           "sess-tp-3",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := Message(msg)
	if m == nil {
		t.Fatal("expected non-nil Message")
	}

	switch m.(type) {
	case *ToolProgressMessage:
		// expected
	default:
		t.Errorf("expected *ToolProgressMessage in type switch, got %T", m)
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

func TestParseMessage_MirrorErrorMessage(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "mirror_error",
		"error":   "append failed: network reset",
		"uuid":    "err-uuid-123",
		"key": map[string]any{
			"project_key": "my-project",
			"session_id":  "abcd",
			"subpath":     "",
		},
		"session_id": "abcd",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	me, ok := msg.(*MirrorErrorMessage)
	if !ok {
		t.Fatalf("expected *MirrorErrorMessage, got %T", msg)
	}
	if me.Error != "append failed: network reset" {
		t.Errorf("Error = %q", me.Error)
	}
	if me.UUID != "err-uuid-123" {
		t.Errorf("UUID = %q", me.UUID)
	}
	if me.SessionID != "abcd" {
		t.Errorf("SessionID = %q", me.SessionID)
	}
	if me.Key == nil {
		t.Fatal("expected Key to be set")
	}
	if me.Key.ProjectKey != "my-project" || me.Key.SessionID != "abcd" {
		t.Errorf("Key = %+v", me.Key)
	}
	if me.Subtype != "mirror_error" {
		t.Errorf("Subtype = %q", me.Subtype)
	}
}

func TestParseMessage_MirrorErrorMessage_NullKey(t *testing.T) {
	// When the key is absent/nil, the Key pointer should remain nil and the
	// rest of the fields should populate.
	data := map[string]any{
		"type":       "system",
		"subtype":    "mirror_error",
		"error":      "adapter unavailable",
		"uuid":       "err-xyz",
		"session_id": "",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	me, ok := msg.(*MirrorErrorMessage)
	if !ok {
		t.Fatalf("expected *MirrorErrorMessage, got %T", msg)
	}
	if me.Key != nil {
		t.Errorf("expected nil Key, got %+v", me.Key)
	}
	if me.Error != "adapter unavailable" {
		t.Errorf("Error = %q", me.Error)
	}
}

func TestParseMessage_BackgroundTasksChangedMessage(t *testing.T) {
	data := map[string]any{
		"type":       "system",
		"subtype":    "background_tasks_changed",
		"uuid":       "btc-uuid-1",
		"session_id": "sess-1",
		"tasks": []any{
			map[string]any{
				"task_id":     "task-1",
				"task_type":   "agent",
				"description": "Refactor the parser",
			},
			map[string]any{
				"task_id":     "task-2",
				"task_type":   "workflow",
				"description": "Run the test suite",
			},
		},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	btc, ok := msg.(*BackgroundTasksChangedMessage)
	if !ok {
		t.Fatalf("expected *BackgroundTasksChangedMessage, got %T", msg)
	}
	if btc.UUID != "btc-uuid-1" {
		t.Errorf("UUID = %q", btc.UUID)
	}
	if btc.SessionID != "sess-1" {
		t.Errorf("SessionID = %q", btc.SessionID)
	}
	if len(btc.Tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(btc.Tasks))
	}
	want := []BackgroundTaskInfo{
		{TaskID: "task-1", TaskType: "agent", Description: "Refactor the parser"},
		{TaskID: "task-2", TaskType: "workflow", Description: "Run the test suite"},
	}
	for i, w := range want {
		if btc.Tasks[i] != w {
			t.Errorf("Tasks[%d] = %+v, want %+v", i, btc.Tasks[i], w)
		}
	}
	if btc.Subtype != "background_tasks_changed" {
		t.Errorf("Subtype = %q", btc.Subtype)
	}
}

func TestParseMessage_BackgroundTasksChangedMessage_EmptyTasks(t *testing.T) {
	// An empty tasks list is the "no more background work" signal and must
	// round-trip as an empty (not nil) slice being acceptable either way —
	// the important thing is no error and no leftover tasks.
	data := map[string]any{
		"type":       "system",
		"subtype":    "background_tasks_changed",
		"uuid":       "btc-uuid-2",
		"session_id": "sess-1",
		"tasks":      []any{},
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	btc, ok := msg.(*BackgroundTasksChangedMessage)
	if !ok {
		t.Fatalf("expected *BackgroundTasksChangedMessage, got %T", msg)
	}
	if len(btc.Tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(btc.Tasks))
	}
}

func TestParseMessage_CommandLifecycleMessage(t *testing.T) {
	states := []CommandLifecycleState{
		CommandLifecycleStateQueued,
		CommandLifecycleStateStarted,
		CommandLifecycleStateCompleted,
		CommandLifecycleStateCancelled,
		CommandLifecycleStateDiscarded,
	}
	for _, state := range states {
		t.Run(string(state), func(t *testing.T) {
			data := map[string]any{
				"type":       "system",
				"subtype":    "command_lifecycle",
				"uuid":       "cmd-uuid-1",
				"session_id": "sess-1",
				"state":      string(state),
			}

			msg, err := ParseMessage(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			cl, ok := msg.(*CommandLifecycleMessage)
			if !ok {
				t.Fatalf("expected *CommandLifecycleMessage, got %T", msg)
			}
			if cl.CommandUUID != "cmd-uuid-1" {
				t.Errorf("CommandUUID = %q", cl.CommandUUID)
			}
			if cl.SessionID != "sess-1" {
				t.Errorf("SessionID = %q", cl.SessionID)
			}
			if cl.State != state {
				t.Errorf("State = %q, want %q", cl.State, state)
			}
			// Must still satisfy the Message interface via the embedded
			// SystemMessage, matching every other typed system subtype.
			var _ Message = cl
		})
	}
}

func TestParseMessage_CommandLifecycleMessage_MinimalPayload(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "command_lifecycle",
		"uuid":    "cmd-uuid-2",
		"state":   "queued",
	}

	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cl, ok := msg.(*CommandLifecycleMessage)
	if !ok {
		t.Fatalf("expected *CommandLifecycleMessage, got %T", msg)
	}
	if cl.SessionID != "" {
		t.Errorf("expected empty SessionID, got %q", cl.SessionID)
	}
	if cl.State != CommandLifecycleStateQueued {
		t.Errorf("State = %q", cl.State)
	}
}

// ---------------------------------------------------------------------------
// Coverage for [Unreleased] message-parser additions (#204):
// AssistantMessage.RequestID, ResultMessage.Origin / RequestID /
// APIErrorStatus / DeferredToolUse, TaskStartedMessage.SubagentType /
// TaskDescription, TaskNotificationMessage.SubagentType / TaskDescription.
// ---------------------------------------------------------------------------

func TestParseMessage_AssistantMessage_RequestID(t *testing.T) {
	data := map[string]any{
		"type":       "assistant",
		"session_id": "sess-1",
		"request_id": "req_abc123",
		"message": map[string]any{
			"model":   "claude-opus-4-7",
			"content": []any{map[string]any{"type": "text", "text": "ok"}},
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	asst := msg.(*AssistantMessage)
	if asst.RequestID != "req_abc123" {
		t.Errorf("RequestID = %q, want req_abc123", asst.RequestID)
	}
}

func TestParseMessage_ResultMessage_Origin(t *testing.T) {
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
		"origin": map[string]any{
			"kind":    "task-notification",
			"subkind": "scheduled-trigger",
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.Origin == nil {
		t.Fatal("Origin = nil, want non-nil")
	}
	if r.Origin.Kind != MessageOriginKindTaskNotification {
		t.Errorf("Origin.Kind = %q, want %q", r.Origin.Kind, MessageOriginKindTaskNotification)
	}
	if r.Origin.Subkind != "scheduled-trigger" {
		t.Errorf("Origin.Subkind = %q, want scheduled-trigger", r.Origin.Subkind)
	}
}

func TestParseMessage_ResultMessage_Origin_Channel(t *testing.T) {
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
		"origin": map[string]any{
			"kind":   "channel",
			"server": "slack-workspace-1",
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.Origin == nil {
		t.Fatal("Origin = nil, want non-nil")
	}
	if r.Origin.Kind != MessageOriginKindChannel {
		t.Errorf("Origin.Kind = %q, want %q", r.Origin.Kind, MessageOriginKindChannel)
	}
	if r.Origin.Server != "slack-workspace-1" {
		t.Errorf("Origin.Server = %q, want slack-workspace-1", r.Origin.Server)
	}
}

func TestParseMessage_ResultMessage_Origin_Absent(t *testing.T) {
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.Origin != nil {
		t.Errorf("Origin = %+v, want nil", r.Origin)
	}
}

func TestParseMessage_UserMessage_Origin_Human(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hi",
		},
		"origin": map[string]any{
			"kind": "human",
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	u := msg.(*UserMessage)
	if u.Origin == nil {
		t.Fatal("Origin = nil, want non-nil")
	}
	if u.Origin.Kind != MessageOriginKindHuman {
		t.Errorf("Origin.Kind = %q, want %q", u.Origin.Kind, MessageOriginKindHuman)
	}
}

func TestParseMessage_ResultMessage_RequestID(t *testing.T) {
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
		"request_id": "req_final_xyz",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.RequestID != "req_final_xyz" {
		t.Errorf("RequestID = %q, want req_final_xyz", r.RequestID)
	}
}

func TestParseMessage_ResultMessage_APIErrorStatus(t *testing.T) {
	// is_error result carries an HTTP status (e.g. 429, 529).
	data := map[string]any{
		"type":             "result",
		"subtype":          "error",
		"is_error":         true,
		"session_id":       "s",
		"api_error_status": float64(429),
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.APIErrorStatus == nil {
		t.Fatal("APIErrorStatus is nil")
	}
	if *r.APIErrorStatus != 429 {
		t.Errorf("*APIErrorStatus = %d, want 429", *r.APIErrorStatus)
	}
}

func TestParseMessage_ResultMessage_APIErrorStatus_Absent(t *testing.T) {
	// Successful result: api_error_status not emitted by the CLI.
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.APIErrorStatus != nil {
		t.Errorf("APIErrorStatus = %v, want nil when absent", *r.APIErrorStatus)
	}
}

func TestParseMessage_ResultMessage_DeferredToolUse(t *testing.T) {
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
		"deferred_tool_use": map[string]any{
			"tool_use_id": "tu_123",
			"tool_name":   "Bash",
			"tool_input":  map[string]any{"command": "rm -rf /tmp/x"},
		},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.DeferredToolUse == nil {
		t.Fatal("DeferredToolUse is nil")
	}
	if r.DeferredToolUse.ToolUseID != "tu_123" {
		t.Errorf("DeferredToolUse.ToolUseID = %q", r.DeferredToolUse.ToolUseID)
	}
	if r.DeferredToolUse.ToolName != "Bash" {
		t.Errorf("DeferredToolUse.ToolName = %q", r.DeferredToolUse.ToolName)
	}
	if cmd, _ := r.DeferredToolUse.ToolInput["command"].(string); cmd != "rm -rf /tmp/x" {
		t.Errorf("DeferredToolUse.ToolInput[command] = %v", r.DeferredToolUse.ToolInput["command"])
	}
}

func TestParseMessage_ResultMessage_DeferredToolUse_Absent(t *testing.T) {
	// Normal (non-defer) result: DeferredToolUse stays nil.
	data := map[string]any{
		"type":       "result",
		"subtype":    "success",
		"is_error":   false,
		"session_id": "s",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.DeferredToolUse != nil {
		t.Errorf("expected nil DeferredToolUse, got %+v", r.DeferredToolUse)
	}
}

func TestParseMessage_ResultMessage_UserMessageUUIDAndRequestSentWallMs(t *testing.T) {
	// Success result carries the triggering user message uuid and the
	// wall-clock send timestamp for cross-host request-latency correlation.
	data := map[string]any{
		"type":                 "result",
		"subtype":              "success",
		"is_error":             false,
		"session_id":           "s",
		"user_message_uuid":    "um_123",
		"request_sent_wall_ms": float64(1700000000123),
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.UserMessageUUID != "um_123" {
		t.Errorf("UserMessageUUID = %q, want %q", r.UserMessageUUID, "um_123")
	}
	if r.RequestSentWallMs == nil {
		t.Fatal("RequestSentWallMs is nil")
	}
	if *r.RequestSentWallMs != 1700000000123 {
		t.Errorf("*RequestSentWallMs = %d, want 1700000000123", *r.RequestSentWallMs)
	}
}

func TestParseMessage_ResultMessage_UserMessageUUIDAndRequestSentWallMs_Absent(t *testing.T) {
	// Non-success subtypes (and older CLIs) omit these fields; they stay
	// zero-value.
	data := map[string]any{
		"type":       "result",
		"subtype":    "error_max_turns",
		"is_error":   true,
		"session_id": "s",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r := msg.(*ResultMessage)
	if r.UserMessageUUID != "" {
		t.Errorf("UserMessageUUID = %q, want empty when absent", r.UserMessageUUID)
	}
	if r.RequestSentWallMs != nil {
		t.Errorf("RequestSentWallMs = %v, want nil when absent", *r.RequestSentWallMs)
	}
}

func TestParseMessage_TaskStarted_SubagentTypeAndDescription(t *testing.T) {
	data := map[string]any{
		"type":             "system",
		"subtype":          "task_started",
		"task_id":          "t1",
		"description":      "Running task",
		"uuid":             "u1",
		"session_id":       "s1",
		"subagent_type":    "general-purpose",
		"task_description": "Find all callers of foo()",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	task := msg.(*TaskStartedMessage)
	if task.SubagentType != "general-purpose" {
		t.Errorf("SubagentType = %q, want general-purpose", task.SubagentType)
	}
	if task.TaskDescription != "Find all callers of foo()" {
		t.Errorf("TaskDescription = %q", task.TaskDescription)
	}
}

func TestParseMessage_TaskProgress_SubagentTypeAndDescription(t *testing.T) {
	data := map[string]any{
		"type":             "system",
		"subtype":          "task_progress",
		"task_id":          "t1",
		"uuid":             "u1",
		"session_id":       "s1",
		"subagent_type":    "explore",
		"task_description": "Explore the codebase",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := msg.(*TaskProgressMessage)
	if p.SubagentType != "explore" {
		t.Errorf("SubagentType = %q, want explore", p.SubagentType)
	}
	if p.TaskDescription != "Explore the codebase" {
		t.Errorf("TaskDescription = %q", p.TaskDescription)
	}
}

func TestParseMessage_ApiRetry(t *testing.T) {
	status := 429
	data := map[string]any{
		"type":           "system",
		"subtype":        "api_retry",
		"attempt_number": float64(2),
		"max_attempts":   float64(5),
		"delay_ms":       float64(1000),
		"error_status":   float64(429),
		"error_message":  "Too Many Requests",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*ApiRetryMessage)
	if !ok {
		t.Fatalf("expected *ApiRetryMessage, got %T", msg)
	}
	if m.AttemptNumber != 2 {
		t.Errorf("AttemptNumber: got %d, want 2", m.AttemptNumber)
	}
	if m.MaxAttempts != 5 {
		t.Errorf("MaxAttempts: got %d, want 5", m.MaxAttempts)
	}
	if m.DelayMs != 1000 {
		t.Errorf("DelayMs: got %d, want 1000", m.DelayMs)
	}
	if m.ErrorStatus == nil || *m.ErrorStatus != status {
		t.Errorf("ErrorStatus: got %v, want %d", m.ErrorStatus, status)
	}
	if m.ErrorMessage != "Too Many Requests" {
		t.Errorf("ErrorMessage: got %q, want 'Too Many Requests'", m.ErrorMessage)
	}
	if m.Subtype != "api_retry" {
		t.Errorf("Subtype: got %q, want 'api_retry'", m.Subtype)
	}
}

func TestParseMessage_ApiRetry_NoErrorStatus(t *testing.T) {
	data := map[string]any{
		"type":           "system",
		"subtype":        "api_retry",
		"attempt_number": float64(1),
		"max_attempts":   float64(3),
		"delay_ms":       float64(500),
		"error_message":  "connection reset by peer",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*ApiRetryMessage)
	if !ok {
		t.Fatalf("expected *ApiRetryMessage, got %T", msg)
	}
	if m.ErrorStatus != nil {
		t.Errorf("ErrorStatus: got %v, want nil", m.ErrorStatus)
	}
	if m.AttemptNumber != 1 {
		t.Errorf("AttemptNumber: got %d, want 1", m.AttemptNumber)
	}
	if m.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: got %d, want 3", m.MaxAttempts)
	}
	if m.DelayMs != 500 {
		t.Errorf("DelayMs: got %d, want 500", m.DelayMs)
	}
	if m.ErrorMessage != "connection reset by peer" {
		t.Errorf("ErrorMessage: got %q, want 'connection reset by peer'", m.ErrorMessage)
	}
	if m.Subtype != "api_retry" {
		t.Errorf("Subtype: got %q, want 'api_retry'", m.Subtype)
	}
}

func TestParseMessage_TaskNotification_SubagentTypeAndDescription(t *testing.T) {
	data := map[string]any{
		"type":             "system",
		"subtype":          "task_notification",
		"task_id":          "t1",
		"status":           "completed",
		"uuid":             "u1",
		"session_id":       "s1",
		"subagent_type":    "plan",
		"task_description": "Plan the migration",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n := msg.(*TaskNotificationMessage)
	if n.SubagentType != "plan" {
		t.Errorf("SubagentType = %q, want plan", n.SubagentType)
	}
	if n.TaskDescription != "Plan the migration" {
		t.Errorf("TaskDescription = %q", n.TaskDescription)
	}
}

func TestParseWorkerShuttingDownMessage_WithReason(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "worker_shutting_down",
		"reason":  "graceful_shutdown",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*WorkerShuttingDownMessage)
	if !ok {
		t.Fatalf("expected *WorkerShuttingDownMessage, got %T", msg)
	}
	if m.Reason != "graceful_shutdown" {
		t.Errorf("Reason = %q, want %q", m.Reason, "graceful_shutdown")
	}
}

func TestParseWorkerShuttingDownMessage_Minimal(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "worker_shutting_down",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*WorkerShuttingDownMessage)
	if !ok {
		t.Fatalf("expected *WorkerShuttingDownMessage, got %T", msg)
	}
	if m.Reason != "" {
		t.Errorf("Reason should be empty, got %q", m.Reason)
	}
}

func TestParsePermissionDeniedAdvisoryMessage_SafetyCheck(t *testing.T) {
	data := map[string]any{
		"type":          "system",
		"subtype":       "permission_denied_advisory",
		"tool_name":     "Bash",
		"denial_reason": "safetyCheck",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*PermissionDeniedAdvisoryMessage)
	if !ok {
		t.Fatalf("expected *PermissionDeniedAdvisoryMessage, got %T", msg)
	}
	if m.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", m.ToolName, "Bash")
	}
	if m.DenialReason != PermissionDeniedAdvisoryReasonSafetyCheck {
		t.Errorf("DenialReason = %q, want %q", m.DenialReason, PermissionDeniedAdvisoryReasonSafetyCheck)
	}
}

func TestParsePermissionDeniedAdvisoryMessage_AsyncAgent(t *testing.T) {
	data := map[string]any{
		"type":          "system",
		"subtype":       "permission_denied_advisory",
		"tool_name":     "Write",
		"denial_reason": "asyncAgent",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*PermissionDeniedAdvisoryMessage)
	if !ok {
		t.Fatalf("expected *PermissionDeniedAdvisoryMessage, got %T", msg)
	}
	if m.DenialReason != PermissionDeniedAdvisoryReasonAsyncAgent {
		t.Errorf("DenialReason = %q, want %q", m.DenialReason, PermissionDeniedAdvisoryReasonAsyncAgent)
	}
}

func TestParsePermissionDeniedAdvisoryMessage_Minimal(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "permission_denied_advisory",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*PermissionDeniedAdvisoryMessage)
	if !ok {
		t.Fatalf("expected *PermissionDeniedAdvisoryMessage, got %T", msg)
	}
	if m.DenialReason != "" {
		t.Errorf("DenialReason should be empty, got %q", m.DenialReason)
	}
}

func TestParseMessage_MemoryRecall(t *testing.T) {
	data := map[string]any{
		"type":    "system",
		"subtype": "memory_recall",
		"paths":   []any{"/home/user/.claude/memory.md", "/project/.claude/memory.md"},
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	m, ok := msg.(*MemoryRecallMessage)
	if !ok {
		t.Fatalf("expected *MemoryRecallMessage, got %T", msg)
	}
	if len(m.Paths) != 2 {
		t.Errorf("Paths length: got %d, want 2", len(m.Paths))
	}
	if m.Paths[0] != "/home/user/.claude/memory.md" {
		t.Errorf("Paths[0]: got %q", m.Paths[0])
	}
	if m.Subtype != "memory_recall" {
		t.Errorf("Subtype: got %q, want 'memory_recall'", m.Subtype)
	}
}

// ---------------------------------------------------------------------------
// Tests for wrong-type field detection (#300)
// ---------------------------------------------------------------------------

func TestParseMessage_RateLimitEvent_RateLimitInfoNull(t *testing.T) {
	// rate_limit_info present but nil — should not error (treat as missing).
	data := map[string]any{
		"type":            "rate_limit_event",
		"uuid":            "rl-uuid",
		"session_id":      "sess-1",
		"rate_limit_info": nil,
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error for nil rate_limit_info: %v", err)
	}
	event, ok := msg.(*RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", msg)
	}
	if event.RateLimitInfo.Status != "" {
		t.Errorf("expected empty RateLimitInfo for nil value, got status=%q", event.RateLimitInfo.Status)
	}
}

func TestParseMessage_RateLimitEvent_RateLimitInfoWrongType(t *testing.T) {
	// rate_limit_info present as a string — should return MessageParseError.
	data := map[string]any{
		"type":            "rate_limit_event",
		"uuid":            "rl-uuid",
		"session_id":      "sess-1",
		"rate_limit_info": "invalid_string",
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for wrong-type rate_limit_info, got nil")
	}
	mpe, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("expected *MessageParseError, got %T", err)
	}
	if mpe.Message != "rate_limit_info has wrong type" {
		t.Errorf("unexpected error message: %q", mpe.Message)
	}
}

func TestParseMessage_UserMessage_MessageNull(t *testing.T) {
	// message field present but nil — should return MessageParseError with "Missing" message.
	data := map[string]any{
		"type":    "user",
		"message": nil,
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for nil message field, got nil")
	}
	mpe, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("expected *MessageParseError, got %T", err)
	}
	if mpe.Message != "Missing 'message' field in user message" {
		t.Errorf("unexpected error message: %q", mpe.Message)
	}
}

func TestParseMessage_UserMessage_MessageWrongType(t *testing.T) {
	// message field present as an int — should return MessageParseError with "Wrong type" message.
	data := map[string]any{
		"type":    "user",
		"message": 42,
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for wrong-type message field, got nil")
	}
	mpe, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("expected *MessageParseError, got %T", err)
	}
	if mpe.Message != "Wrong type for 'message' field in user message" {
		t.Errorf("unexpected error message: %q", mpe.Message)
	}
}

func TestParseMessage_AssistantMessage_MessageNull(t *testing.T) {
	// message field present but nil — should return MessageParseError with "Missing" message.
	data := map[string]any{
		"type":    "assistant",
		"message": nil,
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for nil message field, got nil")
	}
	mpe, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("expected *MessageParseError, got %T", err)
	}
	if mpe.Message != "Missing 'message' field in assistant message" {
		t.Errorf("unexpected error message: %q", mpe.Message)
	}
}

func TestParseMessage_AssistantMessage_MessageWrongType(t *testing.T) {
	// message field present as an int — should return MessageParseError with "Wrong type" message.
	data := map[string]any{
		"type":    "assistant",
		"message": 42,
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for wrong-type message field, got nil")
	}
	mpe, ok := err.(*MessageParseError)
	if !ok {
		t.Fatalf("expected *MessageParseError, got %T", err)
	}
	if mpe.Message != "Wrong type for 'message' field in assistant message" {
		t.Errorf("unexpected error message: %q", mpe.Message)
	}
}

func TestParseAssistantMessage_StopDetails(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content":      []any{},
			"stop_reason":  "refusal",
			"stop_details": map[string]any{"type": "refusal", "reason": "policy"},
		},
	}
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected AssistantMessage, got %T", msg)
	}
	if am.StopReason != "refusal" {
		t.Errorf("expected StopReason 'refusal', got %q", am.StopReason)
	}
	if am.StopDetails == nil {
		t.Fatal("expected StopDetails to be non-nil")
	}
	if am.StopDetails["type"] != "refusal" {
		t.Errorf("expected StopDetails type 'refusal', got %v", am.StopDetails["type"])
	}
}

func TestParseAssistantMessage_Aborted(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{},
			"aborted": true,
		},
	}
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected AssistantMessage, got %T", msg)
	}
	if !am.Aborted {
		t.Error("expected Aborted to be true")
	}
	if am.StopReason != "" {
		t.Errorf("expected empty StopReason for aborted message, got %q", am.StopReason)
	}
}

func TestParseAssistantMessage_NotAborted(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content":     []any{},
			"stop_reason": "end_turn",
		},
	}
	msg, err := ParseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected AssistantMessage, got %T", msg)
	}
	if am.Aborted {
		t.Error("expected Aborted to be false for a normally completed message")
	}
}

// ---------------------------------------------------------------------------
// Tests for HookEventMessage (hook_started / hook_response) — issue #328
// Port of Python SDK PR anthropics/claude-agent-sdk-python#917.
// ---------------------------------------------------------------------------

func TestParseMessage_HookStarted(t *testing.T) {
	data := map[string]any{
		"type":       "system",
		"subtype":    "hook_started",
		"hook_event": "PreToolUse",
		"hook_id":    "hook_abc",
		"hook_name":  "my_hook",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	he, ok := msg.(*HookEventMessage)
	if !ok {
		t.Fatalf("expected *HookEventMessage, got %T", msg)
	}
	if he.HookEvent != "PreToolUse" {
		t.Errorf("HookEvent = %q, want PreToolUse", he.HookEvent)
	}
	if he.HookID != "hook_abc" {
		t.Errorf("HookID = %q, want hook_abc", he.HookID)
	}
	if he.HookName != "my_hook" {
		t.Errorf("HookName = %q, want my_hook", he.HookName)
	}
	if he.Subtype != "hook_started" {
		t.Errorf("Subtype = %q, want hook_started", he.Subtype)
	}
	if he.Output != nil {
		t.Errorf("Output = %v, want nil for hook_started", he.Output)
	}
	if he.ExitCode != nil {
		t.Errorf("ExitCode = %v, want nil for hook_started", he.ExitCode)
	}
	if he.Outcome != "" {
		t.Errorf("Outcome = %q, want empty for hook_started", he.Outcome)
	}
}

func TestParseMessage_HookResponse(t *testing.T) {
	data := map[string]any{
		"type":       "system",
		"subtype":    "hook_response",
		"hook_event": "PreToolUse",
		"hook_id":    "hook_abc",
		"hook_name":  "my_hook",
		"output":     map[string]any{"decision": "approve"},
		"exit_code":  float64(0),
		"outcome":    "approved",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	he, ok := msg.(*HookEventMessage)
	if !ok {
		t.Fatalf("expected *HookEventMessage, got %T", msg)
	}
	if he.HookEvent != "PreToolUse" {
		t.Errorf("HookEvent = %q, want PreToolUse", he.HookEvent)
	}
	if he.HookID != "hook_abc" {
		t.Errorf("HookID = %q, want hook_abc", he.HookID)
	}
	if he.HookName != "my_hook" {
		t.Errorf("HookName = %q, want my_hook", he.HookName)
	}
	if he.Subtype != "hook_response" {
		t.Errorf("Subtype = %q, want hook_response", he.Subtype)
	}
	if he.Output == nil {
		t.Fatal("Output is nil, want map with decision")
	}
	if he.Output["decision"] != "approve" {
		t.Errorf("Output[decision] = %v, want approve", he.Output["decision"])
	}
	if he.ExitCode == nil {
		t.Fatal("ExitCode is nil, want 0")
	}
	if *he.ExitCode != 0 {
		t.Errorf("*ExitCode = %d, want 0", *he.ExitCode)
	}
	if he.Outcome != "approved" {
		t.Errorf("Outcome = %q, want approved", he.Outcome)
	}
}

func TestParseMessage_HookEventMessage_ImplementsSystemMessage(t *testing.T) {
	// HookEventMessage embeds SystemMessage; it should be accessible as *SystemMessage
	// via a type switch on the embedded field.
	data := map[string]any{
		"type":       "system",
		"subtype":    "hook_started",
		"hook_event": "Stop",
		"hook_id":    "hook_xyz",
		"hook_name":  "stop_hook",
	}
	msg, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	he, ok := msg.(*HookEventMessage)
	if !ok {
		t.Fatalf("expected *HookEventMessage, got %T", msg)
	}
	// The embedded SystemMessage should have the correct subtype.
	if he.Subtype != "hook_started" {
		t.Errorf("embedded Subtype = %q, want hook_started", he.Subtype)
	}
	// Verify it satisfies the Message interface.
	var _ Message = he
}

func TestParseSystemMessage_ModelFallback(t *testing.T) {
	tests := []struct {
		name    string
		trigger string
		want    ModelFallbackTrigger
	}{
		{"model_not_found", "model_not_found", ModelFallbackTriggerModelNotFound},
		{"permission_denied", "permission_denied", ModelFallbackTriggerPermissionDenied},
		{"overloaded", "overloaded", ModelFallbackTriggerOverloaded},
		{"server_error", "server_error", ModelFallbackTriggerServerError},
		{"last_resort", "last_resort", ModelFallbackTriggerLastResort},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := map[string]any{
				"type":           "system",
				"subtype":        "model_fallback",
				"trigger":        tt.trigger,
				"model":          "claude-sonnet-4-6",
				"original_model": "claude-fable-5",
				"session_id":     "sess1",
				"uuid":           "uuid1",
			}
			msg, err := ParseMessage(data)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			mf, ok := msg.(*ModelFallbackMessage)
			if !ok {
				t.Fatalf("expected *ModelFallbackMessage, got %T", msg)
			}
			if mf.Trigger != tt.want {
				t.Errorf("Trigger: got %q, want %q", mf.Trigger, tt.want)
			}
			if mf.Model != "claude-sonnet-4-6" {
				t.Errorf("Model: got %q, want %q", mf.Model, "claude-sonnet-4-6")
			}
			if mf.OriginalModel != "claude-fable-5" {
				t.Errorf("OriginalModel: got %q, want %q", mf.OriginalModel, "claude-fable-5")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Tests for non-dict content block validation — issue #459
// Port of Python SDK PR anthropics/claude-agent-sdk-python#1058 (commit d47b180).
// ---------------------------------------------------------------------------

func TestParseMessage_NonDictContentBlock_AssistantReturnsError(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{"unexpected_string"},
			"model":   "claude-sonnet-4-6",
		},
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for non-dict content block in assistant message, got nil")
	}
	if _, ok := err.(*MessageParseError); !ok {
		t.Fatalf("expected *MessageParseError, got %T: %v", err, err)
	}
}

func TestParseMessage_NonDictContentBlock_UserReturnsError(t *testing.T) {
	data := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{42},
		},
	}
	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("expected error for non-dict content block in user message, got nil")
	}
	if _, ok := err.(*MessageParseError); !ok {
		t.Fatalf("expected *MessageParseError, got %T: %v", err, err)
	}
}

package claude

import (
	"encoding/json"
	"testing"
)

// decodeToolUseResult round-trips a UserMessage.ToolUseResult map through
// JSON into the given typed struct, mirroring the documented decode pattern
// for AgentToolCompletedOutput and its siblings.
func decodeToolUseResult(t *testing.T, result map[string]any, out any) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestAgentToolCompletedOutput_DecodesFromToolUseResult(t *testing.T) {
	msg := &UserMessage{
		ToolUseResult: map[string]any{
			"status":            "completed",
			"agentId":           "agent-123",
			"agentType":         "general-purpose",
			"resolvedModel":     "claude-sonnet-5",
			"modelsUsed":        []any{"claude-haiku-4-5", "claude-sonnet-5"},
			"totalToolUseCount": float64(4),
			"totalDurationMs":   float64(1500),
			"totalTokens":       float64(2048),
			"prompt":            "investigate the bug",
			"content": []any{
				map[string]any{"type": "text", "text": "done"},
			},
			"usage": map[string]any{
				"input_tokens":  float64(100),
				"output_tokens": float64(50),
			},
			"toolStats": map[string]any{
				"readCount":  float64(3),
				"bashCount":  float64(1),
				"linesAdded": float64(10),
			},
		},
	}

	var out AgentToolCompletedOutput
	decodeToolUseResult(t, msg.ToolUseResult, &out)

	if out.Status != AgentOutputStatusCompleted {
		t.Errorf("Status = %q, want %q", out.Status, AgentOutputStatusCompleted)
	}
	if out.AgentID != "agent-123" {
		t.Errorf("AgentID = %q, want %q", out.AgentID, "agent-123")
	}
	if out.TotalToolUseCount != 4 {
		t.Errorf("TotalToolUseCount = %d, want 4", out.TotalToolUseCount)
	}
	if len(out.Content) != 1 || out.Content[0].Text != "done" {
		t.Errorf("Content = %+v, want single text block %q", out.Content, "done")
	}
	if out.Usage.InputTokens != 100 || out.Usage.OutputTokens != 50 {
		t.Errorf("Usage = %+v, want input=100 output=50", out.Usage)
	}
	if out.Usage.OutputTokensDetails != nil {
		t.Errorf("OutputTokensDetails = %+v, want nil when absent from payload", out.Usage.OutputTokensDetails)
	}
	if out.ToolStats == nil || out.ToolStats.ReadCount != 3 || out.ToolStats.BashCount != 1 {
		t.Errorf("ToolStats = %+v, want readCount=3 bashCount=1", out.ToolStats)
	}
	wantModelsUsed := []string{"claude-haiku-4-5", "claude-sonnet-5"}
	if len(out.ModelsUsed) != len(wantModelsUsed) || out.ModelsUsed[0] != wantModelsUsed[0] || out.ModelsUsed[1] != wantModelsUsed[1] {
		t.Errorf("ModelsUsed = %v, want %v", out.ModelsUsed, wantModelsUsed)
	}
}

func TestAgentToolUsage_OutputTokensDetailsDecodes(t *testing.T) {
	result := map[string]any{
		"input_tokens":  float64(100),
		"output_tokens": float64(50),
		"output_tokens_details": map[string]any{
			"thinking_tokens": float64(30),
		},
	}

	var out AgentToolUsage
	decodeToolUseResult(t, result, &out)

	if out.OutputTokensDetails == nil {
		t.Fatalf("OutputTokensDetails = nil, want non-nil")
	}
	if out.OutputTokensDetails.ThinkingTokens == nil || *out.OutputTokensDetails.ThinkingTokens != 30 {
		t.Errorf("OutputTokensDetails.ThinkingTokens = %v, want 30", out.OutputTokensDetails.ThinkingTokens)
	}
}

func TestAgentToolAsyncLaunchedOutput_DecodesFromToolUseResult(t *testing.T) {
	result := map[string]any{
		"status":            "async_launched",
		"isAsync":           true,
		"agentId":           "agent-456",
		"description":       "long running task",
		"resolvedModel":     "claude-sonnet-5",
		"modelsUsed":        []any{"claude-sonnet-5"},
		"prompt":            "do the thing",
		"outputFile":        "/tmp/agent-456.out",
		"canReadOutputFile": true,
	}

	var out AgentToolAsyncLaunchedOutput
	decodeToolUseResult(t, result, &out)

	if out.Status != AgentOutputStatusAsyncLaunched {
		t.Errorf("Status = %q, want %q", out.Status, AgentOutputStatusAsyncLaunched)
	}
	if out.AgentID != "agent-456" {
		t.Errorf("AgentID = %q, want %q", out.AgentID, "agent-456")
	}
	if out.OutputFile != "/tmp/agent-456.out" {
		t.Errorf("OutputFile = %q, want %q", out.OutputFile, "/tmp/agent-456.out")
	}
	if !out.CanReadOutputFile {
		t.Errorf("CanReadOutputFile = false, want true")
	}
	if len(out.ModelsUsed) != 1 || out.ModelsUsed[0] != "claude-sonnet-5" {
		t.Errorf("ModelsUsed = %v, want [claude-sonnet-5]", out.ModelsUsed)
	}
}

func TestAgentToolRemoteLaunchedOutput_DecodesFromToolUseResult(t *testing.T) {
	result := map[string]any{
		"status":      "remote_launched",
		"taskId":      "task-789",
		"sessionUrl":  "https://claude.ai/code/session_abc",
		"description": "remote task",
		"prompt":      "do the remote thing",
		"outputFile":  "/tmp/task-789.out",
	}

	var out AgentToolRemoteLaunchedOutput
	decodeToolUseResult(t, result, &out)

	if out.Status != AgentOutputStatusRemoteLaunched {
		t.Errorf("Status = %q, want %q", out.Status, AgentOutputStatusRemoteLaunched)
	}
	if out.TaskID != "task-789" {
		t.Errorf("TaskID = %q, want %q", out.TaskID, "task-789")
	}
	if out.SessionURL != "https://claude.ai/code/session_abc" {
		t.Errorf("SessionURL = %q, want %q", out.SessionURL, "https://claude.ai/code/session_abc")
	}
}

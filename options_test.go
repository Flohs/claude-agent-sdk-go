package claude

import (
	"encoding/json"
	"testing"
)

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

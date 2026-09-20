package claude

import (
	"encoding/json"
	"testing"
)

func TestEscapeSlashCommand(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/ add tests", " / add tests"},
		{"/  add tests", " /  add tests"},    // multiple spaces
		{"/\tadd tests", " /\tadd tests"},    // tab after slash
		{"/command", "/command"},              // slash command — NOT escaped
		{"hello / world", "hello / world"},   // slash not at start
		{"", ""},
		{"normal text", "normal text"},
	}
	for _, tt := range tests {
		got := escapeSlashCommand(tt.input)
		if got != tt.want {
			t.Errorf("escapeSlashCommand(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStampUserMessage(t *testing.T) {
	t.Run("false leaves message unchanged", func(t *testing.T) {
		msg := map[string]any{"type": "user"}
		got := stampUserMessage(msg, false)
		if _, ok := got["client_composed"]; ok {
			t.Errorf("expected no client_composed key, got %v", got)
		}
	})

	t.Run("true stamps client_composed", func(t *testing.T) {
		msg := map[string]any{"type": "user"}
		got := stampUserMessage(msg, true)
		if got["client_composed"] != true {
			t.Errorf("expected client_composed: true, got %v", got["client_composed"])
		}
	})

	t.Run("true overwrites a caller-supplied value", func(t *testing.T) {
		msg := map[string]any{"type": "user", "client_composed": false}
		got := stampUserMessage(msg, true)
		if got["client_composed"] != true {
			t.Errorf("expected client_composed to be overwritten to true, got %v", got["client_composed"])
		}
	})
}

func TestQuery_VerbatimPromptsStampsMessageAndSkipsEscape(t *testing.T) {
	// The map shape stampUserMessage/escapeSlashCommand interact with, built
	// the same way Query/WarmQuery.Query construct their user message.
	prompt := "/ add tests"

	stamped := stampUserMessage(map[string]any{
		"type":               "user",
		"session_id":         "",
		"message":            map[string]any{"role": "user", "content": prompt},
		"parent_tool_use_id": nil,
	}, true)

	data, err := json.Marshal(stamped)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}
	if decoded["client_composed"] != true {
		t.Errorf("expected client_composed: true, got %v", decoded["client_composed"])
	}
	inner, _ := decoded["message"].(map[string]any)
	if inner["content"] != prompt {
		t.Errorf("expected verbatim content %q, got %v", prompt, inner["content"])
	}
}

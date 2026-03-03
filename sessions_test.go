package claude

import (
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/project", "-home-user-project"},
		{"abc123", "abc123"},
		{"/path/with spaces/and!special@chars", "-path-with-spaces-and-special-chars"},
	}

	for _, tt := range tests {
		got := sanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizePath_LongPath(t *testing.T) {
	// Create a path longer than 200 chars
	longPath := "/"
	for i := 0; i < 50; i++ {
		longPath += "abcde/"
	}

	result := sanitizePath(longPath)
	if len(result) <= maxSanitizedLength {
		t.Errorf("expected sanitized path longer than %d (has hash suffix), got len=%d", maxSanitizedLength, len(result))
	}
	// Should contain a hash suffix
	if result[maxSanitizedLength] != '-' {
		t.Errorf("expected dash separator at position %d", maxSanitizedLength)
	}
}

func TestSimpleHash(t *testing.T) {
	// Test basic properties
	h1 := simpleHash("hello")
	h2 := simpleHash("hello")
	if h1 != h2 {
		t.Error("same input should produce same hash")
	}

	h3 := simpleHash("world")
	if h1 == h3 {
		t.Error("different inputs should produce different hashes (usually)")
	}

	h4 := simpleHash("")
	if h4 != "0" {
		t.Errorf("empty string hash should be '0', got %q", h4)
	}
}

func TestIsValidUUID(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"550e8400-e29b-41d4-a716-446655440000", true},
		{"not-a-uuid", false},
		{"550e8400e29b41d4a716446655440000", false}, // no dashes
		{"", false},
	}

	for _, tt := range tests {
		got := isValidUUID(tt.input)
		if got != tt.want {
			t.Errorf("isValidUUID(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestExtractJSONStringField(t *testing.T) {
	tests := []struct {
		text string
		key  string
		want string
	}{
		{`{"name":"hello"}`, "name", "hello"},
		{`{"name": "hello world"}`, "name", "hello world"},
		{`{"other":"x","name":"found"}`, "name", "found"},
		{`{"name":"not here"}`, "missing", ""},
		{`{"name":"escaped \"quotes\""}`, "name", `escaped "quotes"`},
	}

	for _, tt := range tests {
		got := extractJSONStringField(tt.text, tt.key)
		if got != tt.want {
			t.Errorf("extractJSONStringField(%q, %q) = %q, want %q", tt.text, tt.key, got, tt.want)
		}
	}
}

func TestExtractLastJSONStringField(t *testing.T) {
	text := `{"title":"first"}
{"title":"second"}
{"title":"third"}`

	got := extractLastJSONStringField(text, "title")
	if got != "third" {
		t.Errorf("expected 'third', got %q", got)
	}
}

func TestExtractFirstPromptFromHead(t *testing.T) {
	head := `{"type":"system","subtype":"init"}
{"type":"user","message":{"content":"Hello Claude"}}
{"type":"assistant","message":{"content":"Hi!"}}
`

	got := extractFirstPromptFromHead(head)
	if got != "Hello Claude" {
		t.Errorf("expected 'Hello Claude', got %q", got)
	}
}

func TestExtractFirstPromptFromHead_SkipsSystemMessages(t *testing.T) {
	head := `{"type":"user","message":{"content":"<local-command-stdout>something"}}
{"type":"user","message":{"content":"Real prompt here"}}
`

	got := extractFirstPromptFromHead(head)
	if got != "Real prompt here" {
		t.Errorf("expected 'Real prompt here', got %q", got)
	}
}

func TestExtractFirstPromptFromHead_SkipsToolResults(t *testing.T) {
	// The tool_result check looks for "tool_result" as a JSON key in the line
	head := `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"x"}]}}
{"type":"user","message":{"content":"Actual question"}}
`

	got := extractFirstPromptFromHead(head)
	if got != "Actual question" {
		t.Errorf("expected 'Actual question', got %q", got)
	}
}

func TestPermissionUpdate_ToDict(t *testing.T) {
	p := PermissionUpdate{
		Type:     PermissionUpdateAddRules,
		Behavior: PermissionBehaviorAllow,
		Rules: []PermissionRuleValue{
			{ToolName: "Bash", RuleContent: "allow all"},
		},
		Destination: PermissionUpdateDestSession,
	}

	d := p.ToDict()
	if d["type"] != "addRules" {
		t.Errorf("expected type 'addRules', got %v", d["type"])
	}
	if d["behavior"] != "allow" {
		t.Errorf("expected behavior 'allow', got %v", d["behavior"])
	}
	if d["destination"] != "session" {
		t.Errorf("expected destination 'session', got %v", d["destination"])
	}
	rules, ok := d["rules"].([]map[string]any)
	if !ok || len(rules) != 1 {
		t.Fatal("expected 1 rule")
	}
	if rules[0]["toolName"] != "Bash" {
		t.Errorf("expected toolName 'Bash', got %v", rules[0]["toolName"])
	}
}

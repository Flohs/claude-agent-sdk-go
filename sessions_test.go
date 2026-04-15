package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// ---------------------------------------------------------------------------
// Helper utilities for integration tests
// ---------------------------------------------------------------------------

func intPtr(n int) *int { return &n }

// makeSessionLine creates a single JSONL line for a transcript entry.
func makeSessionLine(fields map[string]any) string {
	data, _ := json.Marshal(fields)
	return string(data)
}

// makeUserLine creates a user message transcript line.
func makeUserLine(uuid, parentUUID, content string, extra ...map[string]any) string {
	m := map[string]any{
		"type":    "user",
		"uuid":    uuid,
		"message": map[string]any{"content": content},
	}
	if parentUUID != "" {
		m["parentUuid"] = parentUUID
	}
	for _, e := range extra {
		for k, v := range e {
			m[k] = v
		}
	}
	return makeSessionLine(m)
}

// makeAssistantLine creates an assistant message transcript line.
func makeAssistantLine(uuid, parentUUID, content string, extra ...map[string]any) string {
	m := map[string]any{
		"type":    "assistant",
		"uuid":    uuid,
		"message": map[string]any{"content": content},
	}
	if parentUUID != "" {
		m["parentUuid"] = parentUUID
	}
	for _, e := range extra {
		for k, v := range e {
			m[k] = v
		}
	}
	return makeSessionLine(m)
}

// setupTestProjectDir creates a mock Claude config directory structure and
// sets CLAUDE_CONFIG_DIR so the code under test uses it. Returns a cleanup
// function that restores the env var.
func setupTestProjectDir(t *testing.T, projectPath string) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	sanitized := sanitizePath(projectPath)
	projDir := filepath.Join(tmpDir, "projects", sanitized)
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return projDir
}

// writeSessionFile writes content to a session JSONL file and returns its path.
func writeSessionFile(t *testing.T, projectDir, sessionID, content string) string {
	t.Helper()
	p := filepath.Join(projectDir, sessionID+".jsonl")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// Test UUIDs used across tests.
const (
	testUUID1 = "00000000-0000-0000-0000-000000000001"
	testUUID2 = "00000000-0000-0000-0000-000000000002"
	testUUID3 = "00000000-0000-0000-0000-000000000003"
	testUUID4 = "00000000-0000-0000-0000-000000000004"
	testUUID5 = "00000000-0000-0000-0000-000000000005"
	testUUID6 = "00000000-0000-0000-0000-000000000006"
)

// ---------------------------------------------------------------------------
// ListSessions integration tests
// ---------------------------------------------------------------------------

func TestListSessions_BasicProjectDirectory(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/project")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Hello Claude"),
		makeAssistantLine("a1", "u1", "Hi there!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/project",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].SessionID != testUUID1 {
		t.Errorf("expected session ID %s, got %s", testUUID1, sessions[0].SessionID)
	}
	if sessions[0].FirstPrompt != "Hello Claude" {
		t.Errorf("expected first prompt 'Hello Claude', got %q", sessions[0].FirstPrompt)
	}
}

func TestListSessions_MultipleSessions(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/multi")


	for i, uuid := range []string{testUUID1, testUUID2, testUUID3} {
		content := strings.Join([]string{
			makeUserLine("u-"+uuid, "", fmt.Sprintf("Question %d", i+1)),
			makeAssistantLine("a-"+uuid, "u-"+uuid, fmt.Sprintf("Answer %d", i+1)),
		}, "\n") + "\n"
		p := writeSessionFile(t, projDir, uuid, content)
		// Set different mod times so ordering is deterministic.
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/multi",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(sessions))
	}
	// Should be sorted by LastModified descending.
	for i := 0; i < len(sessions)-1; i++ {
		if sessions[i].LastModified < sessions[i+1].LastModified {
			t.Errorf("sessions not sorted by LastModified descending at index %d", i)
		}
	}
}

func TestListSessions_LimitParameter(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/limited")


	for i, uuid := range []string{testUUID1, testUUID2, testUUID3, testUUID4, testUUID5} {
		content := strings.Join([]string{
			makeUserLine("u-"+uuid, "", fmt.Sprintf("Prompt %d", i+1)),
			makeAssistantLine("a-"+uuid, "u-"+uuid, "Reply"),
		}, "\n") + "\n"
		p := writeSessionFile(t, projDir, uuid, content)
		modTime := time.Now().Add(time.Duration(i) * time.Second)
		if err := os.Chtimes(p, modTime, modTime); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name     string
		limit    *int
		expected int
	}{
		{"nil limit returns all", nil, 5},
		{"limit 0 returns all", intPtr(0), 5},
		{"limit 2 returns 2", intPtr(2), 2},
		{"limit 10 returns all", intPtr(10), 5},
		{"limit 1 returns 1", intPtr(1), 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions, err := ListSessions(ListSessionsOptions{
				Directory:        "/test/limited",
				Limit:            tt.limit,
				IncludeWorktrees: false,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(sessions) != tt.expected {
				t.Errorf("expected %d sessions, got %d", tt.expected, len(sessions))
			}
		})
	}
}

func TestListSessions_EmptyDirectory(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/empty")

	_ = projDir

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/empty",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessions_NonexistentDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/nonexistent/path/that/does/not/exist",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected empty sessions, got %d", len(sessions))
	}
}

func TestListSessions_AllProjects(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	// Create two project directories.
	for _, proj := range []string{"-proj-a", "-proj-b"} {
		projDir := filepath.Join(tmpDir, "projects", proj)
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	content1 := strings.Join([]string{
		makeUserLine("u1", "", "Hello from A"),
		makeAssistantLine("a1", "u1", "Reply A"),
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "projects", "-proj-a", testUUID1+".jsonl"), []byte(content1), 0o644); err != nil {
		t.Fatal(err)
	}

	content2 := strings.Join([]string{
		makeUserLine("u2", "", "Hello from B"),
		makeAssistantLine("a2", "u2", "Reply B"),
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "projects", "-proj-b", testUUID2+".jsonl"), []byte(content2), 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := ListSessions(ListSessionsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions across all projects, got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// Sidechain filtering tests
// ---------------------------------------------------------------------------

func TestListSessions_FiltersSidechainSessions(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/sidechain")


	// Normal session.
	normalContent := strings.Join([]string{
		makeUserLine("u1", "", "Normal question"),
		makeAssistantLine("a1", "u1", "Normal answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, normalContent)

	// Sidechain session (isSidechain in first line).
	sidechainContent := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":        "user",
			"uuid":        "sc-u1",
			"isSidechain": true,
			"message":     map[string]any{"content": "Sidechain question"},
		}),
		makeAssistantLine("sc-a1", "sc-u1", "Sidechain answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID2, sidechainContent)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/sidechain",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (sidechain filtered), got %d", len(sessions))
	}
	if sessions[0].SessionID != testUUID1 {
		t.Errorf("expected non-sidechain session %s, got %s", testUUID1, sessions[0].SessionID)
	}
}

func TestListSessions_FiltersSidechainWithSpaces(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/sidechain-spaces")


	// Sidechain with space in JSON: "isSidechain": true
	sidechainContent := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":        "user",
			"uuid":        "sc-u1",
			"isSidechain": true,
			"message":     map[string]any{"content": "Sidechain question"},
		}),
		makeAssistantLine("sc-a1", "sc-u1", "Sidechain answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, sidechainContent)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/sidechain-spaces",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions (sidechain filtered), got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// Sessions with only metadata (no user messages) are filtered out
// ---------------------------------------------------------------------------

func TestListSessions_FiltersMetadataOnlySessions(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/metadata-only")


	// A session with only system/meta entries and no user prompt.
	metaOnlyContent := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "system",
			"uuid":    "sys1",
			"subtype": "init",
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, metaOnlyContent)

	// A session with isMeta user messages only.
	metaUserContent := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "user",
			"uuid":    "mu1",
			"isMeta":  true,
			"message": map[string]any{"content": "Meta message"},
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID2, metaUserContent)

	// A valid session to confirm filtering works correctly.
	validContent := strings.Join([]string{
		makeUserLine("u1", "", "Real question"),
		makeAssistantLine("a1", "u1", "Real answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID3, validContent)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/metadata-only",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (metadata-only filtered), got %d", len(sessions))
	}
	if sessions[0].SessionID != testUUID3 {
		t.Errorf("expected session %s, got %s", testUUID3, sessions[0].SessionID)
	}
}

// ---------------------------------------------------------------------------
// Edge cases: empty files, only newlines, malformed JSON
// ---------------------------------------------------------------------------

func TestListSessions_EmptyFile(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/empty-file")


	writeSessionFile(t, projDir, testUUID1, "")

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/empty-file",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for empty file, got %d", len(sessions))
	}
}

func TestListSessions_FileWithOnlyNewlines(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/newlines-only")


	writeSessionFile(t, projDir, testUUID1, "\n\n\n\n")

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/newlines-only",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions for newlines-only file, got %d", len(sessions))
	}
}

func TestListSessions_MalformedJSONLines(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/malformed")


	// Mix of malformed and valid lines.
	content := strings.Join([]string{
		`{this is not valid json}`,
		`{"incomplete": true`,
		makeUserLine("u1", "", "Valid question"),
		`random garbage text`,
		makeAssistantLine("a1", "u1", "Valid answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/malformed",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should still find the session because valid user message exists.
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session (malformed lines skipped), got %d", len(sessions))
	}
	if sessions[0].FirstPrompt != "Valid question" {
		t.Errorf("expected first prompt 'Valid question', got %q", sessions[0].FirstPrompt)
	}
}

func TestListSessions_TruncatedJSONL(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/truncated")


	// A file where the last line is truncated mid-JSON.
	content := makeUserLine("u1", "", "Working question") + "\n" +
		makeAssistantLine("a1", "u1", "Working answer") + "\n" +
		`{"type":"user","uuid":"u2","message":{"conten`
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/truncated",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// Non-JSONL files and invalid UUID filenames are ignored
// ---------------------------------------------------------------------------

func TestListSessions_IgnoresNonJSONLFiles(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/non-jsonl")


	// Create non-JSONL files.
	for _, f := range []struct{ name, content string }{
		{"notes.txt", "some notes"},
		{"config.json", "{}"},
		{"not-a-uuid.jsonl", makeUserLine("u1", "", "question") + "\n"},
	} {
		if err := os.WriteFile(filepath.Join(projDir, f.name), []byte(f.content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a valid session.
	content := makeUserLine("u1", "", "Real question") + "\n" +
		makeAssistantLine("a1", "u1", "Real answer") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/non-jsonl",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
}

// ---------------------------------------------------------------------------
// CustomTitle and Summary extraction
// ---------------------------------------------------------------------------

func TestListSessions_CustomTitlePriority(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/title-priority")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Initial question"),
		makeAssistantLine("a1", "u1", "Answer"),
		makeSessionLine(map[string]any{
			"type":    "assistant",
			"uuid":    "a2",
			"summary": "Auto-generated summary",
			"message": map[string]any{"content": "More"},
		}),
		makeSessionLine(map[string]any{
			"type":        "assistant",
			"uuid":        "a3",
			"customTitle": "My Custom Title",
			"message":     map[string]any{"content": "Even more"},
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/title-priority",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].CustomTitle != "My Custom Title" {
		t.Errorf("expected custom title 'My Custom Title', got %q", sessions[0].CustomTitle)
	}
	// Summary should prefer customTitle over summary field.
	if sessions[0].Summary != "My Custom Title" {
		t.Errorf("expected summary to be custom title, got %q", sessions[0].Summary)
	}
}

func TestListSessions_FallsBackToSummaryField(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/summary-fallback")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Question"),
		makeAssistantLine("a1", "u1", "Answer"),
		makeSessionLine(map[string]any{
			"type":    "assistant",
			"uuid":    "a2",
			"summary": "Generated Summary",
			"message": map[string]any{"content": "More"},
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/summary-fallback",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Summary != "Generated Summary" {
		t.Errorf("expected summary 'Generated Summary', got %q", sessions[0].Summary)
	}
}

func TestListSessions_FallsBackToFirstPrompt(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/prompt-fallback")


	// No summary or customTitle, just user messages.
	content := strings.Join([]string{
		makeUserLine("u1", "", "My initial prompt"),
		makeAssistantLine("a1", "u1", "Response"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/prompt-fallback",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Summary != "My initial prompt" {
		t.Errorf("expected summary to be first prompt, got %q", sessions[0].Summary)
	}
}

// ---------------------------------------------------------------------------
// Git branch extraction
// ---------------------------------------------------------------------------

func TestListSessions_GitBranchExtraction(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/gitbranch")


	content := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":      "user",
			"uuid":      "u1",
			"gitBranch": "feature/initial",
			"cwd":       "/test/gitbranch",
			"message":   map[string]any{"content": "Hello"},
		}),
		makeAssistantLine("a1", "u1", "Hi"),
		makeSessionLine(map[string]any{
			"type":      "assistant",
			"uuid":      "a2",
			"gitBranch": "feature/updated",
			"message":   map[string]any{"content": "Switched branch"},
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/gitbranch",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// Should pick up the last gitBranch from the tail.
	if sessions[0].GitBranch != "feature/updated" {
		t.Errorf("expected git branch 'feature/updated', got %q", sessions[0].GitBranch)
	}
}

// ---------------------------------------------------------------------------
// Large session files (>64KB) with metadata split across head/tail
// ---------------------------------------------------------------------------

func TestListSessions_LargeFileHeadTailSplit(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/large-file")


	// Build a file where:
	// - head contains the first user prompt and cwd
	// - middle is padding beyond 64KB
	// - tail contains the summary and customTitle
	headLines := []string{
		makeSessionLine(map[string]any{
			"type":    "user",
			"uuid":    "u1",
			"cwd":     "/test/large-file",
			"message": map[string]any{"content": "Start of conversation"},
		}),
		makeAssistantLine("a1", "u1", "Got it"),
	}

	// Generate padding lines to push past 64KB.
	var paddingLines []string
	paddingMsg := strings.Repeat("x", 500)
	for i := 0; i < 150; i++ {
		uuid := fmt.Sprintf("pad-u-%04d", i)
		parentUUID := "a1"
		if i > 0 {
			parentUUID = fmt.Sprintf("pad-a-%04d", i-1)
		}
		paddingLines = append(paddingLines,
			makeUserLine(uuid, parentUUID, paddingMsg),
			makeAssistantLine(fmt.Sprintf("pad-a-%04d", i), uuid, paddingMsg),
		)
	}

	tailLines := []string{
		makeSessionLine(map[string]any{
			"type":        "assistant",
			"uuid":        "final-a",
			"summary":     "Conversation about testing",
			"customTitle": "Large File Test",
			"gitBranch":   "main",
			"message":     map[string]any{"content": "Final response"},
		}),
	}

	all := append(headLines, paddingLines...)
	all = append(all, tailLines...)
	content := strings.Join(all, "\n") + "\n"

	// Verify file is actually larger than 64KB.
	if len(content) <= liteReadBufSize {
		t.Fatalf("test file should be > %d bytes, got %d", liteReadBufSize, len(content))
	}

	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/large-file",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	s := sessions[0]
	if s.CustomTitle != "Large File Test" {
		t.Errorf("expected custom title 'Large File Test', got %q", s.CustomTitle)
	}
	if s.Summary != "Large File Test" {
		t.Errorf("expected summary 'Large File Test', got %q", s.Summary)
	}
	if s.FirstPrompt != "Start of conversation" {
		t.Errorf("expected first prompt 'Start of conversation', got %q", s.FirstPrompt)
	}
	if s.GitBranch != "main" {
		t.Errorf("expected git branch 'main', got %q", s.GitBranch)
	}
	if s.FileSize == nil || *s.FileSize <= int64(liteReadBufSize) {
		t.Errorf("expected file size > %d, got %v", liteReadBufSize, s.FileSize)
	}
}

// ---------------------------------------------------------------------------
// GetSessionMessages integration tests
// ---------------------------------------------------------------------------

func TestGetSessionMessages_Basic(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/messages")


	content := strings.Join([]string{
		makeUserLine("u1", "", "First question"),
		makeAssistantLine("a1", "u1", "First answer"),
		makeUserLine("u2", "a1", "Follow-up"),
		makeAssistantLine("a2", "u2", "Follow-up answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/messages",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(messages))
	}
	if messages[0].Type != "user" {
		t.Errorf("expected first message type 'user', got %q", messages[0].Type)
	}
	if messages[1].Type != "assistant" {
		t.Errorf("expected second message type 'assistant', got %q", messages[1].Type)
	}
	if messages[0].UUID != "u1" {
		t.Errorf("expected UUID 'u1', got %q", messages[0].UUID)
	}
}

func TestGetSessionMessages_InvalidUUID(t *testing.T) {
	messages, err := GetSessionMessages("not-a-valid-uuid", GetSessionMessagesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if messages != nil {
		t.Errorf("expected nil for invalid UUID, got %v", messages)
	}
}

func TestGetSessionMessages_NonexistentSession(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	if err := os.MkdirAll(filepath.Join(tmpDir, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if messages != nil {
		t.Errorf("expected nil for nonexistent session, got %v", messages)
	}
}

func TestGetSessionMessages_WithOffset(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/offset")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Q1"),
		makeAssistantLine("a1", "u1", "A1"),
		makeUserLine("u2", "a1", "Q2"),
		makeAssistantLine("a2", "u2", "A2"),
		makeUserLine("u3", "a2", "Q3"),
		makeAssistantLine("a3", "u3", "A3"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/offset",
		Offset:    2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages (skipping first 2), got %d", len(messages))
	}
	if messages[0].UUID != "u2" {
		t.Errorf("expected first returned UUID 'u2', got %q", messages[0].UUID)
	}
}

func TestGetSessionMessages_WithLimit(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-limit")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Q1"),
		makeAssistantLine("a1", "u1", "A1"),
		makeUserLine("u2", "a1", "Q2"),
		makeAssistantLine("a2", "u2", "A2"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-limit",
		Limit:     intPtr(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
}

func TestGetSessionMessages_WithOffsetAndLimit(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/off-lim")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Q1"),
		makeAssistantLine("a1", "u1", "A1"),
		makeUserLine("u2", "a1", "Q2"),
		makeAssistantLine("a2", "u2", "A2"),
		makeUserLine("u3", "a2", "Q3"),
		makeAssistantLine("a3", "u3", "A3"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/off-lim",
		Offset:    1,
		Limit:     intPtr(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[0].UUID != "a1" {
		t.Errorf("expected UUID 'a1', got %q", messages[0].UUID)
	}
}

func TestGetSessionMessages_OffsetBeyondLength(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/off-beyond")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Q1"),
		makeAssistantLine("a1", "u1", "A1"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/off-beyond",
		Offset:    100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if messages != nil {
		t.Errorf("expected nil when offset beyond length, got %d messages", len(messages))
	}
}

func TestGetSessionMessages_FiltersSidechainMessages(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-sidechain")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Main question"),
		makeAssistantLine("a1", "u1", "Main answer"),
		// Sidechain messages in the same file should be filtered from visible messages.
		makeSessionLine(map[string]any{
			"type":        "user",
			"uuid":        "sc-u1",
			"parentUuid":  "a1",
			"isSidechain": true,
			"message":     map[string]any{"content": "Sidechain thought"},
		}),
		makeSessionLine(map[string]any{
			"type":        "assistant",
			"uuid":        "sc-a1",
			"parentUuid":  "sc-u1",
			"isSidechain": true,
			"message":     map[string]any{"content": "Sidechain reply"},
		}),
		makeUserLine("u2", "a1", "Follow up"),
		makeAssistantLine("a2", "u2", "Follow up answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-sidechain",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range messages {
		if strings.Contains(msg.UUID, "sc-") {
			t.Errorf("sidechain message %q should have been filtered", msg.UUID)
		}
	}
}

func TestGetSessionMessages_FiltersMetaMessages(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-meta")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Hello"),
		makeAssistantLine("a1", "u1", "Hi"),
		makeSessionLine(map[string]any{
			"type":       "user",
			"uuid":       "meta-u",
			"parentUuid": "a1",
			"isMeta":     true,
			"message":    map[string]any{"content": "Meta info"},
		}),
		makeUserLine("u2", "a1", "Real follow up"),
		makeAssistantLine("a2", "u2", "Real response"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-meta",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range messages {
		if msg.UUID == "meta-u" {
			t.Error("meta message should have been filtered")
		}
	}
}

func TestGetSessionMessages_FiltersTeamMessages(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-team")


	content := strings.Join([]string{
		makeUserLine("u1", "", "Hello"),
		makeAssistantLine("a1", "u1", "Hi"),
		makeSessionLine(map[string]any{
			"type":       "user",
			"uuid":       "team-u",
			"parentUuid": "a1",
			"teamName":   "review-team",
			"message":    map[string]any{"content": "Team message"},
		}),
		makeUserLine("u2", "a1", "Continue"),
		makeAssistantLine("a2", "u2", "Sure"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-team",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, msg := range messages {
		if msg.UUID == "team-u" {
			t.Error("team message should have been filtered")
		}
	}
}

// ---------------------------------------------------------------------------
// GetSessionMessages with malformed/corrupted content
// ---------------------------------------------------------------------------

func TestGetSessionMessages_MalformedLines(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-malformed")


	content := strings.Join([]string{
		`not json at all`,
		makeUserLine("u1", "", "Valid question"),
		`{"broken": json`,
		makeAssistantLine("a1", "u1", "Valid answer"),
		``,
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-malformed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 valid messages, got %d", len(messages))
	}
}

func TestGetSessionMessages_EmptyFile(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/msg-empty")


	writeSessionFile(t, projDir, testUUID1, "")

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/msg-empty",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Empty file means readSessionFile returns "" which means nil messages.
	if messages != nil {
		t.Errorf("expected nil for empty file, got %v", messages)
	}
}

// ---------------------------------------------------------------------------
// Conversation chain building
// ---------------------------------------------------------------------------

func TestGetSessionMessages_BranchedConversation(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/branched")


	// Create a branched conversation where u2 and u3 both descend from a1.
	// The chain builder should pick the branch with the latest entry.
	content := strings.Join([]string{
		makeUserLine("u1", "", "Root question"),
		makeAssistantLine("a1", "u1", "Root answer"),
		makeUserLine("u2", "a1", "Branch A"),
		makeAssistantLine("a2", "u2", "Branch A answer"),
		makeUserLine("u3", "a1", "Branch B"),
		makeAssistantLine("a3", "u3", "Branch B answer"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/branched",
	})
	if err != nil {
		t.Fatal(err)
	}
	// Should get 4 messages: u1 -> a1 -> u3 -> a3 (branch B is later).
	if len(messages) != 4 {
		t.Fatalf("expected 4 messages in main branch, got %d", len(messages))
	}
	// Last message should be from branch B.
	if messages[len(messages)-1].UUID != "a3" {
		t.Errorf("expected last message UUID 'a3', got %q", messages[len(messages)-1].UUID)
	}
}

func TestGetSessionMessages_OnlyProgressEntries(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/progress-only")


	// Progress entries are valid transcript entries but not visible.
	content := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "progress",
			"uuid":    "p1",
			"message": map[string]any{"content": "Thinking..."},
		}),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	messages, err := GetSessionMessages(testUUID1, GetSessionMessagesOptions{
		Directory: "/test/progress-only",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 visible messages for progress-only, got %d", len(messages))
	}
}

// ---------------------------------------------------------------------------
// readSessionLite tests
// ---------------------------------------------------------------------------

func TestReadSessionLite_SmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := makeUserLine("u1", "", "Hello") + "\n"
	path := filepath.Join(tmpDir, "test.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lite := readSessionLite(path)
	if lite == nil {
		t.Fatal("expected non-nil lite session")
		return
	}
	if lite.head != lite.tail {
		t.Error("for small files, head and tail should be identical")
	}
	if lite.size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), lite.size)
	}
}

func TestReadSessionLite_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file larger than 64KB.
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, makeUserLine(
			fmt.Sprintf("u%d", i), "",
			strings.Repeat("z", 500),
		))
	}
	content := strings.Join(lines, "\n") + "\n"
	if len(content) <= liteReadBufSize {
		t.Fatal("test content should be > 64KB")
	}

	path := filepath.Join(tmpDir, "large.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	lite := readSessionLite(path)
	if lite == nil {
		t.Fatal("expected non-nil lite session")
		return
	}
	if lite.head == lite.tail {
		t.Error("for large files, head and tail should differ")
	}
	if lite.size != int64(len(content)) {
		t.Errorf("expected size %d, got %d", len(content), lite.size)
	}
}

func TestReadSessionLite_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "empty.jsonl")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	lite := readSessionLite(path)
	if lite != nil {
		t.Error("expected nil for empty file")
	}
}

func TestReadSessionLite_NonexistentFile(t *testing.T) {
	lite := readSessionLite("/nonexistent/path/file.jsonl")
	if lite != nil {
		t.Error("expected nil for nonexistent file")
	}
}

// ---------------------------------------------------------------------------
// parseTranscriptEntries tests
// ---------------------------------------------------------------------------

func TestParseTranscriptEntries_ValidTypes(t *testing.T) {
	content := strings.Join([]string{
		makeSessionLine(map[string]any{"type": "user", "uuid": "u1", "message": "hi"}),
		makeSessionLine(map[string]any{"type": "assistant", "uuid": "a1", "message": "hello"}),
		makeSessionLine(map[string]any{"type": "progress", "uuid": "p1", "message": "working"}),
		makeSessionLine(map[string]any{"type": "system", "uuid": "s1", "message": "init"}),
		makeSessionLine(map[string]any{"type": "attachment", "uuid": "at1", "message": "file"}),
		// Unknown type should be ignored.
		makeSessionLine(map[string]any{"type": "unknown", "uuid": "x1", "message": "nope"}),
		// Missing uuid should be ignored.
		makeSessionLine(map[string]any{"type": "user", "message": "no uuid"}),
	}, "\n")

	entries := parseTranscriptEntries(content)
	if len(entries) != 5 {
		t.Errorf("expected 5 valid entries, got %d", len(entries))
	}
}

func TestParseTranscriptEntries_SkipsBlanksAndMalformed(t *testing.T) {
	content := "\n\n" +
		makeSessionLine(map[string]any{"type": "user", "uuid": "u1", "message": "hi"}) +
		"\n{bad json}\n\n" +
		makeSessionLine(map[string]any{"type": "assistant", "uuid": "a1", "message": "bye"}) +
		"\n"

	entries := parseTranscriptEntries(content)
	if len(entries) != 2 {
		t.Errorf("expected 2 valid entries, got %d", len(entries))
	}
}

// ---------------------------------------------------------------------------
// buildConversationChain tests
// ---------------------------------------------------------------------------

func TestBuildConversationChain_EmptyInput(t *testing.T) {
	chain := buildConversationChain(nil)
	if chain != nil {
		t.Errorf("expected nil chain for empty input, got %v", chain)
	}
}

func TestBuildConversationChain_LinearChain(t *testing.T) {
	entries := []transcriptEntry{
		{"type": "user", "uuid": "u1", "message": "q1"},
		{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": "r1"},
		{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": "q2"},
		{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "message": "r2"},
	}

	chain := buildConversationChain(entries)
	if len(chain) != 4 {
		t.Fatalf("expected 4 entries in chain, got %d", len(chain))
	}
	// Should be in chronological order.
	expected := []string{"u1", "a1", "u2", "a2"}
	for i, e := range chain {
		if e["uuid"] != expected[i] {
			t.Errorf("chain[%d] UUID = %v, expected %s", i, e["uuid"], expected[i])
		}
	}
}

func TestBuildConversationChain_PreferMainOverSidechain(t *testing.T) {
	entries := []transcriptEntry{
		{"type": "user", "uuid": "u1", "message": "q1"},
		{"type": "assistant", "uuid": "a1", "parentUuid": "u1", "message": "r1"},
		// Sidechain branch.
		{"type": "user", "uuid": "sc-u1", "parentUuid": "a1", "isSidechain": true, "message": "side"},
		{"type": "assistant", "uuid": "sc-a1", "parentUuid": "sc-u1", "isSidechain": true, "message": "side-r"},
		// Main branch continues.
		{"type": "user", "uuid": "u2", "parentUuid": "a1", "message": "q2"},
		{"type": "assistant", "uuid": "a2", "parentUuid": "u2", "message": "r2"},
	}

	chain := buildConversationChain(entries)
	for _, e := range chain {
		uuid, _ := e["uuid"].(string)
		if strings.HasPrefix(uuid, "sc-") {
			t.Errorf("sidechain entry %q should not be in main chain", uuid)
		}
	}
}

// ---------------------------------------------------------------------------
// isVisibleMessage tests
// ---------------------------------------------------------------------------

func TestIsVisibleMessage(t *testing.T) {
	tests := []struct {
		name    string
		entry   transcriptEntry
		visible bool
	}{
		{"user message", transcriptEntry{"type": "user", "uuid": "u1"}, true},
		{"assistant message", transcriptEntry{"type": "assistant", "uuid": "a1"}, true},
		{"progress message", transcriptEntry{"type": "progress", "uuid": "p1"}, false},
		{"system message", transcriptEntry{"type": "system", "uuid": "s1"}, false},
		{"meta user", transcriptEntry{"type": "user", "uuid": "u1", "isMeta": true}, false},
		{"sidechain user", transcriptEntry{"type": "user", "uuid": "u1", "isSidechain": true}, false},
		{"team message", transcriptEntry{"type": "user", "uuid": "u1", "teamName": "alpha"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isVisibleMessage(tt.entry)
			if got != tt.visible {
				t.Errorf("isVisibleMessage() = %v, want %v", got, tt.visible)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// toSessionMessage tests
// ---------------------------------------------------------------------------

func TestToSessionMessage(t *testing.T) {
	entry := transcriptEntry{
		"type":      "user",
		"uuid":      "u123",
		"sessionId": "sess-abc",
		"message":   map[string]any{"content": "hello"},
	}

	msg := toSessionMessage(entry)
	if msg.Type != "user" {
		t.Errorf("expected type 'user', got %q", msg.Type)
	}
	if msg.UUID != "u123" {
		t.Errorf("expected UUID 'u123', got %q", msg.UUID)
	}
	if msg.SessionID != "sess-abc" {
		t.Errorf("expected session ID 'sess-abc', got %q", msg.SessionID)
	}

	entry2 := transcriptEntry{
		"type":    "assistant",
		"uuid":    "a456",
		"message": map[string]any{"content": "world"},
	}
	msg2 := toSessionMessage(entry2)
	if msg2.Type != "assistant" {
		t.Errorf("expected type 'assistant', got %q", msg2.Type)
	}
}

// ---------------------------------------------------------------------------
// extractFirstPromptFromHead additional edge cases
// ---------------------------------------------------------------------------

func TestExtractFirstPromptFromHead_ContentArray(t *testing.T) {
	head := makeSessionLine(map[string]any{
		"type": "user",
		"uuid": "u1",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "Array content prompt"},
			},
		},
	}) + "\n"

	got := extractFirstPromptFromHead(head)
	if got != "Array content prompt" {
		t.Errorf("expected 'Array content prompt', got %q", got)
	}
}

func TestExtractFirstPromptFromHead_TruncatesLongPrompts(t *testing.T) {
	longPrompt := strings.Repeat("a", 300)
	head := makeUserLine("u1", "", longPrompt) + "\n"

	got := extractFirstPromptFromHead(head)
	// Should be truncated to 200 runes + ellipsis.
	runes := []rune(got)
	if len(runes) != 201 { // 200 chars + ellipsis character
		t.Errorf("expected 201 runes, got %d", len(runes))
	}
	if !strings.HasSuffix(got, "\u2026") {
		t.Error("expected ellipsis at end of truncated prompt")
	}
}

func TestExtractFirstPromptFromHead_SkipsCompactSummary(t *testing.T) {
	head := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":             "user",
			"uuid":             "u1",
			"isCompactSummary": true,
			"message":          map[string]any{"content": "Compact summary"},
		}),
		makeUserLine("u2", "", "Actual prompt"),
	}, "\n") + "\n"

	got := extractFirstPromptFromHead(head)
	if got != "Actual prompt" {
		t.Errorf("expected 'Actual prompt', got %q", got)
	}
}

func TestExtractFirstPromptFromHead_CommandNameFallback(t *testing.T) {
	head := makeSessionLine(map[string]any{
		"type":    "user",
		"uuid":    "u1",
		"message": map[string]any{"content": "<command-name>build</command-name>"},
	}) + "\n"

	got := extractFirstPromptFromHead(head)
	if got != "build" {
		t.Errorf("expected 'build' as command name fallback, got %q", got)
	}
}

func TestExtractFirstPromptFromHead_EmptyContent(t *testing.T) {
	head := makeUserLine("u1", "", "") + "\n"
	got := extractFirstPromptFromHead(head)
	if got != "" {
		t.Errorf("expected empty string for empty content, got %q", got)
	}
}

func TestExtractFirstPromptFromHead_SkipsIdeOpenedFile(t *testing.T) {
	head := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "user",
			"uuid":    "u1",
			"message": map[string]any{"content": "  <ide_opened_file>path/to/file</ide_opened_file>  "},
		}),
		makeUserLine("u2", "", "Real prompt"),
	}, "\n") + "\n"

	got := extractFirstPromptFromHead(head)
	if got != "Real prompt" {
		t.Errorf("expected 'Real prompt', got %q", got)
	}
}

func TestExtractFirstPromptFromHead_SkipsIdeSelection(t *testing.T) {
	head := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "user",
			"uuid":    "u1",
			"message": map[string]any{"content": "  <ide_selection>selected text</ide_selection>  "},
		}),
		makeUserLine("u2", "", "After selection"),
	}, "\n") + "\n"

	got := extractFirstPromptFromHead(head)
	if got != "After selection" {
		t.Errorf("expected 'After selection', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Deduplication tests
// ---------------------------------------------------------------------------

func TestDeduplicateBySessionID(t *testing.T) {
	sessions := []SDKSessionInfo{
		{SessionID: "a", LastModified: 100, Summary: "old"},
		{SessionID: "b", LastModified: 200, Summary: "b"},
		{SessionID: "a", LastModified: 300, Summary: "new"},
	}

	deduped := deduplicateBySessionID(sessions)
	if len(deduped) != 2 {
		t.Fatalf("expected 2 deduplicated sessions, got %d", len(deduped))
	}

	byID := make(map[string]SDKSessionInfo)
	for _, s := range deduped {
		byID[s.SessionID] = s
	}

	if byID["a"].LastModified != 300 {
		t.Errorf("expected newer 'a' to win (mtime 300), got %d", byID["a"].LastModified)
	}
	if byID["a"].Summary != "new" {
		t.Errorf("expected summary 'new', got %q", byID["a"].Summary)
	}
}

// ---------------------------------------------------------------------------
// applySortAndLimit tests
// ---------------------------------------------------------------------------

func TestApplySortAndLimit(t *testing.T) {
	sessions := []SDKSessionInfo{
		{SessionID: "a", LastModified: 100},
		{SessionID: "b", LastModified: 300},
		{SessionID: "c", LastModified: 200},
	}

	sorted := applySortAndLimit(sessions, 0, nil)
	if sorted[0].SessionID != "b" || sorted[1].SessionID != "c" || sorted[2].SessionID != "a" {
		t.Errorf("unexpected sort order: %v", sorted)
	}

	limited := applySortAndLimit([]SDKSessionInfo{
		{SessionID: "a", LastModified: 100},
		{SessionID: "b", LastModified: 300},
		{SessionID: "c", LastModified: 200},
	}, 0, intPtr(1))
	if len(limited) != 1 {
		t.Fatalf("expected 1, got %d", len(limited))
	}
	if limited[0].SessionID != "b" {
		t.Errorf("expected session 'b' (highest mtime), got %q", limited[0].SessionID)
	}
}

// ---------------------------------------------------------------------------
// extractJSONStringField / extractLastJSONStringField edge cases
// ---------------------------------------------------------------------------

func TestExtractJSONStringField_EscapedCharacters(t *testing.T) {
	text := `{"path":"C:\\Users\\test\\file"}`
	got := extractJSONStringField(text, "path")
	if got != `C:\Users\test\file` {
		t.Errorf("expected unescaped path, got %q", got)
	}
}

func TestExtractJSONStringField_UnicodeEscape(t *testing.T) {
	text := `{"name":"caf\u00e9"}`
	got := extractJSONStringField(text, "name")
	if got != "caf\u00e9" {
		t.Errorf("expected 'caf\u00e9', got %q", got)
	}
}

func TestExtractJSONStringField_EmptyValue(t *testing.T) {
	text := `{"key":""}`
	got := extractJSONStringField(text, "key")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestExtractLastJSONStringField_SingleOccurrence(t *testing.T) {
	text := `{"title":"only one"}`
	got := extractLastJSONStringField(text, "title")
	if got != "only one" {
		t.Errorf("expected 'only one', got %q", got)
	}
}

func TestExtractLastJSONStringField_NoMatch(t *testing.T) {
	text := `{"other":"value"}`
	got := extractLastJSONStringField(text, "title")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// Cwd extraction tests
// ---------------------------------------------------------------------------

func TestListSessions_CwdExtraction(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/cwd")


	content := strings.Join([]string{
		makeSessionLine(map[string]any{
			"type":    "user",
			"uuid":    "u1",
			"cwd":     "/my/working/dir",
			"message": map[string]any{"content": "Hello"},
		}),
		makeAssistantLine("a1", "u1", "Hi"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/cwd",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Cwd != "/my/working/dir" {
		t.Errorf("expected cwd '/my/working/dir', got %q", sessions[0].Cwd)
	}
}

func TestListSessions_CwdFallsBackToProjectPath(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/cwd-fallback")


	// No cwd field in the session.
	content := strings.Join([]string{
		makeUserLine("u1", "", "Hello"),
		makeAssistantLine("a1", "u1", "Hi"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/cwd-fallback",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	// When no cwd in file, it falls back to the project path passed to readSessionsFromDir.
	// The canonicalized directory is used.
	if sessions[0].Cwd == "" {
		t.Error("expected non-empty cwd (fallback to project path)")
	}
}

// ---------------------------------------------------------------------------
// normalizeNFC tests
// ---------------------------------------------------------------------------

func TestNormalizeNFC(t *testing.T) {
	// NFC normalization: combining characters should be composed.
	// e + combining acute accent -> e-acute
	input := "caf\u0065\u0301" // "cafe" with combining accent on e
	got := normalizeNFC(input)
	if got != "caf\u00e9" {
		t.Errorf("expected NFC normalized string, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// canonicalizePath tests
// ---------------------------------------------------------------------------

func TestCanonicalizePath(t *testing.T) {
	tmpDir := t.TempDir()
	got := canonicalizePath(tmpDir)
	if got == "" {
		t.Error("expected non-empty canonicalized path")
	}
	// Should be an absolute path.
	if !filepath.IsAbs(got) {
		t.Errorf("expected absolute path, got %q", got)
	}
}

func TestCanonicalizePath_NonexistentPath(t *testing.T) {
	got := canonicalizePath("/nonexistent/path/xyz")
	if got == "" {
		t.Error("expected non-empty result even for nonexistent path")
	}
}

// ---------------------------------------------------------------------------
// sanitizeTag tests
// ---------------------------------------------------------------------------

func TestSanitizeTag(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"zero-width chars", "he\u200Bllo", "hello"},
		{"directionality markers", "he\u200Ello", "hello"},
		{"private use chars", "he\uE000llo", "hello"},
		{"NFKC normalization", "\uFB01", "fi"}, // ﬁ ligature -> fi
		{"trims whitespace", "  hello  ", "hello"},
		{"bidi override", "test\u202Atext", "testtext"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTag(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TagSession tests
// ---------------------------------------------------------------------------

func TestTagSession(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/tag-session")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	tag := "my-tag"
	dir := "/test/tag-session"
	err := TagSession(sessionID, &tag, &dir)
	if err != nil {
		t.Fatalf("TagSession failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), `"tag"`) {
		t.Error("expected tag entry in session file")
	}
	if !strings.Contains(string(content), `"my-tag"`) {
		t.Error("expected tag value in session file")
	}
}

func TestTagSession_InvalidUUID(t *testing.T) {
	err := TagSession("not-a-uuid", nil, nil)
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestSanitizeTag_CombinedProblematicChars(t *testing.T) {
	// Mix of zero-width, directionality, and private-use characters
	input := "he\u200B\u200Ell\uE000o"
	got := sanitizeTag(input)
	if got != "hello" {
		t.Errorf("sanitizeTag(%q) = %q, want %q", input, got, "hello")
	}
}

func TestSanitizeTag_EmptyAfterSanitization(t *testing.T) {
	// All characters are stripped
	input := "\u200B\u200E\uE000"
	got := sanitizeTag(input)
	if got != "" {
		t.Errorf("sanitizeTag(%q) = %q, want empty string", input, got)
	}
}

func TestSanitizeTag_EmptyString(t *testing.T) {
	got := sanitizeTag("")
	if got != "" {
		t.Errorf("sanitizeTag(%q) = %q, want empty string", "", got)
	}
}

func TestTagSession_NilTag(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/nil-tag")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	dir := "/test/nil-tag"
	err := TagSession(sessionID, nil, &dir)
	if err != nil {
		t.Fatalf("TagSession with nil tag failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	// Parse the appended line and verify empty tag
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	lastLine := lines[len(lines)-1]

	var entry map[string]any
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil {
		t.Fatalf("appended entry is not valid JSON: %v", err)
	}
	if entry["tag"] != "" {
		t.Errorf("expected empty tag for nil input, got %v", entry["tag"])
	}
}

func TestTagSession_SessionNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	sessionID := "12345678-1234-1234-1234-123456789012"
	tag := "my-tag"
	dir := "/test/nonexistent-project"
	err := TagSession(sessionID, &tag, &dir)
	if err == nil {
		t.Fatal("expected error for missing session file")
	}
	if !strings.Contains(err.Error(), "session file not found") {
		t.Errorf("expected 'session file not found' error, got: %v", err)
	}
}

func TestTagSession_JSONLFormat(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/jsonl-format")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	tag := "release-v1"
	dir := "/test/jsonl-format"
	if err := TagSession(sessionID, &tag, &dir); err != nil {
		t.Fatalf("TagSession failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	lastLine := lines[len(lines)-1]

	var entry map[string]any
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil {
		t.Fatalf("appended entry is not valid JSON: %v\nline: %s", err, lastLine)
	}

	if entry["type"] != "tag" {
		t.Errorf("expected type 'tag', got %v", entry["type"])
	}
	if entry["tag"] != "release-v1" {
		t.Errorf("expected tag 'release-v1', got %v", entry["tag"])
	}
	if entry["sessionId"] != sessionID {
		t.Errorf("expected sessionId %q, got %v", sessionID, entry["sessionId"])
	}
}

func TestRenameSession(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/rename-session")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	dir := "/test/rename-session"
	err := RenameSession(sessionID, "My Custom Title", &dir)
	if err != nil {
		t.Fatalf("RenameSession failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), `"custom-title"`) {
		t.Error("expected custom-title entry in session file")
	}
	if !strings.Contains(string(content), `"My Custom Title"`) {
		t.Error("expected custom title value in session file")
	}
}

func TestRenameSession_InvalidUUID(t *testing.T) {
	err := RenameSession("not-a-uuid", "title", nil)
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "invalid session ID") {
		t.Errorf("expected 'invalid session ID' error, got: %v", err)
	}
}

func TestRenameSession_EmptyTitle(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"

	err := RenameSession(sessionID, "", nil)
	if err == nil {
		t.Error("expected error for empty title")
	}
	if !strings.Contains(err.Error(), "title cannot be empty") {
		t.Errorf("expected 'title cannot be empty' error, got: %v", err)
	}
}

func TestRenameSession_WhitespaceOnlyTitle(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"

	err := RenameSession(sessionID, "   \t\n  ", nil)
	if err == nil {
		t.Error("expected error for whitespace-only title")
	}
	if !strings.Contains(err.Error(), "title cannot be empty") {
		t.Errorf("expected 'title cannot be empty' error, got: %v", err)
	}
}

func TestRenameSession_TitleTrimmed(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/rename-trim")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	dir := "/test/rename-trim"
	err := RenameSession(sessionID, "  padded title  ", &dir)
	if err != nil {
		t.Fatalf("RenameSession failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	lastLine := lines[len(lines)-1]

	var entry map[string]any
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil {
		t.Fatalf("appended entry is not valid JSON: %v", err)
	}
	if entry["customTitle"] != "padded title" {
		t.Errorf("expected trimmed title 'padded title', got %v", entry["customTitle"])
	}
}

func TestRenameSession_SessionNotFound(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())

	sessionID := "12345678-1234-1234-1234-123456789012"
	dir := "/test/nonexistent-project"
	err := RenameSession(sessionID, "title", &dir)
	if err == nil {
		t.Fatal("expected error for missing session file")
	}
	if !strings.Contains(err.Error(), "session file not found") {
		t.Errorf("expected 'session file not found' error, got: %v", err)
	}
}

func TestRenameSession_JSONLFormat(t *testing.T) {
	sessionID := "12345678-1234-1234-1234-123456789012"
	projDir := setupTestProjectDir(t, "/test/rename-jsonl")

	sessionFile := writeSessionFile(t, projDir, sessionID,
		`{"type":"user","uuid":"u1","sessionId":"`+sessionID+`","message":{"role":"user","content":"hello"}}`+"\n")

	dir := "/test/rename-jsonl"
	if err := RenameSession(sessionID, "New Title", &dir); err != nil {
		t.Fatalf("RenameSession failed: %v", err)
	}

	content, err := os.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	lastLine := lines[len(lines)-1]

	var entry map[string]any
	if err := json.Unmarshal([]byte(lastLine), &entry); err != nil {
		t.Fatalf("appended entry is not valid JSON: %v\nline: %s", err, lastLine)
	}

	if entry["type"] != "custom-title" {
		t.Errorf("expected type 'custom-title', got %v", entry["type"])
	}
	if entry["customTitle"] != "New Title" {
		t.Errorf("expected customTitle 'New Title', got %v", entry["customTitle"])
	}
	if entry["sessionId"] != sessionID {
		t.Errorf("expected sessionId %q, got %v", sessionID, entry["sessionId"])
	}
}

// ---------------------------------------------------------------------------
// GetSessionInfo tests
// ---------------------------------------------------------------------------

func TestGetSessionInfo_Basic(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello Claude"}}`,
		makeAssistantLine("a1", "u1", "Hi there!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil session info")
	}
	if info.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, info.SessionID)
	}
	if info.FirstPrompt != "Hello Claude" {
		t.Errorf("expected first prompt 'Hello Claude', got %q", info.FirstPrompt)
	}
	if info.FileSize == nil {
		t.Error("expected non-nil FileSize")
	}
}

func TestGetSessionInfo_WithTag(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-tag")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
		`{"type":"tag","tag":"my-feature-tag","sessionId":"` + sessionID + `"}`,
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info-tag")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.Tag == nil {
		t.Fatal("expected non-nil Tag")
	}
	if *info.Tag != "my-feature-tag" {
		t.Errorf("expected tag 'my-feature-tag', got %q", *info.Tag)
	}
}

func TestGetSessionInfo_TagOnlyFromTagType(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-tag-type")

	// Include a user message that mentions "tag" in its content — should NOT be picked up
	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"please tag this"}}`,
		makeAssistantLine("a1", "u1", "Done!"),
		// Entry with a "tag" key but type is NOT "tag" — should NOT be picked up
		`{"type":"user","uuid":"u2","parentUuid":"a1","message":{"role":"user","content":"result"},"tag":"not-this-one"}`,
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info-tag-type")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.Tag != nil {
		t.Errorf("expected nil Tag (no type:tag entry), got %q", *info.Tag)
	}
}

func TestGetSessionInfo_CreatedAt(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-created")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info-created")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.CreatedAt == nil {
		t.Fatal("expected non-nil CreatedAt")
	}
	// 2025-01-15T10:30:00.000Z in Unix milliseconds
	expected := int64(1736937000000)
	if *info.CreatedAt != expected {
		t.Errorf("expected CreatedAt %d, got %d", expected, *info.CreatedAt)
	}
}

func TestGetSessionInfo_CreatedAtNumericTimestamp(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-created-num")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":1736937000000,"message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info-created-num")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.CreatedAt == nil {
		t.Fatal("expected non-nil CreatedAt")
	}
	if *info.CreatedAt != 1736937000000 {
		t.Errorf("expected CreatedAt 1736937000000, got %d", *info.CreatedAt)
	}
}

func TestGetSessionInfo_NoTimestamp(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-no-ts")

	content := strings.Join([]string{
		makeUserLine("u1", "", "Hello"),
		makeAssistantLine("a1", "u1", "Hi!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	info, err := GetSessionInfo(sessionID, "/test/get-info-no-ts")
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.CreatedAt != nil {
		t.Errorf("expected nil CreatedAt, got %d", *info.CreatedAt)
	}
}

func TestGetSessionInfo_NotFound(t *testing.T) {
	setupTestProjectDir(t, "/test/get-info-notfound")

	_, err := GetSessionInfo(testUUID1, "/test/get-info-notfound")
	if err == nil {
		t.Error("expected error for non-existent session")
	}
}

func TestGetSessionInfo_InvalidUUID(t *testing.T) {
	_, err := GetSessionInfo("not-a-uuid")
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}

func TestGetSessionInfo_NoDirectory(t *testing.T) {
	sessionID := testUUID1
	projDir := setupTestProjectDir(t, "/test/get-info-nodir")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
		`{"type":"tag","tag":"test-tag","sessionId":"` + sessionID + `"}`,
	}, "\n") + "\n"
	writeSessionFile(t, projDir, sessionID, content)

	// Call without directory — should search all project dirs
	info, err := GetSessionInfo(sessionID)
	if err != nil {
		t.Fatalf("GetSessionInfo failed: %v", err)
	}
	if info.SessionID != sessionID {
		t.Errorf("expected session ID %s, got %s", sessionID, info.SessionID)
	}
	if info.Tag == nil || *info.Tag != "test-tag" {
		t.Error("expected tag 'test-tag'")
	}
}

// ---------------------------------------------------------------------------
// ListSessions tag/created_at integration tests
// ---------------------------------------------------------------------------

func TestListSessions_TagExtraction(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/list-tag")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
		`{"type":"tag","tag":"feature-x","sessionId":"` + testUUID1 + `"}`,
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/list-tag",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Tag == nil {
		t.Fatal("expected non-nil Tag")
	}
	if *sessions[0].Tag != "feature-x" {
		t.Errorf("expected tag 'feature-x', got %q", *sessions[0].Tag)
	}
}

func TestListSessions_CreatedAtExtraction(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/list-created")

	content := strings.Join([]string{
		`{"type":"user","uuid":"u1","timestamp":"2025-01-15T10:30:00.000Z","message":{"role":"user","content":"Hello"}}`,
		makeAssistantLine("a1", "u1", "Hi!"),
	}, "\n") + "\n"
	writeSessionFile(t, projDir, testUUID1, content)

	sessions, err := ListSessions(ListSessionsOptions{
		Directory:        "/test/list-created",
		IncludeWorktrees: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].CreatedAt == nil {
		t.Fatal("expected non-nil CreatedAt")
	}
	expected := int64(1736937000000)
	if *sessions[0].CreatedAt != expected {
		t.Errorf("expected CreatedAt %d, got %d", expected, *sessions[0].CreatedAt)
	}
}

func TestExtractTagFromTranscript_OnlyTagType(t *testing.T) {
	// A line with type "user" that has a "tag" field should NOT be extracted
	head := `{"type":"user","uuid":"u1","tag":"wrong","message":{"role":"user","content":"hi"}}` + "\n"
	tail := head

	tag := extractTagFromTranscript(head, tail)
	if tag != nil {
		t.Errorf("expected nil tag, got %q", *tag)
	}
}

func TestExtractTagFromTranscript_LastTagWins(t *testing.T) {
	head := strings.Join([]string{
		`{"type":"tag","tag":"first-tag","sessionId":"s1"}`,
		`{"type":"tag","tag":"second-tag","sessionId":"s1"}`,
	}, "\n") + "\n"
	tail := head

	tag := extractTagFromTranscript(head, tail)
	if tag == nil {
		t.Fatal("expected non-nil tag")
	}
	if *tag != "second-tag" {
		t.Errorf("expected 'second-tag', got %q", *tag)
	}
}

func TestExtractCreatedAtFromHead_NoTimestamp(t *testing.T) {
	head := `{"type":"user","uuid":"u1","message":{"role":"user","content":"hello"}}` + "\n"
	result := extractCreatedAtFromHead(head)
	if result != nil {
		t.Errorf("expected nil, got %d", *result)
	}
}

// ---------------------------------------------------------------------------
// DeleteSession
// ---------------------------------------------------------------------------

func TestDeleteSession_RemovesJSONLAndSubagentDir(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/delete-cascade")
	filePath := writeSessionFile(t, projDir, testUUID1, "{}\n")

	subagentDir := filepath.Join(projDir, testUUID1)
	if err := os.MkdirAll(subagentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	subagentFile := filepath.Join(subagentDir, testUUID2+".jsonl")
	if err := os.WriteFile(subagentFile, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteSession(testUUID1, "/test/delete-cascade"); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected .jsonl to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(subagentDir); !os.IsNotExist(err) {
		t.Errorf("expected subagent dir to be removed, stat err=%v", err)
	}
}

func TestDeleteSession_NoSubagentDirNoError(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/delete-no-subagent")
	filePath := writeSessionFile(t, projDir, testUUID1, "{}\n")

	if err := DeleteSession(testUUID1, "/test/delete-no-subagent"); err != nil {
		t.Fatalf("DeleteSession returned error: %v", err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Errorf("expected .jsonl to be removed, stat err=%v", err)
	}
}

func TestDeleteSession_MissingSessionStillErrors(t *testing.T) {
	// Preserves existing "session not found" behavior when the .jsonl is
	// absent. Ensures the cascade logic didn't change the primary contract.
	_ = setupTestProjectDir(t, "/test/delete-missing")

	err := DeleteSession(testUUID1, "/test/delete-missing")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
}

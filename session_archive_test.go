package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func archiveTestUserLine(ts, text string, extra map[string]any) string {
	m := map[string]any{
		"type":      "user",
		"uuid":      "u-" + ts,
		"timestamp": ts,
		"message":   map[string]any{"role": "user", "content": text},
	}
	for k, v := range extra {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func listOne(t *testing.T, dir string) SDKSessionInfo {
	t.Helper()
	sessions, err := ListSessions(ListSessionsOptions{Directory: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	return sessions[0]
}

// TestArchiveSession_RoundTrip verifies archiving writes the mark beside the
// transcript, ListSessions and GetSessionInfo report it, and unarchiving
// flips it to false. Port of TypeScript SDK v0.3.324.
func TestArchiveSession_RoundTrip(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/archive")
	transcript := archiveTestUserLine("2020-01-01T00:00:00.000Z", "Hello", nil) + "\n"
	writeSessionFile(t, projDir, testUUID1, transcript)

	if got := listOne(t, "/test/archive").Archived; got != nil {
		t.Fatalf("Archived = %v before archiving, want nil", *got)
	}

	if err := ArchiveSession(testUUID1, "/test/archive"); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	markPath := filepath.Join(projDir, testUUID1, "archived.json")
	data, err := os.ReadFile(markPath)
	if err != nil {
		t.Fatalf("reading mark: %v", err)
	}
	var mark map[string]any
	if err := json.Unmarshal(data, &mark); err != nil {
		t.Fatalf("mark is not JSON: %v", err)
	}
	if mark["archived"] != true || mark["by"] != "sdk" {
		t.Errorf("mark = %v, want archived true by sdk", mark)
	}
	if at, _ := mark["at"].(string); !strings.HasSuffix(at, "Z") {
		t.Errorf("at = %q, want a UTC ISO timestamp", at)
	} else if _, err := time.Parse(time.RFC3339Nano, at); err != nil {
		t.Errorf("at = %q does not parse: %v", at, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(markPath); err == nil && info.Mode().Perm() != 0o600 {
			t.Errorf("mark mode = %v, want 0600", info.Mode().Perm())
		}
	}

	if got := listOne(t, "/test/archive").Archived; got == nil || !*got {
		t.Errorf("ListSessions Archived = %v, want true", got)
	}
	info, err := GetSessionInfo(testUUID1, "/test/archive")
	if err != nil {
		t.Fatal(err)
	}
	if info.Archived == nil || !*info.Archived {
		t.Errorf("GetSessionInfo Archived = %v, want true", info.Archived)
	}

	// The transcript is left untouched.
	if b, _ := os.ReadFile(filepath.Join(projDir, testUUID1+".jsonl")); string(b) != transcript {
		t.Errorf("transcript changed: %q", b)
	}

	if err := UnarchiveSession(testUUID1, "/test/archive"); err != nil {
		t.Fatalf("UnarchiveSession: %v", err)
	}
	if got := listOne(t, "/test/archive").Archived; got == nil || *got {
		t.Errorf("Archived after unarchive = %v, want false", got)
	}
}

// TestArchiveSession_AllProjects verifies archiving without a directory
// marks the session in every project directory holding a copy.
func TestArchiveSession_AllProjects(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/archive-a")
	other := filepath.Join(filepath.Dir(projDir), sanitizePath("/test/archive-b"))
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	line := archiveTestUserLine("2020-01-01T00:00:00.000Z", "Hello", nil) + "\n"
	writeSessionFile(t, projDir, testUUID1, line)
	writeSessionFile(t, other, testUUID1, line)

	if err := ArchiveSession(testUUID1); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
	for _, d := range []string{projDir, other} {
		if _, err := os.Stat(filepath.Join(d, testUUID1, "archived.json")); err != nil {
			t.Errorf("no mark in %s: %v", d, err)
		}
	}
}

// TestArchiveSession_Errors verifies invalid and missing sessions fail.
func TestArchiveSession_Errors(t *testing.T) {
	_ = setupTestProjectDir(t, "/test/archive-missing")
	if err := ArchiveSession("not-a-uuid"); err == nil {
		t.Error("expected an error for an invalid session ID")
	}
	if err := ArchiveSession(testUUID1, "/test/archive-missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want not found", err)
	}
	if err := UnarchiveSession(testUUID1); err == nil || !strings.Contains(err.Error(), "not found in any project directory") {
		t.Errorf("err = %v, want not found in any project directory", err)
	}
}

// TestSessionArchived_PromptAfterUnarchives verifies a prompt timestamped
// after the mark makes Archived false, while earlier prompts, tool results
// and non-human entries do not.
func TestSessionArchived_PromptAfterUnarchives(t *testing.T) {
	projDir := setupTestProjectDir(t, "/test/archive-prompt")
	mark := `{"archived":true,"at":"2026-01-01T00:00:00.000Z","by":"sdk"}`
	writeMark := func() {
		if err := os.MkdirAll(filepath.Join(projDir, testUUID1), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(projDir, testUUID1, "archived.json"), []byte(mark), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeMark()

	cases := []struct {
		name  string
		later string
		want  bool
	}{
		{"none", "", true},
		{"typed prompt", archiveTestUserLine("2026-02-01T00:00:00.000Z", "keep going", nil), false},
		{"meta entry", archiveTestUserLine("2026-02-01T00:00:00.000Z", "x", map[string]any{"isMeta": true}), true},
		{"non-human origin", archiveTestUserLine("2026-02-01T00:00:00.000Z", "x", map[string]any{"origin": map[string]any{"kind": "task-notification"}}), true},
		{"human origin", archiveTestUserLine("2026-02-01T00:00:00.000Z", "x", map[string]any{"origin": map[string]any{"kind": "human"}}), false},
		{"markup only", archiveTestUserLine("2026-02-01T00:00:00.000Z", "<local-command-stdout>ok</local-command-stdout>", nil), true},
		{"slash command", archiveTestUserLine("2026-02-01T00:00:00.000Z", "<command-message>review</command-message>\n<command-name>/review</command-name>", nil), false},
		{"scheduled", archiveTestUserLine("2026-02-01T00:00:00.000Z", "x", map[string]any{"scheduledTaskId": "t1"}), true},
		{"queued command", `{"type":"attachment","timestamp":"2026-02-01T00:00:00.000Z","attachment":{"type":"queued_command","prompt":"next"}}`, false},
		{"tool result", `{"type":"user","timestamp":"2026-02-01T00:00:00.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"t","content":"ok"}]}}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := []string{archiveTestUserLine("2025-12-01T00:00:00.000Z", "Hello", nil)}
			if tc.later != "" {
				lines = append(lines, tc.later)
			}
			writeSessionFile(t, projDir, testUUID1, strings.Join(lines, "\n")+"\n")
			got := listOne(t, "/test/archive-prompt").Archived
			if got == nil || *got != tc.want {
				t.Errorf("Archived = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestReadSessionArchiveMark_Malformed verifies malformed or oversized marks
// are ignored.
func TestReadSessionArchiveMark_Malformed(t *testing.T) {
	for _, data := range []string{
		`not json`,
		`{"archived":"yes","at":"2026-01-01T00:00:00Z"}`,
		`{"archived":true}`,
		`{"archived":true,"at":"2026-01-01T00:00:00"}`,
	} {
		if m := parseSessionArchiveMark([]byte(data)); m != nil {
			t.Errorf("parseSessionArchiveMark(%s) = %+v, want nil", data, m)
		}
	}
	if m := parseSessionArchiveMark([]byte(`{"archived":true,"at":"2026-01-01T00:00:00+02:00"}`)); m == nil || !m.archived {
		t.Errorf("offset timestamp not accepted: %+v", m)
	}

	dir := t.TempDir()
	transcript := filepath.Join(dir, testUUID1+".jsonl")
	if err := os.MkdirAll(filepath.Join(dir, testUUID1), 0o700); err != nil {
		t.Fatal(err)
	}
	big := `{"archived":true,"at":"2026-01-01T00:00:00Z","pad":"` + strings.Repeat("x", 5000) + `"}`
	if err := os.WriteFile(filepath.Join(dir, testUUID1, "archived.json"), []byte(big), 0o600); err != nil {
		t.Fatal(err)
	}
	if m := readSessionArchiveMark(transcript, testUUID1); m != nil {
		t.Errorf("oversized mark read: %+v", m)
	}
}

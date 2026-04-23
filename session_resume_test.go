package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sessionA/sessionB/sessionC are canonical UUIDs used throughout the
// tests as session IDs. Plain placeholders so the test signal isn't
// drowned in boilerplate.
const (
	sessionA = "aaaaaaaa-1111-2222-3333-444444444444"
	sessionB = "bbbbbbbb-1111-2222-3333-444444444444"
	sessionC = "cccccccc-1111-2222-3333-444444444444"
)

// readJSONLFile reads a JSONL file into parsed map entries for
// assertions.
func readJSONLFile(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	var out []map[string]any
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			t.Fatalf("parse JSONL in %s: %v", path, err)
		}
		out = append(out, m)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return out
}

func TestMaterializeResumeSession_BasicMainTranscript(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)

	entries := []SessionStoreEntry{
		entry(map[string]any{"type": "user", "message": map[string]any{"content": "hello"}}),
		entry(map[string]any{"type": "assistant", "message": map[string]any{"content": "hi"}}),
	}
	if err := store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, entries); err != nil {
		t.Fatalf("seed Append: %v", err)
	}

	opts := &Options{
		SessionStore: store,
		Resume:       sessionA,
		Cwd:          cwd,
	}

	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr == nil {
		t.Fatal("expected materializedResume, got nil")
	}
	defer mr.cleanup()

	if mr.resumeSessionID != sessionA {
		t.Errorf("resumeSessionID = %q, want %q", mr.resumeSessionID, sessionA)
	}
	mainFile := filepath.Join(mr.configDir, "projects", projectKey, sessionA+".jsonl")
	got := readJSONLFile(t, mainFile)
	if len(got) != 2 {
		t.Fatalf("want 2 entries in main transcript, got %d", len(got))
	}
	if got[0]["type"] != "user" || got[1]["type"] != "assistant" {
		t.Errorf("entries out of order: %+v", got)
	}
}

func TestMaterializeResumeSession_SubkeysMaterialized(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)

	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user"}),
	})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA, Subpath: "subagents/agent-xyz"}, []SessionStoreEntry{
		entry(map[string]any{"type": "user", "from": "agent-xyz"}),
	})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA, Subpath: "subagents/agent-abc"}, []SessionStoreEntry{
		entry(map[string]any{"type": "assistant", "from": "agent-abc"}),
	})

	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr == nil {
		t.Fatal("expected materializedResume, got nil")
	}
	defer mr.cleanup()

	projectDir := filepath.Join(mr.configDir, "projects", projectKey)
	for _, suffix := range []string{
		sessionA + ".jsonl",
		filepath.Join(sessionA, "subagents", "agent-xyz.jsonl"),
		filepath.Join(sessionA, "subagents", "agent-abc.jsonl"),
	} {
		path := filepath.Join(projectDir, suffix)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected file %s to exist: %v", path, err)
		}
	}
}

func TestMaterializeResumeSession_SubkeysWithAgentMetadataSplit(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)

	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user"}),
	})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA, Subpath: "subagents/agent-xyz"}, []SessionStoreEntry{
		entry(map[string]any{"type": "agent_metadata", "name": "xyz", "task": "t1"}),
		entry(map[string]any{"type": "user", "content": "hi"}),
		entry(map[string]any{"type": "agent_metadata", "name": "xyz", "task": "t2"}), // last wins
	})

	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	defer mr.cleanup()

	subDir := filepath.Join(mr.configDir, "projects", projectKey, sessionA, "subagents")
	transcript := readJSONLFile(t, filepath.Join(subDir, "agent-xyz.jsonl"))
	if len(transcript) != 1 {
		t.Fatalf("want 1 transcript entry (metadata partitioned out), got %d", len(transcript))
	}
	if transcript[0]["type"] != "user" {
		t.Errorf("expected transcript[0] to be user entry, got %+v", transcript[0])
	}

	metaBytes, err := os.ReadFile(filepath.Join(subDir, "agent-xyz.meta.json"))
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("parse meta.json: %v", err)
	}
	if _, has := meta["type"]; has {
		t.Error("meta.json should not contain the synthetic 'type' field")
	}
	if meta["task"] != "t2" {
		t.Errorf("last-wins: want task=t2, got %v", meta["task"])
	}
}

func TestMaterializeResumeSession_ContinueResolvesNewest(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)

	// Seed three sessions; Append bumps mtime on every call so inserting
	// sessionC last makes it the newest.
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{entry(map[string]any{"type": "user", "content": "A"})})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionB}, []SessionStoreEntry{entry(map[string]any{"type": "user", "content": "B"})})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionC}, []SessionStoreEntry{entry(map[string]any{"type": "user", "content": "C"})})

	opts := &Options{SessionStore: store, ContinueConversation: true, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr == nil {
		t.Fatal("expected materializedResume, got nil")
	}
	defer mr.cleanup()

	if mr.resumeSessionID != sessionC {
		t.Errorf("resumeSessionID = %q, want newest %q", mr.resumeSessionID, sessionC)
	}
}

func TestMaterializeResumeSession_ContinueSkipsSidechains(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()

	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)

	// Non-sidechain first, then a sidechain with a later mtime.
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user", "content": "user convo"}),
	})
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionB}, []SessionStoreEntry{
		entry(map[string]any{"type": "user", "content": "side", "isSidechain": true}),
	})

	opts := &Options{SessionStore: store, ContinueConversation: true, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr == nil {
		t.Fatal("expected materializedResume, got nil")
	}
	defer mr.cleanup()

	if mr.resumeSessionID != sessionA {
		t.Errorf("resumeSessionID = %q, want non-sidechain %q", mr.resumeSessionID, sessionA)
	}
}

func TestMaterializeResumeSession_ContinueEmptyStoreReturnsNil(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	cwd := t.TempDir()

	opts := &Options{SessionStore: store, ContinueConversation: true, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr != nil {
		defer mr.cleanup()
		t.Fatalf("expected nil for empty store, got %+v", mr)
	}
}

func TestMaterializeResumeSession_ResumeUnknownSessionErrors(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	cwd := t.TempDir()

	// Unknown but valid UUID.
	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	_, err := materializeResumeSession(ctx, opts)
	if err == nil {
		t.Fatal("expected error for unknown session UUID, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

func TestMaterializeResumeSession_InvalidUUIDErrors(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	opts := &Options{SessionStore: store, Resume: "not-a-uuid", Cwd: t.TempDir()}
	_, err := materializeResumeSession(ctx, opts)
	if err == nil {
		t.Fatal("expected error for invalid UUID, got nil")
	}
	if !strings.Contains(err.Error(), "not a valid session UUID") {
		t.Errorf("expected UUID validation error, got %v", err)
	}
}

func TestMaterializeResumeSession_NoStoreReturnsNil(t *testing.T) {
	ctx := context.Background()
	opts := &Options{Resume: sessionA}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr != nil {
		defer mr.cleanup()
		t.Fatalf("expected nil when no store set, got %+v", mr)
	}
}

func TestMaterializeResumeSession_NoResumeReturnsNil(t *testing.T) {
	ctx := context.Background()
	opts := &Options{SessionStore: NewInMemorySessionStore()}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if mr != nil {
		defer mr.cleanup()
		t.Fatalf("expected nil when neither Resume nor ContinueConversation set, got %+v", mr)
	}
}

// slowStore is a SessionStore whose Load blocks longer than the
// configured LoadTimeoutMs; used to exercise the timeout error path.
type slowStore struct {
	delay time.Duration
}

func (s *slowStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	return nil
}
func (s *slowStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	select {
	case <-time.After(s.delay):
		return []SessionStoreEntry{entry(map[string]any{"type": "user"})}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestMaterializeResumeSession_LoadTimeoutHonored(t *testing.T) {
	ctx := context.Background()
	opts := &Options{
		SessionStore:  &slowStore{delay: 500 * time.Millisecond},
		Resume:        sessionA,
		Cwd:           t.TempDir(),
		LoadTimeoutMs: 50, // much shorter than slowStore's delay
	}
	start := time.Now()
	_, err := materializeResumeSession(ctx, opts)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got %v", err)
	}
	// Sanity: should fire well before the 500ms delay.
	if elapsed > 400*time.Millisecond {
		t.Errorf("timeout fired too late (%v); bound was 50ms", elapsed)
	}
}

func TestSafeRemoveAll_Cleanup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "claude-resume-test-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	// Populate with a nested file.
	if err := os.MkdirAll(filepath.Join(tempDir, "a", "b"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "a", "b", "c.json"), []byte(`{"k":1}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	safeRemoveAll(tempDir)
	if _, err := os.Stat(tempDir); !os.IsNotExist(err) {
		t.Errorf("expected temp dir removed, got stat err %v", err)
	}

	// Idempotent on missing path.
	safeRemoveAll(tempDir)
}

func TestSafeRemoveAll_EmptyPath(t *testing.T) {
	// Must not panic or touch the filesystem.
	safeRemoveAll("")
}

func TestMaterializeResumeSession_CleanupRemovesTempDir(t *testing.T) {
	ctx := context.Background()
	store := NewInMemorySessionStore()
	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user"}),
	})

	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	if _, err := os.Stat(mr.configDir); err != nil {
		t.Fatalf("configDir should exist before cleanup: %v", err)
	}
	mr.cleanup()
	if _, err := os.Stat(mr.configDir); !os.IsNotExist(err) {
		t.Errorf("configDir should be removed after cleanup, stat err: %v", err)
	}
}

func TestMaterializeResumeSession_PartialFailureCleanup(t *testing.T) {
	// A store that returns entries for Load (so we reach subkeys) but
	// fails ListSubkeys. materializeResumeSession must clean up the temp
	// dir before returning the error.
	ctx := context.Background()
	store := &failingSubkeysStore{
		main: []SessionStoreEntry{entry(map[string]any{"type": "user"})},
	}
	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: t.TempDir()}

	// Snapshot pre-existing claude-resume-* dirs to detect a leak from this call.
	preExisting := countClaudeResumeDirs(t)

	_, err := materializeResumeSession(ctx, opts)
	if err == nil {
		t.Fatal("expected error from ListSubkeys failure, got nil")
	}
	post := countClaudeResumeDirs(t)
	if post > preExisting {
		t.Errorf("partial failure leaked a temp dir: pre=%d post=%d", preExisting, post)
	}
}

// countClaudeResumeDirs returns the number of claude-resume-* dirs under
// os.TempDir(). Used by the leak check in TestMaterializeResumeSession_PartialFailureCleanup.
func countClaudeResumeDirs(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatalf("ReadDir TempDir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "claude-resume-") {
			n++
		}
	}
	return n
}

// failingSubkeysStore is a store that returns entries for main Load but
// errors on ListSubkeys.
type failingSubkeysStore struct {
	main []SessionStoreEntry
}

func (s *failingSubkeysStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	return nil
}
func (s *failingSubkeysStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	if key.Subpath == "" {
		return s.main, nil
	}
	return nil, nil
}
func (s *failingSubkeysStore) ListSubkeys(ctx context.Context, key SessionListSubkeysKey) ([]string, error) {
	return nil, errors.New("simulated subkeys failure")
}

func TestMaterializeResumeSession_AuthFilesCopied(t *testing.T) {
	// Use a fake CLAUDE_CONFIG_DIR with stub credential + settings files
	// and verify they land in the temp dir.
	fakeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	// Python's source of truth copies `.credentials.json` and
	// `.claude.json`. `.credentials.json` gets the refreshToken redaction
	// dance; `.claude.json` copies as-is. `.claude.json` lives at
	// $CLAUDE_CONFIG_DIR/.claude.json when set.
	credsPath := filepath.Join(fakeHome, ".credentials.json")
	credsContent := `{"claudeAiOauth":{"accessToken":"AT","refreshToken":"RT","expiresAt":"tomorrow"},"other":"x"}`
	if err := os.WriteFile(credsPath, []byte(credsContent), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	claudePath := filepath.Join(fakeHome, ".claude.json")
	if err := os.WriteFile(claudePath, []byte(`{"foo":"bar"}`), 0o600); err != nil {
		t.Fatalf("write .claude.json: %v", err)
	}

	ctx := context.Background()
	store := NewInMemorySessionStore()
	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user"}),
	})

	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	defer mr.cleanup()

	// .credentials.json should be present with refreshToken redacted.
	credsOut, err := os.ReadFile(filepath.Join(mr.configDir, ".credentials.json"))
	if err != nil {
		t.Fatalf("read copied credentials: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(credsOut, &got); err != nil {
		t.Fatalf("parse copied credentials: %v", err)
	}
	oauth, ok := got["claudeAiOauth"].(map[string]any)
	if !ok {
		t.Fatalf("claudeAiOauth missing in copied credentials: %+v", got)
	}
	if _, present := oauth["refreshToken"]; present {
		t.Error("refreshToken should be redacted from copied credentials")
	}
	if oauth["accessToken"] != "AT" {
		t.Errorf("accessToken should survive redaction, got %v", oauth["accessToken"])
	}

	// .claude.json copies verbatim.
	claudeOut, err := os.ReadFile(filepath.Join(mr.configDir, ".claude.json"))
	if err != nil {
		t.Fatalf("read copied .claude.json: %v", err)
	}
	if string(claudeOut) != `{"foo":"bar"}` {
		t.Errorf(".claude.json not copied verbatim, got %q", string(claudeOut))
	}
}

func TestMaterializeResumeSession_AuthFilesMissingIsOK(t *testing.T) {
	// Fresh install: no .credentials.json, no .claude.json. Must not error.
	fakeHome := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", fakeHome)

	ctx := context.Background()
	store := NewInMemorySessionStore()
	cwd := t.TempDir()
	projectKey := ProjectKeyForDirectory(cwd)
	_ = store.Append(ctx, SessionKey{ProjectKey: projectKey, SessionID: sessionA}, []SessionStoreEntry{
		entry(map[string]any{"type": "user"}),
	})

	opts := &Options{SessionStore: store, Resume: sessionA, Cwd: cwd}
	mr, err := materializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("materializeResumeSession: %v", err)
	}
	defer mr.cleanup()

	// No credential file should exist at the target.
	if _, err := os.Stat(filepath.Join(mr.configDir, ".credentials.json")); !os.IsNotExist(err) {
		t.Errorf(".credentials.json should not exist when source is missing, stat err: %v", err)
	}
}

func TestApplyMaterializedOptions_SetsEnvAndResume(t *testing.T) {
	orig := &Options{
		Env:                  map[string]string{"FOO": "bar"},
		Resume:               "",
		ContinueConversation: true,
	}
	mr := &materializedResume{
		configDir:       "/tmp/fake-resume",
		resumeSessionID: sessionA,
	}
	out := applyMaterializedOptions(orig, mr)

	if out.Env["CLAUDE_CONFIG_DIR"] != "/tmp/fake-resume" {
		t.Errorf("CLAUDE_CONFIG_DIR not set in Env: %+v", out.Env)
	}
	if out.Env["FOO"] != "bar" {
		t.Error("existing env vars should survive")
	}
	if out.Resume != sessionA {
		t.Errorf("Resume = %q, want %q", out.Resume, sessionA)
	}
	if out.ContinueConversation {
		t.Error("ContinueConversation should be cleared")
	}

	// Mutation guarantee: modifying out.Env must not affect orig.Env.
	out.Env["NEW"] = "x"
	if _, has := orig.Env["NEW"]; has {
		t.Error("applyMaterializedOptions leaked mutation into caller's Env")
	}
}

func TestApplyMaterializedOptions_DoesNotOverrideExistingConfigDir(t *testing.T) {
	orig := &Options{
		Env: map[string]string{"CLAUDE_CONFIG_DIR": "/caller/preset"},
	}
	mr := &materializedResume{configDir: "/tmp/fake-resume", resumeSessionID: sessionA}
	out := applyMaterializedOptions(orig, mr)
	if out.Env["CLAUDE_CONFIG_DIR"] != "/caller/preset" {
		t.Errorf("pre-existing CLAUDE_CONFIG_DIR should not be overwritten, got %q", out.Env["CLAUDE_CONFIG_DIR"])
	}
}

func TestIsSafeSubpath_RejectsTraversalAndAbsolute(t *testing.T) {
	sessionDir := t.TempDir()
	cases := []struct {
		in   string
		safe bool
	}{
		{"", false},
		{"subagents/agent-xyz", true},
		{"subagents/nested/agent-xyz", true},
		{"../escape", false},
		{"subagents/../../escape", false},
		{"/abs/path", false},
		{`\abs\path`, false},
		{`C:\Windows\file`, false},
		{"C:foo", false},
		{"//server/share/file", false},
		{`\\server\share\file`, false},
		{"with\x00null", false},
		{"./relative", false},
	}
	for _, tc := range cases {
		got := isSafeSubpath(tc.in, sessionDir)
		if got != tc.safe {
			t.Errorf("isSafeSubpath(%q) = %v, want %v", tc.in, got, tc.safe)
		}
	}
}

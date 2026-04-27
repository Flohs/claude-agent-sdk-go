package claude

import (
	"context"
	"path/filepath"
	"sort"
	"sync"
	"testing"
)

// entry is a tiny helper for building SessionStoreEntry literals in tests.
func entry(fields map[string]any) SessionStoreEntry {
	return SessionStoreEntry(fields)
}

func TestInMemorySessionStore_AppendLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	key := SessionKey{ProjectKey: "proj", SessionID: "sess-1"}
	entries := []SessionStoreEntry{
		entry(map[string]any{"type": "user", "message": map[string]any{"content": "hello"}}),
		entry(map[string]any{"type": "assistant", "message": map[string]any{"content": "hi"}}),
	}

	if err := s.Append(ctx, key, entries); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := s.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 entries, got %d", len(got))
	}
	if got[0]["type"] != "user" || got[1]["type"] != "assistant" {
		t.Errorf("entries out of order: %+v", got)
	}

	// Appending more entries extends the list.
	more := []SessionStoreEntry{
		entry(map[string]any{"type": "user", "message": map[string]any{"content": "again"}}),
	}
	if err := s.Append(ctx, key, more); err != nil {
		t.Fatalf("Append 2: %v", err)
	}
	got, _ = s.Load(ctx, key)
	if len(got) != 3 {
		t.Fatalf("want 3 entries after second append, got %d", len(got))
	}
}

func TestInMemorySessionStore_LoadMissingKeyReturnsNilNil(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	got, err := s.Load(ctx, SessionKey{ProjectKey: "p", SessionID: "missing"})
	if err != nil {
		t.Fatalf("Load on missing key returned error: %v", err)
	}
	if got != nil {
		t.Errorf("Load on missing key should return nil slice, got %+v", got)
	}
}

func TestInMemorySessionStore_MultiKeyIsolation(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	keyX := SessionKey{ProjectKey: "p", SessionID: "x"}
	keyY := SessionKey{ProjectKey: "p", SessionID: "y"}

	_ = s.Append(ctx, keyX, []SessionStoreEntry{entry(map[string]any{"tag": "x1"})})
	_ = s.Append(ctx, keyY, []SessionStoreEntry{entry(map[string]any{"tag": "y1"})})
	_ = s.Append(ctx, keyX, []SessionStoreEntry{entry(map[string]any{"tag": "x2"})})

	gotX, _ := s.Load(ctx, keyX)
	gotY, _ := s.Load(ctx, keyY)
	if len(gotX) != 2 || gotX[0]["tag"] != "x1" || gotX[1]["tag"] != "x2" {
		t.Errorf("keyX isolation broken: %+v", gotX)
	}
	if len(gotY) != 1 || gotY[0]["tag"] != "y1" {
		t.Errorf("keyY isolation broken: %+v", gotY)
	}
}

func TestInMemorySessionStore_AppendCopiesEntries(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "p", SessionID: "s"}

	in := []SessionStoreEntry{entry(map[string]any{"k": "original"})}
	if err := s.Append(ctx, key, in); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// Mutate the caller slice; stored copy must be unaffected.
	in[0] = entry(map[string]any{"k": "mutated"})

	got, _ := s.Load(ctx, key)
	if got[0]["k"] != "original" {
		t.Errorf("store did not defensively copy entries slice: got %+v", got[0])
	}
}

func TestInMemorySessionStore_ListSessionsOrderedByMtimeDesc(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	// Drive a deterministic clock so mtimes are predictable.
	tick := int64(1000)
	s.nowMilli = func() int64 {
		tick++
		return tick
	}

	for _, id := range []string{"a", "b", "c"} {
		if err := s.Append(ctx, SessionKey{ProjectKey: "proj", SessionID: id},
			[]SessionStoreEntry{entry(map[string]any{"type": "user"})}); err != nil {
			t.Fatalf("Append %s: %v", id, err)
		}
	}

	list, err := s.ListSessions(ctx, "proj")
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("want 3 entries, got %d", len(list))
	}
	if list[0].SessionID != "c" || list[1].SessionID != "b" || list[2].SessionID != "a" {
		t.Errorf("want mtime-desc order [c,b,a], got %+v", list)
	}
	// Subagent subkeys must not appear in ListSessions.
	_ = s.Append(ctx, SessionKey{ProjectKey: "proj", SessionID: "a", Subpath: "subagents/agent-x"},
		[]SessionStoreEntry{entry(map[string]any{})})
	list2, _ := s.ListSessions(ctx, "proj")
	for _, e := range list2 {
		if e.SessionID == "subagents/agent-x" {
			t.Errorf("ListSessions leaked subkey: %+v", e)
		}
	}
	if len(list2) != 3 {
		t.Errorf("ListSessions should still report 3 main sessions, got %d", len(list2))
	}
}

func TestInMemorySessionStore_ListSessionsFiltersByProject(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	_ = s.Append(ctx, SessionKey{ProjectKey: "p1", SessionID: "a"}, []SessionStoreEntry{entry(map[string]any{})})
	_ = s.Append(ctx, SessionKey{ProjectKey: "p2", SessionID: "b"}, []SessionStoreEntry{entry(map[string]any{})})

	p1, _ := s.ListSessions(ctx, "p1")
	p2, _ := s.ListSessions(ctx, "p2")
	if len(p1) != 1 || p1[0].SessionID != "a" {
		t.Errorf("p1 listing wrong: %+v", p1)
	}
	if len(p2) != 1 || p2[0].SessionID != "b" {
		t.Errorf("p2 listing wrong: %+v", p2)
	}
}

func TestInMemorySessionStore_DeleteCascadesSubkeys(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	main := SessionKey{ProjectKey: "proj", SessionID: "sess"}
	sub1 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-a"}
	sub2 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-b"}
	unrelated := SessionKey{ProjectKey: "proj", SessionID: "other"}

	_ = s.Append(ctx, main, []SessionStoreEntry{entry(map[string]any{"type": "user"})})
	_ = s.Append(ctx, sub1, []SessionStoreEntry{entry(map[string]any{"type": "user"})})
	_ = s.Append(ctx, sub2, []SessionStoreEntry{entry(map[string]any{"type": "user"})})
	_ = s.Append(ctx, unrelated, []SessionStoreEntry{entry(map[string]any{"type": "user"})})

	if err := s.Delete(ctx, main); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got, _ := s.Load(ctx, main); got != nil {
		t.Errorf("main key not deleted: %+v", got)
	}
	if got, _ := s.Load(ctx, sub1); got != nil {
		t.Errorf("subkey sub1 not cascade-deleted: %+v", got)
	}
	if got, _ := s.Load(ctx, sub2); got != nil {
		t.Errorf("subkey sub2 not cascade-deleted: %+v", got)
	}
	if got, _ := s.Load(ctx, unrelated); got == nil {
		t.Errorf("unrelated sibling session was wrongly deleted")
	}

	// Summary sidecar for the deleted session is gone.
	summaries, _ := s.ListSessionSummaries(ctx, "proj")
	for _, sum := range summaries {
		if sum.SessionID == "sess" {
			t.Errorf("summary for deleted session still present: %+v", sum)
		}
	}
}

func TestInMemorySessionStore_DeleteTargetedSubkeyOnly(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	main := SessionKey{ProjectKey: "proj", SessionID: "sess"}
	sub1 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-a"}
	sub2 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-b"}

	_ = s.Append(ctx, main, []SessionStoreEntry{entry(map[string]any{"type": "user"})})
	_ = s.Append(ctx, sub1, []SessionStoreEntry{entry(map[string]any{"type": "user"})})
	_ = s.Append(ctx, sub2, []SessionStoreEntry{entry(map[string]any{"type": "user"})})

	// Targeted delete of only sub1; main and sub2 survive.
	if err := s.Delete(ctx, sub1); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := s.Load(ctx, sub1); got != nil {
		t.Errorf("sub1 should be deleted, got %+v", got)
	}
	if got, _ := s.Load(ctx, main); got == nil {
		t.Errorf("main key must not be cascade-deleted from a targeted subpath delete")
	}
	if got, _ := s.Load(ctx, sub2); got == nil {
		t.Errorf("sub2 must not be cascade-deleted from a targeted sub1 delete")
	}
}

func TestInMemorySessionStore_ListSubkeysFiltersBySession(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()

	_ = s.Append(ctx, SessionKey{ProjectKey: "proj", SessionID: "sess-a", Subpath: "subagents/agent-1"},
		[]SessionStoreEntry{entry(map[string]any{})})
	_ = s.Append(ctx, SessionKey{ProjectKey: "proj", SessionID: "sess-a", Subpath: "subagents/agent-2"},
		[]SessionStoreEntry{entry(map[string]any{})})
	_ = s.Append(ctx, SessionKey{ProjectKey: "proj", SessionID: "sess-b", Subpath: "subagents/agent-3"},
		[]SessionStoreEntry{entry(map[string]any{})})

	subs, err := s.ListSubkeys(ctx, SessionListSubkeysKey{ProjectKey: "proj", SessionID: "sess-a"})
	if err != nil {
		t.Fatalf("ListSubkeys: %v", err)
	}
	sort.Strings(subs)
	want := []string{"subagents/agent-1", "subagents/agent-2"}
	if len(subs) != len(want) || subs[0] != want[0] || subs[1] != want[1] {
		t.Errorf("want %+v, got %+v", want, subs)
	}
}

func TestInMemorySessionStore_ConcurrentAppendsRaceSafe(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	const N = 10
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				_ = s.Append(ctx, key, []SessionStoreEntry{
					entry(map[string]any{"g": i, "j": j}),
				})
			}
		}()
	}
	wg.Wait()

	got, err := s.Load(ctx, key)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != N*perGoroutine {
		t.Errorf("want %d total entries, got %d", N*perGoroutine, len(got))
	}
}

func TestInMemorySessionStore_InterfaceConformance(t *testing.T) {
	var s interface{} = NewInMemorySessionStore()

	if _, ok := s.(SessionStore); !ok {
		t.Error("InMemorySessionStore must implement SessionStore")
	}
	if _, ok := s.(SessionStoreLister); !ok {
		t.Error("InMemorySessionStore must implement SessionStoreLister")
	}
	if _, ok := s.(SessionStoreSummarizer); !ok {
		t.Error("InMemorySessionStore must implement SessionStoreSummarizer")
	}
	if _, ok := s.(SessionStoreDeleter); !ok {
		t.Error("InMemorySessionStore must implement SessionStoreDeleter")
	}
	if _, ok := s.(SessionStoreSubkeys); !ok {
		t.Error("InMemorySessionStore must implement SessionStoreSubkeys")
	}
}

func TestInMemorySessionStore_ContextCancellation(t *testing.T) {
	s := NewInMemorySessionStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Append(ctx, SessionKey{ProjectKey: "p", SessionID: "s"}, nil); err == nil {
		t.Error("Append should honor cancelled context")
	}
	if _, err := s.Load(ctx, SessionKey{ProjectKey: "p", SessionID: "s"}); err == nil {
		t.Error("Load should honor cancelled context")
	}
}

func TestProjectKeyForDirectory(t *testing.T) {
	// Matches the existing sanitizePath contract (see sessions_test.go).
	tests := []struct {
		in   string
		want string
	}{
		{"abc123", "abc123"},
		{"/path/with spaces/x!y", "-path-with-spaces-x-y"},
	}
	for _, tt := range tests {
		got := ProjectKeyForDirectory(tt.in)
		if got != tt.want {
			t.Errorf("ProjectKeyForDirectory(%q) = %q, want (canonicalized form of) %q", tt.in, got, tt.want)
		}
	}
}

func TestProjectKeyForDirectory_EmptyDefaultsToCwd(t *testing.T) {
	// Empty string should be treated as "." — should not panic and should
	// produce a non-empty sanitized key.
	if got := ProjectKeyForDirectory(""); got == "" {
		t.Error("ProjectKeyForDirectory(\"\") returned empty string")
	}
}

func TestResolveProjectsDir(t *testing.T) {
	// Establish a known parent-process CLAUDE_CONFIG_DIR via t.Setenv
	// (not the user's real one) so the fallback is reproducible.
	t.Setenv("CLAUDE_CONFIG_DIR", "/parent/.claude")

	t.Run("no Options.Env falls back to parent env", func(t *testing.T) {
		got := resolveProjectsDir(nil)
		want := filepath.Join("/parent/.claude", "projects")
		if got != want {
			t.Errorf("resolveProjectsDir(nil) = %q, want %q", got, want)
		}
	})

	t.Run("Options.Env overrides parent env", func(t *testing.T) {
		got := resolveProjectsDir(map[string]string{
			"CLAUDE_CONFIG_DIR": "/from/opts",
		})
		want := filepath.Join("/from/opts", "projects")
		if got != want {
			t.Errorf("resolveProjectsDir(opts.Env) = %q, want %q", got, want)
		}
	})

	t.Run("empty value in Options.Env is ignored, falls back to parent", func(t *testing.T) {
		got := resolveProjectsDir(map[string]string{
			"CLAUDE_CONFIG_DIR": "",
		})
		want := filepath.Join("/parent/.claude", "projects")
		if got != want {
			t.Errorf("resolveProjectsDir(empty) = %q, want %q", got, want)
		}
	})

	t.Run("unrelated keys in Options.Env do not affect resolution", func(t *testing.T) {
		got := resolveProjectsDir(map[string]string{"OTHER_VAR": "x"})
		want := filepath.Join("/parent/.claude", "projects")
		if got != want {
			t.Errorf("resolveProjectsDir(other) = %q, want %q", got, want)
		}
	})
}

func TestFilePathToSessionKey_MainTranscript(t *testing.T) {
	projectsDir := filepath.Join("home", "user", ".claude", "projects")
	abs := filepath.Join(projectsDir, "-home-user-project", "550e8400-e29b-41d4-a716-446655440000.jsonl")

	key, ok := FilePathToSessionKey(abs, projectsDir)
	if !ok {
		t.Fatalf("FilePathToSessionKey returned ok=false for %s", abs)
	}
	if key.ProjectKey != "-home-user-project" {
		t.Errorf("ProjectKey = %q, want %q", key.ProjectKey, "-home-user-project")
	}
	if key.SessionID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("SessionID = %q", key.SessionID)
	}
	if key.Subpath != "" {
		t.Errorf("Subpath = %q, want empty", key.Subpath)
	}
}

func TestFilePathToSessionKey_SubagentTranscript(t *testing.T) {
	projectsDir := filepath.Join("home", "user", ".claude", "projects")
	abs := filepath.Join(projectsDir, "-home-user-project", "550e8400-e29b-41d4-a716-446655440000", "subagents", "agent-xyz.jsonl")

	key, ok := FilePathToSessionKey(abs, projectsDir)
	if !ok {
		t.Fatalf("FilePathToSessionKey returned ok=false for %s", abs)
	}
	if key.ProjectKey != "-home-user-project" {
		t.Errorf("ProjectKey = %q", key.ProjectKey)
	}
	if key.SessionID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("SessionID = %q", key.SessionID)
	}
	if key.Subpath != "subagents/agent-xyz" {
		t.Errorf("Subpath = %q, want %q", key.Subpath, "subagents/agent-xyz")
	}
}

func TestFilePathToSessionKey_NestedSubagent(t *testing.T) {
	projectsDir := filepath.Join("home", "user", ".claude", "projects")
	abs := filepath.Join(projectsDir, "-proj", "sess", "subagents", "workflows", "run-1", "agent-x.jsonl")

	key, ok := FilePathToSessionKey(abs, projectsDir)
	if !ok {
		t.Fatalf("nested subagent path was rejected")
	}
	if key.Subpath != "subagents/workflows/run-1/agent-x" {
		t.Errorf("Subpath = %q", key.Subpath)
	}
}

func TestFilePathToSessionKey_NotUnderProjectsDir(t *testing.T) {
	projectsDir := filepath.Join("home", "user", ".claude", "projects")
	abs := filepath.Join("somewhere", "else", "file.jsonl")

	if _, ok := FilePathToSessionKey(abs, projectsDir); ok {
		t.Errorf("expected ok=false for path outside projectsDir")
	}
}

func TestFilePathToSessionKey_UnrecognizedShape(t *testing.T) {
	projectsDir := filepath.Join("projects")
	// Only the project-key component.
	abs := filepath.Join(projectsDir, "-proj")
	if _, ok := FilePathToSessionKey(abs, projectsDir); ok {
		t.Errorf("single-component rel path should be rejected")
	}
	// Three components but not a subagent-shaped path: <proj>/<sess>/<bare>
	abs = filepath.Join(projectsDir, "-proj", "sess", "bare.jsonl")
	if _, ok := FilePathToSessionKey(abs, projectsDir); ok {
		t.Errorf("three-component path should be rejected (requires >= 4)")
	}
}

func TestFoldSessionSummary_NewSession(t *testing.T) {
	key := SessionKey{ProjectKey: "p", SessionID: "s"}
	entries := []SessionStoreEntry{
		entry(map[string]any{
			"type":        "user",
			"timestamp":   "2026-04-23T10:00:00.000Z",
			"isSidechain": false,
			"cwd":         "/work",
			"message":     map[string]any{"content": "hello there"},
		}),
	}
	got := FoldSessionSummary(nil, key, entries)

	if got == nil {
		t.Fatal("FoldSessionSummary returned nil")
	}
	if got.SessionID != "s" {
		t.Errorf("SessionID = %q", got.SessionID)
	}
	if got.Mtime != 0 {
		t.Errorf("Mtime should be 0 placeholder for new session, got %d", got.Mtime)
	}
	if got.Data["cwd"] != "/work" {
		t.Errorf("cwd = %v", got.Data["cwd"])
	}
	if got.Data["first_prompt"] != "hello there" {
		t.Errorf("first_prompt = %v", got.Data["first_prompt"])
	}
	if locked, _ := got.Data["first_prompt_locked"].(bool); !locked {
		t.Errorf("first_prompt_locked should be true")
	}
	if _, ok := got.Data["created_at"]; !ok {
		t.Errorf("created_at should be set")
	}
}

func TestFoldSessionSummary_MergesWithPrev(t *testing.T) {
	key := SessionKey{ProjectKey: "p", SessionID: "s"}

	first := FoldSessionSummary(nil, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":      "user",
			"timestamp": "2026-04-23T10:00:00.000Z",
			"cwd":       "/work",
			"message":   map[string]any{"content": "first prompt"},
		}),
	})
	// Preserve first.Data for non-leaking check later.
	firstCwd := first.Data["cwd"]
	firstPrompt := first.Data["first_prompt"]

	// Second batch: custom title + a tag + another user message (should
	// NOT override the locked first prompt).
	second := FoldSessionSummary(first, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":        "custom-title",
			"customTitle": "My Title",
		}),
		entry(map[string]any{
			"type":    "user",
			"message": map[string]any{"content": "second prompt"},
		}),
		entry(map[string]any{
			"type": "tag",
			"tag":  "important",
		}),
	})

	if second.Data["custom_title"] != "My Title" {
		t.Errorf("custom_title not folded: %+v", second.Data)
	}
	if second.Data["tag"] != "important" {
		t.Errorf("tag not folded: %+v", second.Data)
	}
	if second.Data["first_prompt"] != firstPrompt {
		t.Errorf("first_prompt must stay locked: got %v want %v", second.Data["first_prompt"], firstPrompt)
	}
	if second.Data["cwd"] != firstCwd {
		t.Errorf("cwd should be set-once: got %v want %v", second.Data["cwd"], firstCwd)
	}

	// Tag-entry with empty string should clear the tag.
	third := FoldSessionSummary(second, key, []SessionStoreEntry{
		entry(map[string]any{"type": "tag", "tag": ""}),
	})
	if _, ok := third.Data["tag"]; ok {
		t.Errorf("empty tag should clear tag key: %+v", third.Data)
	}
}

func TestFoldSessionSummary_IncrementalIndependence(t *testing.T) {
	// Calling Fold with prev must not mutate the prev summary passed in.
	key := SessionKey{ProjectKey: "p", SessionID: "s"}
	prev := FoldSessionSummary(nil, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":      "user",
			"message":   map[string]any{"content": "first"},
			"timestamp": "2026-04-23T10:00:00.000Z",
		}),
	})
	snapshot := make(map[string]any, len(prev.Data))
	for k, v := range prev.Data {
		snapshot[k] = v
	}

	_ = FoldSessionSummary(prev, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":        "custom-title",
			"customTitle": "Added Later",
		}),
	})

	// prev.Data should be unchanged.
	if prev.Data["custom_title"] != nil {
		t.Errorf("FoldSessionSummary must not mutate prev.Data, saw custom_title=%v", prev.Data["custom_title"])
	}
	for k, v := range snapshot {
		if prev.Data[k] != v {
			t.Errorf("prev.Data[%q] mutated: was %v now %v", k, v, prev.Data[k])
		}
	}
}

func TestFoldSessionSummary_SkipsSlashCommandForFirstPrompt(t *testing.T) {
	key := SessionKey{ProjectKey: "p", SessionID: "s"}
	got := FoldSessionSummary(nil, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":      "user",
			"message":   map[string]any{"content": "<command-name>loop</command-name>"},
			"timestamp": "2026-04-23T10:00:00.000Z",
		}),
		entry(map[string]any{
			"type":      "user",
			"message":   map[string]any{"content": "real first prompt"},
			"timestamp": "2026-04-23T10:00:01.000Z",
		}),
	})
	if got.Data["first_prompt"] != "real first prompt" {
		t.Errorf("first_prompt = %v", got.Data["first_prompt"])
	}
	if got.Data["command_fallback"] != "loop" {
		t.Errorf("command_fallback = %v", got.Data["command_fallback"])
	}
}

func TestInMemorySessionStore_SummaryStampedWithAppendMtime(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()
	tick := int64(5000)
	s.nowMilli = func() int64 { tick++; return tick }

	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}
	if err := s.Append(ctx, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":    "user",
			"message": map[string]any{"content": "hi"},
		}),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, _ := s.ListSessions(ctx, "proj")
	summaries, _ := s.ListSessionSummaries(ctx, "proj")
	if len(list) != 1 || len(summaries) != 1 {
		t.Fatalf("want 1 list + 1 summary, got list=%d summary=%d", len(list), len(summaries))
	}
	if summaries[0].Mtime != list[0].Mtime {
		t.Errorf("summary mtime %d must match list mtime %d (shared clock invariant)",
			summaries[0].Mtime, list[0].Mtime)
	}
	if summaries[0].Mtime == 0 {
		t.Errorf("adapter must overwrite fold's 0 placeholder with a real mtime")
	}
}

func TestInMemorySessionStore_SubagentAppendDoesNotFoldSummary(t *testing.T) {
	ctx := context.Background()
	s := NewInMemorySessionStore()
	// Append ONLY to a subagent key.
	key := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-x"}
	if err := s.Append(ctx, key, []SessionStoreEntry{
		entry(map[string]any{
			"type":        "custom-title",
			"customTitle": "SHOULD NOT APPEAR",
		}),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	summaries, _ := s.ListSessionSummaries(ctx, "proj")
	for _, sum := range summaries {
		if sum.Data["custom_title"] == "SHOULD NOT APPEAR" {
			t.Errorf("subagent append must not contribute to main summary: %+v", sum)
		}
	}
}

package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// materializedResume is the result of materializeResumeSession.
//
// configDir is a temporary directory laid out like ~/.claude/; the caller
// points the subprocess at it via the CLAUDE_CONFIG_DIR env var.
// resumeSessionID is the session ID to pass as --resume (for
// ContinueConversation inputs this is the store's newest session; for an
// explicit Resume input this is that same value). cleanup removes the
// temp dir — it must be called after the subprocess exits, even on error
// paths.
type materializedResume struct {
	configDir       string
	resumeSessionID string
	cleanup         func()
}

// defaultLoadTimeoutMs is applied when Options.LoadTimeoutMs is zero.
// Matches the Python default of 10 seconds.
const defaultLoadTimeoutMs = 10000

// materializeResumeSession loads a session from opts.SessionStore and
// writes it to a temp dir in the ~/.claude/ layout so the CLI subprocess
// can --resume it.
//
// Returns (nil, nil) when no materialization is needed (no store, neither
// Resume nor ContinueConversation, empty list, or invalid UUID). In those
// cases the caller falls through to the normal no-store resume/spawn path;
// for ContinueConversation this produces a fresh session (matching the CLI
// behavior with no history), and for an explicit Resume the CLI receives
// the value unchanged.
//
// Returns an error if a store call fails or times out. Partial failures
// clean up the temp dir before returning.
func materializeResumeSession(ctx context.Context, opts *Options) (*materializedResume, error) {
	if opts == nil || opts.SessionStore == nil {
		return nil, nil
	}
	if opts.Resume == "" && !opts.ContinueConversation {
		return nil, nil
	}

	timeoutMs := opts.LoadTimeoutMs
	if timeoutMs <= 0 {
		timeoutMs = defaultLoadTimeoutMs
	}

	cwd := opts.Cwd
	if cwd == "" {
		cwd = "."
	}
	projectKey := ProjectKeyForDirectory(cwd)

	var (
		sessionID string
		entries   []SessionStoreEntry
		err       error
	)
	if opts.Resume != "" {
		// Validate before use as a filesystem path component.
		if !isValidUUID(opts.Resume) {
			return nil, fmt.Errorf("session_store resume: %q is not a valid session UUID", opts.Resume)
		}
		sessionID = opts.Resume
		entries, err = loadWithTimeout(
			ctx, opts.SessionStore,
			SessionKey{ProjectKey: projectKey, SessionID: sessionID},
			timeoutMs,
			fmt.Sprintf("SessionStore.Load() for session %s", sessionID),
		)
		if err != nil {
			return nil, err
		}
		if len(entries) == 0 {
			return nil, fmt.Errorf("session_store resume: session %q not found in store for project %q", sessionID, projectKey)
		}
	} else {
		// ContinueConversation: pick the newest non-sidechain session.
		resolved, resolvedEntries, rerr := resolveContinueCandidate(ctx, opts.SessionStore, projectKey, timeoutMs)
		if rerr != nil {
			return nil, rerr
		}
		if resolved == "" {
			// Empty list → fall through to the CLI's ordinary --continue
			// behavior (which will start a fresh session).
			return nil, nil
		}
		sessionID = resolved
		entries = resolvedEntries
	}

	tempDir, err := os.MkdirTemp("", "claude-resume-")
	if err != nil {
		return nil, fmt.Errorf("session_store resume: create temp dir: %w", err)
	}

	// Any failure after MkdirTemp leaves tempDir on disk (possibly with
	// copied credentials). Remove it before returning. Cleared on full
	// success so the caller owns cleanup via the returned closure.
	cleanupOnError := true
	defer func() {
		if cleanupOnError {
			safeRemoveAll(tempDir)
		}
	}()

	// Sanitize tool_use.id values before writing to ensure they conform to
	// the toolu_[a-zA-Z0-9_-]+ format required by the Claude API.
	entries = sanitizeSessionEntries(entries)

	projectDir := filepath.Join(tempDir, "projects", projectKey)
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return nil, fmt.Errorf("session_store resume: create project dir: %w", err)
	}
	mainFile := filepath.Join(projectDir, sessionID+".jsonl")
	if err := writeJSONL(mainFile, entries); err != nil {
		return nil, fmt.Errorf("session_store resume: write main transcript: %w", err)
	}

	// Copy caller auth files (best-effort: missing files are fine).
	copyAuthFiles(tempDir, opts.Env)

	// Materialize subkeys if the store implements the extension interface.
	if subkeyer, ok := opts.SessionStore.(SessionStoreSubkeys); ok {
		if err := materializeSubkeys(ctx, opts.SessionStore, subkeyer, projectDir, projectKey, sessionID, timeoutMs); err != nil {
			return nil, err
		}
	}

	// Restore in-flight background agent / remote agent / MCP task state by
	// scanning the main transcript for task_started events that have no
	// matching terminal task_notification entry.  Port of TypeScript SDK
	// v0.3.176.
	sessionDir := filepath.Join(projectDir, sessionID)
	if err := materializeActiveTasks(entries, sessionDir); err != nil {
		return nil, err
	}

	cleanupOnError = false
	return &materializedResume{
		configDir:       tempDir,
		resumeSessionID: sessionID,
		cleanup:         func() { safeRemoveAll(tempDir) },
	}, nil
}

// applyMaterializedOptions returns a copy of opts repointed at a
// materialized temp config dir.
//
// The copy has CLAUDE_CONFIG_DIR set in Env (without clobbering a
// pre-existing caller value), Resume set to the materialized session ID,
// and ContinueConversation cleared (already resolved to a concrete
// session ID during materialization).
//
// The returned Options always contains a fresh Env map so callers that
// shared their input map are not surprised by mutation.
func applyMaterializedOptions(opts *Options, mr *materializedResume) *Options {
	out := *opts

	// Fresh map so we never mutate the caller's.
	newEnv := make(map[string]string, len(opts.Env)+1)
	for k, v := range opts.Env {
		newEnv[k] = v
	}
	if _, already := newEnv["CLAUDE_CONFIG_DIR"]; !already {
		newEnv["CLAUDE_CONFIG_DIR"] = mr.configDir
	}
	out.Env = newEnv
	out.Resume = mr.resumeSessionID
	out.ContinueConversation = false
	return &out
}

// ----------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------

// loadWithTimeout wraps store.Load with a per-call timeout. Timeouts and
// adapter failures are surfaced as errors with context.
func loadWithTimeout(ctx context.Context, store SessionStore, key SessionKey, timeoutMs int, what string) ([]SessionStoreEntry, error) {
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	type result struct {
		entries []SessionStoreEntry
		err     error
	}
	done := make(chan result, 1)
	go func() {
		entries, err := store.Load(tctx, key)
		done <- result{entries: entries, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("%s timed out after %dms during resume materialization", what, timeoutMs)
			}
			return nil, fmt.Errorf("%s failed during resume materialization: %w", what, r.err)
		}
		return r.entries, nil
	case <-tctx.Done():
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s timed out after %dms during resume materialization", what, timeoutMs)
		}
		return nil, fmt.Errorf("%s cancelled during resume materialization: %w", what, tctx.Err())
	}
}

// listSessionsWithTimeout wraps lister.ListSessions with a per-call timeout.
func listSessionsWithTimeout(ctx context.Context, lister SessionStoreLister, projectKey string, timeoutMs int) ([]SessionStoreListEntry, error) {
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	type result struct {
		sessions []SessionStoreListEntry
		err      error
	}
	done := make(chan result, 1)
	go func() {
		sessions, err := lister.ListSessions(tctx, projectKey)
		done <- result{sessions: sessions, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("SessionStore.ListSessions() timed out after %dms during resume materialization", timeoutMs)
			}
			return nil, fmt.Errorf("SessionStore.ListSessions() failed during resume materialization: %w", r.err)
		}
		return r.sessions, nil
	case <-tctx.Done():
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("SessionStore.ListSessions() timed out after %dms during resume materialization", timeoutMs)
		}
		return nil, fmt.Errorf("SessionStore.ListSessions() cancelled during resume materialization: %w", tctx.Err())
	}
}

// listSubkeysWithTimeout wraps subkeyer.ListSubkeys with a per-call timeout.
func listSubkeysWithTimeout(ctx context.Context, subkeyer SessionStoreSubkeys, key SessionListSubkeysKey, timeoutMs int) ([]string, error) {
	tctx, cancel := context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
	defer cancel()

	type result struct {
		subkeys []string
		err     error
	}
	done := make(chan result, 1)
	go func() {
		subkeys, err := subkeyer.ListSubkeys(tctx, key)
		done <- result{subkeys: subkeys, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil {
			if errors.Is(r.err, context.DeadlineExceeded) {
				return nil, fmt.Errorf("SessionStore.ListSubkeys() for session %s timed out after %dms during resume materialization", key.SessionID, timeoutMs)
			}
			return nil, fmt.Errorf("SessionStore.ListSubkeys() for session %s failed during resume materialization: %w", key.SessionID, r.err)
		}
		return r.subkeys, nil
	case <-tctx.Done():
		if errors.Is(tctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("SessionStore.ListSubkeys() for session %s timed out after %dms during resume materialization", key.SessionID, timeoutMs)
		}
		return nil, fmt.Errorf("SessionStore.ListSubkeys() for session %s cancelled during resume materialization: %w", key.SessionID, tctx.Err())
	}
}

// resolveContinueCandidate picks the most-recently-modified non-sidechain
// session for a project. Walks newest→oldest, loading each candidate and
// skipping sidechains so --continue resumes the user's conversation, not a
// subagent's.
func resolveContinueCandidate(ctx context.Context, store SessionStore, projectKey string, timeoutMs int) (string, []SessionStoreEntry, error) {
	lister, ok := store.(SessionStoreLister)
	if !ok {
		return "", nil, fmt.Errorf("session_store resume: ContinueConversation requires store to implement SessionStoreLister")
	}
	sessions, err := listSessionsWithTimeout(ctx, lister, projectKey, timeoutMs)
	if err != nil {
		return "", nil, err
	}
	if len(sessions) == 0 {
		return "", nil, nil
	}

	// Stable sort by Mtime descending.
	sorted := make([]SessionStoreListEntry, len(sessions))
	copy(sorted, sessions)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Mtime > sorted[j].Mtime })

	for _, cand := range sorted {
		if !isValidUUID(cand.SessionID) {
			continue
		}
		entries, lerr := loadWithTimeout(
			ctx, store,
			SessionKey{ProjectKey: projectKey, SessionID: cand.SessionID},
			timeoutMs,
			fmt.Sprintf("SessionStore.Load() for session %s", cand.SessionID),
		)
		if lerr != nil {
			return "", nil, lerr
		}
		if len(entries) == 0 {
			continue
		}
		if isSidechainEntry(entries[0]) {
			continue
		}
		return cand.SessionID, entries, nil
	}
	return "", nil, nil
}

// isSidechainEntry reports whether the given transcript entry is a
// sidechain entry. Sidechain transcripts are mirrored as top-level keys
// and often have the highest mtime (their append lands after the main
// session's in the same flush); --continue should resume the user's
// conversation instead.
func isSidechainEntry(entry SessionStoreEntry) bool {
	if entry == nil {
		return false
	}
	v, _ := entry["isSidechain"].(bool)
	return v
}

// writeJSONL writes one JSON line per entry to path, creating parent dirs
// as needed. File mode is 0600 to match the Python reference — the temp
// dir may contain copied credentials.
func writeJSONL(path string, entries []SessionStoreEntry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	// Default SetEscapeHTML(true) is fine — the CLI reads the JSONL with
	// a standard parser that handles both escaped and unescaped forms.
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}

// materializeSubkeys loads and writes every subagent transcript (and
// any other sub-stream) sibling to the main session. Each subkey lands
// at <projectDir>/<sessionID>/<subpath>.jsonl.
//
// Agent metadata entries (type == "agent_metadata") are split off into an
// adjacent .meta.json sidecar — last-wins semantics match the Python
// reference.
func materializeSubkeys(ctx context.Context, store SessionStore, subkeyer SessionStoreSubkeys, projectDir, projectKey, sessionID string, timeoutMs int) error {
	sessionDir := filepath.Join(projectDir, sessionID)

	subkeys, err := listSubkeysWithTimeout(ctx, subkeyer, SessionListSubkeysKey{ProjectKey: projectKey, SessionID: sessionID}, timeoutMs)
	if err != nil {
		return err
	}

	for _, subpath := range subkeys {
		if !isSafeSubpath(subpath, sessionDir) {
			// Skip with a stderr warning; matches Python behavior.
			fmt.Fprintf(os.Stderr, "[SessionStore] skipping unsafe subpath from ListSubkeys: %q\n", subpath)
			continue
		}

		subEntries, err := loadWithTimeout(
			ctx, store,
			SessionKey{ProjectKey: projectKey, SessionID: sessionID, Subpath: subpath},
			timeoutMs,
			fmt.Sprintf("SessionStore.Load() for session %s subpath %s", sessionID, subpath),
		)
		if err != nil {
			return err
		}
		if len(subEntries) == 0 {
			continue
		}

		var metadata []map[string]any
		var transcript []SessionStoreEntry
		for _, e := range subEntries {
			if e == nil {
				continue
			}
			if t, _ := e["type"].(string); t == "agent_metadata" {
				metadata = append(metadata, e)
				continue
			}
			transcript = append(transcript, e)
		}

		// Resolve the on-disk target. Store keys use "/" separators
		// regardless of host OS; filepath.FromSlash converts to the
		// local form.
		relPath := filepath.FromSlash(subpath)
		subFile := filepath.Join(sessionDir, relPath) + ".jsonl"
		if len(transcript) > 0 {
			if err := writeJSONL(subFile, transcript); err != nil {
				return fmt.Errorf("session_store resume: write subagent transcript %s: %w", subpath, err)
			}
		}

		if len(metadata) > 0 {
			// Last metadata entry wins; strip the synthetic "type" field.
			last := metadata[len(metadata)-1]
			meta := make(map[string]any, len(last))
			for k, v := range last {
				if k == "type" {
					continue
				}
				meta[k] = v
			}
			// subFile is "<subpath>.jsonl"; replace suffix with ".meta.json".
			metaFile := strings.TrimSuffix(subFile, ".jsonl") + ".meta.json"
			if err := os.MkdirAll(filepath.Dir(metaFile), 0o700); err != nil {
				return fmt.Errorf("session_store resume: create metadata dir: %w", err)
			}
			metaJSON, err := json.Marshal(meta)
			if err != nil {
				return fmt.Errorf("session_store resume: marshal metadata: %w", err)
			}
			if err := os.WriteFile(metaFile, metaJSON, 0o600); err != nil {
				return fmt.Errorf("session_store resume: write metadata %s: %w", metaFile, err)
			}
		}
	}
	return nil
}

// activeTaskEntry holds the snapshot of a task_started transcript entry for
// a task that was still in-flight when the session was interrupted.
type activeTaskEntry struct {
	taskID string
	data   map[string]any
}

// detectActiveTasks scans a slice of main-transcript entries and returns
// the task_started entries whose tasks had not yet reached a terminal state
// (completed, failed, or stopped) at the time the session was interrupted.
//
// Terminal state indicators used:
//   - type:"system", subtype:"task_notification" with status "completed",
//     "failed", or "stopped"
//   - type:"system", subtype:"task_updated" with status "completed",
//     "failed", or "killed"
//
// Entries are processed in order; the last status wins if a task transitions
// through multiple states.
func detectActiveTasks(entries []SessionStoreEntry) []activeTaskEntry {
	// started maps task_id → the task_started entry data.
	started := make(map[string]map[string]any)
	// terminated tracks task IDs that have reached a terminal state.
	terminated := make(map[string]bool)

	for _, e := range entries {
		if e == nil {
			continue
		}
		eType, _ := e["type"].(string)
		if eType != "system" {
			continue
		}
		subtype, _ := e["subtype"].(string)
		switch subtype {
		case "task_started":
			taskID, _ := e["task_id"].(string)
			if taskID == "" {
				continue
			}
			// Re-start: if a task is restarted, treat as a fresh start.
			delete(terminated, taskID)
			// Copy the entry so callers cannot mutate our map.
			snap := make(map[string]any, len(e))
			for k, v := range e {
				snap[k] = v
			}
			started[taskID] = snap

		case "task_notification":
			taskID, _ := e["task_id"].(string)
			status, _ := e["status"].(string)
			if taskID == "" {
				continue
			}
			switch status {
			case "completed", "failed", "stopped":
				terminated[taskID] = true
			}

		case "task_updated":
			taskID, _ := e["task_id"].(string)
			status, _ := e["status"].(string)
			if taskID == "" {
				continue
			}
			switch status {
			case "completed", "failed", "killed":
				terminated[taskID] = true
			}
		}
	}

	var active []activeTaskEntry
	for taskID, data := range started {
		if !terminated[taskID] {
			active = append(active, activeTaskEntry{taskID: taskID, data: data})
		}
	}
	// Sort by task_id for deterministic output in tests and logs.
	for i := 1; i < len(active); i++ {
		j := i
		for j > 0 && active[j].taskID < active[j-1].taskID {
			active[j], active[j-1] = active[j-1], active[j]
			j--
		}
	}
	return active
}

// materializeActiveTasks writes a .meta.json sidecar for each still-active
// task under sessionDir/tasks/<taskID>/.meta.json so the CLI can resume
// tracking those tasks when it --resumes the session.
//
// The file layout mirrors the agent_metadata sidecar convention already used
// by materializeSubkeys: a JSON object whose keys are the fields from the
// original task_started entry (minus the synthetic "type" field).
//
// Port of TypeScript SDK v0.3.176: "Fixed background agent, remote agent,
// and MCP task state not being restored when resuming a session via the SDK."
func materializeActiveTasks(entries []SessionStoreEntry, sessionDir string) error {
	active := detectActiveTasks(entries)
	if len(active) == 0 {
		return nil
	}

	for _, t := range active {
		taskDir := filepath.Join(sessionDir, "tasks", t.taskID)
		if err := os.MkdirAll(taskDir, 0o700); err != nil {
			return fmt.Errorf("session_store resume: create task dir %s: %w", taskDir, err)
		}
		// Build the metadata object: drop the "type" field (synthetic) to
		// match the agent_metadata sidecar convention.
		meta := make(map[string]any, len(t.data))
		for k, v := range t.data {
			if k == "type" {
				continue
			}
			meta[k] = v
		}
		metaJSON, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("session_store resume: marshal task metadata for %s: %w", t.taskID, err)
		}
		metaFile := filepath.Join(taskDir, ".meta.json")
		if err := os.WriteFile(metaFile, metaJSON, 0o600); err != nil {
			return fmt.Errorf("session_store resume: write task metadata %s: %w", metaFile, err)
		}
	}
	return nil
}

// isSafeSubpath reports whether subpath is safe to use as a path
// component under sessionDir. The check is purely lexical: rejects
// empty, NUL-containing, absolute, drive-prefixed, UNC, and
// dot/double-dot subpaths, then verifies the lexically-joined target
// stays inside sessionDir via filepath.Rel. It does NOT call
// filepath.EvalSymlinks — symlinks inside sessionDir that point
// outward are not detected. In current SDK usage sessionDir is a
// fresh tempdir with no symlinks, so a lexical check is sufficient.
func isSafeSubpath(subpath, sessionDir string) bool {
	if subpath == "" {
		return false
	}
	if strings.Contains(subpath, "\x00") {
		return false
	}
	// Reject absolute paths in either separator form.
	if strings.HasPrefix(subpath, "/") || strings.HasPrefix(subpath, `\`) {
		return false
	}
	if filepath.IsAbs(subpath) {
		return false
	}
	// Windows drive prefix (e.g. "C:foo", "C:\\foo"). Check even on
	// POSIX since subpaths are store keys that may have been written by
	// a Windows producer.
	if len(subpath) >= 2 && subpath[1] == ':' {
		return false
	}
	// UNC-style leading double-slash or double-backslash.
	if strings.HasPrefix(subpath, `\\`) || strings.HasPrefix(subpath, "//") {
		return false
	}
	// Reject "." and ".." components (in either separator).
	parts := strings.FieldsFunc(subpath, func(r rune) bool { return r == '/' || r == '\\' })
	for _, p := range parts {
		if p == "." || p == ".." {
			return false
		}
	}

	// Final escape check: the target must still live under sessionDir
	// after resolution. Use the same expression as the writer.
	target := filepath.Join(sessionDir, filepath.FromSlash(subpath)) + ".jsonl"
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absBase, err := filepath.Abs(sessionDir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// copyAuthFiles copies ~/.credentials.json (with refreshToken redacted),
// ~/.claude.json, and settings.json/cowork_settings.json (settings.json with
// resume-incompatible keys stripped) from the caller's effective config dir
// into tempDir so the spawned CLI can reuse existing credentials and
// settings.
//
// Resolution mirrors the CLI:
//   - .credentials.json, settings.json, and cowork_settings.json live under
//     the config dir (default ~/.claude/)
//   - .claude.json lives at $CLAUDE_CONFIG_DIR/.claude.json when set,
//     else ~/.claude.json (NOT ~/.claude/.claude.json)
//
// All copies are best-effort; missing files are silently skipped (API-key
// auth or a fresh install is fine).
//
// The resumed subprocess runs under a redirected CLAUDE_CONFIG_DIR, so
// without this copy, settings.json (and the apiKeyHelper/env/hooks/
// permissions it carries) would silently stop applying for a
// sessionStore-backed resume even though it applies for a normal resume.
// Port of TypeScript SDK v0.3.222.
func copyAuthFiles(tempDir string, optEnv map[string]string) {
	callerConfigDir := optEnv["CLAUDE_CONFIG_DIR"]
	if callerConfigDir == "" {
		callerConfigDir = os.Getenv("CLAUDE_CONFIG_DIR")
	}

	var sourceConfigDir string
	if callerConfigDir != "" {
		sourceConfigDir = callerConfigDir
	} else {
		home, _ := os.UserHomeDir()
		sourceConfigDir = filepath.Join(home, ".claude")
	}

	credsSrc := filepath.Join(sourceConfigDir, ".credentials.json")
	credsJSON, _ := os.ReadFile(credsSrc)
	writeRedactedCredentials(credsJSON, filepath.Join(tempDir, ".credentials.json"))

	settingsSrc := filepath.Join(sourceConfigDir, "settings.json")
	copyIfPresentTransform(settingsSrc, filepath.Join(tempDir, "settings.json"), stripSettingsForResume)

	coworkSettingsSrc := filepath.Join(sourceConfigDir, "cowork_settings.json")
	copyIfPresent(coworkSettingsSrc, filepath.Join(tempDir, "cowork_settings.json"))

	var claudeJSONSrc string
	if callerConfigDir != "" {
		claudeJSONSrc = filepath.Join(callerConfigDir, ".claude.json")
	} else {
		home, _ := os.UserHomeDir()
		claudeJSONSrc = filepath.Join(home, ".claude.json")
	}
	copyIfPresent(claudeJSONSrc, filepath.Join(tempDir, ".claude.json"))
}

// writeRedactedCredentials writes credsJSON to dst with
// claudeAiOauth.refreshToken stripped. If credsJSON is nil/empty, the
// function is a no-op.
//
// The resumed subprocess runs under a redirected CLAUDE_CONFIG_DIR; if it
// refreshed, the single-use refresh token would be consumed server-side
// and new tokens written where the parent never reads them back — leaving
// the parent's stored credentials revoked. With no refreshToken, the
// subprocess's refresh check short-circuits.
func writeRedactedCredentials(credsJSON []byte, dst string) {
	if len(credsJSON) == 0 {
		return
	}
	out := credsJSON
	var data map[string]any
	if err := json.Unmarshal(credsJSON, &data); err == nil {
		if oauth, ok := data["claudeAiOauth"].(map[string]any); ok {
			if _, present := oauth["refreshToken"]; present {
				delete(oauth, "refreshToken")
				if redacted, err := json.Marshal(data); err == nil {
					out = redacted
				}
			}
		}
	}
	_ = os.WriteFile(dst, out, 0o600)
}

// copyIfPresent copies src to dst when src exists. Missing src is not an
// error; all other errors are swallowed (best-effort).
func copyIfPresent(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = out.Close() }()
	_, _ = io.Copy(out, in)
}

// copyIfPresentTransform copies src to dst when src exists, running its
// content through transform before writing. Missing src is not an error;
// all other errors are swallowed (best-effort), matching copyIfPresent.
func copyIfPresentTransform(src, dst string, transform func([]byte) []byte) {
	content, err := os.ReadFile(src)
	if err != nil {
		return
	}
	_ = os.WriteFile(dst, transform(content), 0o600)
}

// utf8BOM is the UTF-8 byte-order mark some tools (notably PowerShell)
// prepend to written JSON files.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// resumeSettingsStrippedKeys are removed outright from a copied
// settings.json before the resumed subprocess sees it.
var resumeSettingsStrippedKeys = []string{"enabledPlugins", "extraKnownMarketplaces"}

// stripSettingsForResume removes settings.json keys that misbehave under the
// resumed subprocess's redirected CLAUDE_CONFIG_DIR:
//   - enabledPlugins / extraKnownMarketplaces would reconcile against the
//     empty temp plugin cache.
//   - env.CLAUDE_CONFIG_DIR would redirect the subprocess's own config reads
//     away from the temp dir being built for it, defeating the point of this
//     copy.
//
// Malformed JSON or a non-object top level is returned unmodified so the
// subprocess still sees the original content rather than aborting the
// resume. A leading UTF-8 BOM (as PowerShell writes) is tolerated on
// decode and dropped from the output. Port of Python SDK commit b4d65f5
// (anthropics/claude-agent-sdk-python#1197).
func stripSettingsForResume(content []byte) []byte {
	trimmed := bytes.TrimPrefix(content, utf8BOM)

	var parsed map[string]any
	if err := json.Unmarshal(trimmed, &parsed); err != nil {
		return content
	}

	for _, key := range resumeSettingsStrippedKeys {
		delete(parsed, key)
	}
	if env, ok := parsed["env"].(map[string]any); ok {
		delete(env, "CLAUDE_CONFIG_DIR")
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		return content
	}
	return out
}

// sanitizeToolUseID ensures a tool use ID conforms to toolu_[a-zA-Z0-9_-]+.
// If the ID doesn't start with "toolu_", the prefix is prepended.
// Any characters outside [a-zA-Z0-9_-] are replaced with "_".
func sanitizeToolUseID(id string) string {
	if id == "" {
		return id
	}
	// Replace invalid characters
	var buf strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			buf.WriteRune(r)
		} else {
			buf.WriteRune('_')
		}
	}
	sanitized := buf.String()
	if !strings.HasPrefix(sanitized, "toolu_") {
		sanitized = "toolu_" + sanitized
	}
	return sanitized
}

// sanitizeSessionEntries walks a slice of SessionStoreEntry values and
// sanitizes all tool_use.id and tool_use_id fields so they conform to the
// toolu_[a-zA-Z0-9_-]+ format required by the Claude API. Entries whose
// content does not contain tool_use blocks are returned unchanged. The
// returned slice may share entries with the input when no modification was
// needed for a given entry.
func sanitizeSessionEntries(entries []SessionStoreEntry) []SessionStoreEntry {
	out := make([]SessionStoreEntry, len(entries))
	for i, e := range entries {
		out[i] = sanitizeEntry(e)
	}
	return out
}

// sanitizeEntry sanitizes all tool_use.id and tool_use_id values inside a
// single SessionStoreEntry. Returns the original entry unchanged when there
// is nothing to sanitize.
func sanitizeEntry(e SessionStoreEntry) SessionStoreEntry {
	if e == nil {
		return e
	}

	// Top-level "message" object may contain a "content" array.
	msg, _ := e["message"].(map[string]any)
	if msg == nil {
		// Also handle entries where "content" is directly on the entry itself
		// (e.g. tool_result blocks at the top level in some CLI formats).
		return sanitizeEntryContent(e)
	}

	newContent, changed := sanitizeContentArray(msg["content"])
	if !changed {
		// Also check for tool_use_id at message level.
		if id, ok := msg["tool_use_id"].(string); ok {
			sanitized := sanitizeToolUseID(id)
			if sanitized != id {
				newEntry := copyEntry(e)
				newMsg := copyMap(msg)
				newMsg["tool_use_id"] = sanitized
				newEntry["message"] = newMsg
				return newEntry
			}
		}
		return e
	}

	newEntry := copyEntry(e)
	newMsg := copyMap(msg)
	newMsg["content"] = newContent
	newEntry["message"] = newMsg
	return newEntry
}

// sanitizeEntryContent handles entries where "content" is directly on the
// entry (not under a "message" key).
func sanitizeEntryContent(e SessionStoreEntry) SessionStoreEntry {
	newContent, changed := sanitizeContentArray(e["content"])
	if !changed {
		if id, ok := e["tool_use_id"].(string); ok {
			sanitized := sanitizeToolUseID(id)
			if sanitized != id {
				newEntry := copyEntry(e)
				newEntry["tool_use_id"] = sanitized
				return newEntry
			}
		}
		return e
	}
	newEntry := copyEntry(e)
	newEntry["content"] = newContent
	return newEntry
}

// sanitizeContentArray sanitizes tool_use blocks within a content array.
// Returns (newContent, true) when at least one ID was changed, or
// (nil, false) when nothing needed changing.
func sanitizeContentArray(raw any) (any, bool) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, false
	}
	changed := false
	newArr := make([]any, len(arr))
	for i, item := range arr {
		block, ok := item.(map[string]any)
		if !ok {
			newArr[i] = item
			continue
		}
		blockType, _ := block["type"].(string)
		switch blockType {
		case "tool_use":
			id, _ := block["id"].(string)
			sanitized := sanitizeToolUseID(id)
			if sanitized != id {
				newBlock := copyMap(block)
				newBlock["id"] = sanitized
				newArr[i] = newBlock
				changed = true
			} else {
				newArr[i] = item
			}
		case "tool_result":
			id, _ := block["tool_use_id"].(string)
			sanitized := sanitizeToolUseID(id)
			if sanitized != id {
				newBlock := copyMap(block)
				newBlock["tool_use_id"] = sanitized
				newArr[i] = newBlock
				changed = true
			} else {
				newArr[i] = item
			}
		default:
			newArr[i] = item
		}
	}
	if !changed {
		return nil, false
	}
	return newArr, true
}

// copyEntry returns a shallow copy of a SessionStoreEntry map.
func copyEntry(e SessionStoreEntry) SessionStoreEntry {
	out := make(SessionStoreEntry, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// copyMap returns a shallow copy of a map[string]any.
func copyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// safeRemoveAll removes path with Windows-safe retries. On Windows,
// AV/indexer can briefly hold a handle on freshly-written files (notably
// .credentials.json), causing the initial rmtree to fail. Retry with a
// short backoff; never panics.
//
// Matches the Python reference's _rmtree_with_retry (3 retries × 100ms)
// plus a final best-effort sweep.
func safeRemoveAll(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}
	const retries = 3
	const delay = 100 * time.Millisecond
	for i := 0; i < retries; i++ {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		time.Sleep(delay)
	}
	// Final best-effort sweep. Report the error from the final attempt
	// (the most recent + actionable one), not a stale prior-retry error.
	if err := os.RemoveAll(path); err != nil {
		fmt.Fprintf(os.Stderr, "[SessionStore] warning: failed to remove temp dir %s: %v\n", path, err)
	}
}

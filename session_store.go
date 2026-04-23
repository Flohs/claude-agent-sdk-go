package claude

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SessionStore is the required interface for every session-store adapter.
//
// Adapters persist one [SessionStoreEntry] per JSONL line of the transcript
// identified by a [SessionKey]. Append must be additive (never overwrite
// prior entries for the same key) and Load must return the exact sequence
// that Append produced, in insertion order.
//
// Load returns (nil, nil) when the key has no entries — that is not an
// error. Errors are reserved for genuine I/O or corruption failures.
//
// Adapters may additionally implement one or more of the optional extension
// interfaces ([SessionStoreLister], [SessionStoreSummarizer],
// [SessionStoreDeleter], [SessionStoreSubkeys]); the SDK probes them via
// type assertion.
//
// Implementations must be safe for concurrent use.
type SessionStore interface {
	// Append adds entries to the transcript for key in insertion order.
	Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error
	// Load returns every entry previously appended for key, in order.
	// Returns (nil, nil) when the key is absent.
	Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error)
}

// SessionStoreLister is an optional [SessionStore] extension that lists
// the main-transcript sessions within a project. Subagent and other
// sub-streams (keys with a non-empty Subpath) must be excluded from the
// result.
type SessionStoreLister interface {
	ListSessions(ctx context.Context, projectKey string) ([]SessionStoreListEntry, error)
}

// SessionStoreSummarizer is an optional [SessionStore] extension that
// returns per-session summary sidecars maintained incrementally via
// [FoldSessionSummary]. Consumers can hydrate a full session listing
// without issuing per-session Load calls.
type SessionStoreSummarizer interface {
	ListSessionSummaries(ctx context.Context, projectKey string) ([]SessionSummaryEntry, error)
}

// SessionStoreDeleter is an optional [SessionStore] extension that removes
// a stored key. When the deleted key has no Subpath (i.e. is a main
// transcript), the adapter must also cascade-delete any sibling subkeys
// (subagent transcripts, etc.) so they are not orphaned. A targeted delete
// with an explicit Subpath removes only that one entry.
type SessionStoreDeleter interface {
	Delete(ctx context.Context, key SessionKey) error
}

// SessionStoreSubkeys is an optional [SessionStore] extension that lists
// the subkeys (Subpath values) associated with a given main session. The
// return slice contains the Subpath portion only, stripped of the leading
// "<project_key>/<session_id>/" prefix.
type SessionStoreSubkeys interface {
	ListSubkeys(ctx context.Context, key SessionListSubkeysKey) ([]string, error)
}

// ProjectKeyForDirectory derives the [SessionStore] project_key for a
// directory using the same realpath + NFC normalization + hashed
// sanitization the CLI uses for project directory names, so keys match
// between local-disk transcripts and store-mirrored transcripts even on
// filesystems that decompose Unicode (e.g. macOS HFS+).
//
// An empty dir is treated as the current working directory.
func ProjectKeyForDirectory(dir string) string {
	if dir == "" {
		dir = "."
	}
	return sanitizePath(canonicalizePath(dir))
}

// FilePathToSessionKey parses an absolute transcript file path rooted under
// projectsDir into its [SessionKey].
//
// Two layouts are recognized:
//
//   - Main transcript: "<projectsDir>/<project_key>/<session_id>.jsonl"
//     → {ProjectKey, SessionID, Subpath: ""}
//   - Subagent transcript: "<projectsDir>/<project_key>/<session_id>/subagents/agent-<id>.jsonl"
//     (with any depth under subagents/, e.g. "workflows/<runId>/agent-<id>.jsonl")
//     → {ProjectKey, SessionID, Subpath: "subagents/.../agent-<id>"}
//
// Subpath is always "/"-joined regardless of the host OS separator so keys
// are portable across platforms. The ".jsonl" suffix is stripped from the
// final Subpath component.
//
// Returns (SessionKey{}, false) when absPath is not under projectsDir or
// the path does not match either recognized shape.
func FilePathToSessionKey(absPath, projectsDir string) (SessionKey, bool) {
	rel, err := filepath.Rel(projectsDir, absPath)
	if err != nil {
		return SessionKey{}, false
	}
	if rel == "" || rel == "." {
		return SessionKey{}, false
	}
	// filepath.Rel happily returns ".." for paths outside the base.
	if strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return SessionKey{}, false
	}

	parts := splitPathAll(rel)
	if len(parts) < 2 {
		return SessionKey{}, false
	}

	projectKey := parts[0]
	second := parts[1]

	// Main transcript: <project_key>/<session_id>.jsonl
	if len(parts) == 2 && strings.HasSuffix(second, ".jsonl") {
		return SessionKey{
			ProjectKey: projectKey,
			SessionID:  strings.TrimSuffix(second, ".jsonl"),
		}, true
	}

	// Subagent (or other) sub-stream: <project_key>/<session_id>/<...>/<name>.jsonl
	if len(parts) >= 4 {
		subParts := make([]string, len(parts)-2)
		copy(subParts, parts[2:])
		last := subParts[len(subParts)-1]
		if strings.HasSuffix(last, ".jsonl") {
			subParts[len(subParts)-1] = strings.TrimSuffix(last, ".jsonl")
		}
		return SessionKey{
			ProjectKey: projectKey,
			SessionID:  second,
			Subpath:    strings.Join(subParts, "/"),
		}, true
	}

	return SessionKey{}, false
}

// splitPathAll splits rel into its path components using the host OS
// separator. It is a small wrapper around filepath.SplitList semantics that
// handles both "/" and "\\" separators uniformly.
func splitPathAll(rel string) []string {
	// filepath.ToSlash collapses the OS separator to "/". That lets us use
	// strings.Split without caring whether we're on Windows or POSIX.
	slashed := filepath.ToSlash(rel)
	if slashed == "" {
		return nil
	}
	return strings.Split(slashed, "/")
}

// FoldSessionSummary folds a batch of newly-appended entries into a
// running [SessionSummaryEntry] for key.
//
// Adapters should call this from inside Append to maintain a per-session
// summary sidecar so [SessionStoreSummarizer.ListSessionSummaries] can
// return results without re-reading the full transcript. prev is the
// previous summary for the same session (or nil for the first append for a
// new session).
//
// IMPORTANT: only call this for keys where Subpath == "". Subagent
// transcripts must not contribute to the main session's summary — the
// adapter is responsible for skipping those keys.
//
// Each call processes exactly the entries passed in: adapters should pass
// only the newly-appended slice so the fold stays append-incremental (no
// re-reads). All derived state lives in the opaque Data map; adapters
// persist the returned summary verbatim.
//
// Mtime is NOT set by the fold — it is the sidecar's storage write time
// and must be stamped by the adapter after persisting so it shares a clock
// with the mtime returned by [SessionStoreLister.ListSessions]. For a new
// session (prev == nil) the returned summary has Mtime == 0 as a
// placeholder.
func FoldSessionSummary(prev *SessionSummaryEntry, key SessionKey, entries []SessionStoreEntry) *SessionSummaryEntry {
	var summary SessionSummaryEntry
	if prev != nil {
		summary = SessionSummaryEntry{
			SessionID: prev.SessionID,
			Mtime:     prev.Mtime,
			Data:      cloneStringAnyMap(prev.Data),
		}
	} else {
		summary = SessionSummaryEntry{
			SessionID: key.SessionID,
			Mtime:     0,
			Data:      map[string]any{},
		}
	}
	data := summary.Data

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		ms := isoToEpochMs(entry["timestamp"])

		if _, ok := data["is_sidechain"]; !ok {
			isSC, _ := entry["isSidechain"].(bool)
			data["is_sidechain"] = isSC
		}
		if _, ok := data["created_at"]; !ok && ms != nil {
			data["created_at"] = *ms
		}
		if _, ok := data["cwd"]; !ok {
			if cwd, ok := entry["cwd"].(string); ok && cwd != "" {
				data["cwd"] = cwd
			}
		}

		foldFirstPrompt(data, entry)

		for src, dst := range lastWinsFields {
			if val, ok := entry[src].(string); ok {
				data[dst] = val
			}
		}

		if t, _ := entry["type"].(string); t == "tag" {
			if tagVal, ok := entry["tag"].(string); ok && tagVal != "" {
				data["tag"] = tagVal
			} else {
				delete(data, "tag")
			}
		}
	}

	return &summary
}

// lastWinsFields maps JSONL entry keys to [SessionSummaryEntry.Data] keys
// for string fields where each appended entry overwrites the previous value
// (last-wins semantics). Matches the Python fold.
var lastWinsFields = map[string]string{
	"customTitle": "custom_title",
	"aiTitle":     "ai_title",
	"lastPrompt":  "last_prompt",
	"summary":     "summary_hint",
	"gitBranch":   "git_branch",
}

// isoToEpochMs parses an ISO-8601 timestamp string into Unix epoch
// milliseconds, or returns nil if the value is not a parseable string.
func isoToEpochMs(v any) *int64 {
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.000Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05.000-07:00",
		"2006-01-02T15:04:05-07:00",
		time.RFC3339Nano,
		time.RFC3339,
	} {
		if t, err := time.Parse(layout, s); err == nil {
			ms := t.UnixMilli()
			return &ms
		}
	}
	return nil
}

// foldFirstPrompt mirrors Python's _fold_first_prompt: sets
// data["first_prompt"] + data["first_prompt_locked"] on a real user
// message, or stashes data["command_fallback"] for slash-command entries.
// It skips tool_result, isMeta, isCompactSummary, and auto-generated
// patterns.
func foldFirstPrompt(data map[string]any, entry map[string]any) {
	if locked, _ := data["first_prompt_locked"].(bool); locked {
		return
	}
	if t, _ := entry["type"].(string); t != "user" {
		return
	}
	if m, _ := entry["isMeta"].(bool); m {
		return
	}
	if c, _ := entry["isCompactSummary"].(bool); c {
		return
	}
	message, ok := entry["message"].(map[string]any)
	if !ok {
		return
	}
	// Skip tool_result-carrying user messages.
	if content, ok := message["content"].([]any); ok {
		for _, block := range content {
			if bm, ok := block.(map[string]any); ok {
				if bm["type"] == "tool_result" {
					return
				}
			}
		}
	}

	for _, raw := range entryTextBlocks(message) {
		result := strings.ReplaceAll(raw, "\n", " ")
		result = strings.TrimSpace(result)
		if result == "" {
			continue
		}
		if m := commandNameRE.FindStringSubmatch(result); m != nil {
			if _, already := data["command_fallback"]; !already {
				data["command_fallback"] = m[1]
			}
			continue
		}
		if skipFirstPromptPattern.MatchString(result) {
			continue
		}
		runes := []rune(result)
		if len(runes) > 200 {
			result = strings.TrimRightFunc(string(runes[:200]), func(r rune) bool {
				return r == ' ' || r == '\t' || r == '\n' || r == '\r'
			}) + "…"
		}
		data["first_prompt"] = result
		data["first_prompt_locked"] = true
		return
	}
}

// entryTextBlocks returns the text strings inside a user entry's
// message.content — either the raw string content or the text blocks from
// a content array.
func entryTextBlocks(message map[string]any) []string {
	content := message["content"]
	var texts []string
	switch v := content.(type) {
	case string:
		texts = append(texts, v)
	case []any:
		for _, block := range v {
			if bm, ok := block.(map[string]any); ok {
				if bm["type"] == "text" {
					if t, ok := bm["text"].(string); ok {
						texts = append(texts, t)
					}
				}
			}
		}
	}
	return texts
}

func cloneStringAnyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// InMemorySessionStore is the reference [SessionStore] implementation.
//
// It stores entries in memory keyed by a composite
// "<project_key>/<session_id>[/<subpath>]" string and implements all four
// optional extension interfaces ([SessionStoreLister],
// [SessionStoreSummarizer], [SessionStoreDeleter], [SessionStoreSubkeys]).
//
// Data is lost when the process exits — this is intended for tests and
// development, not production workloads. Safe for concurrent use.
type InMemorySessionStore struct {
	mu        sync.RWMutex
	store     map[string][]SessionStoreEntry
	mtimes    map[string]int64
	summaries map[summaryKey]SessionSummaryEntry
	lastMtime int64
	nowMilli  func() int64 // override-able for tests; nil → time.Now
}

type summaryKey struct {
	projectKey string
	sessionID  string
}

// NewInMemorySessionStore returns a fresh [InMemorySessionStore].
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		store:     map[string][]SessionStoreEntry{},
		mtimes:    map[string]int64{},
		summaries: map[summaryKey]SessionSummaryEntry{},
	}
}

// keyToString produces the composite map key used internally. Matches the
// Python reference.
func keyToString(key SessionKey) string {
	if key.Subpath == "" {
		return key.ProjectKey + "/" + key.SessionID
	}
	return key.ProjectKey + "/" + key.SessionID + "/" + key.Subpath
}

// nextMtime returns a strictly monotonically increasing Unix-epoch-ms
// timestamp. Callers must hold s.mu for writing.
func (s *InMemorySessionStore) nextMtime() int64 {
	var now int64
	if s.nowMilli != nil {
		now = s.nowMilli()
	} else {
		now = time.Now().UnixMilli()
	}
	if now <= s.lastMtime {
		now = s.lastMtime + 1
	}
	s.lastMtime = now
	return now
}

// Append satisfies [SessionStore].
func (s *InMemorySessionStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	k := keyToString(key)
	// Copy entries defensively so caller mutations after Append don't
	// corrupt the store.
	cp := make([]SessionStoreEntry, len(entries))
	copy(cp, entries)
	s.store[k] = append(s.store[k], cp...)

	now := s.nextMtime()
	s.mtimes[k] = now

	// Only main transcripts contribute to the per-session summary sidecar.
	if key.Subpath == "" {
		sk := summaryKey{projectKey: key.ProjectKey, sessionID: key.SessionID}
		var prev *SessionSummaryEntry
		if existing, ok := s.summaries[sk]; ok {
			prevCopy := existing
			prev = &prevCopy
		}
		folded := FoldSessionSummary(prev, key, cp)
		folded.Mtime = now
		s.summaries[sk] = *folded
	}
	return nil
}

// Load satisfies [SessionStore].
func (s *InMemorySessionStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, ok := s.store[keyToString(key)]
	if !ok {
		return nil, nil
	}
	out := make([]SessionStoreEntry, len(entries))
	copy(out, entries)
	return out, nil
}

// ListSessions satisfies [SessionStoreLister]. Main transcripts only —
// subagent and other sub-streams (keys with a non-empty Subpath) are
// excluded. Results are ordered by Mtime descending.
func (s *InMemorySessionStore) ListSessions(ctx context.Context, projectKey string) ([]SessionStoreListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := projectKey + "/"
	var results []SessionStoreListEntry
	for k := range s.store {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := k[len(prefix):]
		// Main transcripts have exactly one component after the project
		// prefix — no further "/".
		if strings.Contains(rest, "/") {
			continue
		}
		results = append(results, SessionStoreListEntry{
			SessionID: rest,
			Mtime:     s.mtimes[k],
		})
	}

	// Order by mtime descending to match the convention established by
	// the on-disk ListSessions and the fast-path staleness check.
	sortByMtimeDesc(results)
	return results, nil
}

// sortByMtimeDesc does a small in-place insertion sort on results. The
// result set is tiny in practice (bounded by number of sessions in a
// single project), so this keeps the dependency surface minimal.
func sortByMtimeDesc(results []SessionStoreListEntry) {
	for i := 1; i < len(results); i++ {
		j := i
		for j > 0 && results[j].Mtime > results[j-1].Mtime {
			results[j], results[j-1] = results[j-1], results[j]
			j--
		}
	}
}

// ListSessionSummaries satisfies [SessionStoreSummarizer].
func (s *InMemorySessionStore) ListSessionSummaries(ctx context.Context, projectKey string) ([]SessionSummaryEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []SessionSummaryEntry
	for sk, summary := range s.summaries {
		if sk.projectKey != projectKey {
			continue
		}
		// Defensive copy: hand out a fresh Data map so callers can't
		// mutate shared state.
		cp := summary
		cp.Data = cloneStringAnyMap(summary.Data)
		out = append(out, cp)
	}
	return out, nil
}

// Delete satisfies [SessionStoreDeleter]. Deleting a main-transcript key
// (Subpath == "") cascades to the session's sibling subkeys (subagent
// transcripts and other sub-streams) and the per-session summary sidecar.
// A targeted delete with an explicit Subpath removes only that one entry.
func (s *InMemorySessionStore) Delete(ctx context.Context, key SessionKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	k := keyToString(key)
	delete(s.store, k)
	delete(s.mtimes, k)

	if key.Subpath == "" {
		delete(s.summaries, summaryKey{projectKey: key.ProjectKey, sessionID: key.SessionID})
		prefix := key.ProjectKey + "/" + key.SessionID + "/"
		for storeKey := range s.store {
			if strings.HasPrefix(storeKey, prefix) {
				delete(s.store, storeKey)
				delete(s.mtimes, storeKey)
			}
		}
	}
	return nil
}

// ListSubkeys satisfies [SessionStoreSubkeys]. Returns only the Subpath
// portion of each matching stored key (i.e. the part after
// "<project_key>/<session_id>/").
func (s *InMemorySessionStore) ListSubkeys(ctx context.Context, key SessionListSubkeysKey) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	prefix := key.ProjectKey + "/" + key.SessionID + "/"
	var out []string
	for k := range s.store {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k[len(prefix):])
		}
	}
	return out, nil
}

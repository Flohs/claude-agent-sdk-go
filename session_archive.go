package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// sessionArchiveFileName is the archive mark the CLI and the TypeScript SDK
// keep in the {session_id}/ directory beside a session's transcript.
const sessionArchiveFileName = "archived.json"

// sessionArchiveMaxBytes caps how much of an archive mark is read; a larger
// file is ignored, as upstream does.
const sessionArchiveMaxBytes = 4096

// ArchiveSession archives a session. It stays on disk, and [ListSessions]
// and [GetSessionInfo] still return it, with [SDKSessionInfo.Archived] true.
// A prompt sent in the session later unarchives it. ArchiveSession does not
// change the transcript or stop a process that runs the session.
//
// The mark is written as {session_id}/archived.json beside every copy of
// the session's transcript (the project directory and its git worktrees, or
// every project directory when directory is omitted). An error is returned
// if the session is not found. A [SessionStore] does not hold the mark, so
// there is no store-backed variant. Port of TypeScript SDK v0.3.324.
func ArchiveSession(sessionID string, directory ...string) error {
	return setSessionArchived(sessionID, true, directory)
}

// UnarchiveSession unarchives a session, so that [ListSessions] and
// [GetSessionInfo] return it with [SDKSessionInfo.Archived] false. An error
// is returned if the session is not found (see [ArchiveSession]). Port of
// TypeScript SDK v0.3.324.
func UnarchiveSession(sessionID string, directory ...string) error {
	return setSessionArchived(sessionID, false, directory)
}

func setSessionArchived(sessionID string, archived bool, directory []string) error {
	if !isValidUUID(sessionID) {
		return fmt.Errorf("invalid session ID: %s", sessionID)
	}
	dir := ""
	if len(directory) > 0 {
		dir = directory[0]
	}
	paths := findAllSessionFilePaths(sessionID, dir)
	if len(paths) == 0 {
		if dir != "" {
			return fmt.Errorf("session %s not found in project directory for %s", sessionID, dir)
		}
		return fmt.Errorf("session %s not found in any project directory", sessionID)
	}

	data, err := json.Marshal(map[string]any{
		"archived": archived,
		"at":       time.Now().UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		"by":       "sdk",
	})
	if err != nil {
		return err
	}
	var errs []error
	for _, p := range paths {
		if err := writeSessionArchiveMark(sessionArchivePath(p, sessionID), data); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// sessionArchivePath returns the archive mark path for the transcript at
// transcriptPath.
func sessionArchivePath(transcriptPath, sessionID string) string {
	return filepath.Join(filepath.Dir(transcriptPath), sessionID, sessionArchiveFileName)
}

// writeSessionArchiveMark writes data to path atomically (temp file plus
// rename), creating the directory with mode 0700 and the file with 0600.
func writeSessionArchiveMark(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("session archive: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".archived-*.json.tmp")
	if err != nil {
		return fmt.Errorf("session archive: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("session archive: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("session archive: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("session archive: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("session archive: %w", err)
	}
	return nil
}

// findAllSessionFilePaths returns every transcript copy of sessionID: in
// directory's project dir and those of its git worktrees, or in every
// project directory when directory is empty.
func findAllSessionFilePaths(sessionID, directory string) []string {
	fileName := sessionID + ".jsonl"
	var paths []string
	seen := map[string]bool{}
	add := func(projectDir string) {
		if projectDir == "" {
			return
		}
		p := filepath.Join(projectDir, fileName)
		if seen[p] {
			return
		}
		if info, err := os.Stat(p); err == nil && info.Mode().IsRegular() {
			seen[p] = true
			paths = append(paths, p)
		}
	}

	if directory != "" {
		canonicalDir := canonicalizePath(directory)
		add(findProjectDir(canonicalDir))
		for _, wt := range getWorktreePaths(canonicalDir) {
			if wt == canonicalDir {
				continue
			}
			add(findProjectDir(wt))
		}
		return paths
	}

	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil
	}
	for _, entry := range entries {
		if entry.IsDir() {
			add(filepath.Join(projectsDir, entry.Name()))
		}
	}
	return paths
}

// sessionArchiveMark is a parsed archive mark.
type sessionArchiveMark struct {
	archived bool
	at       time.Time
}

// readSessionArchiveMark reads the archive mark beside the transcript at
// transcriptPath. It returns nil when there is none, it is not a regular
// file, it is larger than sessionArchiveMaxBytes, or it does not hold a
// boolean "archived" and an "at" timestamp with a zone offset.
func readSessionArchiveMark(transcriptPath, sessionID string) *sessionArchiveMark {
	path := sessionArchivePath(transcriptPath, sessionID)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, sessionArchiveMaxBytes+1))
	if err != nil || len(data) > sessionArchiveMaxBytes {
		return nil
	}
	return parseSessionArchiveMark(data)
}

func parseSessionArchiveMark(data []byte) *sessionArchiveMark {
	var raw map[string]any
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	archived, ok := raw["archived"].(bool)
	if !ok {
		return nil
	}
	atStr, ok := raw["at"].(string)
	if !ok {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, atStr)
	if err != nil {
		return nil
	}
	return &sessionArchiveMark{archived: archived, at: at}
}

// sessionArchivedState returns the value of [SDKSessionInfo.Archived] for
// the transcript at transcriptPath, whose head and tail are in lite: nil
// when the session was never archived; false when it was unarchived, or
// when the head or tail holds a prompt sent after it was archived; true
// otherwise.
func sessionArchivedState(transcriptPath, sessionID string, lite *liteSessionFile) *bool {
	mark := readSessionArchiveMark(transcriptPath, sessionID)
	if mark == nil {
		return nil
	}
	archived := mark.archived
	if archived && lite != nil {
		atMs := mark.at.UnixMilli()
		if hasPromptAfter(lite.tail, atMs) || (lite.head != lite.tail && hasPromptAfter(lite.head, atMs)) {
			archived = false
		}
	}
	return &archived
}

// hasPromptAfter reports whether text holds a prompt line timestamped after
// atMs.
func hasPromptAfter(text string, atMs int64) bool {
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, `"user"`) && !strings.Contains(line, `"queued_command"`) {
			continue
		}
		if strings.Contains(line, `"tool_result"`) {
			continue
		}
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil {
			continue
		}
		if ts, ok := promptTimestampMs(entry); ok && ts > atMs {
			return true
		}
	}
	return false
}

// promptLeadingTagRE matches text the TypeScript SDK does not count as a
// typed prompt: text that opens with a markup tag, or an interruption
// notice.
var promptLeadingTagRE = regexp.MustCompile(`^(?:\s*<[a-z][\w-]*[\s>]|\[Request interrupted by user[^\]]*\])`)

var bashInputRE = regexp.MustCompile(`<bash-input>([\s\S]*?)</bash-input>`)

// promptFlagKeys must be absent or false on a transcript user entry for it
// to count as a prompt.
var promptFlagKeys = []string{
	"isCompactSummary", "isSidechain", "isVirtual", "isVisibleInTranscriptOnly",
	"isMeta", "textNotTyped", "handedOff", "queueSkipAttachments",
}

// promptTimestampMs returns the timestamp of a transcript entry that is a
// prompt a person sent: a user entry, or a queued_command attachment, from a
// human (or with no origin), that is not a tool result, synthetic or meta
// entry and holds typed text, a slash command, an image or a document. It
// follows the TypeScript SDK v0.3.324 check that decides whether a prompt
// unarchived a session.
func promptTimestampMs(entry map[string]any) (int64, bool) {
	tsStr, ok := entry["timestamp"].(string)
	if !ok {
		return 0, false
	}
	var content any
	var origin map[string]any
	hasOrigin := false

	switch entry["type"] {
	case "user":
		for _, k := range promptFlagKeys {
			if v, present := entry[k]; present && v != false {
				return 0, false
			}
		}
		for _, k := range []string{"persist", "wakeupSource", "scheduledTaskId"} {
			if _, present := entry[k]; present {
				return 0, false
			}
		}
		if v, present := entry["toolUseResult"]; present && v != nil {
			return 0, false
		}
		if v, present := entry["promptSource"]; present {
			if _, ok := v.(string); !ok {
				return 0, false
			}
		}
		if v, present := entry["turnOrigin"]; present {
			if s, _ := v.(string); s != "human" && s != "sdk" && s != "unknown" {
				return 0, false
			}
		}
		msg, ok := entry["message"].(map[string]any)
		if !ok {
			return 0, false
		}
		content = msg["content"]
		if v, present := entry["origin"]; present {
			if origin, ok = v.(map[string]any); !ok {
				return 0, false
			}
			hasOrigin = true
		}
	case "attachment":
		if v, present := entry["isSidechain"]; present && v != false {
			return 0, false
		}
		att, ok := entry["attachment"].(map[string]any)
		if !ok || att["type"] != "queued_command" {
			return 0, false
		}
		if v, present := att["isMeta"]; present && v != false {
			return 0, false
		}
		if v, present := att["commandMode"]; present && v != "prompt" && v != "bash" {
			return 0, false
		}
		content = att["prompt"]
		if v, present := att["origin"]; present {
			if origin, ok = v.(map[string]any); !ok {
				return 0, false
			}
			hasOrigin = true
		}
	default:
		return 0, false
	}

	if hasOrigin {
		kind, ok := origin["kind"].(string)
		if !ok || kind != "human" {
			return 0, false
		}
	}

	// Content blocks: a string is one text block; an array counts only when
	// every element is an object with a string type.
	var blocks []map[string]any
	switch c := content.(type) {
	case string:
		blocks = []map[string]any{{"type": "text", "text": c}}
	case []any:
		for _, b := range c {
			bm, ok := b.(map[string]any)
			if !ok {
				blocks = nil
				break
			}
			if _, ok := bm["type"].(string); !ok {
				blocks = nil
				break
			}
			blocks = append(blocks, bm)
		}
	}
	var texts []string
	hasMedia := false
	for _, b := range blocks {
		switch b["type"] {
		case "tool_result":
			return 0, false
		case "image", "document":
			hasMedia = true
		}
		t, _ := b["text"].(string)
		texts = append(texts, strings.TrimLeftFunc(t, unicode.IsSpace))
	}

	isPrompt := hasMedia
	if !isPrompt {
		if hasOrigin {
			nonEmpty, command := false, false
			for _, t := range texts {
				if t != "" {
					nonEmpty = true
				}
				if strings.HasPrefix(t, "<command-name>") {
					command = true
				}
			}
			isPrompt = nonEmpty && !command
		} else {
			isPrompt = contentHasTypedPrompt(content)
			for _, t := range texts {
				if strings.HasPrefix(t, "<command-message>") {
					isPrompt = true
				}
			}
		}
	}
	if !isPrompt {
		return 0, false
	}
	ts, err := time.Parse(time.RFC3339Nano, tsStr)
	if err != nil {
		return 0, false
	}
	return ts.UnixMilli(), true
}

// contentHasTypedPrompt reports whether user message content holds text a
// session listing would show as a prompt: not empty, not only a slash
// command name, and not opening with markup unless it is a bash input.
func contentHasTypedPrompt(content any) bool {
	var texts []string
	switch c := content.(type) {
	case string:
		texts = []string{c}
	case []any:
		for _, b := range c {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			if bm["type"] == "tool_result" {
				return false
			}
			if t, ok := bm["text"].(string); ok && bm["type"] == "text" {
				texts = append(texts, t)
			}
		}
	}
	for _, raw := range texts {
		t := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
		if t == "" || commandNameRE.MatchString(t) {
			continue
		}
		if bashInputRE.MatchString(t) {
			return true
		}
		if promptLeadingTagRE.MatchString(t) {
			continue
		}
		return true
	}
	return false
}

// Package main provides a reference filesystem-backed implementation of
// the SessionStore interface. See main.go for the driver and top-of-file
// docs explaining the use case.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

// FileSystemSessionStore is a pure-Go reference SessionStore adapter that
// persists transcripts as JSONL files on local disk.
//
// On-disk layout:
//
//	<Root>/
//	  <project_key>/
//	    <session_id>.jsonl                    main session
//	    <session_id>/                         subkey directory (only if any subkeys exist)
//	      subagents/
//	        agent-xyz.jsonl                   subkey at subpath "subagents/agent-xyz"
//
// Concurrency: a single in-process sync.RWMutex serialises Append and
// guards file metadata reads. This is sufficient for single-process demos.
// Production deployments running multiple processes against the same
// directory should use OS-level file locks (flock(2) or LockFileEx on
// Windows) instead.
//
// This adapter implements [claude.SessionStore],
// [claude.SessionStoreLister], [claude.SessionStoreDeleter] and
// [claude.SessionStoreSubkeys]. It intentionally does NOT implement
// [claude.SessionStoreSummarizer] — a real adapter would persist a
// summary sidecar updated incrementally via [claude.FoldSessionSummary].
// That is left as an exercise for the reader; without the summarizer the
// SDK falls back to a per-session Load on the listing path, which is
// correct but slower.
type FileSystemSessionStore struct {
	// Root is the directory under which all transcripts are written.
	Root string
	mu   sync.RWMutex
}

// NewFileSystemSessionStore returns an adapter rooted at root. The
// directory is created if it does not yet exist.
func NewFileSystemSessionStore(root string) (*FileSystemSessionStore, error) {
	if root == "" {
		return nil, errors.New("root must not be empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create root: %w", err)
	}
	return &FileSystemSessionStore{Root: abs}, nil
}

// pathFor returns the absolute path to the JSONL file backing key.
//
// Main transcript:    <Root>/<project_key>/<session_id>.jsonl
// Sub-stream:         <Root>/<project_key>/<session_id>/<subpath>.jsonl
//
// Subpath uses "/" as separator regardless of host OS — we split it back
// into native path components here.
func (s *FileSystemSessionStore) pathFor(key claude.SessionKey) string {
	if key.Subpath == "" {
		return filepath.Join(s.Root, key.ProjectKey, key.SessionID+".jsonl")
	}
	subParts := strings.Split(key.Subpath, "/")
	parts := append([]string{s.Root, key.ProjectKey, key.SessionID}, subParts...)
	parts[len(parts)-1] = parts[len(parts)-1] + ".jsonl"
	return filepath.Join(parts...)
}

// Append satisfies [claude.SessionStore]. It serialises each entry as a
// JSONL line and appends to the per-key file, creating parent directories
// as needed. ctx is honored before any filesystem work begins; the SDK
// gives this call a 60s timeout from the mirror batcher and we must
// return promptly when it fires.
func (s *FileSystemSessionStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}

	// Build the JSONL payload outside the lock so we hold the write lock
	// only for the open+write portion. Marshal failures abort the whole
	// batch so the on-disk transcript stays consistent.
	var buf strings.Builder
	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Ctx may have expired while we waited on the mutex.
	if err := ctx.Err(); err != nil {
		return err
	}

	path := s.pathFor(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(buf.String()); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}

// Load satisfies [claude.SessionStore]. Returns (nil, nil) when the file
// does not exist — that signals "no entries", not an error, per the
// SessionStore contract.
func (s *FileSystemSessionStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.pathFor(key)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open: %w", err)
	}
	defer func() { _ = f.Close() }()

	var out []claude.SessionStoreEntry
	scanner := bufio.NewScanner(f)
	// Transcript lines can contain large embedded payloads (tool results
	// with file contents, etc.). Bump the scanner's max line size from
	// the 64 KiB default to 16 MiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry claude.SessionStoreEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			return nil, fmt.Errorf("parse line: %w", err)
		}
		out = append(out, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// ListSessions satisfies [claude.SessionStoreLister]. Returns one entry
// per *.jsonl file directly under <Root>/<project_key>/ (i.e. main
// transcripts only — sub-streams live under a subdirectory and are
// excluded). Mtime is the file modification time in Unix milliseconds so
// it shares a clock with what the SDK expects from the listing.
func (s *FileSystemSessionStore) ListSessions(ctx context.Context, projectKey string) ([]claude.SessionStoreListEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Join(s.Root, projectKey)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}

	var out []claude.SessionStoreListEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", name, err)
		}
		out = append(out, claude.SessionStoreListEntry{
			SessionID: strings.TrimSuffix(name, ".jsonl"),
			Mtime:     info.ModTime().UnixMilli(),
		})
	}
	return out, nil
}

// Delete satisfies [claude.SessionStoreDeleter]. When the deleted key is
// a main transcript (Subpath == "") the cascade convention requires us to
// also remove the sibling <session_id>/ directory so subagent transcripts
// are not orphaned. A targeted delete with an explicit Subpath removes
// only that one entry.
func (s *FileSystemSessionStore) Delete(ctx context.Context, key claude.SessionKey) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.pathFor(key)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", path, err)
	}

	if key.Subpath == "" {
		siblingDir := filepath.Join(s.Root, key.ProjectKey, key.SessionID)
		if err := os.RemoveAll(siblingDir); err != nil {
			return fmt.Errorf("remove sibling dir %s: %w", siblingDir, err)
		}
	}
	return nil
}

// ListSubkeys satisfies [claude.SessionStoreSubkeys]. Walks
// <Root>/<project_key>/<session_id>/ recursively and returns each *.jsonl
// path expressed as a "/"-joined subpath relative to the session root,
// with the .jsonl suffix stripped. The returned shape matches the format
// expected by GetSubagentMessagesFromStore (e.g. "subagents/agent-xyz").
func (s *FileSystemSessionStore) ListSubkeys(ctx context.Context, key claude.SessionListSubkeysKey) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	base := filepath.Join(s.Root, key.ProjectKey, key.SessionID)
	var out []string

	err := filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && path == base {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		// Always present subpaths with "/" so they round-trip with
		// SessionKey.Subpath, which is "/"-joined regardless of host OS.
		rel = filepath.ToSlash(rel)
		rel = strings.TrimSuffix(rel, ".jsonl")
		out = append(out, rel)
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("walk: %w", err)
	}
	return out, nil
}

// fileMtime is a small helper used by main.go to report the on-disk mtime
// when verifying state. Exposed here so the driver doesn't need to import
// io/fs directly.
func fileMtime(path string) (time.Time, error) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

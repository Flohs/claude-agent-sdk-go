// FileSystem SessionStore example — a custom adapter pattern walkthrough.
//
// This example shows how to implement a custom SessionStore from scratch.
// FileSystemSessionStore (in filestore.go) is a pure-Go, stdlib-only
// reference implementation that persists transcripts as JSONL files on
// local disk. It implements four interfaces:
//
//   - claude.SessionStore         (Append + Load — required)
//   - claude.SessionStoreLister   (ListSessions)
//   - claude.SessionStoreDeleter  (Delete with subkey cascade)
//   - claude.SessionStoreSubkeys  (ListSubkeys)
//
// claude.SessionStoreSummarizer is intentionally skipped to keep the
// example focused — see the comment on FileSystemSessionStore for what a
// real adapter would do there.
//
// This driver does not run a Claude query — that is covered in
// examples/session_store, which shows how to wire a SessionStore into
// Options.SessionStore so the SDK actively mirrors transcripts as the CLI
// emits them. Here we focus on the adapter contract itself: append a
// handful of fake transcript entries, load them back, list them via the
// Lister, list a subagent subkey via Subkeys, and Delete with cascade.
//
// Usage:
//
//	go run ./examples/session_store_filesystem
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

// fakeEntry constructs a minimal SessionStoreEntry that round-trips
// through the chain builder used by GetSessionMessagesFromStore. The
// shape mirrors what the CLI writes for a "user" line.
func fakeEntry(sessionID, role, text string) claude.SessionStoreEntry {
	return claude.SessionStoreEntry{
		"type":      role,
		"sessionId": sessionID,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message":   map[string]any{"role": role, "content": text},
	}
}

func main() {
	ctx := context.Background()

	// Root the store under a temp dir so the example is self-cleaning.
	root := filepath.Join(os.TempDir(), "session-store-demo")
	defer func() {
		if err := os.RemoveAll(root); err != nil {
			log.Printf("cleanup: %v", err)
		}
	}()

	store, err := NewFileSystemSessionStore(root)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Store rooted at: %s\n\n", store.Root)

	// Use the SDK's project key derivation so a future caller passing
	// the same Cwd to Options.SessionStore would land on the same
	// directory. ProjectKeyForDirectory("") defaults to "." (cwd).
	projectKey := claude.ProjectKeyForDirectory("")
	sessionID := "11111111-2222-3333-4444-555555555555"
	mainKey := claude.SessionKey{ProjectKey: projectKey, SessionID: sessionID}

	// 1. Append a few transcript entries to the main session.
	mainEntries := []claude.SessionStoreEntry{
		fakeEntry(sessionID, "user", "Hello"),
		fakeEntry(sessionID, "assistant", "Hi there"),
		fakeEntry(sessionID, "user", "How are you?"),
	}
	if err := store.Append(ctx, mainKey, mainEntries); err != nil {
		log.Fatalf("Append main: %v", err)
	}
	fmt.Printf("Appended %d entries to main session %s\n", len(mainEntries), sessionID)

	// 2. Append entries to a subkey — the conventional shape used for
	// subagent transcripts is "subagents/agent-<id>".
	subKey := claude.SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
		Subpath:    "subagents/agent-helper",
	}
	subEntries := []claude.SessionStoreEntry{
		fakeEntry(sessionID, "user", "[subagent] start"),
		fakeEntry(sessionID, "assistant", "[subagent] done"),
	}
	if err := store.Append(ctx, subKey, subEntries); err != nil {
		log.Fatalf("Append sub: %v", err)
	}
	fmt.Printf("Appended %d entries to subkey %s\n\n", len(subEntries), subKey.Subpath)

	// 3. Load the main transcript back and verify the count round-trips.
	loaded, err := store.Load(ctx, mainKey)
	if err != nil {
		log.Fatalf("Load: %v", err)
	}
	if len(loaded) != len(mainEntries) {
		log.Fatalf("count mismatch: got %d entries, want %d", len(loaded), len(mainEntries))
	}
	fmt.Printf("Loaded %d entries from main session — count matches.\n", len(loaded))

	// Print on-disk path so the user can inspect manually.
	mainPath := filepath.Join(store.Root, projectKey, sessionID+".jsonl")
	if mt, err := fileMtime(mainPath); err == nil {
		fmt.Printf("On-disk file: %s (mtime %s)\n\n", mainPath, mt.Format(time.RFC3339))
	}

	// 4. ListSessions: should return the one main session we wrote.
	sessions, err := store.ListSessions(ctx, projectKey)
	if err != nil {
		log.Fatalf("ListSessions: %v", err)
	}
	fmt.Printf("ListSessions returned %d session(s):\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  - id=%s mtime=%d\n", s.SessionID, s.Mtime)
	}
	fmt.Println()

	// 5. ListSubkeys: should return our one subagent subpath.
	subkeys, err := store.ListSubkeys(ctx, claude.SessionListSubkeysKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
	})
	if err != nil {
		log.Fatalf("ListSubkeys: %v", err)
	}
	fmt.Printf("ListSubkeys returned %d subkey(s):\n", len(subkeys))
	for _, sk := range subkeys {
		fmt.Printf("  - %s\n", sk)
	}
	fmt.Println()

	// 6. Delete with cascade: removing the main key wipes the sibling
	// subkey directory too.
	if err := store.Delete(ctx, mainKey); err != nil {
		log.Fatalf("Delete: %v", err)
	}
	sessions, err = store.ListSessions(ctx, projectKey)
	if err != nil {
		log.Fatalf("ListSessions after delete: %v", err)
	}
	subkeys, err = store.ListSubkeys(ctx, claude.SessionListSubkeysKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
	})
	if err != nil {
		log.Fatalf("ListSubkeys after delete: %v", err)
	}
	fmt.Printf("After Delete: %d session(s), %d subkey(s) remain.\n", len(sessions), len(subkeys))
}

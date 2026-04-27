// SessionStore example demonstrating end-to-end usage of the SessionStore
// API: mirror-mode wiring, store-backed read/mutation helpers, mirror-error
// surfacing, and bounded close semantics.
//
// Use case: deploying Claude Code as a backend behind a load balancer or in
// ephemeral containers. The CLI subprocess only ever knows about its local
// $HOME/.claude tree, so transcripts written to disk vanish when the
// container exits. Configure Options.SessionStore and the SDK actively
// mirrors every transcript line to your store as the CLI emits it. After
// the run completes you can read the session back from any process via the
// *FromStore helpers, or mutate it via the *ViaStore helpers, without
// requiring the original CLI process to still be alive.
//
// Quick-start: this example uses InMemorySessionStore so it has zero
// external dependencies. For production, swap in your own adapter — see
// examples/session_store_filesystem for a pure-Go reference implementation
// of the SessionStore interface.
//
// Expected runtime: a single short Claude query plus a handful of in-memory
// store operations. Auth requirement: standard Claude CLI auth (the same
// auth all other examples use).
//
// Usage:
//
//	go run ./examples/session_store
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

// faultyStore wraps a SessionStore and starts returning an error from
// Append after a configured number of successful calls. It exists to
// demonstrate the SDK's MirrorErrorMessage flow: the transcript mirror
// batcher retries with backoff, and once it exhausts its retry budget the
// SDK injects a *MirrorErrorMessage into the inbound message stream so
// callers can react via Client.ReceiveMessages.
//
// All other SessionStore-related interfaces are passed through to the
// inner store unchanged so the rest of the SDK still treats it like a
// fully-featured adapter.
type faultyStore struct {
	inner     claude.SessionStore
	failAfter int
	calls     atomic.Int64
}

// Append delegates to the inner store, but starts returning an error after
// failAfter successful calls. The error is intentionally generic — the SDK
// treats any non-nil error as a retry-eligible failure.
func (f *faultyStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if f.calls.Add(1) > int64(f.failAfter) {
		return errors.New("simulated transient store failure")
	}
	return f.inner.Append(ctx, key, entries)
}

// Load delegates to the inner store. Mirror-mode never calls Load — it's
// only invoked by the resume path or the *FromStore helpers — but we
// implement it for interface completeness.
func (f *faultyStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	return f.inner.Load(ctx, key)
}

// ListSessions delegates so the read helpers still work after a faulty
// run. Without this passthrough, *FromStore helpers would fail because
// the SDK probes the store via type assertion on the wrapper, not the
// inner type.
func (f *faultyStore) ListSessions(ctx context.Context, projectKey string) ([]claude.SessionStoreListEntry, error) {
	if l, ok := f.inner.(claude.SessionStoreLister); ok {
		return l.ListSessions(ctx, projectKey)
	}
	return nil, nil
}

// slowStore wraps a SessionStore and sleeps before each Append call. It
// simulates a slow remote backend so we can demonstrate Client.CloseContext
// honoring a caller-supplied deadline — the SDK returns from CloseContext
// as soon as ctx fires, while the underlying batcher continues draining
// in the background.
type slowStore struct {
	inner claude.SessionStore
	delay time.Duration
}

// Append sleeps for delay before delegating, but returns early if ctx is
// canceled or expires. Honoring ctx is part of the SessionStore contract
// (see godoc on SessionStore) — adapters that ignore ctx leak a goroutine
// per timed-out call.
func (s *slowStore) Append(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return ctx.Err()
	}
	return s.inner.Append(ctx, key, entries)
}

// Load delegates straight through.
func (s *slowStore) Load(ctx context.Context, key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	return s.inner.Load(ctx, key)
}

// mirrorModeExample runs a short Claude query with Options.SessionStore set
// and then reads back what landed in the store via the *FromStore helpers.
// This is the primary use case: the store is populated as a side effect of
// a normal CLI run.
func mirrorModeExample(ctx context.Context) *claude.InMemorySessionStore {
	fmt.Println("=== Mirror Mode ===")
	fmt.Println("Configures Options.SessionStore so every transcript line the CLI writes")
	fmt.Println("is parallel-copied into our store. Then reads it back via *FromStore helpers.")

	store := claude.NewInMemorySessionStore()
	maxTurns := 1

	// One-shot Query is the simplest path. The mirror batcher inside the
	// SDK is responsible for batching transcript writes and calling
	// store.Append; callers don't see those calls directly.
	messages, errs := claude.Query(ctx, "Say hello in one short sentence.", &claude.Options{
		SessionStore: store,
		MaxTurns:     &maxTurns,
		Title:        "Mirror-mode demo",
	})

	for msg := range messages {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	for err := range errs {
		log.Println("Query error:", err)
	}

	// Now inspect the store. ListSessionsFromStore takes the
	// SessionStoreSummarizer fast path because InMemorySessionStore
	// implements it.
	infos, err := claude.ListSessionsFromStore(ctx, store, claude.ListSessionsOptions{})
	if err != nil {
		log.Println("ListSessionsFromStore error:", err)
		fmt.Println()
		return store
	}
	fmt.Printf("Store now contains %d session(s):\n", len(infos))
	for _, info := range infos {
		fmt.Printf("  - %s | summary=%q\n", info.SessionID, info.Summary)

		msgs, err := claude.GetSessionMessagesFromStore(ctx, store, info.SessionID, claude.GetSessionMessagesOptions{})
		if err != nil {
			log.Println("  GetSessionMessagesFromStore error:", err)
			continue
		}
		fmt.Printf("    %d transcript message(s) in store\n", len(msgs))
	}
	fmt.Println()
	return store
}

// mutationExample applies the *ViaStore mutators against an existing store.
// Each mutator is append-only — it writes a small synthetic JSONL entry
// (custom-title, tag, etc.) that the read-side fold turns into the
// presented title/tag/etc. So calling them twice is harmless; the latest
// append wins.
//
// If the mirror-mode pass populated the store with a real session we
// operate on it; otherwise we synthesize one via Append so this scenario
// is runnable in isolation.
func mutationExample(ctx context.Context, store *claude.InMemorySessionStore) {
	fmt.Println("=== Mutation Helpers ===")
	fmt.Println("Demonstrates RenameSessionViaStore + TagSessionViaStore + DeleteSessionViaStore.")

	infos, err := claude.ListSessionsFromStore(ctx, store, claude.ListSessionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	var sessionID string
	if len(infos) > 0 {
		sessionID = infos[0].SessionID
	} else {
		// Seed a synthetic session so the scenario runs without a live
		// CLI. We use a fixed UUID — every *ViaStore helper validates
		// that sessionID parses as a UUID before doing anything.
		sessionID = "11111111-2222-3333-4444-555555555555"
		projectKey := claude.ProjectKeyForDirectory("")
		err := store.Append(ctx, claude.SessionKey{ProjectKey: projectKey, SessionID: sessionID}, []claude.SessionStoreEntry{
			{
				"type":      "user",
				"sessionId": sessionID,
				"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
				"message":   map[string]any{"role": "user", "content": "Synthetic seed entry"},
			},
		})
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Seeded synthetic session %s\n", sessionID)
	}

	// Rename: appends a {type:"custom-title"} entry. Pass
	// StoreMutationOptions{} when the project is the current working
	// directory; supply Directory to scope to a different project.
	if err := claude.RenameSessionViaStore(ctx, store, sessionID, "Hand-picked title", claude.StoreMutationOptions{}); err != nil {
		log.Fatal(err)
	}

	// Tag: appends a {type:"tag"} entry. Pass nil to clear the tag.
	tag := "demo"
	if err := claude.TagSessionViaStore(ctx, store, sessionID, &tag, claude.StoreMutationOptions{}); err != nil {
		log.Fatal(err)
	}

	// Re-read so we can show the title and tag landed.
	infos, err = claude.ListSessionsFromStore(ctx, store, claude.ListSessionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	for _, info := range infos {
		if info.SessionID != sessionID {
			continue
		}
		tagStr := "<nil>"
		if info.Tag != nil {
			tagStr = *info.Tag
		}
		fmt.Printf("After mutators: id=%s custom_title=%q tag=%s summary=%q\n",
			info.SessionID, info.CustomTitle, tagStr, info.Summary)
	}

	// Delete cascades to subkeys when the adapter implements
	// SessionStoreSubkeys (InMemorySessionStore does). After this call
	// the session is gone from the store entirely.
	if err := claude.DeleteSessionViaStore(ctx, store, sessionID, claude.StoreMutationOptions{}); err != nil {
		log.Fatal(err)
	}
	infos, err = claude.ListSessionsFromStore(ctx, store, claude.ListSessionsOptions{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("After delete: %d session(s) remain in store\n", len(infos))
	fmt.Println()
}

// mirrorErrorExample wraps the store with a faulty adapter so the SDK's
// retry budget is exhausted and a *MirrorErrorMessage appears on the
// inbound message channel. We use NewClient + Connect here (instead of
// Query) so we get direct access to ReceiveMessages and can iterate the
// raw stream including system messages.
func mirrorErrorExample(ctx context.Context) {
	fmt.Println("=== Mirror Error Surfacing ===")
	fmt.Println("Wraps the store so Append fails after 3 successful calls. The mirror batcher")
	fmt.Println("retries with backoff and once it gives up, the SDK synthesizes a")
	fmt.Println("*MirrorErrorMessage on the message stream so callers can react.")

	store := &faultyStore{inner: claude.NewInMemorySessionStore(), failAfter: 3}
	maxTurns := 1

	client := claude.NewClient(&claude.Options{
		SessionStore: store,
		MaxTurns:     &maxTurns,
	})
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendQuery(ctx, "Count to three, one number per line."); err != nil {
		log.Fatal(err)
	}

	// Drain ReceiveMessages so we see system frames including any
	// MirrorErrorMessage. ReceiveResponse would also work but stops at
	// the first ResultMessage; mirror errors can land just before the
	// result so iterating ReceiveMessages until we observe Result is the
	// safer pattern when the goal is specifically to catch them.
	mirrorErrors := 0
	for msg := range client.ReceiveMessages(ctx) {
		switch m := msg.(type) {
		case *claude.MirrorErrorMessage:
			mirrorErrors++
			keyStr := "<nil>"
			if m.Key != nil {
				keyStr = fmt.Sprintf("%s/%s", m.Key.ProjectKey, m.Key.SessionID)
			}
			fmt.Printf("MirrorErrorMessage: key=%s error=%q\n", keyStr, m.Error)
		case *claude.ResultMessage:
			// End of conversation; break out of the receive loop.
			fmt.Printf("Run finished: %d mirror error(s) surfaced; %d Append calls observed total.\n",
				mirrorErrors, store.calls.Load())
			fmt.Println()
			return
		}
	}
}

// closeContextExample shows that Client.CloseContext returns once the
// caller-supplied deadline fires, even if the mirror batcher is still
// trying to drain a slow store. The batcher continues to drain in the
// background so any pending writes still get a chance to land — but the
// caller is no longer blocked on them.
func closeContextExample(ctx context.Context) {
	fmt.Println("=== CloseContext Deadline ===")
	fmt.Println("Wraps the store so each Append sleeps 30s. Then CloseContext is called with")
	fmt.Println("a 5-second deadline. The deadline fires; Close returns; the batcher worker")
	fmt.Println("keeps draining in the background.")

	store := &slowStore{inner: claude.NewInMemorySessionStore(), delay: 30 * time.Second}
	maxTurns := 1

	client := claude.NewClient(&claude.Options{
		SessionStore: store,
		MaxTurns:     &maxTurns,
	})
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}

	if err := client.SendQuery(ctx, "Reply with the single word 'ok'."); err != nil {
		log.Fatal(err)
	}

	// Wait for the conversation to end before initiating close so there
	// is real data queued for the batcher to drain.
	for msg := range client.ReceiveResponse(ctx) {
		_ = msg
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	start := time.Now()
	err := client.CloseContext(closeCtx)
	elapsed := time.Since(start)
	fmt.Printf("CloseContext returned after %s (err=%v)\n", elapsed.Round(100*time.Millisecond), err)
	fmt.Println()
}

func main() {
	ctx := context.Background()

	store := mirrorModeExample(ctx)
	mutationExample(ctx, store)
	mirrorErrorExample(ctx)
	closeContextExample(ctx)
}

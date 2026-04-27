package claude

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeStore is a SessionStore test double that records Append calls,
// optionally fails a configured number of times, and can sleep inside
// Append to exercise timeout semantics.
type fakeStore struct {
	mu          sync.Mutex
	appends     [][]SessionStoreEntry // per-call slice, in call order
	keysByCall  []SessionKey
	callCount   atomic.Int64
	failCount   atomic.Int64 // number of initial failures
	sleepOnCall time.Duration
	sleepOnly   int64 // only sleep on the first N calls; 0 = always when sleepOnCall > 0
	failErr     error
}

func (s *fakeStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	n := s.callCount.Add(1)
	if s.sleepOnCall > 0 && (s.sleepOnly == 0 || n <= s.sleepOnly) {
		select {
		case <-time.After(s.sleepOnCall):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if s.failCount.Load() > 0 {
		s.failCount.Add(-1)
		if s.failErr != nil {
			return s.failErr
		}
		return errors.New("transient failure")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]SessionStoreEntry, len(entries))
	copy(cp, entries)
	s.appends = append(s.appends, cp)
	s.keysByCall = append(s.keysByCall, key)
	return nil
}

func (s *fakeStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	return nil, nil
}

func (s *fakeStore) snapshot() ([][]SessionStoreEntry, []SessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	appends := make([][]SessionStoreEntry, len(s.appends))
	copy(appends, s.appends)
	keys := make([]SessionKey, len(s.keysByCall))
	copy(keys, s.keysByCall)
	return appends, keys
}

// testProjectsDir returns a projects directory and a helper that builds a
// transcript file path under it for a given session ID. The projects
// directory is a subdirectory of t.TempDir() so it is cleaned up
// automatically.
func testProjectsDir(t *testing.T) (string, func(sessionID string) string) {
	t.Helper()
	projectsDir := filepath.Join(t.TempDir(), "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	return projectsDir, func(sessionID string) string {
		return filepath.Join(projectsDir, "my-project", sessionID+".jsonl")
	}
}

func TestTranscriptMirrorBatcher_HappyPath(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	b.Enqueue(pathFor("11111111-1111-1111-1111-111111111111"), []SessionStoreEntry{
		{"uuid": "a"}, {"uuid": "b"},
	})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	appends, keys := store.snapshot()
	if len(appends) != 1 {
		t.Fatalf("expected 1 append, got %d", len(appends))
	}
	if len(appends[0]) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(appends[0]))
	}
	if keys[0].SessionID != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("unexpected session id: %+v", keys[0])
	}
}

func TestTranscriptMirrorBatcher_CoalesceByFilePath(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	path := pathFor("22222222-2222-2222-2222-222222222222")
	b.Enqueue(path, []SessionStoreEntry{{"uuid": "a"}})
	b.Enqueue(path, []SessionStoreEntry{{"uuid": "b"}, {"uuid": "c"}})
	// Different filePath should NOT coalesce into the first key.
	path2 := pathFor("33333333-3333-3333-3333-333333333333")
	b.Enqueue(path2, []SessionStoreEntry{{"uuid": "x"}})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	appends, keys := store.snapshot()
	if len(appends) != 2 {
		t.Fatalf("expected 2 appends (one per key), got %d", len(appends))
	}
	// First append should have 3 entries (coalesced from two enqueues).
	if len(appends[0]) != 3 {
		t.Fatalf("expected first append to have 3 coalesced entries, got %d", len(appends[0]))
	}
	if keys[0].SessionID == keys[1].SessionID {
		t.Fatalf("expected two distinct sessions, both = %s", keys[0].SessionID)
	}
}

func TestTranscriptMirrorBatcher_RetryThenSuccess(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	store.failCount.Store(1) // fail once, succeed on retry

	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	b.Enqueue(pathFor("44444444-4444-4444-4444-444444444444"), []SessionStoreEntry{{"uuid": "a"}})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	appends, _ := store.snapshot()
	if len(appends) != 1 {
		t.Fatalf("expected 1 successful append after retry, got %d", len(appends))
	}
	if store.callCount.Load() != 2 {
		t.Fatalf("expected 2 total attempts (1 fail + 1 retry), got %d", store.callCount.Load())
	}
}

func TestTranscriptMirrorBatcher_RetryExhaustion_FiresOnError(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	store.failCount.Store(100) // exhaust all retries

	var errCalls atomic.Int64
	var gotKey SessionKey
	var gotErr error
	var errMu sync.Mutex
	onError := func(key SessionKey, err error) {
		errMu.Lock()
		defer errMu.Unlock()
		errCalls.Add(1)
		gotKey = key
		gotErr = err
	}

	b := newTranscriptMirrorBatcher(store, projectsDir, onError, nil)
	defer b.Close()

	sessionID := "55555555-5555-5555-5555-555555555555"
	b.Enqueue(pathFor(sessionID), []SessionStoreEntry{{"uuid": "a"}})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := store.callCount.Load(); got != MirrorAppendMaxAttempts {
		t.Fatalf("expected %d append attempts, got %d", MirrorAppendMaxAttempts, got)
	}
	if got := errCalls.Load(); got != 1 {
		t.Fatalf("expected onError called once, got %d", got)
	}
	errMu.Lock()
	defer errMu.Unlock()
	if gotKey.SessionID != sessionID {
		t.Fatalf("unexpected key: %+v", gotKey)
	}
	if gotErr == nil {
		t.Fatal("expected an error, got nil")
	}
}

func TestTranscriptMirrorBatcher_TimeoutIsTerminal_NoRetry(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	store.sleepOnCall = 200 * time.Millisecond

	var errCalls atomic.Int64
	onError := func(key SessionKey, err error) {
		errCalls.Add(1)
	}

	b := newTranscriptMirrorBatcher(store, projectsDir, onError, nil)
	// Shrink the per-append timeout so the test is fast.
	b.sendTimeout = 20 * time.Millisecond
	defer b.Close()

	b.Enqueue(pathFor("66666666-6666-6666-6666-666666666666"), []SessionStoreEntry{{"uuid": "a"}})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	// Timeout must NOT retry — exactly one call to the store.
	if got := store.callCount.Load(); got != 1 {
		t.Fatalf("expected exactly 1 attempt on timeout, got %d", got)
	}
	if got := errCalls.Load(); got != 1 {
		t.Fatalf("expected onError called once, got %d", got)
	}
}

func TestTranscriptMirrorBatcher_ThresholdAutoFlush(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	// Enqueue > MirrorMaxPendingEntries entries in a single frame. That
	// should trigger an auto-flush without an explicit Flush call.
	entries := make([]SessionStoreEntry, MirrorMaxPendingEntries+1)
	for i := range entries {
		entries[i] = SessionStoreEntry{"uuid": fmt.Sprintf("e%d", i)}
	}
	b.Enqueue(pathFor("77777777-7777-7777-7777-777777777777"), entries)

	// Wait for the worker to consume the auto-flush request. A bounded poll
	// is fine — if the threshold never auto-fires we'd hang forever here
	// rather than fail silently.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if store.callCount.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := store.callCount.Load(); got == 0 {
		t.Fatal("expected auto-flush to trigger at least one Append call")
	}
}

func TestTranscriptMirrorBatcher_ConcurrentEnqueueAndFlush(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	const workers = 8
	const perWorker = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := pathFor(fmt.Sprintf("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaa%d", id))
			for i := 0; i < perWorker; i++ {
				b.Enqueue(path, []SessionStoreEntry{{"uuid": fmt.Sprintf("w%d-%d", id, i)}})
			}
		}(w)
	}
	// Racing Flush calls must not double-send the same buffer.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = b.Flush(context.Background())
		}()
	}
	wg.Wait()

	// Final flush to drain anything still pending.
	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("final Flush: %v", err)
	}

	// Count entries that reached the store — must equal the total enqueued.
	appends, _ := store.snapshot()
	total := 0
	for _, a := range appends {
		total += len(a)
	}
	if total != workers*perWorker {
		t.Fatalf("expected %d total entries, got %d", workers*perWorker, total)
	}
}

func TestTranscriptMirrorBatcher_CloseDrainsPending(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)

	b.Enqueue(pathFor("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"), []SessionStoreEntry{
		{"uuid": "a"}, {"uuid": "b"},
	})

	b.Close()

	// After Close returns, the worker must have fully drained.
	appends, _ := store.snapshot()
	if len(appends) != 1 || len(appends[0]) != 2 {
		t.Fatalf("expected Close to drain pending entries, got %v", appends)
	}

	// Done channel should be closed.
	select {
	case <-b.Done():
	case <-time.After(time.Second):
		t.Fatal("Done channel was not closed after Close()")
	}

	// Calling Close a second time must be a no-op.
	b.Close()
}

// blockingStore is a SessionStore whose Append blocks forever (or until
// the test signals release), used to exercise CloseContext deadline
// behavior without depending on an unhealthy real adapter.
type blockingStore struct {
	release chan struct{}
	called  atomic.Int64
}

func newBlockingStore() *blockingStore {
	return &blockingStore{release: make(chan struct{})}
}

func (b *blockingStore) Append(ctx context.Context, key SessionKey, entries []SessionStoreEntry) error {
	b.called.Add(1)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.release:
		return nil
	}
}

func (b *blockingStore) Load(ctx context.Context, key SessionKey) ([]SessionStoreEntry, error) {
	return nil, nil
}

func TestTranscriptMirrorBatcher_CloseContext_RespectsDeadline(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := newBlockingStore()
	defer close(store.release) // unblock the worker so the test goroutine exits

	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)

	b.Enqueue(pathFor("cccccccc-cccc-cccc-cccc-cccccccccccc"), []SessionStoreEntry{
		{"uuid": "z"},
	})

	// Tight deadline — Append blocks forever, so CloseContext must return
	// ctx.Err() rather than waiting up to ~3 minutes.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := b.CloseContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected CloseContext to return ctx.Err() on deadline")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("CloseContext blocked %v, should have honored deadline", elapsed)
	}
}

func TestTranscriptMirrorBatcher_CloseContext_DrainsWhenAdapterHealthy(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)

	b.Enqueue(pathFor("dddddddd-dddd-dddd-dddd-dddddddddddd"), []SessionStoreEntry{
		{"uuid": "x"},
	})

	// Generous deadline — adapter is healthy so drain completes quickly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := b.CloseContext(ctx); err != nil {
		t.Fatalf("CloseContext: %v", err)
	}

	appends, _ := store.snapshot()
	if len(appends) != 1 {
		t.Fatalf("expected drain to complete, got %d Append calls", len(appends))
	}
}

func TestTranscriptMirrorBatcher_UnresolvableFilePath_DropsWithWarning(t *testing.T) {
	projectsDir, _ := testProjectsDir(t)
	store := &fakeStore{}
	var warnings atomic.Int64
	var errCalls atomic.Int64
	stderr := func(s string) { warnings.Add(1) }
	onError := func(key SessionKey, err error) { errCalls.Add(1) }

	b := newTranscriptMirrorBatcher(store, projectsDir, onError, stderr)
	defer b.Close()

	// Path is outside the configured projects directory — should be dropped
	// with a stderr warning and NOT surfaced to onError.
	b.Enqueue("/absolute/elsewhere/session.jsonl", []SessionStoreEntry{{"uuid": "a"}})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if got := store.callCount.Load(); got != 0 {
		t.Fatalf("expected no Append calls for unresolvable path, got %d", got)
	}
	if got := errCalls.Load(); got != 0 {
		t.Fatalf("expected no onError calls for unresolvable path, got %d", got)
	}
	if got := warnings.Load(); got == 0 {
		t.Fatal("expected at least one stderr warning")
	}
}

func TestTranscriptMirrorBatcher_EmptyFrameIsNoop(t *testing.T) {
	projectsDir, pathFor := testProjectsDir(t)
	store := &fakeStore{}
	b := newTranscriptMirrorBatcher(store, projectsDir, nil, nil)
	defer b.Close()

	b.Enqueue(pathFor("cccccccc-cccc-cccc-cccc-cccccccccccc"), nil)
	b.Enqueue(pathFor("cccccccc-cccc-cccc-cccc-cccccccccccc"), []SessionStoreEntry{})

	if err := b.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := store.callCount.Load(); got != 0 {
		t.Fatalf("expected no Append calls for empty frames, got %d", got)
	}
}

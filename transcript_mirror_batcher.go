package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Exported flush thresholds. Tests reference these to drive coverage.
const (
	// MirrorMaxPendingEntries is the entry count threshold that triggers an
	// eager background flush.
	MirrorMaxPendingEntries = 500
	// MirrorMaxPendingBytes is the cumulative JSON-byte threshold that
	// triggers an eager background flush. 1 MiB.
	MirrorMaxPendingBytes = 1 << 20
	// MirrorSendTimeout bounds a single [SessionStore.Append] call. A
	// timeout is treated as a terminal failure for the batch — it is NOT
	// retried because the in-flight Append may still land and a retry would
	// launch a concurrent duplicate.
	MirrorSendTimeout = 60 * time.Second
	// MirrorAppendMaxAttempts is the total number of [SessionStore.Append]
	// attempts per batch (initial + retries).
	MirrorAppendMaxAttempts = 3
)

// MirrorAppendBackoff lists the delays between attempts. Length must equal
// [MirrorAppendMaxAttempts] - 1.
var MirrorAppendBackoff = []time.Duration{
	200 * time.Millisecond,
	800 * time.Millisecond,
}

// mirrorErrorFunc is the callback invoked exactly once per terminal batch
// failure. It is passed the [SessionKey] for which the flush failed (never
// nil — frames whose filePath cannot be resolved to a SessionKey are dropped
// with a warning before they reach the callback) and the underlying error.
type mirrorErrorFunc func(key SessionKey, err error)

// mirrorBatchItem is a single pending enqueued frame awaiting flush.
type mirrorBatchItem struct {
	filePath string
	entries  []SessionStoreEntry
	bytes    int
}

// transcriptMirrorBatcher accumulates transcript_mirror frames and flushes
// them to a [SessionStore] in the background.
//
// Flush triggers:
//   - buffer reaches [MirrorMaxPendingEntries] entries,
//   - buffer reaches [MirrorMaxPendingBytes] bytes, or
//   - an explicit Flush / Close call.
//
// Each flush coalesces pending items by filePath so a single [SessionKey]
// receives at most one Append per flush. Individual Append calls are retried
// up to [MirrorAppendMaxAttempts] times with [MirrorAppendBackoff] delays;
// timeouts are NOT retried (see [MirrorSendTimeout]). On terminal failure
// the configured onError callback is invoked once per failed key.
//
// The batcher owns a single worker goroutine so callers never block on I/O.
// Enqueue is non-blocking; Flush and Close block until pending work
// completes.
type transcriptMirrorBatcher struct {
	store       SessionStore
	projectsDir string
	onError     mirrorErrorFunc
	stderr      func(string)
	sendTimeout time.Duration

	mu            sync.Mutex
	pending       []mirrorBatchItem
	pendingItems  int
	pendingBytes  int
	flushRequests chan flushRequest
	closed        bool

	wg     sync.WaitGroup
	doneCh chan struct{}
}

// flushRequest is a work item sent to the worker goroutine. When explicit
// is non-nil, the worker closes it after the flush completes.
type flushRequest struct {
	explicit chan struct{}
}

// newTranscriptMirrorBatcher returns a started batcher. Callers must invoke
// Close exactly once to drain pending work and stop the worker goroutine.
func newTranscriptMirrorBatcher(
	store SessionStore,
	projectsDir string,
	onError mirrorErrorFunc,
	stderr func(string),
) *transcriptMirrorBatcher {
	b := &transcriptMirrorBatcher{
		store:         store,
		projectsDir:   projectsDir,
		onError:       onError,
		stderr:        stderr,
		sendTimeout:   MirrorSendTimeout,
		flushRequests: make(chan flushRequest, 64),
		doneCh:        make(chan struct{}),
	}
	b.wg.Add(1)
	go b.run()
	return b
}

// Enqueue buffers a frame. When pending thresholds are crossed, Enqueue
// schedules an eager flush on the worker goroutine. It never blocks.
func (b *transcriptMirrorBatcher) Enqueue(filePath string, entries []SessionStoreEntry) {
	if len(entries) == 0 {
		return
	}
	// Approximate wire size — one stringify per frame, not per entry.
	encoded, err := json.Marshal(entries)
	size := 0
	if err == nil {
		size = len(encoded)
	}

	// Defensive copy: the caller's slice backing array may be reused for the
	// next frame. We retain the entries until they are handed to the store.
	cp := make([]SessionStoreEntry, len(entries))
	copy(cp, entries)

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.pending = append(b.pending, mirrorBatchItem{
		filePath: filePath,
		entries:  cp,
		bytes:    size,
	})
	b.pendingItems += len(entries)
	b.pendingBytes += size
	shouldFlush := b.pendingItems > MirrorMaxPendingEntries || b.pendingBytes > MirrorMaxPendingBytes
	b.mu.Unlock()

	if shouldFlush {
		// Fire-and-forget auto-flush. Send is non-blocking: if the worker
		// is busy and the request buffer is full, the next explicit Flush /
		// the next threshold-cross will pick up the still-pending items.
		select {
		case b.flushRequests <- flushRequest{}:
		default:
		}
	}
}

// Flush blocks until all currently-pending entries are flushed (or fail
// terminally). Returns when the worker has processed a flush request whose
// snapshot included every item enqueued prior to Flush.
func (b *transcriptMirrorBatcher) Flush(ctx context.Context) error {
	done := make(chan struct{})
	req := flushRequest{explicit: done}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return fmt.Errorf("transcriptMirrorBatcher: closed")
	}
	b.mu.Unlock()

	select {
	case b.flushRequests <- req:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Close performs a final drain flush with the same retry policy, then stops
// the worker goroutine. It is safe to call Close more than once — subsequent
// calls are no-ops. Close does not return until the goroutine has exited.
func (b *transcriptMirrorBatcher) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()

	// Signal the worker to drain any remaining items and exit. The worker
	// reads from flushRequests until it is closed.
	close(b.flushRequests)
	b.wg.Wait()
	close(b.doneCh)
}

// Done returns a channel that is closed after Close has fully drained and
// the worker goroutine has exited. Useful in tests.
func (b *transcriptMirrorBatcher) Done() <-chan struct{} {
	return b.doneCh
}

// run is the worker goroutine. It drains flush requests sequentially, then
// performs a final drain on shutdown.
func (b *transcriptMirrorBatcher) run() {
	defer b.wg.Done()

	for req := range b.flushRequests {
		b.drain()
		if req.explicit != nil {
			close(req.explicit)
		}
	}
	// Channel closed by Close — perform a final best-effort drain.
	b.drain()
}

// drain detaches the pending buffer and writes each coalesced key to the
// store. Never panics. Append errors beyond the retry budget surface via
// onError after the batch lock is released so a slow callback cannot stall
// subsequent drains.
func (b *transcriptMirrorBatcher) drain() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}
	items := b.pending
	b.pending = nil
	b.pendingItems = 0
	b.pendingBytes = 0
	b.mu.Unlock()

	// Coalesce by filePath. Python uses an insertion-ordered dict; we mirror
	// that with a slice of keys alongside a map.
	byPath := make(map[string][]SessionStoreEntry)
	var order []string
	for _, item := range items {
		if _, ok := byPath[item.filePath]; !ok {
			order = append(order, item.filePath)
		}
		byPath[item.filePath] = append(byPath[item.filePath], item.entries...)
	}

	type errReport struct {
		key SessionKey
		err error
	}
	var errors []errReport

	for _, filePath := range order {
		entries := byPath[filePath]
		if len(entries) == 0 {
			continue
		}
		key, ok := FilePathToSessionKey(filePath, b.projectsDir)
		if !ok {
			b.warnf(
				"[SessionStore] dropping mirror frame: filePath %s is not "+
					"under %s -- subprocess CLAUDE_CONFIG_DIR likely differs "+
					"from parent (custom env / container?)",
				filePath, b.projectsDir,
			)
			continue
		}

		if err := b.attemptAppend(key, entries); err != nil {
			errors = append(errors, errReport{key: key, err: err})
		}
	}

	for _, er := range errors {
		if b.onError != nil {
			b.safeInvokeOnError(er.key, er.err)
		}
	}
}

// attemptAppend runs up to MirrorAppendMaxAttempts attempts against the
// store. Returns nil on success, or the last error after exhausting
// attempts. A timeout is treated as a terminal failure (no retry).
func (b *transcriptMirrorBatcher) attemptAppend(key SessionKey, entries []SessionStoreEntry) error {
	var lastErr error
	for attempt := 0; attempt < MirrorAppendMaxAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(MirrorAppendBackoff[attempt-1])
		}
		err := b.callAppend(key, entries)
		if err == nil {
			return nil
		}
		lastErr = err
		if isTimeoutErr(err) {
			// Do not retry on timeout: the in-flight Append may still land.
			return err
		}
	}
	return lastErr
}

// callAppend invokes [SessionStore.Append] with a per-call timeout.
func (b *transcriptMirrorBatcher) callAppend(key SessionKey, entries []SessionStoreEntry) error {
	ctx, cancel := context.WithTimeout(context.Background(), b.sendTimeout)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		// Panic in user adapter code should not take down the worker.
		defer func() {
			if r := recover(); r != nil {
				errCh <- fmt.Errorf("session store Append panicked: %v", r)
			}
		}()
		errCh <- b.store.Append(ctx, key, entries)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// safeInvokeOnError calls the onError callback, swallowing panics so a
// misbehaving caller cannot kill the worker goroutine.
func (b *transcriptMirrorBatcher) safeInvokeOnError(key SessionKey, err error) {
	defer func() {
		if r := recover(); r != nil {
			b.warnf("[SessionStore] onError callback panicked: %v", r)
		}
	}()
	b.onError(key, err)
}

// warnf emits a diagnostic through the configured stderr callback. When no
// callback is configured the message is silently dropped — the local-disk
// transcript is still authoritative.
func (b *transcriptMirrorBatcher) warnf(format string, args ...any) {
	if b.stderr == nil {
		return
	}
	b.stderr(fmt.Sprintf(format, args...))
}

// isTimeoutErr reports whether err represents a context deadline expiry. We
// match both [context.DeadlineExceeded] and any wrapping error that surfaces
// the same sentinel via errors.Is.
func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	if err == context.DeadlineExceeded {
		return true
	}
	// Walk the chain manually to avoid an extra errors package import cost;
	// the stdlib already does this in errors.Is.
	for e := err; e != nil; {
		if e == context.DeadlineExceeded {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

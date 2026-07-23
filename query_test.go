package claude

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport is a test Transport that records calls and allows control over behavior.
type mockTransport struct {
	mu             sync.Mutex
	written        []string
	endInputCalled bool
	endInputTime   time.Time
	messages       chan map[string]any
	closed         bool
}

func newMockTransport() *mockTransport {
	return &mockTransport{
		messages: make(chan map[string]any, 100),
	}
}

func (m *mockTransport) Connect(ctx context.Context) error { return nil }

func (m *mockTransport) Write(data string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, data)
	return nil
}

func (m *mockTransport) ReadMessages(ctx context.Context) <-chan map[string]any {
	return m.messages
}

func (m *mockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	close(m.messages)
	return nil
}

func (m *mockTransport) IsReady() bool { return true }

func (m *mockTransport) EndInput() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.endInputCalled = true
	m.endInputTime = time.Now()
	return nil
}

func (m *mockTransport) getEndInputCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.endInputCalled
}

func TestWaitForResultAndEndInput_NoMcpOrHooks(t *testing.T) {
	// Without MCP servers or hooks, EndInput should be called immediately.
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})

	q.waitForResultAndEndInput()

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called")
	}
}

func TestWaitForResultAndEndInput_WithMcpServers_WaitsForResult(t *testing.T) {
	// With MCP servers configured, EndInput should wait for firstResultCh.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		mcpServers: map[string]*McpSdkServerConfig{
			"test-server": {
				Name: "test",
			},
		},
	})

	done := make(chan struct{})
	go func() {
		q.waitForResultAndEndInput()
		close(done)
	}()

	// EndInput should NOT have been called yet
	time.Sleep(50 * time.Millisecond)
	if mt.getEndInputCalled() {
		t.Fatal("EndInput should not be called before first result")
	}

	// Signal main-session result (empty origin).
	q.mainResultOnce.Do(func() { close(q.mainResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after main-session result")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after main-session result")
	}
}

func TestWaitForResultAndEndInput_WithHooks_WaitsForResult(t *testing.T) {
	// With hooks configured, EndInput should wait for firstResultCh.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{
					Matcher: "Bash",
					Hooks: []HookCallback{
						func(ctx context.Context, input HookInput, toolUseID string, hctx HookContext) (HookJSONOutput, error) {
							return nil, nil
						},
					},
				},
			},
		},
	})

	done := make(chan struct{})
	go func() {
		q.waitForResultAndEndInput()
		close(done)
	}()

	// EndInput should NOT have been called yet
	time.Sleep(50 * time.Millisecond)
	if mt.getEndInputCalled() {
		t.Fatal("EndInput should not be called before first result when hooks are configured")
	}

	// Signal main-session result (empty origin).
	q.mainResultOnce.Do(func() { close(q.mainResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after main-session result")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after main-session result")
	}
}

func TestWaitForResultAndEndInput_ContextCancellation(t *testing.T) {
	// When context is cancelled, EndInput should still be called.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		mcpServers: map[string]*McpSdkServerConfig{
			"test-server": {
				Name: "test",
			},
		},
	})

	done := make(chan struct{})
	go func() {
		q.waitForResultAndEndInput()
		close(done)
	}()

	// Cancel the query context
	q.cancelFn()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after context cancellation")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after context cancellation")
	}
}

func TestStreamInput_UsesWaitForResultAndEndInput(t *testing.T) {
	// Verify streamInput still works correctly with the refactored method.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		mcpServers: map[string]*McpSdkServerConfig{
			"test-server": {
				Name: "test",
			},
		},
	})

	inputCh := make(chan map[string]any, 1)
	inputCh <- map[string]any{"type": "user", "message": "hello"}
	close(inputCh)

	done := make(chan struct{})
	go func() {
		q.streamInput(inputCh)
		close(done)
	}()

	// Should be waiting for first result
	time.Sleep(50 * time.Millisecond)
	if mt.getEndInputCalled() {
		t.Fatal("streamInput should wait for first result before calling EndInput")
	}

	// Signal main-session result (empty origin).
	q.mainResultOnce.Do(func() { close(q.mainResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("streamInput did not complete after main-session result")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called")
	}

	// Verify the message was written
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.written) == 0 {
		t.Fatal("expected at least one message to be written")
	}
}

func TestWaitForResultAndEndInput_BackgroundAgentResultDoesNotTriggerEndInput(t *testing.T) {
	// A result with non-empty origin (background agent) must not unblock
	// waitForResultAndEndInput — only the main-session result (empty origin) should.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		mcpServers: map[string]*McpSdkServerConfig{
			"test-server": {Name: "test"},
		},
	})

	done := make(chan struct{})
	go func() {
		q.waitForResultAndEndInput()
		close(done)
	}()

	// Signal that a background-agent result arrived (non-empty origin).
	// firstResultCh closes, but mainResultCh must stay open.
	q.firstResultOnce.Do(func() { close(q.firstResultCh) })

	// waitForResultAndEndInput must still be blocked.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("waitForResultAndEndInput returned after background-agent result; should still be waiting")
	default:
	}
	if mt.getEndInputCalled() {
		t.Fatal("EndInput must not be called on background-agent result")
	}

	// Now signal the main-session result.
	q.mainResultOnce.Do(func() { close(q.mainResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after main-session result")
	}
	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after main-session result")
	}
}

// mainResultChClosed reports whether q.mainResultCh has been closed, without blocking.
func mainResultChClosed(q *query) bool {
	select {
	case <-q.mainResultCh:
		return true
	default:
		return false
	}
}

// TestReadMessages_InFlightTaskDelaysMainResultClose drains a background
// task lifecycle (task_started for a deferring task_type, then
// task_notification) through readMessages and verifies mainResultCh only
// closes on the main-session result that arrives once the task is no longer
// in flight — reproducing the fix for Python SDK #1088/#1103 (a background
// task still needing stdin for hook/SDK-MCP control responses past the
// first result frame).
func TestReadMessages_InFlightTaskDelaysMainResultClose(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()
	out := q.receiveMessages()
	drain := func(n int) {
		for i := 0; i < n; i++ {
			select {
			case <-out:
			case <-time.After(time.Second):
				t.Fatalf("timed out draining message %d/%d", i+1, n)
			}
		}
	}

	mt.messages <- map[string]any{
		"type": "system", "subtype": "task_started",
		"task_id": "t1", "task_type": "local_agent",
	}
	mt.messages <- map[string]any{"type": "result", "subtype": "success"}
	drain(2)

	time.Sleep(50 * time.Millisecond)
	if mainResultChClosed(q) {
		t.Fatal("mainResultCh must not close while task t1 is in flight")
	}

	mt.messages <- map[string]any{
		"type": "system", "subtype": "task_notification",
		"task_id": "t1", "status": "completed",
	}
	mt.messages <- map[string]any{"type": "result", "subtype": "success"}
	drain(2)

	deadline := time.After(time.Second)
	for !mainResultChClosed(q) {
		select {
		case <-deadline:
			t.Fatal("mainResultCh did not close after task t1 completed and a follow-up main-session result arrived")
		case <-time.After(time.Millisecond):
		}
	}
}

// TestReadMessages_InFlightTaskViaTaskUpdatedDelaysMainResultClose is the
// same as TestReadMessages_InFlightTaskDelaysMainResultClose, but clears the
// in-flight task via a terminal task_updated patch instead of
// task_notification (background tasks may reach a terminal state via either
// message).
func TestReadMessages_InFlightTaskViaTaskUpdatedDelaysMainResultClose(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()
	out := q.receiveMessages()
	drain := func(n int) {
		for i := 0; i < n; i++ {
			select {
			case <-out:
			case <-time.After(time.Second):
				t.Fatalf("timed out draining message %d/%d", i+1, n)
			}
		}
	}

	mt.messages <- map[string]any{
		"type": "system", "subtype": "task_started",
		"task_id": "t1", "task_type": "local_workflow",
	}
	mt.messages <- map[string]any{"type": "result", "subtype": "success"}
	drain(2)

	time.Sleep(50 * time.Millisecond)
	if mainResultChClosed(q) {
		t.Fatal("mainResultCh must not close while task t1 is in flight")
	}

	mt.messages <- map[string]any{
		"type": "system", "subtype": "task_updated",
		"task_id": "t1", "patch": map[string]any{"status": "completed"},
	}
	mt.messages <- map[string]any{"type": "result", "subtype": "success"}
	drain(2)

	deadline := time.After(time.Second)
	for !mainResultChClosed(q) {
		select {
		case <-deadline:
			t.Fatal("mainResultCh did not close after task t1 completed and a follow-up main-session result arrived")
		case <-time.After(time.Millisecond):
		}
	}
}

// autoRespondTransport extends mockTransport to automatically respond to control requests.
// It overrides ReadMessages to respect context cancellation, so query.close() doesn't deadlock.
type autoRespondTransport struct {
	mockTransport
}

func newAutoRespondTransport() *autoRespondTransport {
	return &autoRespondTransport{
		mockTransport: mockTransport{
			messages: make(chan map[string]any, 100),
		},
	}
}

func (a *autoRespondTransport) Write(data string) error {
	a.mu.Lock()
	a.written = append(a.written, data)
	a.mu.Unlock()

	// Auto-respond to control requests
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err == nil {
		if msg["type"] == "control_request" {
			reqID, _ := msg["request_id"].(string)
			go func() {
				a.messages <- map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"subtype":    "success",
						"request_id": reqID,
						"response":   map[string]any{},
					},
				}
			}()
		}
	}
	return nil
}

func (a *autoRespondTransport) ReadMessages(ctx context.Context) <-chan map[string]any {
	out := make(chan map[string]any, 100)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-a.messages:
				if !ok {
					return
				}
				out <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// TestQueryFramePeel_TranscriptMirror drives a fake transport that emits
// interleaved transcript_mirror and regular messages. The test verifies:
//   - mirror frames land in the attached SessionStore,
//   - mirror frames are NEVER yielded through receiveMessages to the caller,
//   - a result message unblocks receiveMessages as usual.
func TestQueryFramePeel_TranscriptMirror(t *testing.T) {
	mt := newMockTransport()
	store := NewInMemorySessionStore()

	projectsDir := filepath.Join(t.TempDir(), "projects")
	sessionID := "deadbeef-dead-beef-dead-beefdeadbeef"
	mirrorPath := filepath.Join(projectsDir, "my-project", sessionID+".jsonl")

	q := newQuery(queryConfig{
		transport:    mt,
		sessionStore: store,
		projectsDir:  projectsDir,
	})
	q.start()

	// Emit: regular user msg, transcript_mirror (should be peeled),
	// regular assistant msg, another transcript_mirror, result.
	frames := []map[string]any{
		{"type": "user", "uuid": "u1"},
		{
			"type":     "transcript_mirror",
			"filePath": mirrorPath,
			"entries":  []any{map[string]any{"uuid": "e1"}, map[string]any{"uuid": "e2"}},
		},
		{"type": "assistant", "uuid": "a1"},
		{
			"type":     "transcript_mirror",
			"filePath": mirrorPath,
			"entries":  []any{map[string]any{"uuid": "e3"}},
		},
		{"type": "result", "subtype": "success"},
	}
	go func() {
		for _, f := range frames {
			mt.messages <- f
		}
	}()

	// Receive messages through the query's receive channel. Collect types
	// observed; transcript_mirror must not appear.
	out := q.receiveMessages()
	var observed []string
	timeout := time.After(2 * time.Second)
collectLoop:
	for {
		select {
		case msg, ok := <-out:
			if !ok {
				break collectLoop
			}
			t, _ := msg["type"].(string)
			observed = append(observed, t)
			if t == "result" {
				break collectLoop
			}
		case <-timeout:
			t.Fatalf("timed out waiting for messages; observed so far: %v", observed)
		}
	}

	for _, obs := range observed {
		if obs == "transcript_mirror" {
			t.Fatalf("caller observed transcript_mirror frame (should be peeled): %v", observed)
		}
	}
	want := []string{"user", "assistant", "result"}
	if !stringSlicesEqual(observed, want) {
		t.Fatalf("observed = %v, want %v", observed, want)
	}

	// The store must have received all 3 entries in order.
	key := SessionKey{
		ProjectKey: "my-project",
		SessionID:  sessionID,
	}
	entries, err := store.Load(context.Background(), key)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries in store, got %d: %+v", len(entries), entries)
	}
	for i, want := range []string{"e1", "e2", "e3"} {
		if entries[i]["uuid"] != want {
			t.Errorf("entries[%d].uuid = %v, want %s", i, entries[i]["uuid"], want)
		}
	}

	// Close the transport so readMessages exits; then close the query.
	_ = mt.Close()
	_ = q.close()
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestInitialize_ExcludeDynamicSections(t *testing.T) {
	t.Run("sends excludeDynamicSections when true", func(t *testing.T) {
		mt := newAutoRespondTransport()
		q := newQuery(queryConfig{
			transport:              mt,
			excludeDynamicSections: true,
		})

		q.start()
		_, err := q.initialize()
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		// Copy written data before close (close acquires mu)
		mt.mu.Lock()
		written := make([]string, len(mt.written))
		copy(written, mt.written)
		mt.mu.Unlock()

		_ = q.close()

		found := false
		for _, w := range written {
			var msg map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(w)), &msg); err != nil {
				continue
			}
			req, _ := msg["request"].(map[string]any)
			if req != nil && req["subtype"] == "initialize" {
				if val, ok := req["excludeDynamicSections"]; ok && val == true {
					found = true
				}
			}
		}
		if !found {
			t.Error("initialize request should contain excludeDynamicSections: true")
		}
	})

	t.Run("omits excludeDynamicSections when false", func(t *testing.T) {
		mt := newAutoRespondTransport()
		q := newQuery(queryConfig{
			transport:              mt,
			excludeDynamicSections: false,
		})

		q.start()
		_, err := q.initialize()
		if err != nil {
			t.Fatalf("initialize failed: %v", err)
		}

		mt.mu.Lock()
		written := make([]string, len(mt.written))
		copy(written, mt.written)
		mt.mu.Unlock()

		_ = q.close()

		for _, w := range written {
			var msg map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(w)), &msg); err != nil {
				continue
			}
			req, _ := msg["request"].(map[string]any)
			if req != nil && req["subtype"] == "initialize" {
				if _, ok := req["excludeDynamicSections"]; ok {
					t.Error("initialize request should not contain excludeDynamicSections when false")
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// ProcessError suppression branch (#204 follow-up for #174).
// query.readMessages must NOT generate a ProcessError when an is_error result
// was already delivered, but MUST generate one if the subprocess exits with
// no prior is_error result.
// ---------------------------------------------------------------------------

func TestReadMessages_ProcessErrorSuppressedAfterIsErrorResult(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()

	frames := []map[string]any{
		// is_error result flips the suppression flag.
		{"type": "result", "subtype": "error", "is_error": true, "session_id": "s"},
		// Exit-error frame that follows must be suppressed.
		{"type": "error", "error": "exit 1"},
	}
	go func() {
		for _, f := range frames {
			mt.messages <- f
		}
		_ = mt.Close()
	}()

	// Drain the message channel until it closes.
	out := q.receiveMessages()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				goto done
			}
		case <-timeout:
			t.Fatal("timed out waiting for message channel to close")
		}
	}
done:

	if q.processError != nil {
		t.Errorf("processError should be suppressed after is_error result, got: %v", q.processError)
	}
	_ = q.close()
}

func TestReadMessages_ProcessErrorSurfacedWhenNoIsErrorResult(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()

	frames := []map[string]any{
		// A normal (non-error) result first.
		{"type": "result", "subtype": "success", "is_error": false, "session_id": "s"},
		// Then the subprocess exits with an error frame: no prior is_error,
		// so processError must capture it.
		{"type": "error", "error": "exec failed: signal: killed"},
	}
	go func() {
		for _, f := range frames {
			mt.messages <- f
		}
		_ = mt.Close()
	}()

	out := q.receiveMessages()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				goto done
			}
		case <-timeout:
			t.Fatal("timed out waiting for message channel to close")
		}
	}
done:

	if q.processError == nil {
		t.Fatal("processError should be set when no is_error result preceded the exit error")
	}
	if !strings.Contains(q.processError.Error(), "exec failed") {
		t.Errorf("processError.Error() = %q, want it to include 'exec failed'", q.processError.Error())
	}
	if _, ok := q.processError.(*ProcessError); !ok {
		t.Errorf("processError should be *ProcessError, got %T", q.processError)
	}
	_ = q.close()
}

// ---------------------------------------------------------------------------
// parsePermissionUpdate coverage (#204 follow-up for #176): exercise each
// PermissionUpdateType branch so the typed parsing path stays correct.
// ---------------------------------------------------------------------------

func TestParsePermissionUpdate_AddRules(t *testing.T) {
	in := map[string]any{
		"type":     "addRules",
		"behavior": "allow",
		"rules": []any{
			map[string]any{"toolName": "Bash", "ruleContent": "ls *"},
			map[string]any{"toolName": "Read"},
		},
		"destination": "session",
	}
	p := parsePermissionUpdate(in)
	if p.Type != PermissionUpdateAddRules {
		t.Errorf("Type = %q", p.Type)
	}
	if p.Behavior != "allow" {
		t.Errorf("Behavior = %q", p.Behavior)
	}
	if p.Destination != PermissionUpdateDestSession {
		t.Errorf("Destination = %q", p.Destination)
	}
	if len(p.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(p.Rules))
	}
	if p.Rules[0].ToolName != "Bash" || p.Rules[0].RuleContent != "ls *" {
		t.Errorf("Rules[0] = %+v", p.Rules[0])
	}
	if p.Rules[1].ToolName != "Read" || p.Rules[1].RuleContent != "" {
		t.Errorf("Rules[1] = %+v", p.Rules[1])
	}
}

func TestParsePermissionUpdate_SetMode(t *testing.T) {
	in := map[string]any{
		"type": "setMode",
		"mode": "acceptEdits",
	}
	p := parsePermissionUpdate(in)
	if p.Type != PermissionUpdateSetMode {
		t.Errorf("Type = %q", p.Type)
	}
	if p.Mode != PermissionMode("acceptEdits") {
		t.Errorf("Mode = %q", p.Mode)
	}
}

func TestParsePermissionUpdate_AddDirectories(t *testing.T) {
	in := map[string]any{
		"type":        "addDirectories",
		"directories": []any{"/a", "/b", "/c"},
	}
	p := parsePermissionUpdate(in)
	if p.Type != PermissionUpdateAddDirectories {
		t.Errorf("Type = %q", p.Type)
	}
	if len(p.Directories) != 3 || p.Directories[0] != "/a" || p.Directories[2] != "/c" {
		t.Errorf("Directories = %v", p.Directories)
	}
}

func TestParsePermissionUpdate_IgnoresNonStringDirectoryEntries(t *testing.T) {
	// Defensive: directories array could contain mixed types under a buggy CLI.
	in := map[string]any{
		"type":        "addDirectories",
		"directories": []any{"/a", 42, "/c"},
	}
	p := parsePermissionUpdate(in)
	if len(p.Directories) != 2 {
		t.Errorf("expected 2 valid string directories, got %v", p.Directories)
	}
}

func TestParsePermissionUpdate_IgnoresNonMapRuleEntries(t *testing.T) {
	in := map[string]any{
		"type": "addRules",
		"rules": []any{
			map[string]any{"toolName": "Bash"},
			"not-a-map", // should be skipped without panic.
		},
	}
	p := parsePermissionUpdate(in)
	if len(p.Rules) != 1 {
		t.Errorf("expected 1 valid rule, got %v", p.Rules)
	}
}

func TestWarmQuery_Close_Idempotent(t *testing.T) {
	// Verify WarmQuery.Close does not hang or panic when no query has been sent.
	// Uses autoRespondTransport whose ReadMessages respects context cancellation
	// so that query.close() can unblock the readMessages goroutine via cancelFn.
	mt := newAutoRespondTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()
	wq := &WarmQuery{transport: mt, q: q}
	done := make(chan struct{})
	go func() {
		wq.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("WarmQuery.Close did not return within 5 seconds")
	}
}

func TestReadMessages_ResultForwardedBeforeFirstResultSignal(t *testing.T) {
	// Verifies that q.firstResultCh is closed AFTER the result message is
	// put on messageCh, so waitForResultAndEndInput cannot call EndInput
	// before block-feedback messages are accessible to consumers.
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		hooks: map[HookEvent][]HookMatcher{
			HookEventUserPromptSubmit: {
				{
					Matcher: "",
					Hooks: []HookCallback{
						func(ctx context.Context, input HookInput, toolUseID string, hctx HookContext) (HookJSONOutput, error) {
							return HookJSONOutput{"decision": "block", "reason": "blocked"}, nil
						},
					},
				},
			},
		},
	})
	q.start()

	// Feed a result message (simulating the CLI's block-feedback result)
	resultMsg := map[string]any{
		"type":     "result",
		"is_error": true,
	}

	// Drain messageCh in the background so sends don't block.
	// Capture whether a "result" message arrived before firstResultCh closes.
	resultInCh := make(chan struct{})
	go func() {
		for msg := range q.messageCh {
			if tp, _ := msg["type"].(string); tp == "result" {
				// Signal that the result reached the consumer channel.
				select {
				case <-resultInCh:
				default:
					close(resultInCh)
				}
			}
		}
	}()

	// Inject the result message, then close the transport so readMessages exits.
	mt.messages <- resultMsg
	_ = mt.Close()

	// The result should appear in messageCh before we check firstResultCh.
	select {
	case <-resultInCh:
		// good - message reached consumer
	case <-time.After(time.Second):
		t.Fatal("result message did not reach messageCh within 1s")
	}

	// firstResultCh should be closed now (signal comes after message forwarding).
	select {
	case <-q.firstResultCh:
		// good
	case <-time.After(time.Second):
		t.Fatal("firstResultCh not closed after result message was forwarded")
	}

	_ = q.close()
}

func TestHandleCanUseTool_PopulatesRequestID(t *testing.T) {
	mt := newMockTransport()
	var gotRequestID string
	q := newQuery(queryConfig{
		transport: mt,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			gotRequestID = permCtx.RequestID
			return PermissionResultAllow{}, nil
		},
	})

	q.handleControlRequest(map[string]any{
		"request_id": "req-123",
		"request": map[string]any{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"tool_use_id": "tu-1",
			"input":       map[string]any{},
		},
	})

	if gotRequestID != "req-123" {
		t.Fatalf("expected RequestID %q, got %q", "req-123", gotRequestID)
	}

	written := mt.written
	if len(written) != 1 {
		t.Fatalf("expected exactly one control_response write, got %d", len(written))
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(written[0]), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	inner, _ := resp["response"].(map[string]any)
	if inner["subtype"] != "success" {
		t.Fatalf("expected success response, got %v", resp)
	}
}

func TestHandleCanUseTool_PopulatesMatchedAskRuleAndSuppressAlwaysAllowRule(t *testing.T) {
	mt := newMockTransport()
	var gotCtx ToolPermissionContext
	q := newQuery(queryConfig{
		transport: mt,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			gotCtx = permCtx
			return PermissionResultAllow{}, nil
		},
	})

	q.handleControlRequest(map[string]any{
		"request_id": "req-789",
		"request": map[string]any{
			"subtype":                    "can_use_tool",
			"tool_name":                  "Bash",
			"tool_use_id":                "tu-3",
			"input":                      map[string]any{},
			"suppress_always_allow_rule": true,
			"matched_ask_rule": map[string]any{
				"source":       "/home/user/.claude/settings.json",
				"tool_name":    "Bash",
				"rule_content": "Bash(rm:*)",
			},
		},
	})

	if !gotCtx.SuppressAlwaysAllowRule {
		t.Error("expected SuppressAlwaysAllowRule to be true")
	}
	if gotCtx.MatchedAskRule == nil {
		t.Fatal("expected MatchedAskRule to be non-nil")
	}
	if gotCtx.MatchedAskRule.Source != "/home/user/.claude/settings.json" {
		t.Errorf("MatchedAskRule.Source = %q, want %q", gotCtx.MatchedAskRule.Source, "/home/user/.claude/settings.json")
	}
	if gotCtx.MatchedAskRule.ToolName != "Bash" {
		t.Errorf("MatchedAskRule.ToolName = %q, want %q", gotCtx.MatchedAskRule.ToolName, "Bash")
	}
	if gotCtx.MatchedAskRule.RuleContent != "Bash(rm:*)" {
		t.Errorf("MatchedAskRule.RuleContent = %q, want %q", gotCtx.MatchedAskRule.RuleContent, "Bash(rm:*)")
	}
}

func TestHandleCanUseTool_NilResultSuppressesControlResponse(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{
		transport: mt,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			// Simulate a consumer that already answered out-of-band.
			return nil, nil
		},
	})

	q.handleControlRequest(map[string]any{
		"request_id": "req-456",
		"request": map[string]any{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"tool_use_id": "tu-2",
			"input":       map[string]any{},
		},
	})

	if len(mt.written) != 0 {
		t.Fatalf("expected no control_response to be written, got %v", mt.written)
	}
}

// TestHandleControlRequest_DuplicateInFlightRequestIsIgnored guards against a
// regression where a control_request redelivered while the original is still
// being handled (e.g. a redelivered pending_permission_requests entry, or a
// transport-level redelivery) would invoke the permission callback a second
// time and write a second control_response for the same request_id. Port of
// TypeScript SDK v0.3.196.
func TestHandleControlRequest_DuplicateInFlightRequestIsIgnored(t *testing.T) {
	mt := newMockTransport()
	var callCount int32
	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	q := newQuery(queryConfig{
		transport: mt,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			atomic.AddInt32(&callCount, 1)
			close(callbackEntered)
			<-releaseCallback
			return PermissionResultAllow{}, nil
		},
	})

	msg := map[string]any{
		"request_id": "req-dup-1",
		"request": map[string]any{
			"subtype":     "can_use_tool",
			"tool_name":   "Bash",
			"tool_use_id": "tu-dup-1",
			"input":       map[string]any{},
		},
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		q.handleControlRequest(msg)
	}()

	select {
	case <-callbackEntered:
	case <-time.After(time.Second):
		t.Fatal("callback was never invoked")
	}

	// Redeliver the same request_id while the first call is still in flight.
	// This must return immediately without invoking the callback again.
	q.handleControlRequest(msg)

	close(releaseCallback)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("first handleControlRequest call never returned")
	}

	if got := atomic.LoadInt32(&callCount); got != 1 {
		t.Fatalf("expected canUseTool to be invoked exactly once, got %d", got)
	}
	if len(mt.written) != 1 {
		t.Fatalf("expected exactly one control_response write, got %d: %v", len(mt.written), mt.written)
	}
}

// interruptRespondTransport auto-responds to control requests like
// autoRespondTransport, but replies to "interrupt" requests with a
// configurable response body so tests can simulate both the
// interrupt_receipt_v1 payload and older CLIs that omit it.
type interruptRespondTransport struct {
	mockTransport
	interruptResponse map[string]any
}

func newInterruptRespondTransport(interruptResponse map[string]any) *interruptRespondTransport {
	return &interruptRespondTransport{
		mockTransport:     mockTransport{messages: make(chan map[string]any, 100)},
		interruptResponse: interruptResponse,
	}
}

func (a *interruptRespondTransport) Write(data string) error {
	a.mu.Lock()
	a.written = append(a.written, data)
	a.mu.Unlock()

	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err == nil {
		if msg["type"] == "control_request" {
			reqID, _ := msg["request_id"].(string)
			response := map[string]any{}
			if req, ok := msg["request"].(map[string]any); ok && req["subtype"] == "interrupt" {
				response = a.interruptResponse
			}
			go func() {
				a.messages <- map[string]any{
					"type": "control_response",
					"response": map[string]any{
						"subtype":    "success",
						"request_id": reqID,
						"response":   response,
					},
				}
			}()
		}
	}
	return nil
}

func (a *interruptRespondTransport) ReadMessages(ctx context.Context) <-chan map[string]any {
	out := make(chan map[string]any, 100)
	go func() {
		defer close(out)
		for {
			select {
			case msg, ok := <-a.messages:
				if !ok {
					return
				}
				out <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// TestInterrupt_PopulatesStillQueued verifies that query.interrupt surfaces
// the still_queued uuids from a CLI advertising interrupt_receipt_v1. Port
// of TypeScript SDK v0.3.205.
func TestInterrupt_PopulatesStillQueued(t *testing.T) {
	mt := newInterruptRespondTransport(map[string]any{
		"still_queued": []any{"uuid-1", "uuid-2"},
	})
	q := newQuery(queryConfig{transport: mt})
	q.start()
	defer func() { _ = q.close() }()

	receipt, err := q.interrupt(context.Background())
	if err != nil {
		t.Fatalf("interrupt failed: %v", err)
	}
	if receipt == nil {
		t.Fatal("expected non-nil receipt")
	}
	want := []string{"uuid-1", "uuid-2"}
	if len(receipt.StillQueued) != len(want) {
		t.Fatalf("StillQueued = %v, want %v", receipt.StillQueued, want)
	}
	for i, s := range want {
		if receipt.StillQueued[i] != s {
			t.Fatalf("StillQueued[%d] = %q, want %q", i, receipt.StillQueued[i], s)
		}
	}
}

// TestInterrupt_OlderCLIOmitsStillQueued verifies that an empty success
// response (older CLIs without interrupt_receipt_v1) yields a non-nil
// receipt with a nil StillQueued, rather than an error.
func TestInterrupt_OlderCLIOmitsStillQueued(t *testing.T) {
	mt := newInterruptRespondTransport(map[string]any{})
	q := newQuery(queryConfig{transport: mt})
	q.start()
	defer func() { _ = q.close() }()

	receipt, err := q.interrupt(context.Background())
	if err != nil {
		t.Fatalf("interrupt failed: %v", err)
	}
	if receipt == nil {
		t.Fatal("expected non-nil receipt")
	}
	if receipt.StillQueued != nil {
		t.Fatalf("StillQueued = %v, want nil", receipt.StillQueued)
	}
}

// TestHandleHookCallback_TimesOutHungCallback verifies that a hook callback
// which never returns is bounded by its configured per-callback timeout
// instead of wedging the control request forever.
func TestHandleHookCallback_TimesOutHungCallback(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})

	observedCancel := make(chan struct{})
	q.hookCallbacks["hook_0"] = func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
		<-ctx.Done()
		close(observedCancel)
		return nil, ctx.Err()
	}
	q.hookCallbackTimeouts["hook_0"] = 20 * time.Millisecond

	_, err := q.handleHookCallback(map[string]any{
		"callback_id": "hook_0",
		"input":       map[string]any{},
		"tool_use_id": "tu-1",
	})
	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error message, got: %v", err)
	}

	select {
	case <-observedCancel:
	case <-time.After(time.Second):
		t.Fatal("callback goroutine never observed its context being cancelled")
	}
}

// TestHandleHookCallback_DefaultTimeoutWhenUnset verifies that a callback ID
// with no recorded timeout (e.g. registered without going through
// initialize()) still gets the documented 60s default rather than blocking
// forever, by using a fast-returning callback and asserting success.
func TestHandleHookCallback_DefaultTimeoutWhenUnset(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})

	q.hookCallbacks["hook_0"] = func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
		return HookJSONOutput{"decision": "approve"}, nil
	}
	// Intentionally not populating q.hookCallbackTimeouts["hook_0"].

	resp, err := q.handleHookCallback(map[string]any{
		"callback_id": "hook_0",
		"input":       map[string]any{},
		"tool_use_id": "tu-1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp["decision"] != "approve" {
		t.Fatalf("expected decision=approve, got: %v", resp)
	}
}

// TestHandleHookCallback_SessionCancellationPropagates verifies that when the
// query's own context is cancelled (not just the per-callback timeout), the
// returned error reflects context cancellation rather than a misleading
// "timed out" message.
func TestHandleHookCallback_SessionCancellationPropagates(t *testing.T) {
	mt := newMockTransport()
	q := newQuery(queryConfig{transport: mt})

	started := make(chan struct{})
	q.hookCallbacks["hook_0"] = func(ctx context.Context, input HookInput, toolUseID string, hookCtx HookContext) (HookJSONOutput, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	q.hookCallbackTimeouts["hook_0"] = time.Minute

	done := make(chan error, 1)
	go func() {
		_, err := q.handleHookCallback(map[string]any{
			"callback_id": "hook_0",
			"input":       map[string]any{},
			"tool_use_id": "tu-1",
		})
		done <- err
	}()

	<-started
	q.cancelFn()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected an error from session cancellation")
		}
		if strings.Contains(err.Error(), "timed out") {
			t.Fatalf("expected session-cancellation error, got timeout error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handleHookCallback never returned after session cancellation")
	}
}

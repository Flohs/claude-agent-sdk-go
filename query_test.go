package claude

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
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

	// Signal first result
	q.firstResultOnce.Do(func() { close(q.firstResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after first result")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after first result")
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

	// Signal first result
	q.firstResultOnce.Do(func() { close(q.firstResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("waitForResultAndEndInput did not return after first result")
	}

	if !mt.getEndInputCalled() {
		t.Fatal("expected EndInput to be called after first result")
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

	// Signal first result
	q.firstResultOnce.Do(func() { close(q.firstResultCh) })

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("streamInput did not complete after first result")
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

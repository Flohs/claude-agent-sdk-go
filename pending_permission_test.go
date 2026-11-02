package claude

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// pendingRespondTransport answers every control request with a success
// response whose outer control response (or, with inner set, the "response"
// payload) carries extra fields, such as pending_permission_requests.
type pendingRespondTransport struct {
	autoRespondTransport
	extra map[string]any
	inner bool
}

func (p *pendingRespondTransport) Write(data string) error {
	p.mu.Lock()
	p.written = append(p.written, data)
	p.mu.Unlock()

	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err == nil && msg["type"] == "control_request" {
		reqID, _ := msg["request_id"].(string)
		payload := map[string]any{}
		resp := map[string]any{
			"subtype":    "success",
			"request_id": reqID,
			"response":   payload,
		}
		target := resp
		if p.inner {
			target = payload
		}
		for k, v := range p.extra {
			target[k] = v
		}
		go func() {
			p.messages <- map[string]any{"type": "control_response", "response": resp}
		}()
	}
	return nil
}

func (p *pendingRespondTransport) controlResponses() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []map[string]any
	for _, w := range p.written {
		var msg map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(w)), &msg); err == nil && msg["type"] == "control_response" {
			resp, _ := msg["response"].(map[string]any)
			out = append(out, resp)
		}
	}
	return out
}

func pendingCanUseToolFrame(requestID, toolName string) map[string]any {
	return map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request": map[string]any{
			"subtype":     "can_use_tool",
			"tool_name":   toolName,
			"tool_use_id": "tu-" + requestID,
			"input":       map[string]any{},
		},
	}
}

// runPendingInitialize initializes a query against p and returns the tool
// names CanUseTool saw and the control responses the SDK wrote, once
// wantCalls callbacks have run (or after a short grace period when
// wantCalls is 0).
func runPendingInitialize(t *testing.T, p *pendingRespondTransport, wantCalls int) ([]string, []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var tools []string
	called := make(chan struct{}, 10)
	q := newQuery(queryConfig{
		transport: p,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			mu.Lock()
			tools = append(tools, toolName)
			mu.Unlock()
			called <- struct{}{}
			return PermissionResultAllow{}, nil
		},
	})
	q.start()
	if _, err := q.initialize(); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	for i := 0; i < wantCalls; i++ {
		select {
		case <-called:
		case <-time.After(2 * time.Second):
			t.Fatalf("CanUseTool called %d times, want %d", i, wantCalls)
		}
	}
	if wantCalls == 0 {
		select {
		case <-called:
			t.Fatal("CanUseTool called, want no calls")
		case <-time.After(100 * time.Millisecond):
		}
	}
	_ = q.close()
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), tools...), p.controlResponses()
}

// TestInitialize_PendingPermissionRequests_OuterFrames covers the shape the
// CLI sends: pending_permission_requests on the control response itself,
// each entry a whole control_request frame. Each can_use_tool entry must
// reach CanUseTool and be answered under its original request_id; other
// subtypes are ignored.
func TestInitialize_PendingPermissionRequests_OuterFrames(t *testing.T) {
	p := &pendingRespondTransport{
		autoRespondTransport: autoRespondTransport{mockTransport: mockTransport{messages: make(chan map[string]any, 100)}},
		extra: map[string]any{
			"pending_permission_requests": []any{
				pendingCanUseToolFrame("cli-req-7", "Bash"),
				map[string]any{
					"type":       "control_request",
					"request_id": "cli-req-8",
					"request":    map[string]any{"subtype": "request_user_dialog"},
				},
			},
			"pending_elicitation_requests": []any{
				map[string]any{
					"type":       "control_request",
					"request_id": "cli-req-9",
					"request":    map[string]any{"subtype": "elicitation", "mcp_server_name": "s", "message": "?"},
				},
			},
		},
	}

	tools, responses := runPendingInitialize(t, p, 1)
	if len(tools) != 1 || tools[0] != "Bash" {
		t.Fatalf("CanUseTool saw %v, want [Bash]", tools)
	}
	var ids []string
	for _, r := range responses {
		id, _ := r["request_id"].(string)
		ids = append(ids, id)
	}
	if len(ids) != 1 || ids[0] != "cli-req-7" {
		t.Fatalf("control_response request_ids = %v, want [cli-req-7]", ids)
	}
	if responses[0]["subtype"] != "success" {
		t.Fatalf("control_response = %v, want success", responses[0])
	}
}

// TestInitialize_PendingPermissionRequests_InnerBareRequests keeps the shape
// the SDK accepted before: the list inside the "response" payload, entries
// being bare requests.
func TestInitialize_PendingPermissionRequests_InnerBareRequests(t *testing.T) {
	p := &pendingRespondTransport{
		autoRespondTransport: autoRespondTransport{mockTransport: mockTransport{messages: make(chan map[string]any, 100)}},
		inner:                true,
		extra: map[string]any{
			"pending_permission_requests": []any{
				map[string]any{
					"subtype":     "can_use_tool",
					"tool_name":   "Write",
					"tool_use_id": "tu-1",
					"input":       map[string]any{},
				},
			},
		},
	}

	tools, _ := runPendingInitialize(t, p, 1)
	if len(tools) != 1 || tools[0] != "Write" {
		t.Fatalf("CanUseTool saw %v, want [Write]", tools)
	}
}

// TestSendControlRequest_PendingPermissionRequestsIgnoredOffInitialize checks
// that only the initialize response redelivers prompts, as in the
// TypeScript SDK.
func TestSendControlRequest_PendingPermissionRequestsIgnoredOffInitialize(t *testing.T) {
	p := &pendingRespondTransport{
		autoRespondTransport: autoRespondTransport{mockTransport: mockTransport{messages: make(chan map[string]any, 100)}},
		extra: map[string]any{
			"pending_permission_requests": []any{pendingCanUseToolFrame("cli-req-1", "Bash")},
		},
	}
	called := make(chan struct{}, 1)
	q := newQuery(queryConfig{
		transport: p,
		canUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			called <- struct{}{}
			return PermissionResultAllow{}, nil
		},
	})
	q.start()
	defer func() { _ = q.close() }()

	if _, err := q.sendControlRequest(map[string]any{"subtype": "mcp_status"}, time.Second); err != nil {
		t.Fatalf("sendControlRequest failed: %v", err)
	}
	select {
	case <-called:
		t.Fatal("CanUseTool called from a non-initialize response")
	case <-time.After(100 * time.Millisecond):
	}
}

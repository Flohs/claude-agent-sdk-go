package claude

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// setPermissionModeRespondTransport answers every control request with
// success, replying to set_permission_mode with a configurable payload.
type setPermissionModeRespondTransport struct {
	*interruptRespondTransport
	setModeResponse map[string]any
}

func newSetPermissionModeRespondTransport(resp map[string]any) *setPermissionModeRespondTransport {
	return &setPermissionModeRespondTransport{
		interruptRespondTransport: newInterruptRespondTransport(map[string]any{}),
		setModeResponse:           resp,
	}
}

func (a *setPermissionModeRespondTransport) Write(data string) error {
	a.mu.Lock()
	a.written = append(a.written, data)
	a.mu.Unlock()

	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err == nil && msg["type"] == "control_request" {
		reqID, _ := msg["request_id"].(string)
		response := map[string]any{}
		if req, ok := msg["request"].(map[string]any); ok && req["subtype"] == "set_permission_mode" {
			response = a.setModeResponse
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
	return nil
}

func TestSetPermissionModeWithResult(t *testing.T) {
	tests := []struct {
		name string
		resp map[string]any
		want PermissionMode // "" means a nil response
	}{
		{"mode differs from request", map[string]any{"mode": "default"}, PermissionModeDefault},
		{"same mode", map[string]any{"mode": "acceptEdits"}, PermissionModeAcceptEdits},
		{"manual reported as default", map[string]any{"mode": "manual"}, PermissionModeDefault},
		{"no payload", map[string]any{}, ""},
		{"unknown mode", map[string]any{"mode": "yolo"}, ""},
		{"empty mode", map[string]any{"mode": ""}, ""},
		{"non-string mode", map[string]any{"mode": float64(1)}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mt := newSetPermissionModeRespondTransport(tt.resp)
			q := newQuery(queryConfig{transport: mt})
			q.start()
			defer func() { _ = q.close() }()
			c := &Client{q: q}

			got, err := c.SetPermissionModeWithResult(context.Background(), "acceptEdits")
			if err != nil {
				t.Fatalf("SetPermissionModeWithResult failed: %v", err)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("response = %+v, want nil", got)
				}
				return
			}
			if got == nil || got.Mode != tt.want {
				t.Fatalf("response = %+v, want Mode %q", got, tt.want)
			}

			mt.mu.Lock()
			written := append([]string(nil), mt.written...)
			mt.mu.Unlock()
			found := false
			for _, w := range written {
				if strings.Contains(w, `"subtype":"set_permission_mode"`) && strings.Contains(w, `"mode":"acceptEdits"`) {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected a set_permission_mode request for acceptEdits, got %v", written)
			}
		})
	}
}

func TestSetPermissionMode_IgnoresResponse(t *testing.T) {
	mt := newSetPermissionModeRespondTransport(map[string]any{"mode": "plan"})
	q := newQuery(queryConfig{transport: mt})
	q.start()
	defer func() { _ = q.close() }()
	c := &Client{q: q}

	if err := c.SetPermissionMode(context.Background(), "acceptEdits"); err != nil {
		t.Fatalf("SetPermissionMode failed: %v", err)
	}
}

func TestSetPermissionModeWithResult_NotConnected(t *testing.T) {
	c := &Client{}
	got, err := c.SetPermissionModeWithResult(context.Background(), "plan")
	if got != nil {
		t.Errorf("response = %+v, want nil", got)
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

func TestSetPermissionModeWithResult_RejectsInvalidMode(t *testing.T) {
	mt := newMockTransport()
	c := &Client{q: newQuery(queryConfig{transport: mt})}
	if _, err := c.SetPermissionModeWithResult(context.Background(), "acceptEdit"); err == nil {
		t.Fatal("expected error for invalid PermissionMode, got nil")
	}
	mt.mu.Lock()
	defer mt.mu.Unlock()
	if len(mt.written) != 0 {
		t.Errorf("expected no control request to be written, got %v", mt.written)
	}
}

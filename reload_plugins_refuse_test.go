package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// errorRespondTransport answers every control request with an error
// control response carrying errText and errCode.
type errorRespondTransport struct {
	mockTransport
	errText string
	errCode any
}

func (e *errorRespondTransport) Write(data string) error {
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &msg); err == nil && msg["type"] == "control_request" {
		reqID, _ := msg["request_id"].(string)
		resp := map[string]any{"subtype": "error", "request_id": reqID, "error": e.errText}
		if e.errCode != nil {
			resp["error_code"] = e.errCode
		}
		go func() { e.messages <- map[string]any{"type": "control_response", "response": resp} }()
	}
	return nil
}

func (e *errorRespondTransport) ReadMessages(ctx context.Context) <-chan map[string]any {
	out := make(chan map[string]any, 100)
	go func() {
		defer close(out)
		for {
			select {
			case msg := <-e.messages:
				out <- msg
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// TestReloadPluginsWithConfig_RefuseIfBusySendsKey verifies refuse_if_busy is
// sent only when RefuseIfBusy is set, alongside the other options. Port of
// TypeScript SDK v0.3.324.
func TestReloadPluginsWithConfig_RefuseIfBusySendsKey(t *testing.T) {
	mt := newAutoRespondTransport()
	q := newQuery(queryConfig{transport: mt})
	q.start()
	defer func() { _ = q.close() }()

	if _, err := q.reloadPluginsWithConfig(ReloadPluginsOptions{RefuseIfBusy: true, HoldOnCacheImpact: true}); err != nil {
		t.Fatalf("reloadPluginsWithConfig failed: %v", err)
	}
	if _, err := q.reloadPluginsWithConfig(ReloadPluginsOptions{}); err != nil {
		t.Fatalf("reloadPluginsWithConfig failed: %v", err)
	}

	mt.mu.Lock()
	written := append([]string(nil), mt.written...)
	mt.mu.Unlock()
	var reqs []map[string]any
	for _, w := range written {
		var msg map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(w)), &msg) != nil {
			continue
		}
		if r, _ := msg["request"].(map[string]any); r["subtype"] == "reload_plugins" {
			reqs = append(reqs, r)
		}
	}
	if len(reqs) != 2 {
		t.Fatalf("got %d reload_plugins requests, want 2: %v", len(reqs), written)
	}
	if reqs[0]["refuse_if_busy"] != true || reqs[0]["hold_on_cache_impact"] != true {
		t.Errorf("first request = %v, want refuse_if_busy and hold_on_cache_impact true", reqs[0])
	}
	if _, ok := reqs[1]["refuse_if_busy"]; ok {
		t.Errorf("second request = %v, want no refuse_if_busy key", reqs[1])
	}
}

// TestControlRequestError_CarriesErrorCode verifies a busy refusal surfaces
// as a *ControlRequestError with ReloadPluginsBusyErrorCode and the CLI's
// text as Error().
func TestControlRequestError_CarriesErrorCode(t *testing.T) {
	mt := &errorRespondTransport{
		mockTransport: mockTransport{messages: make(chan map[string]any, 100)},
		errText:       "session is busy",
		errCode:       ReloadPluginsBusyErrorCode,
	}
	q := newQuery(queryConfig{transport: mt})
	q.start()
	defer func() { _ = q.close() }()

	_, err := q.reloadPluginsWithConfig(ReloadPluginsOptions{RefuseIfBusy: true})
	var ce *ControlRequestError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %T %v, want *ControlRequestError", err, err)
	}
	if ce.ErrorCode != ReloadPluginsBusyErrorCode {
		t.Errorf("ErrorCode = %q, want %q", ce.ErrorCode, ReloadPluginsBusyErrorCode)
	}
	if err.Error() != "session is busy" {
		t.Errorf("Error() = %q, want the CLI's text", err.Error())
	}
}

// TestNewControlRequestError_DropsMalformedCodes verifies error_code values
// that are not lowercase identifiers of at most 64 characters are dropped.
func TestNewControlRequestError_DropsMalformedCodes(t *testing.T) {
	cases := []struct {
		code any
		want string
	}{
		{nil, ""},
		{"reload_plugins_busy", "reload_plugins_busy"},
		{"Upper", ""},
		{"1abc", ""},
		{"has-dash", ""},
		{"a" + strings.Repeat("b", 63), "a" + strings.Repeat("b", 63)},
		{"a" + strings.Repeat("b", 64), ""},
		{42, ""},
	}
	for _, c := range cases {
		resp := map[string]any{"error": "x"}
		if c.code != nil {
			resp["error_code"] = c.code
		}
		if got := newControlRequestError(resp).ErrorCode; got != c.want {
			t.Errorf("code %v: ErrorCode = %q, want %q", c.code, got, c.want)
		}
	}
}

// TestReloadPluginsWithConfig_NotConnected verifies the method refuses
// before Connect.
func TestReloadPluginsWithConfig_NotConnected(t *testing.T) {
	c := NewClient(nil)
	_, err := c.ReloadPluginsWithConfig(context.Background(), ReloadPluginsOptions{})
	var ce *ConnectionError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want *ConnectionError", err)
	}
}

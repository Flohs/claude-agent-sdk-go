package claude

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func noopHook(context.Context, HookInput, string, HookContext) (HookJSONOutput, error) {
	return HookJSONOutput{}, nil
}

// runHookOnFailureInitialize runs one initialize against a transport whose
// response payload carries extra, and returns the initialize request the SDK
// sent and the error.
func runHookOnFailureInitialize(t *testing.T, onFailure HookOnFailure, extra map[string]any) (map[string]any, error) {
	t.Helper()
	p := &pendingRespondTransport{
		autoRespondTransport: autoRespondTransport{mockTransport: mockTransport{messages: make(chan map[string]any, 100)}},
		inner:                true,
		extra:                extra,
	}
	q := newQuery(queryConfig{
		transport: p,
		hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {{Matcher: "Bash", Hooks: []HookCallback{noopHook}, OnFailure: onFailure}},
		},
	})
	q.start()
	_, err := q.initialize()

	p.mu.Lock()
	written := append([]string(nil), p.written...)
	p.mu.Unlock()
	_ = q.close()

	for _, w := range written {
		var msg map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(w)), &msg) != nil {
			continue
		}
		if req, _ := msg["request"].(map[string]any); req != nil && req["subtype"] == "initialize" {
			return req, err
		}
	}
	t.Fatal("no initialize request written")
	return nil, nil
}

func preToolUseMatcher(t *testing.T, req map[string]any) map[string]any {
	t.Helper()
	hooks, _ := req["hooks"].(map[string]any)
	matchers, _ := hooks["PreToolUse"].([]any)
	if len(matchers) != 1 {
		t.Fatalf("PreToolUse matchers = %v, want one", hooks["PreToolUse"])
	}
	m, _ := matchers[0].(map[string]any)
	return m
}

func TestHookOnFailure_SentOnMatcher(t *testing.T) {
	req, err := runHookOnFailureInitialize(t, HookOnFailureBlock, map[string]any{"hook_on_failure_supported": true})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if got := preToolUseMatcher(t, req)["onFailure"]; got != "block" {
		t.Errorf("onFailure = %v, want block", got)
	}
}

func TestHookOnFailure_OmittedWhenUnset(t *testing.T) {
	req, err := runHookOnFailureInitialize(t, "", nil)
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	if got, ok := preToolUseMatcher(t, req)["onFailure"]; ok {
		t.Errorf("onFailure = %v, want absent", got)
	}
}

func TestHookOnFailure_InitializeChecks(t *testing.T) {
	cases := []struct {
		name      string
		onFailure HookOnFailure
		extra     map[string]any
		wantErr   string
	}{
		{"block on old CLI", HookOnFailureBlock, nil, "does not honor OnFailure"},
		{"block with false support", HookOnFailureBlock, map[string]any{"hook_on_failure_supported": false}, "does not honor OnFailure"},
		{"unknown value counts as block", "deny", nil, "does not honor OnFailure"},
		{"block with hooks not applied", HookOnFailureBlock, map[string]any{"hook_on_failure_supported": true, "hooks_applied": false}, "already running"},
		{"block supported and applied", HookOnFailureBlock, map[string]any{"hook_on_failure_supported": true, "hooks_applied": true}, ""},
		{"block supported, hooks_applied absent", HookOnFailureBlock, map[string]any{"hook_on_failure_supported": true}, ""},
		{"continue on old CLI", HookOnFailureContinue, nil, ""},
		{"continue with hooks not applied", HookOnFailureContinue, map[string]any{"hooks_applied": false}, ""},
		{"unset on old CLI", "", nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runHookOnFailureInitialize(t, tc.onFailure, tc.extra)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("initialize: %v", err)
				}
				return
			}
			var ce *ConnectionError
			if !errors.As(err, &ce) {
				t.Fatalf("err = %v (%T), want *ConnectionError", err, err)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// TestHookOnFailure_HooksAppliedOnlyCheckedFirst mirrors the TypeScript SDK:
// hooks_applied: false is refused on the first initialize only.
func TestHookOnFailure_HooksAppliedOnlyCheckedFirst(t *testing.T) {
	q := &query{hooks: map[string][]hookMatcherInternal{
		"PreToolUse": {{matcher: "Bash", onFailure: HookOnFailureBlock}},
	}}
	resp := map[string]any{"hook_on_failure_supported": true, "hooks_applied": false}
	if err := q.checkHookOnFailure(resp, true); err == nil {
		t.Error("first initialize: want error")
	}
	if err := q.checkHookOnFailure(resp, false); err != nil {
		t.Errorf("later initialize: %v", err)
	}
}

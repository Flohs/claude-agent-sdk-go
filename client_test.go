package claude

import (
	"context"
	"strings"
	"testing"
)

// TestGetServerCapabilities_PopulatesCapabilities verifies that the open-set
// "capabilities" field from the CLI's initialization result is surfaced on
// ServerCapabilities.Capabilities. Port of TypeScript SDK v0.3.205.
func TestGetServerCapabilities_PopulatesCapabilities(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsEffort": true,
				"capabilities":   []any{"interrupt_receipt_v1", "some_future_capability"},
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	want := []string{"interrupt_receipt_v1", "some_future_capability"}
	if len(caps.Capabilities) != len(want) {
		t.Fatalf("Capabilities = %v, want %v", caps.Capabilities, want)
	}
	for i, s := range want {
		if caps.Capabilities[i] != s {
			t.Fatalf("Capabilities[%d] = %q, want %q", i, caps.Capabilities[i], s)
		}
	}
}

// TestGetServerCapabilities_OlderCLIOmitsCapabilities verifies that older
// CLIs without a "capabilities" key yield a nil Capabilities slice rather
// than an error.
func TestGetServerCapabilities_OlderCLIOmitsCapabilities(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsEffort": true,
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	if caps.Capabilities != nil {
		t.Fatalf("Capabilities = %v, want nil", caps.Capabilities)
	}
}

// TestGetServerCapabilities_PopulatesFastModeState verifies that
// fast_mode_state and fast_mode_disabled_reason from the CLI's
// initialization result are surfaced on ServerCapabilities. Port of
// TypeScript SDK v0.3.219.
func TestGetServerCapabilities_PopulatesFastModeState(t *testing.T) {
	c := &Client{
		q: &query{
			initializationResult: map[string]any{
				"supportsFastMode":          true,
				"fast_mode_state":           "cooldown",
				"fast_mode_disabled_reason": "extra_usage_disabled",
			},
		},
	}

	caps := c.GetServerCapabilities()
	if caps == nil {
		t.Fatal("expected non-nil ServerCapabilities")
	}
	if caps.FastModeState != FastModeStateCooldown {
		t.Errorf("FastModeState = %q, want %q", caps.FastModeState, FastModeStateCooldown)
	}
	if caps.FastModeDisabledReason != FastModeDisabledReasonExtraUsageDisabled {
		t.Errorf("FastModeDisabledReason = %q, want %q", caps.FastModeDisabledReason, FastModeDisabledReasonExtraUsageDisabled)
	}
}

// TestReloadOutputStyles_NotConnectedReturnsConnectionError verifies that
// Client.ReloadOutputStyles fails fast with a ConnectionError when called
// before Connect(), mirroring the nil-query guard used by the other control
// methods (e.g. ReloadPlugins). Port of TypeScript SDK v0.3.261. ([#673])
func TestReloadOutputStyles_NotConnectedReturnsConnectionError(t *testing.T) {
	c := &Client{}

	_, err := c.ReloadOutputStyles(context.Background())
	if err == nil {
		t.Fatal("expected an error when not connected, got nil")
	}
	if _, ok := err.(*ConnectionError); !ok {
		t.Fatalf("expected a *ConnectionError, got %T: %v", err, err)
	}
}

// TestSetPermissionMode_RejectsInvalidModeWithoutSendingRequest verifies
// that Client.SetPermissionMode validates the mode before dispatching a
// set_permission_mode control request, so a typo never reaches the CLI.
func TestSetPermissionMode_RejectsInvalidModeWithoutSendingRequest(t *testing.T) {
	mt := newMockTransport()
	c := &Client{q: newQuery(queryConfig{transport: mt})}

	err := c.SetPermissionMode(context.Background(), "acceptEdit")
	if err == nil {
		t.Fatal("expected error for invalid PermissionMode, got nil")
	}
	if !strings.Contains(err.Error(), "acceptEdit") {
		t.Errorf("error message = %q, want it to contain %q", err.Error(), "acceptEdit")
	}

	mt.mu.Lock()
	written := len(mt.written)
	mt.mu.Unlock()
	if written != 0 {
		t.Errorf("expected no control request to be written, got %d write(s): %v", written, mt.written)
	}
}

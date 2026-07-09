package claude

import "testing"

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
